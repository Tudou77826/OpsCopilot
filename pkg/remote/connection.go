package remote

import (
	"context"
	"io"

	"github.com/pkg/sftp"
)

// Connection 是所有远程连接协议必须实现的核心能力契约。
//
// 设计原则:
//   - 接口只包含所有协议都能实现的能力;协议特有的能力走可选能力接口
//     (SFTPCapable / AutoLoginCapable),调用方类型断言查询。
//   - StartShell 返回三元组(stdin, stdout, err),不再暴露 *ssh.Session
//     等协议专属类型。SSH 实现内部自行持有 session 引用以支持 Resize。
//   - Resize / Healthy / Close 由各协议自行映射到本协议的等价语义:
//       SSH:   Resize -> session.WindowChange; Healthy -> 试建 NewSession
//       Telnet: Resize -> IAC SB NAWS;        Healthy -> IAC NOP
type Connection interface {
	// StartShell 启动交互式 shell,返回 stdin(可写)、stdout(可读)。
	// cols/rows 为初始终端尺寸。
	StartShell(cols, rows int) (stdin io.WriteCloser, stdout io.Reader, err error)

	// Resize 调整远端终端尺寸。各协议映射到本协议的尺寸通知机制。
	Resize(cols, rows int) error

	// Run 执行单条命令并返回 stdout。ctx 取消时应尽快返回。
	// 协议若无"单次执行"语义(如 Telnet),实现内部可起临时 shell 完成。
	Run(ctx context.Context, cmd string) (string, error)

	// Healthy 探测连接是否仍然可用,返回 true 表示连接健康。
	// 各协议映射到本协议的探活机制;失败应返回 false 而非 panic。
	Healthy() bool

	// Close 关闭连接,释放底层资源。重复调用应安全。
	Close() error

	// Protocol 返回协议标识(ProtocolSSH / ProtocolTelnet)。
	Protocol() string
}

// SFTPCapable 是可选能力接口:仅支持 SFTP 的协议实现。
// 调用方(如文件传输)应通过类型断言查询:
//
//	sftpConn, ok := conn.(remote.SFTPCapable)
//	if !ok { return errors.New("当前协议不支持文件传输") }
//
// 这样不支持 SFTP 的协议(如 Telnet)在调用点优雅降级,符合接口隔离原则,
// 而非让所有协议假装实现一个返回 error 的方法。
type SFTPCapable interface {
	Connection

	// SFTPClient 返回底层 SSH 连接上的 SFTP 客户端。
	// 实现可缓存客户端,多次调用复用同一实例。
	SFTPClient() (*sftp.Client, error)
}

// AutoLoginCapable 是可选能力接口:协议本身无标准认证(如 Telnet),
// 连上后在 PTY 数据流里手工输入凭据。实现本接口的协议可由上层自动回填
// 用户名/密码(监控 Login:/Password: 提示)。
//
// 调用方通过类型断言查询:
//
//	if al, ok := conn.(remote.AutoLoginCapable); ok && user != "" {
//	    stdin, stdout, err = al.StartShellWithAutoLogin(cols, rows, user, pwd)
//	}
//
// SSH 不实现本接口,断言失败即跳过,SSH 路径不受影响。
type AutoLoginCapable interface {
	// StartShellWithAutoLogin 启动 shell,并在 stdout 上套一层监控装饰器:
	// 看到登录提示时自动向 stdin 回填用户名/密码。数据原样透传给调用方。
	StartShellWithAutoLogin(cols, rows int, user, password string) (stdin io.WriteCloser, stdout io.Reader, err error)
}
