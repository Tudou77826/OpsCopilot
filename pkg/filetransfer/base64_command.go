package filetransfer

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

const base64CommandOutputLimit = 3 * 1024 * 1024

var errBase64CommandFailed = errors.New("兼容传输命令失败")

// A data command may contain file contents. Never send it to runAsRoot's command
// logger. Markers are unique and split in source to tolerate PTY command echo.
func (t *RootRelayTransport) runBase64Command(ctx context.Context, cmd string) (string, error) {
	return t.base64Exchange(ctx, cmd, nil)
}

type base64Input struct {
	temp string
	data []byte
}

func (t *RootRelayTransport) uploadBase64Block(ctx context.Context, temp string, data []byte) error {
	_, err := t.base64Exchange(ctx, "", &base64Input{temp: temp, data: data})
	return err
}

func (t *RootRelayTransport) base64Exchange(ctx context.Context, cmd string, input *base64Input) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	var shell *rootShellSession
	var unlock func()
	for {
		var err error
		shell, err = t.getSession(ctx)
		if err != nil {
			return "", err
		}
		unlock, err = shell.lockCommand(ctx)
		if err != nil {
			return "", err
		}
		if input == nil || shell.base64Ready {
			break
		}
		unlock()
		if _, err := t.runBase64Command(ctx, ":"); err != nil {
			return "", err
		}
		// Re-acquire the current shell after preparation: another command may
		// have invalidated it. Retry without growing the call stack.
	}
	defer unlock()
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	id := strings.ReplaceAll(uuid.NewString(), "-", "")
	start := "__OC_B64_START_" + id + "__"
	ok := "__OC_B64_OK_" + id + "__"
	fail := "__OC_B64_FAIL_" + id + "__"
	readyMarker := "__OC_B64_READY_" + id + "__"
	rawTag := "__OC_B64_RAW_" + id + "__"
	lineTag := "__OC_B64_LINE_" + id + "__"
	marker := func(s string) string { return "printf '%s\\n' " + shellSingleQuote(s[:5]) + shellSingleQuote(s[5:]) }
	prepare := ""
	if !shell.base64Ready {
		// Some bastions ignore SSH terminal modes, or su restores canonical mode.
		// Only this dedicated file-transfer shell is affected. Plain pipes and
		// restricted hosts without stty retain the compatible wrapped-line path.
		prepare = "if [ ! -t 0 ] || stty -echo -icanon -opost min 1 time 0 2>/dev/null; then " + marker(rawTag) + "; else " + marker(lineTag) + "; fi; "
	}
	if input != nil && !shell.base64Stream {
		cmd = uploadBlock(input.temp, input.data)
		input = nil // bounded fallback for hosts without stty
	}
	if input != nil {
		q := shellSingleQuote(input.temp)
		encodedSize := base64.StdEncoding.EncodedLen(len(input.data))
		// Wait for READY before sending bytes: the interactive shell must finish
		// parsing the command before its child consumes stdin. The decoder's
		// failure path drains input, so encoded bytes never become shell commands.
		cmd = "{ if [ ! -t 0 ] || stty -echo -icanon -opost min 1 time 0 2>/dev/null; then _oc_before=$(stat -Lc '%s' -- " + q + ") && " + marker(readyMarker) + " && head -c " + strconv.Itoa(encodedSize) + " | (base64 -d >> " + q + "; _oc_decode=$?; cat >/dev/null; exit \"$_oc_decode\") && [ \"$(stat -Lc '%s' -- " + q + ")\" -eq \"$((_oc_before + " + strconv.Itoa(len(input.data)) + "))\" ]; else false; fi; }"
	}
	full := marker(start) + "; " + prepare + cmd + " && " + marker(ok) + " || " + marker(fail) + "\n"
	ready := make(chan struct{})
	written := make(chan error, 1)
	go func() {
		_, e := io.WriteString(shell.stdin, full)
		if e == nil && input != nil {
			select {
			case <-ready:
			case <-ctx.Done():
				written <- ctx.Err()
				return
			}
			buffered := bufio.NewWriterSize(shell.stdin, 64*1024)
			encoder := base64.NewEncoder(base64.StdEncoding, buffered)
			_, e = encoder.Write(input.data)
			if e == nil {
				e = encoder.Close()
			}
			if e == nil {
				e = buffered.Flush()
			}
		}
		written <- e
	}()
	// Read concurrently with writing: echoed here-doc data can otherwise fill
	// the SSH receive window and deadlock a large upload before Write returns.
	readCtx, stopRead := context.WithCancel(ctx)
	defer stopRead()
	type reply struct {
		out string
		err error
	}
	read := make(chan reply, 1)
	readyTag := ""
	if input != nil {
		readyTag = readyMarker
	}
	go func() {
		out, e := readBase64ReplyReady(readCtx, shell.stdout, start, ok, fail, readyTag, func() { close(ready) })
		read <- reply{out, e}
	}()
	var writeDone bool
	for {
		select {
		case e := <-written:
			writeDone = true
			written = nil
			if e != nil {
				t.invalidateSession(shell)
				return "", fmt.Errorf("发送兼容传输命令失败: %w", e)
			}
		case r := <-read:
			if r.err != nil {
				t.invalidateSession(shell)
				return "", r.err
			}
			if !writeDone {
				select {
				case e := <-written:
					if e != nil {
						t.invalidateSession(shell)
						return "", e
					}
				case <-ctx.Done():
					t.invalidateSession(shell)
					return "", ctx.Err()
				}
			}
			if !shell.base64Ready {
				shell.base64Stream = strings.HasPrefix(r.out, rawTag)
				r.out = strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(r.out, rawTag), lineTag))
				shell.base64Ready = true
			}
			return r.out, nil
		case <-ctx.Done():
			t.invalidateSession(shell)
			return "", ctx.Err()
		}
	}
}

// One reader goroutine per command, not per packet. Session.Close unblocks a
// cancelled read. Buffering and marker searches are bounded by one data block.
func readBase64Reply(ctx context.Context, r io.Reader, start, ok, fail string) (string, error) {
	return readBase64ReplyReady(ctx, r, start, ok, fail, "", nil)
}

func readBase64ReplyReady(ctx context.Context, r io.Reader, start, ok, fail, ready string, onReady func()) (string, error) {
	var b bytes.Buffer
	buf := make([]byte, 32*1024)
	maxMarker := len(ok)
	if len(fail) > maxMarker {
		maxMarker = len(fail)
	}
	if len(ready) > maxMarker {
		maxMarker = len(ready)
	}
	readySent := false
	for {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		old := b.Len()
		n, err := r.Read(buf)
		if n > 0 {
			if b.Len()+n > base64CommandOutputLimit {
				return "", fmt.Errorf("兼容传输响应超出分块大小限制")
			}
			b.Write(buf[:n])
			data := b.Bytes()
			from := old - maxMarker
			if from < 0 {
				from = 0
			}
			if ready != "" && !readySent && bytes.Contains(data[from:], []byte(ready)) {
				readySent = true
				onReady()
			}
			end := bytes.Index(data[from:], []byte(ok))
			failed := false
			if end < 0 {
				end = bytes.Index(data[from:], []byte(fail))
				failed = end >= 0
			}
			if end >= 0 {
				if failed {
					return "", fmt.Errorf("%w，请检查路径权限、磁盘空间和远端命令支持", errBase64CommandFailed)
				}
				begin := bytes.Index(data, []byte(start))
				if begin < 0 {
					return "", fmt.Errorf("兼容传输响应缺少起始标记")
				}
				end += from
				begin += len(start)
				if end < begin {
					return "", fmt.Errorf("兼容传输响应顺序异常")
				}
				return strings.TrimSpace(strings.ReplaceAll(string(data[begin:end]), "\r", "")), nil
			}
		}
		if err != nil {
			return "", fmt.Errorf("兼容传输响应中断: %w", err)
		}
	}
}
