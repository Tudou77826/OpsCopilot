package filetransfer

import (
	"context"
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

// A multiple of three keeps each independently decoded block self-contained.
// Wrapped here-doc lines also stay below canonical PTY line limits.
const base64BlockBytes = 768 * 1024
const base64LineBytes = 1024 // Below Linux canonical PTY's 4096-byte line buffer.

type rootCommand func(context.Context, string) (string, error)
type base64Transfer struct {
	run         rootCommand
	uploadChunk func(context.Context, string, []byte) error
}

func base64SizeError(size int64) error {
	return &TransferError{Code: ErrorCodeFileSizeExceeded, Message: fmt.Sprintf("兼容传输单文件上限 %d MiB，当前文件 %s", maxBase64DirectBytes/(1024*1024), HumanBytes(size))}
}

func checkBase64Size(size int64) error {
	if size < 0 {
		return fmt.Errorf("无法确定文件大小，未开始传输")
	}
	if size > maxBase64DirectBytes {
		return base64SizeError(size)
	}
	return nil
}

func transferPath(p string) error {
	if strings.ContainsAny(p, "\r\n\x00") {
		return fmt.Errorf("兼容传输路径不能包含换行或空字符")
	}
	return nil
}

func parseTransferMD5(output string) (string, error) {
	f := strings.Fields(output)
	if len(f) > 0 {
		if b, err := hex.DecodeString(f[0]); err == nil && len(b) == md5.Size {
			return strings.ToLower(f[0]), nil
		}
	}
	return "", fmt.Errorf("无法取得有效的文件校验和，未提交目标文件")
}

func (b base64Transfer) remoteMD5(ctx context.Context, p string) (string, error) {
	out, err := b.run(ctx, "md5sum -- "+shellSingleQuote(p))
	if err != nil {
		return "", err
	}
	return parseTransferMD5(out)
}

// Cleanup has its own bounded context: cancellation must not turn a temporary
// file into a final destination. An unreachable server is reported explicitly.
func (b base64Transfer) cleanup(temp string, err *error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, e := b.run(ctx, "rm -f -- "+shellSingleQuote(temp)); e != nil {
		*err = fmt.Errorf("%w；远端暂时无法清理，临时文件可能保留在 %s", *err, temp)
	}
}

func uploadBlock(temp string, data []byte) string {
	encoded := base64.StdEncoding.EncodeToString(data)
	delimiter := "OC_DATA_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	var cmd strings.Builder
	cmd.Grow(len(encoded) + len(encoded)/base64LineBytes + 200)
	cmd.WriteString("{ base64 -d >> " + shellSingleQuote(temp) + " <<'" + delimiter + "'\n")
	for len(encoded) > 0 {
		n := base64LineBytes
		if len(encoded) < n {
			n = len(encoded)
		}
		cmd.WriteString(encoded[:n])
		cmd.WriteByte('\n')
		encoded = encoded[n:]
	}
	cmd.WriteString(delimiter + "\n}")
	return cmd.String()
}

func (b base64Transfer) upload(ctx context.Context, localPath, remotePath string, progress func(Progress)) (result TransferResult, err error) {
	if err = ctx.Err(); err != nil {
		return result, err
	}
	if err = transferPath(remotePath); err != nil {
		return result, err
	}
	lp := filepath.Clean(localPath)
	initial, err := os.Stat(lp)
	if err != nil {
		return result, err
	}
	if !initial.Mode().IsRegular() {
		return result, fmt.Errorf("兼容传输仅支持普通文件")
	}
	if err = checkBase64Size(initial.Size()); err != nil {
		return result, err
	}
	f, err := os.Open(lp)
	if err != nil {
		return result, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return result, err
	}
	if !info.Mode().IsRegular() {
		return result, fmt.Errorf("兼容传输仅支持普通文件")
	}
	if err = checkBase64Size(info.Size()); err != nil {
		return result, err
	}
	// Resolve an existing symlink once, preserving the symlink itself. readlink is
	// needed only for that case; ordinary transfers use the existing core tools.
	q := shellSingleQuote(remotePath)
	resolved, err := b.run(ctx, "if [ -L "+q+" ]; then readlink -f -- "+q+"; else printf '%s\\n' "+q+"; fi")
	if err != nil {
		return result, err
	}
	destination := strings.TrimSpace(resolved)
	if destination == "" {
		return result, fmt.Errorf("无法解析上传目标")
	}
	if err = transferPath(destination); err != nil {
		return result, err
	}
	temp := path.Join(path.Dir(destination), ".opscopilot-upload-"+uuid.NewString())
	tq := shellSingleQuote(temp)
	// Noclobber prevents an existing file or symlink from becoming our staging file.
	if _, err = b.run(ctx, "(umask 077; set -C; : > "+tq+")"); err != nil {
		// A lost reply can mean creation succeeded before disconnect. A definite
		// command failure (e.g. noclobber collision) must not delete an older file.
		if !errors.Is(err, errBase64CommandFailed) {
			b.cleanup(temp, &err)
		}
		return result, err
	}
	committed := false
	defer func() {
		if !committed {
			b.cleanup(temp, &err)
		}
	}()
	h := md5.New()
	buf := make([]byte, base64BlockBytes)
	var done int64
	started := time.Now()
	emitStep(progress, "正在通过兼容通道分块上传…")
	for done < info.Size() {
		if err = ctx.Err(); err != nil {
			return result, err
		}
		next := int64(len(buf))
		if info.Size()-done < next {
			next = info.Size() - done
		}
		n, e := io.ReadFull(f, buf[:next])
		if e != nil {
			return result, fmt.Errorf("上传过程中本地文件发生变化: %w", e)
		}
		h.Write(buf[:n])
		if b.uploadChunk != nil {
			err = b.uploadChunk(ctx, temp, buf[:n])
		} else {
			_, err = b.run(ctx, uploadBlock(temp, buf[:n]))
		}
		if err != nil {
			return result, err
		}
		done += int64(n)
		reportBase64Progress(progress, done, info.Size(), started)
	}
	var extra [1]byte
	if n, e := f.Read(extra[:]); n != 0 || e != io.EOF {
		return result, fmt.Errorf("上传过程中本地文件大小发生变化，未替换目标")
	}
	emitStep(progress, "正在校验上传文件…")
	remoteHash, err := b.remoteMD5(ctx, temp)
	if err != nil {
		return result, err
	}
	if remoteHash != hex.EncodeToString(h.Sum(nil)) {
		return result, &TransferError{Code: ErrorCodeChecksumMismatch, Message: "文件校验失败，未替换远端原文件"}
	}
	if err = ctx.Err(); err != nil {
		return result, err
	}
	q = shellSingleQuote(destination)
	// Preserve metadata of existing regular files; reject a late symlink change.
	commit := "{ if [ -L " + q + " ]; then false; elif [ -e " + q + " ]; then [ -f " + q + " ] && _oc_mode=$(stat -Lc '%a' -- " + q + ") && _oc_owner=$(stat -Lc '%u:%g' -- " + q + ") && chown \"$_oc_owner\" " + tq + " && chmod \"$_oc_mode\" " + tq + "; else chmod 0644 " + tq + "; fi; } && mv -f -- " + tq + " " + q
	if _, err = b.run(ctx, commit); err != nil {
		return result, fmt.Errorf("提交上传文件失败（若连接中断，请核对目标状态）: %w", err)
	}
	committed = true
	reportBase64Progress(progress, done, info.Size(), started)
	return TransferResult{Bytes: done, Transport: "Base64 分块直传"}, nil
}

func (b base64Transfer) download(ctx context.Context, remotePath, localPath string, size int64, progress func(Progress)) (result TransferResult, err error) {
	if err = ctx.Err(); err != nil {
		return result, err
	}
	if err = transferPath(remotePath); err != nil {
		return result, err
	}
	if err = checkBase64Size(size); err != nil {
		return result, err
	}
	q := shellSingleQuote(remotePath)
	if _, err = b.run(ctx, "test -f "+q); err != nil {
		return result, err
	}
	emitStep(progress, "正在校验远端源文件…")
	expectedHash, err := b.remoteMD5(ctx, remotePath)
	if err != nil {
		return result, err
	}
	lp := filepath.Clean(localPath)
	if i, e := os.Lstat(lp); e == nil && i.Mode()&os.ModeSymlink != 0 {
		lp, err = filepath.EvalSymlinks(lp)
		if err != nil {
			return result, err
		}
	}
	mode := os.FileMode(0644)
	if i, e := os.Stat(lp); e == nil {
		if !i.Mode().IsRegular() {
			return result, fmt.Errorf("下载目标必须为普通文件")
		}
		mode = i.Mode().Perm()
	} else if !os.IsNotExist(e) {
		return result, e
	}
	if err = os.MkdirAll(filepath.Dir(lp), 0755); err != nil {
		return result, err
	}
	f, err := os.CreateTemp(filepath.Dir(lp), ".opscopilot-download-*")
	if err != nil {
		return result, err
	}
	temp := f.Name()
	defer func() {
		f.Close()
		// A preserved read-only mode can otherwise prevent deleting our staging
		// file on Windows when cancellation/rename fails after Chmod.
		_ = os.Chmod(temp, 0600)
		if e := os.Remove(temp); e != nil && !os.IsNotExist(e) {
			if err == nil {
				err = fmt.Errorf("本地临时文件清理失败: %s", temp)
			} else {
				err = fmt.Errorf("%w；本地临时文件清理失败: %s", err, temp)
			}
		}
	}()
	h := md5.New()
	var done int64
	started := time.Now()
	emitStep(progress, "正在通过兼容通道分块下载…")
	for done < size {
		if err = ctx.Err(); err != nil {
			return result, err
		}
		out, e := b.run(ctx, "dd if="+q+" bs="+strconv.Itoa(base64BlockBytes)+" skip="+strconv.FormatInt(done/base64BlockBytes, 10)+" count=1 2>/dev/null | base64")
		if e != nil {
			return result, e
		}
		decoded, e := base64.StdEncoding.DecodeString(strings.TrimSpace(out))
		if e != nil {
			return result, fmt.Errorf("兼容传输数据解码失败，未替换本地原文件: %w", e)
		}
		next := int64(base64BlockBytes)
		if size-done < next {
			next = size - done
		}
		if int64(len(decoded)) != next {
			return result, fmt.Errorf("远端文件大小发生变化或数据不完整，未替换本地原文件")
		}
		if _, err = f.Write(decoded); err != nil {
			return result, err
		}
		h.Write(decoded)
		done += next
		reportBase64Progress(progress, done, size, started)
	}
	emitStep(progress, "正在校验下载文件…")
	actualHash, err := b.remoteMD5(ctx, remotePath)
	if err != nil {
		return result, err
	}
	if expectedHash != actualHash || expectedHash != hex.EncodeToString(h.Sum(nil)) {
		return result, &TransferError{Code: ErrorCodeChecksumMismatch, Message: "源文件发生变化或校验失败，未替换本地原文件"}
	}
	// Detect a zero-byte file that grew after its initial stat, too.
	finalSize, err := b.run(ctx, "stat -Lc '%s' -- "+q)
	if err != nil {
		return result, err
	}
	n, err := strconv.ParseInt(strings.TrimSpace(finalSize), 10, 64)
	if err != nil || n != size {
		return result, fmt.Errorf("远端文件大小发生变化，未替换本地原文件")
	}
	if err = f.Chmod(mode); err != nil {
		return result, err
	}
	if err = f.Sync(); err != nil {
		return result, err
	}
	if err = f.Close(); err != nil {
		return result, err
	}
	if err = ctx.Err(); err != nil {
		return result, err
	}
	if err = os.Rename(temp, lp); err != nil {
		return result, fmt.Errorf("替换本地目标文件失败: %w", err)
	}
	reportBase64Progress(progress, done, size, started)
	return TransferResult{Bytes: done, Transport: "Base64 分块直传"}, nil
}

func reportBase64Progress(progress func(Progress), done, total int64, started time.Time) {
	if progress != nil {
		elapsed := time.Since(started).Seconds()
		var speed int64
		if elapsed > 0 {
			speed = int64(float64(done) / elapsed)
		}
		progress(Progress{BytesDone: done, BytesTotal: total, SpeedBps: speed})
	}
}
