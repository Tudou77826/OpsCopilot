package filetransfer

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

type SFTPTransport struct {
	client *ssh.Client
}

func removeRemoteRecursive(ctx context.Context, c *sftp.Client, p string) error {
	select {
	case <-ctx.Done():
		return &TransferError{Code: ErrorCodeUnknown, Message: "传输已取消"}
	default:
	}

	fi, err := c.Stat(p)
	if err != nil {
		return toTransferError(err)
	}
	if !fi.IsDir() {
		if err := c.Remove(p); err != nil {
			return toTransferError(err)
		}
		return nil
	}

	entries, err := c.ReadDir(p)
	if err != nil {
		return toTransferError(err)
	}
	for _, e := range entries {
		child := joinRemote(p, e.Name())
		if e.IsDir() {
			if err := removeRemoteRecursive(ctx, c, child); err != nil {
				return err
			}
		} else {
			if err := c.Remove(child); err != nil {
				return toTransferError(err)
			}
		}
	}
	if err := c.RemoveDirectory(p); err != nil {
		return toTransferError(err)
	}
	return nil
}

func NewSFTPTransport(client *ssh.Client) *SFTPTransport {
	return &SFTPTransport{client: client}
}

func (t *SFTPTransport) Check(ctx context.Context) (bool, string, error) {
	c, err := t.newClient()
	if err != nil {
		slog.Debug("sftp availability check failed", "error", err)
		var te *TransferError
		if errors.As(err, &te) && te.Code == ErrorCodeSFTPNotSupported {
			return false, te.Message, te
		}
		return false, "", err
	}
	_ = c.Close()
	slog.Info("sftp available")
	return true, "", nil
}

func (t *SFTPTransport) List(ctx context.Context, remotePath string) ([]Entry, error) {
	c, err := t.newClient()
	if err != nil {
		return nil, err
	}
	defer c.Close()

	p := normalizeRemotePath(remotePath)
	slog.Debug("sftp list directory", "path", p)
	infos, err := c.ReadDir(p)
	if err != nil {
		return nil, toTransferError(err)
	}

	out := make([]Entry, 0, len(infos))
	for _, fi := range infos {
		owner, group := sftpOwnerGroup(fi)
		out = append(out, Entry{
			Path:    joinRemote(p, fi.Name()),
			Name:    fi.Name(),
			IsDir:   fi.IsDir(),
			Size:    fi.Size(),
			Mode:    uint32(fi.Mode()),
			ModTime: fi.ModTime(),
			Owner:   owner,
			Group:   group,
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
	owner, group := sftpOwnerGroup(fi)
	return Entry{
		Path:    p,
		Name:    filepath.Base(p),
		IsDir:   fi.IsDir(),
		Size:    fi.Size(),
		Mode:    uint32(fi.Mode()),
		ModTime: fi.ModTime(),
		Owner:   owner,
		Group:   group,
	}, nil
}

func sftpOwnerGroup(fi os.FileInfo) (string, string) {
	if ext, ok := fi.(sftp.FileInfoUidGid); ok {
		return strconv.FormatUint(uint64(ext.Uid()), 10), strconv.FormatUint(uint64(ext.Gid()), 10)
	}
	return "", ""
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

	slog.Info("sftp upload started", "src", lp, "dst", remotePath, "size", total)

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
	slog.Info("sftp upload completed", "src", lp, "dst", rp, "bytes", n)
	return TransferResult{Bytes: n}, nil
}

func (t *SFTPTransport) Download(ctx context.Context, remotePath, localPath string, progress func(Progress)) (TransferResult, error) {
	c, err := t.newClient()
	if err != nil {
		return TransferResult{}, err
	}
	defer c.Close()

	rp := normalizeRemotePath(remotePath)
	slog.Info("sftp download started", "src", rp, "dst", localPath)
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
	slog.Info("sftp download completed", "src", rp, "dst", lp, "bytes", n)
	return TransferResult{Bytes: n}, nil
}

func (t *SFTPTransport) Mkdir(ctx context.Context, remotePath string) error {
	c, err := t.newClient()
	if err != nil {
		return err
	}
	defer c.Close()

	p := normalizeRemotePath(remotePath)
	if err := c.MkdirAll(p); err != nil {
		return toTransferError(err)
	}
	return nil
}

func (t *SFTPTransport) Rename(ctx context.Context, oldPath, newPath string) error {
	c, err := t.newClient()
	if err != nil {
		return err
	}
	defer c.Close()

	oldP := normalizeRemotePath(oldPath)
	newP := normalizeRemotePath(newPath)
	if err := c.Rename(oldP, newP); err != nil {
		return toTransferError(err)
	}
	return nil
}

func (t *SFTPTransport) Remove(ctx context.Context, remotePath string, recursive bool) error {
	c, err := t.newClient()
	if err != nil {
		return err
	}
	defer c.Close()

	p := normalizeRemotePath(remotePath)
	if !recursive {
		fi, err := c.Stat(p)
		if err != nil {
			return toTransferError(err)
		}
		if fi.IsDir() {
			if err := c.RemoveDirectory(p); err != nil {
				return toTransferError(err)
			}
			return nil
		}
		if err := c.Remove(p); err != nil {
			return toTransferError(err)
		}
		return nil
	}

	return removeRemoteRecursive(ctx, c, p)
}

func (t *SFTPTransport) ReadFile(ctx context.Context, remotePath string, maxBytes int64) ([]byte, error) {
	c, err := t.newClient()
	if err != nil {
		return nil, err
	}
	defer c.Close()

	p := normalizeRemotePath(remotePath)
	slog.Debug("sftp read file", "path", p, "maxBytes", maxBytes)
	f, err := c.Open(p)
	if err != nil {
		return nil, toTransferError(err)
	}
	defer f.Close()

	if maxBytes <= 0 {
		maxBytes = 256 * 1024
	}
	b, err := io.ReadAll(io.LimitReader(f, maxBytes+1))
	if err != nil {
		return nil, toTransferError(err)
	}
	if int64(len(b)) > maxBytes {
		return nil, &TransferError{Code: ErrorCodeNotSupported, Message: "文件过大，暂不支持直接编辑"}
	}
	return b, nil
}

func (t *SFTPTransport) WriteFile(ctx context.Context, remotePath string, content []byte) error {
	c, err := t.newClient()
	if err != nil {
		return err
	}
	defer c.Close()

	p := normalizeRemotePath(remotePath)
	slog.Debug("sftp write file", "path", p, "size", len(content))
	f, err := c.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_TRUNC)
	if err != nil {
		return toTransferError(err)
	}
	defer f.Close()

	if _, err := f.Write(content); err != nil {
		return toTransferError(err)
	}
	return nil
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
	// sshd MaxSessions/MaxStartups 拒绝新通道:并发传输过多时出现,
	// 与"对端不支持 SFTP"不同,重试或降低并发即可恢复。
	if strings.Contains(lower, "administratively prohibited") ||
		strings.Contains(lower, "channel open failed") ||
		strings.Contains(lower, "too many open sessions") {
		return &TransferError{Code: ErrorCodeSessionLimit, Message: "并发传输过多，服务端拒绝新会话，请稍后重试或减少批量大小"}
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
