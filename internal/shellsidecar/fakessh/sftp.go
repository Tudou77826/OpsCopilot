// fakessh 的 SFTP 子系统：根目录沙箱化处理器（仅测试用途）。
// 只有 fakessh.Start 收到非空 sftpRoot 时才会启用 sftp 通道；所有路径被
// 强制限制在 root 之下。实现 pkg/sftp 的 Handlers 接口。
package fakessh

import (
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/pkg/sftp"
)

type rootedFS struct{ root string }

// resolve 把客户端路径强制限制在沙箱根内。
func (h rootedFS) resolve(p string) string {
	clean := path.Clean("/" + strings.ReplaceAll(p, "\\", "/"))
	return filepath.Join(h.root, filepath.FromSlash(clean))
}

func (h rootedFS) Filecmd(r *sftp.Request) error {
	switch r.Method {
	case "Setstat":
		return nil
	case "Rename":
		return os.Rename(h.resolve(r.Filepath), h.resolve(r.Target))
	case "Rmdir":
		return os.Remove(h.resolve(r.Filepath))
	case "Mkdir":
		return os.MkdirAll(h.resolve(r.Filepath), 0o755)
	case "Remove":
		return os.Remove(h.resolve(r.Filepath))
	default:
		return sftp.ErrSSHFxOpUnsupported
	}
}

func (h rootedFS) Fileread(r *sftp.Request) (io.ReaderAt, error) {
	return os.Open(h.resolve(r.Filepath)) // *os.File 实现 io.ReaderAt
}

func (h rootedFS) Filewrite(r *sftp.Request) (io.WriterAt, error) {
	return os.OpenFile(h.resolve(r.Filepath), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644) // *os.File 实现 io.WriterAt
}

type listerAt []fs.FileInfo

func (l listerAt) ListAt(at []fs.FileInfo, offset int64) (int, error) {
	if offset >= int64(len(l)) {
		return 0, io.EOF
	}
	n := copy(at, l[offset:])
	if offset+int64(n) >= int64(len(l)) {
		return n, io.EOF
	}
	return n, nil
}

func (h rootedFS) Filelist(r *sftp.Request) (sftp.ListerAt, error) {
	switch r.Method {
	case "Stat":
		info, err := os.Stat(h.resolve(r.Filepath))
		if err != nil {
			return nil, err
		}
		return listerAt{info}, nil
	case "List":
		entries, err := os.ReadDir(h.resolve(r.Filepath))
		if err != nil {
			return nil, err
		}
		infos := make([]fs.FileInfo, 0, len(entries))
		for _, e := range entries {
			info, err := e.Info()
			if err == nil {
				infos = append(infos, info)
			}
		}
		return listerAt(infos), nil
	default:
		return nil, sftp.ErrSSHFxOpUnsupported
	}
}

// serveSFTP 在沙箱根上启动 SFTP 请求服务器（阻塞，调用方放 goroutine）。
func serveSFTP(ch io.ReadWriteCloser, root string) {
	handler := rootedFS{root: root}
	server := sftp.NewRequestServer(ch, sftp.Handlers{
		FileGet:  handler,
		FilePut:  handler,
		FileCmd:  handler,
		FileList: handler,
	})
	if server == nil {
		_ = ch.Close()
		return
	}
	_ = server.Serve()
	_ = ch.Close()
}
