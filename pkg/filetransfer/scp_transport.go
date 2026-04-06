package filetransfer

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
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
		log.Printf("[SCP] 检查 SCP 可用性: 错误 (%v)", err)
		return false, "", toTransferError(err)
	}
	if strings.TrimSpace(out) == "1" {
		log.Printf("[SCP] 检查 SCP 可用性: 可用")
		return true, "", nil
	}
	log.Printf("[SCP] 检查 SCP 可用性: 不可用（对端未安装 scp）")
	return false, "对端未安装 scp", nil
}

func (t *SCPTransport) Upload(ctx context.Context, localPath, remotePath string, progress func(Progress)) (TransferResult, error) {
	lp := filepath.Clean(localPath)
	log.Printf("[SCP] 上传开始: %s -> %s", lp, remotePath)
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
	defer func() { go session.Close() }()

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

	// session.Start sends an exec request with wantReply=true, which blocks
	// if the remote is unresponsive (e.g. SCP through bastion). Wrap with context.
	if err := startSessionContext(ctx, session, cmd); err != nil {
		return TransferResult{}, err
	}

	br := bufio.NewReader(stdout)
	bw := bufio.NewWriter(stdin)

	if err := readSCPAckContext(ctx, br); err != nil {
		return TransferResult{}, err
	}

	header := fmt.Sprintf("C0644 %d %s\n", size, remoteName)
	if _, err := bw.WriteString(header); err != nil {
		return TransferResult{}, toTransferError(err)
	}
	if err := bw.Flush(); err != nil {
		return TransferResult{}, toTransferError(err)
	}
	if err := readSCPAckContext(ctx, br); err != nil {
		return TransferResult{}, err
	}

	n, err := copyWithProgress(ctx, bw, f, size, progress)
	if err != nil {
		return TransferResult{}, err
	}
	if err := bw.WriteByte(0); err != nil {
		return TransferResult{}, toTransferError(err)
	}
	if err := bw.Flush(); err != nil {
		return TransferResult{}, toTransferError(err)
	}

	if err := readSCPAckContext(ctx, br); err != nil {
		return TransferResult{}, err
	}

	if err := waitSessionContext(ctx, session); err != nil {
		if stderr.Len() > 0 {
			return TransferResult{}, &TransferError{Code: ErrorCodeUnknown, Message: strings.TrimSpace(stderr.String())}
		}
		return TransferResult{}, toTransferError(err)
	}

	log.Printf("[SCP] 上传完成: %s -> %s (传输=%d 字节)", lp, remotePath, n)
	return TransferResult{Bytes: n}, nil
}

func (t *SCPTransport) Download(ctx context.Context, remotePath, localPath string, progress func(Progress)) (TransferResult, error) {
	rp := strings.TrimSpace(remotePath)
	if rp == "" {
		return TransferResult{}, &TransferError{Code: ErrorCodeUnknown, Message: "远端路径为空"}
	}

	log.Printf("[SCP] 下载开始: %s -> %s", rp, localPath)

	cmd := "scp -f " + shellSingleQuote(rp)

	session, err := t.client.NewSession()
	if err != nil {
		return TransferResult{}, toTransferError(err)
	}
	defer func() { go session.Close() }()

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

	// session.Start sends an exec request with wantReply=true, which blocks
	// if the remote is unresponsive (e.g. SCP through bastion). Wrap with context.
	if err := startSessionContext(ctx, session, cmd); err != nil {
		return TransferResult{}, err
	}

	br := bufio.NewReader(stdout)
	bw := bufio.NewWriter(stdin)

	if err := writeSCPAckContext(ctx, bw); err != nil {
		return TransferResult{}, toTransferError(err)
	}

	h, err := readSCPHeaderContext(ctx, br)
	if err != nil {
		return TransferResult{}, err
	}

	if err := writeSCPAckContext(ctx, bw); err != nil {
		return TransferResult{}, toTransferError(err)
	}

	lp := filepath.Clean(localPath)
	if err := os.MkdirAll(filepath.Dir(lp), 0755); err != nil {
		return TransferResult{}, toTransferError(err)
	}
	w, err := os.Create(lp)
	if err != nil {
		return TransferResult{}, toTransferError(err)
	}
	defer w.Close()

	lr := io.LimitReader(br, h.Size)
	n, err := copyWithProgress(ctx, w, lr, h.Size, progress)
	if err != nil {
		return TransferResult{}, err
	}

	// Read trailing byte (end-of-file marker from scp)
	{
		doneCh := make(chan error, 1)
		go func() {
			_, err := br.ReadByte()
			doneCh <- err
		}()
		select {
		case <-ctx.Done():
			return TransferResult{}, &TransferError{Code: ErrorCodeNetwork, Message: "操作已取消"}
		case err := <-doneCh:
			if err != nil {
				return TransferResult{}, toTransferError(err)
			}
		}
	}

	if err := writeSCPAckContext(ctx, bw); err != nil {
		return TransferResult{}, toTransferError(err)
	}

	if err := waitSessionContext(ctx, session); err != nil {
		if stderr.Len() > 0 {
			return TransferResult{}, &TransferError{Code: ErrorCodeUnknown, Message: strings.TrimSpace(stderr.String())}
		}
		return TransferResult{}, toTransferError(err)
	}

	log.Printf("[SCP] 下载完成: %s -> %s (传输=%d 字节)", rp, localPath, n)
	return TransferResult{Bytes: n}, nil
}

// startSessionContext wraps session.Start with context cancellation support.
// session.Start sends an exec request with wantReply=true, which can block
// indefinitely if the remote SSH server is unresponsive (e.g. through a bastion).
func startSessionContext(ctx context.Context, session *ssh.Session, cmd string) error {
	type result struct {
		err error
	}
	ch := make(chan result, 1)
	go func() {
		ch <- result{err: session.Start(cmd)}
	}()

	select {
	case <-ctx.Done():
		go session.Close()
		return &TransferError{Code: ErrorCodeNetwork, Message: "操作已取消"}
	case r := <-ch:
		return toTransferError(r.err)
	}
}

// waitSessionContext wraps session.Wait with context cancellation support.
func waitSessionContext(ctx context.Context, session *ssh.Session) error {
	type result struct {
		err error
	}
	ch := make(chan result, 1)
	go func() {
		ch <- result{err: session.Wait()}
	}()

	select {
	case <-ctx.Done():
		go session.Close()
		return &TransferError{Code: ErrorCodeNetwork, Message: "操作已取消"}
	case r := <-ch:
		return r.err
	}
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

// readSCPHeaderContext reads an SCP header with context cancellation support.
func readSCPHeaderContext(ctx context.Context, r *bufio.Reader) (scpHeader, error) {
	type result struct {
		h   scpHeader
		err error
	}
	ch := make(chan result, 1)
	go func() {
		h, err := readSCPHeader(r)
		ch <- result{h: h, err: err}
	}()

	select {
	case <-ctx.Done():
		return scpHeader{}, &TransferError{Code: ErrorCodeNetwork, Message: "操作已取消"}
	case res := <-ch:
		return res.h, res.err
	}
}

// writeSCPAckContext writes a zero ack byte with context support.
func writeSCPAckContext(ctx context.Context, bw *bufio.Writer) error {
	type result struct {
		err error
	}
	ch := make(chan result, 1)
	go func() {
		if err := bw.WriteByte(0); err != nil {
			ch <- result{err: err}
			return
		}
		ch <- result{err: bw.Flush()}
	}()

	select {
	case <-ctx.Done():
		return &TransferError{Code: ErrorCodeNetwork, Message: "操作已取消"}
	case res := <-ch:
		return toTransferError(res.err)
	}
}

// readSCPAckContext reads an SCP ack with context cancellation support.
func readSCPAckContext(ctx context.Context, r *bufio.Reader) error {
	type result struct {
		b   byte
		err error
	}
	ch := make(chan result, 1)
	go func() {
		b, err := r.ReadByte()
		ch <- result{b: b, err: err}
	}()

	select {
	case <-ctx.Done():
		return &TransferError{Code: ErrorCodeNetwork, Message: "操作已取消"}
	case res := <-ch:
		if res.err != nil {
			return toTransferError(res.err)
		}
		if res.b == 0 {
			return nil
		}
		if res.b == 1 || res.b == 2 {
			msg, _ := r.ReadString('\n')
			return &TransferError{Code: ErrorCodeUnknown, Message: strings.TrimSpace(msg)}
		}
		return &TransferError{Code: ErrorCodeUnknown, Message: "scp ack 解析失败"}
	}
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
