// Package fakessh 是测试设施：进程内最小 SSH 服务器（仅回显）。
// 用途：让 sidecar → 真实 pkg/sshclient → PTY → 数据面的集成测试与浏览器
// 冒烟能走完整 SSH 协议，而不依赖任何真实服务器/凭据。
// 只实现 sshclient.NewClient 需要的部分：密码认证、pty-req、shell、
// window-change、env；会话行为 = 启动横幅 + 全量回显。
package fakessh

import (
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"net"
	"strings"
	"sync"

	"golang.org/x/crypto/ssh"
)

// fakeMonitorSample 是 exec 采样命令的 canned 输出（loadavg + meminfo + df）。
const fakeMonitorSample = `0.42 0.31 0.28 1/312 4242
MemTotal:        2048000 kB
MemAvailable:    1536000 kB
512  409600 286720 122880  31% /
`

// Server 是一个测试 SSH 服务器。零值不可用，用 Start 创建。
type Server struct {
	listener net.Listener
	config   *ssh.ServerConfig

	mu       sync.Mutex
	sessions int
	Closed   chan struct{}

	// Banner 是连接建立后主动下发的一行文本（默认空）。
	Banner string
	// sftpRoot 非空时启用 SFTP 子系统（沙箱）。
	sftpRoot string
}

// Close 停止监听并断开接受循环（已建立连接由其会话自行结束）。
func (s *Server) Close() error { return s.listener.Close() }

// Host 返回实际监听的 host:port。
func (s *Server) Host() string { return s.listener.Addr().String() }

// Port 返回监听端口。
func (s *Server) Port() int {
	return s.listener.Addr().(*net.TCPAddr).Port
}

// Start 在 127.0.0.1 随机端口启动服务器。认证：user=test password=test。
func Start(banner string, sftpRoot string) (*Server, error) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		return nil, err
	}
	config := &ssh.ServerConfig{
		PasswordCallback: func(meta ssh.ConnMetadata, password []byte) (*ssh.Permissions, error) {
			if meta.User() == "test" && string(password) == "test" {
				return nil, nil
			}
			return nil, fmt.Errorf("认证失败")
		},
	}
	config.AddHostKey(signer)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	s := &Server{listener: ln, config: config, Closed: make(chan struct{}), Banner: banner, sftpRoot: sftpRoot}
	go s.serve()
	return s, nil
}

func (s *Server) serve() {
	defer close(s.Closed)
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return
		}
		go s.handleConn(conn)
	}
}

func (s *Server) handleConn(netConn net.Conn) {
	sconn, chans, reqs, err := ssh.NewServerConn(netConn, s.config)
	if err != nil {
		return
	}
	defer sconn.Close()
	go ssh.DiscardRequests(reqs)
	for newCh := range chans {
		if newCh.ChannelType() == "sftp" {
			if s.sftpRoot == "" {
				_ = newCh.Reject(ssh.UnknownChannelType, "sftp disabled")
				continue
			}
			ch, _, err := newCh.Accept()
			if err != nil {
				continue
			}
			go serveSFTP(ch, s.sftpRoot)
			continue
		}
		if newCh.ChannelType() != "session" {
			_ = newCh.Reject(ssh.UnknownChannelType, "unsupported")
			continue
		}
		ch, chReqs, err := newCh.Accept()
		if err != nil {
			continue
		}
		s.mu.Lock()
		s.sessions++
		s.mu.Unlock()
		go s.handleSession(ch, chReqs)
	}
}

func (s *Server) handleSession(ch ssh.Channel, reqs <-chan *ssh.Request) {
	defer ch.Close()
	pty := false
	shellUp := false
	for req := range reqs {
		switch req.Type {
		case "pty-req":
			_ = req.Reply(true, nil)
			pty = true
		case "shell":
			if !pty {
				_ = req.Reply(false, nil)
				continue
			}
			_ = req.Reply(true, nil)
			shellUp = true
			if s.Banner != "" {
				_, _ = ch.Write([]byte(s.Banner))
			}
			// 回显循环：客户端发的每个字节原样返回（测试语义：所见即所发）。
			go func() {
				buf := make([]byte, 4096)
				for {
					n, err := ch.Read(buf)
					if n > 0 {
						if _, werr := ch.Write(buf[:n]); werr != nil {
							return
						}
					}
					if err != nil {
						return
					}
				}
			}()
		case "subsystem":
			// 标准 SFTP 客户端：session 通道上请求 subsystem "sftp"，
			// 协议直接跑在该通道上（不是独立通道类型）。
			var sub struct {
				Name string
			}
			_ = ssh.Unmarshal(req.Payload, &sub)
			if sub.Name == "sftp" && s.sftpRoot != "" {
				_ = req.Reply(true, nil)
				serveSFTP(ch, s.sftpRoot)
				return // 通道已被 SFTP 接管
			}
			_ = req.Reply(false, nil)
		case "window-change":
			_ = req.Reply(true, nil)
		case "env":
			if len(req.Payload) >= 4 {
				_ = req.Reply(true, nil)
			} else {
				_ = req.Reply(false, nil)
			}
		case "exec":
			_ = req.Reply(true, nil)
			go func() {
				var cmd struct {
					Command string `json:"command"`
				}
				_ = ssh.Unmarshal(req.Payload, &cmd)
				if strings.Contains(cmd.Command, "/proc/loadavg") {
					_, _ = ch.Write([]byte(fakeMonitorSample))
				}
				// exit-status 0 + 关闭，client.Run 的 Wait 正常返回
				_, _ = ch.SendRequest("exit-status", false, []byte{0, 0, 0, 0})
				_ = ch.Close()
			}()
		default:
			if req.WantReply {
				_ = req.Reply(false, nil)
			}
		}
		_ = shellUp
	}
}

// SessionCount 返回历史上打开过的会话数（断言用）。
func (s *Server) SessionCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sessions
}
