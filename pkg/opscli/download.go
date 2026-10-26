package opscli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"opscopilot/pkg/core/ops"
	"opscopilot/pkg/core/security"
	"opscopilot/pkg/filetransfer"
	"opscopilot/pkg/opssession"
	"opscopilot/pkg/sshclient"
)

func download(ctx context.Context, store *opssession.Store, session string, node opssession.Node, remotePath, local string, maxBytes int) (any, error) {
	if node.Kind != "ssh" || node.SSH == nil {
		return nil, errors.New("下载需要 SSH 节点")
	}
	if err := validateDownloadOptions(remotePath, local, maxBytes); err != nil {
		return nil, err
	}
	work, err := store.WorkDirContext(ctx, session)
	if err != nil {
		return nil, err
	}
	checker, err := security.LoadFileAccessChecker(os.Getenv("OPSCOPILOT_FILE_ACCESS_PATH"))
	if err != nil {
		return nil, err
	}
	check := checker.CheckRead(remotePath, filepath.Join(work, local), node.SSH.Host, 0)
	if !check.Allowed {
		return nil, fmt.Errorf("%w：%s", ops.ErrPolicy, check.Reason)
	}
	conn, err := sshclient.NewClientWithTrace(ctx, node.SSH)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()
	sftp, err := filetransfer.DialSFTP(ctx, conn.SSHClient(), 10*time.Second)
	if err != nil {
		return nil, err
	}
	defer sftp.Close()
	canonical, err := sftp.RealPath(remotePath)
	if err != nil {
		return nil, err
	}
	check = checker.CheckRead(canonical, filepath.Join(work, local), node.SSH.Host, 0)
	if !check.Allowed {
		return nil, fmt.Errorf("%w：%s", ops.ErrPolicy, check.Reason)
	}
	file, err := sftp.Open(canonical)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("仅支持普通文件下载")
	}
	check = checker.CheckRead(canonical, filepath.Join(work, local), node.SSH.Host, info.Size())
	if !check.Allowed {
		return nil, fmt.Errorf("%w：%s", ops.ErrPolicy, check.Reason)
	}
	for _, policy := range checker.GetConfig().Policies {
		if security.MatchesIPRange(node.SSH.Host, policy.IPRanges) {
			if policy.MaxReadBytes > 0 && policy.MaxReadBytes < maxBytes {
				maxBytes = policy.MaxReadBytes
			}
			break
		}
	}
	if info.Size() > int64(maxBytes) {
		return nil, errors.New("文件超过本次下载大小限制")
	}
	root, err := os.OpenRoot(work)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	if err = root.MkdirAll(filepath.Dir(local), 0700); err != nil {
		return nil, err
	}
	target, err := root.OpenFile(local, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return nil, errors.New("无法创建本地文件：目标可能已经存在或路径越界")
	}
	defer target.Close()
	result := map[string]any{"localPath": filepath.Join(work, local), "remotePath": canonical, "partial": true, "bytes": int64(0)}
	n, err := io.Copy(target, io.LimitReader(file, int64(maxBytes)+1))
	result["bytes"] = n
	if ctx.Err() != nil {
		return result, ctx.Err()
	}
	if err != nil {
		return result, err
	}
	if n > int64(maxBytes) {
		return result, errors.New("远端文件增长超过下载限制，已保留部分文件")
	}
	if n != info.Size() {
		return result, errors.New("远端文件在传输中发生变化，已保留取得的文件，请重新核对")
	}
	if err = target.Sync(); err != nil {
		return result, err
	}
	result["partial"] = false
	return result, nil
}

func validateDownloadOptions(remotePath, local string, maxBytes int) error {
	if remotePath == "" || !strings.HasPrefix(remotePath, "/") || strings.ContainsAny(remotePath, "\x00\r\n") {
		return errors.New("--remote 必填，且必须为远端绝对路径")
	}
	if local == "" || !filepath.IsLocal(local) || strings.ContainsRune(local, 0) {
		return errors.New("--local 必填，且必须为 Session 工作目录内的相对文件名，例如 logs/app.log")
	}
	if maxBytes < 1 || maxBytes > 1<<30 {
		return errors.New("--max-bytes 必须为 1–1073741824 字节")
	}
	return nil
}
