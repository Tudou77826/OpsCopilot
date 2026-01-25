package filetransfer

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/crypto/ssh"
)

type SCPTransport struct {
	client *ssh.Client
}

func NewSCPTransport(client *ssh.Client) *SCPTransport {
	return &SCPTransport{client: client}
}

func (t *SCPTransport) Check(ctx context.Context) (bool, string, error) {
	out, err := t.run(ctx, "command -v scp >/dev/null 2>&1 && echo 1 || echo 0")
	if err != nil {
		return false, "", toTransferError(err)
	}
	if strings.TrimSpace(out) == "1" {
		return true, "", nil
	}
	return false, "对端未安装 scp", nil
}

func (t *SCPTransport) Upload(ctx context.Context, localPath, remotePath string, progress func(Progress)) (TransferResult, error) {
	lp := filepath.Clean(localPath)
	f, err := os.Open(lp)
	if err != nil {
		return TransferResult{}, toTransferError(err)
	}
	defer f.Close()

	st, err := f.Stat()
	if err != nil {
		return TransferResult{}, toTransferError(err)
	}
	size := st.Size()

	rp := strings.TrimSpace(remotePath)
	if rp == "" {
		return TransferResult{}, &TransferError{Code: ErrorCodeUnknown, Message: "远端路径为空"}
	}

	remoteDir, remoteName := splitRemoteTarget(rp, filepath.Base(lp))
	cmd := "scp -t " + shellSingleQuote(remoteDir)

	session, err := t.client.NewSession()
	if err != nil {
		return TransferResult{}, toTransferError(err)
	}
	defer session.Close()

	stdin, err := session.StdinPipe()
	if err != nil {
		return TransferResult{}, toTransferError(err)
	}
	stdout, err := session.StdoutPipe()
	if err != nil {
		return TransferResult{}, toTransferError(err)
	}
	var stderr bytes.Buffer
	session.Stderr = &stderr

	if err := session.Start(cmd); err != nil {
		return TransferResult{}, toTransferError(err)
	}

	br := bufio.NewReader(stdout)
	bw := bufio.NewWriter(stdin)

	if err := readSCPAck(br); err != nil {
		_ = session.Wait()
		return TransferResult{}, err
	}

	header := fmt.Sprintf("C0644 %d %s\n", size, remoteName)
	if _, err := bw.WriteString(header); err != nil {
		_ = session.Wait()
		return TransferResult{}, toTransferError(err)
	}
	if err := bw.Flush(); err != nil {
		_ = session.Wait()
		return TransferResult{}, toTransferError(err)
	}
	if err := readSCPAck(br); err != nil {
		_ = session.Wait()
		return TransferResult{}, err
	}

	n, err := copyWithProgress(ctx, bw, f, size, progress)
	if err != nil {
		_ = session.Wait()
		return TransferResult{}, err
	}
	if err := bw.WriteByte(0); err != nil {
		_ = session.Wait()
		return TransferResult{}, toTransferError(err)
	}
	if err := bw.Flush(); err != nil {
		_ = session.Wait()
		return TransferResult{}, toTransferError(err)
	}

	if err := readSCPAck(br); err != nil {
		_ = session.Wait()
		return TransferResult{}, err
	}

	if err := session.Wait(); err != nil {
		if stderr.Len() > 0 {
			return TransferResult{}, &TransferError{Code: ErrorCodeUnknown, Message: strings.TrimSpace(stderr.String())}
		}
		return TransferResult{}, toTransferError(err)
	}

	return TransferResult{Bytes: n}, nil
}

func (t *SCPTransport) Download(ctx context.Context, remotePath, localPath string, progress func(Progress)) (TransferResult, error) {
	rp := strings.TrimSpace(remotePath)
	if rp == "" {
		return TransferResult{}, &TransferError{Code: ErrorCodeUnknown, Message: "远端路径为空"}
	}

	cmd := "scp -f " + shellSingleQuote(rp)

	session, err := t.client.NewSession()
	if err != nil {
		return TransferResult{}, toTransferError(err)
	}
	defer session.Close()

	stdin, err := session.StdinPipe()
	if err != nil {
		return TransferResult{}, toTransferError(err)
	}
	stdout, err := session.StdoutPipe()
	if err != nil {
		return TransferResult{}, toTransferError(err)
	}
	var stderr bytes.Buffer
	session.Stderr = &stderr

	if err := session.Start(cmd); err != nil {
		return TransferResult{}, toTransferError(err)
	}

	br := bufio.NewReader(stdout)
	bw := bufio.NewWriter(stdin)

	if err := bw.WriteByte(0); err != nil {
		_ = session.Wait()
		return TransferResult{}, toTransferError(err)
	}
	if err := bw.Flush(); err != nil {
		_ = session.Wait()
		return TransferResult{}, toTransferError(err)
	}

	h, err := readSCPHeader(br)
	if err != nil {
		_ = session.Wait()
		return TransferResult{}, err
	}

	if err := bw.WriteByte(0); err != nil {
		_ = session.Wait()
		return TransferResult{}, toTransferError(err)
	}
	if err := bw.Flush(); err != nil {
		_ = session.Wait()
		return TransferResult{}, toTransferError(err)
	}

	lp := filepath.Clean(localPath)
	if err := os.MkdirAll(filepath.Dir(lp), 0755); err != nil {
		_ = session.Wait()
		return TransferResult{}, toTransferError(err)
	}
	w, err := os.Create(lp)
	if err != nil {
		_ = session.Wait()
		return TransferResult{}, toTransferError(err)
	}
	defer w.Close()

	lr := io.LimitReader(br, h.Size)
	n, err := copyWithProgress(ctx, w, lr, h.Size, progress)
	if err != nil {
		_ = session.Wait()
		return TransferResult{}, err
	}

	if _, err := br.ReadByte(); err != nil {
		_ = session.Wait()
		return TransferResult{}, toTransferError(err)
	}

	if err := bw.WriteByte(0); err != nil {
		_ = session.Wait()
		return TransferResult{}, toTransferError(err)
	}
	if err := bw.Flush(); err != nil {
		_ = session.Wait()
		return TransferResult{}, toTransferError(err)
	}

	if err := session.Wait(); err != nil {
		if stderr.Len() > 0 {
			return TransferResult{}, &TransferError{Code: ErrorCodeUnknown, Message: strings.TrimSpace(stderr.String())}
		}
		return TransferResult{}, toTransferError(err)
	}

	return TransferResult{Bytes: n}, nil
}

func (t *SCPTransport) run(ctx context.Context, cmd string) (string, error) {
	session, err := t.client.NewSession()
	if err != nil {
		return "", err
	}
	defer session.Close()

	type res struct {
		out []byte
		err error
	}
	ch := make(chan res, 1)
	go func() {
		out, err := session.CombinedOutput(cmd)
		ch <- res{out: out, err: err}
	}()

	select {
	case <-ctx.Done():
		_ = session.Close()
		return "", ctx.Err()
	case r := <-ch:
		return string(r.out), r.err
	}
}

type scpHeader struct {
	Size int64
	Name string
}

func readSCPHeader(r *bufio.Reader) (scpHeader, error) {
	b, err := r.ReadByte()
	if err != nil {
		return scpHeader{}, toTransferError(err)
	}
	if b == 1 || b == 2 {
		msg, _ := r.ReadString('\n')
		return scpHeader{}, &TransferError{Code: ErrorCodeUnknown, Message: strings.TrimSpace(msg)}
	}
	if b != 'C' {
		return scpHeader{}, &TransferError{Code: ErrorCodeUnknown, Message: "scp 协议错误"}
	}
	line, err := r.ReadString('\n')
	if err != nil {
		return scpHeader{}, toTransferError(err)
	}
	parts := strings.SplitN(strings.TrimSpace(line), " ", 3)
	if len(parts) != 3 {
		return scpHeader{}, &TransferError{Code: ErrorCodeUnknown, Message: "scp header 解析失败"}
	}
	size, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return scpHeader{}, &TransferError{Code: ErrorCodeUnknown, Message: "scp header size 解析失败"}
	}
	return scpHeader{Size: size, Name: parts[2]}, nil
}

func readSCPAck(r *bufio.Reader) error {
	b, err := r.ReadByte()
	if err != nil {
		return toTransferError(err)
	}
	if b == 0 {
		return nil
	}
	if b == 1 || b == 2 {
		msg, _ := r.ReadString('\n')
		return &TransferError{Code: ErrorCodeUnknown, Message: strings.TrimSpace(msg)}
	}
	return &TransferError{Code: ErrorCodeUnknown, Message: "scp ack 解析失败"}
}

func splitRemoteTarget(remotePath, defaultName string) (dir, name string) {
	p := path.Clean(remotePath)
	if strings.HasSuffix(remotePath, "/") {
		return p, defaultName
	}
	if path.Base(p) == "." || path.Base(p) == "/" {
		return p, defaultName
	}
	return path.Dir(p), path.Base(p)
}

func shellSingleQuote(s string) string {
	if s == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
