package filetransfer

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

type SFTPTransport struct {
	client *ssh.Client
}

func NewSFTPTransport(client *ssh.Client) *SFTPTransport {
	return &SFTPTransport{client: client}
}

func (t *SFTPTransport) Check(ctx context.Context) (bool, string, error) {
	c, err := t.newClient()
	if err != nil {
		var te *TransferError
		if errors.As(err, &te) && te.Code == ErrorCodeSFTPNotSupported {
			return false, te.Message, te
		}
		return false, "", err
	}
	_ = c.Close()
	return true, "", nil
}

func (t *SFTPTransport) List(ctx context.Context, remotePath string) ([]Entry, error) {
	c, err := t.newClient()
	if err != nil {
		return nil, err
	}
	defer c.Close()

	p := normalizeRemotePath(remotePath)
	infos, err := c.ReadDir(p)
	if err != nil {
		return nil, toTransferError(err)
	}

	out := make([]Entry, 0, len(infos))
	for _, fi := range infos {
		out = append(out, Entry{
			Path:    joinRemote(p, fi.Name()),
			Name:    fi.Name(),
			IsDir:   fi.IsDir(),
			Size:    fi.Size(),
			Mode:    uint32(fi.Mode()),
			ModTime: fi.ModTime(),
		})
	}
	return out, nil
}

func (t *SFTPTransport) Stat(ctx context.Context, remotePath string) (Entry, error) {
	c, err := t.newClient()
	if err != nil {
		return Entry{}, err
	}
	defer c.Close()

	p := normalizeRemotePath(remotePath)
	fi, err := c.Stat(p)
	if err != nil {
		return Entry{}, toTransferError(err)
	}
	return Entry{
		Path:    p,
		Name:    filepath.Base(p),
		IsDir:   fi.IsDir(),
		Size:    fi.Size(),
		Mode:    uint32(fi.Mode()),
		ModTime: fi.ModTime(),
	}, nil
}

func (t *SFTPTransport) Upload(ctx context.Context, localPath, remotePath string, progress func(Progress)) (TransferResult, error) {
	c, err := t.newClient()
	if err != nil {
		return TransferResult{}, err
	}
	defer c.Close()

	lp := filepath.Clean(localPath)
	f, err := os.Open(lp)
	if err != nil {
		return TransferResult{}, toTransferError(err)
	}
	defer f.Close()

	var total int64 = -1
	if st, err := f.Stat(); err == nil {
		total = st.Size()
	}

	rp := normalizeRemotePath(remotePath)
	w, err := c.Create(rp)
	if err != nil {
		return TransferResult{}, toTransferError(err)
	}
	defer w.Close()

	n, err := copyWithProgress(ctx, w, f, total, progress)
	if err != nil {
		return TransferResult{}, err
	}
	return TransferResult{Bytes: n}, nil
}

func (t *SFTPTransport) Download(ctx context.Context, remotePath, localPath string, progress func(Progress)) (TransferResult, error) {
	c, err := t.newClient()
	if err != nil {
		return TransferResult{}, err
	}
	defer c.Close()

	rp := normalizeRemotePath(remotePath)
	r, err := c.Open(rp)
	if err != nil {
		return TransferResult{}, toTransferError(err)
	}
	defer r.Close()

	var total int64 = -1
	if st, err := r.Stat(); err == nil {
		total = st.Size()
	}

	lp := filepath.Clean(localPath)
	if err := os.MkdirAll(filepath.Dir(lp), 0755); err != nil {
		return TransferResult{}, err
	}
	w, err := os.Create(lp)
	if err != nil {
		return TransferResult{}, toTransferError(err)
	}
	defer w.Close()

	n, err := copyWithProgress(ctx, w, r, total, progress)
	if err != nil {
		return TransferResult{}, err
	}
	return TransferResult{Bytes: n}, nil
}

func (t *SFTPTransport) newClient() (*sftp.Client, error) {
	c, err := sftp.NewClient(t.client)
	if err != nil {
		return nil, toTransferError(err)
	}
	return c, nil
}

func normalizeRemotePath(p string) string {
	s := strings.TrimSpace(p)
	if s == "" {
		return "."
	}
	return s
}

func joinRemote(dir, name string) string {
	if dir == "." || dir == "/" {
		return dir + name
	}
	if strings.HasSuffix(dir, "/") {
		return dir + name
	}
	return dir + "/" + name
}

func toTransferError(err error) error {
	if err == nil {
		return nil
	}
	var te *TransferError
	if errors.As(err, &te) {
		return te
	}
	msg := err.Error()
	lower := strings.ToLower(msg)

	if strings.Contains(lower, "subsystem") && strings.Contains(lower, "failed") {
		return &TransferError{Code: ErrorCodeSFTPNotSupported, Message: "对端未开启 SFTP（Subsystem sftp 不可用）"}
	}
	if strings.Contains(lower, "unknown channel") || strings.Contains(lower, "channel open failure") {
		return &TransferError{Code: ErrorCodeSFTPNotSupported, Message: "对端不支持 SFTP 通道"}
	}
	if strings.Contains(lower, "permission denied") {
		return &TransferError{Code: ErrorCodePermissionDenied, Message: "权限不足"}
	}
	if strings.Contains(lower, "no such file") || strings.Contains(lower, "not found") {
		return &TransferError{Code: ErrorCodeNotFound, Message: "文件或目录不存在"}
	}
	if strings.Contains(lower, "unable to authenticate") || strings.Contains(lower, "authentication") {
		return &TransferError{Code: ErrorCodeAuthFailed, Message: "认证失败"}
	}
	if strings.Contains(lower, "connection refused") || strings.Contains(lower, "connection reset") || strings.Contains(lower, "broken pipe") || strings.Contains(lower, "i/o timeout") {
		return &TransferError{Code: ErrorCodeNetwork, Message: "网络连接异常"}
	}
	return &TransferError{Code: ErrorCodeUnknown, Message: msg}
}
