// Package shellsidecar — Shell 插件的本地 Go 进程（sidecar）。
//
// 职责与约束（docs/workbench-shell-plugin-plan.md §1–2）：
//   - 连接所有权在本进程：浏览器刷新/前端插件重载不杀 SSH；
//   - 控制面 stdio JSON-RPC 2.0（--dev 模式下同协议镜像到 WS /rpc 供独立调试）；
//   - 数据面本地 WebSocket 二进制 PTY 字节流——刻意不经过宿主事件通道
//     （宿主下行事件白名单不可扩展，且会话日志不适合承载字节流）；
//   - 复用 pkg/remote 注册的协议栈（SSH 由 pkg/sshclient 提供，TELNET 同样可用），
//     本包不感知具体协议。
package shellsidecar

import (
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"time"

	// 注册 SSH 协议 Dialer（remote.Dial 依赖 init 副作用；TELNET 同理由
	// telnetclient 注册——按需在宿主装配处追加）。
	"opscopilot/pkg/remote"
	_ "opscopilot/pkg/sshclient"
)

const (
	// replayBufferSize 断线重连时的输出重放环形缓冲。
	replayBufferSize = 256 << 10
	// subscriberBuffer 单个订阅者的输出队列长度；写满视为消费者过慢，踢出
	// （客户端重连后从重放缓冲续流），绝不让慢客户端阻塞 PTY 泵。
	subscriberBuffer = 256
	// pumpReadBuf 单次读取 PTY 的缓冲。
	pumpReadBuf = 32 << 10
)

// TerminalService 管理 sidecar 名下的连接与终端会话。
type TerminalService struct {
	version string
	// notify 把 sidecar 事件推给宿主（stdio 通知）；可为 nil（纯测试）。
	notify func(method string, params any)
	// inputRecorder 可选输入录制钩子（脚本录制：Write 路径拦截，前端零参与）。
	inputRecorder func(terminalID string, data []byte)

	mu          sync.Mutex
	dialer      func(*remote.ConnectConfig) (remote.Connection, error)
	connections map[string]*managedConnection
	terminals   map[string]*terminal
}

type managedConnection struct {
	id        string
	cfg       remote.ConnectConfig
	conn      remote.Connection
	terminals map[string]struct{}
}

type terminal struct {
	id           string
	connectionID string
	stdin        io.WriteCloser

	mu   sync.Mutex
	ring *ringBuffer
	subs map[*subscriber]struct{}
}

// subscriber 是一个数据面消费者（一条 WebSocket）。
type subscriber struct {
	ch        chan []byte
	closeOnce sync.Once
	done      chan struct{}
}

func (s *subscriber) send(data []byte) (overflowed bool) {
	select {
	case s.ch <- data:
		return false
	default:
		return true
	}
}

func (s *subscriber) close() {
	s.closeOnce.Do(func() { close(s.ch); close(s.done) })
}

// Attachment 是一次数据面挂载：先重放环形缓冲，再从 Ch 续流。
type Attachment struct {
	Replay []byte
	Ch     <-chan []byte
	detach func()
}

// Detach 取消挂载（客户端主动断开时调用）。
func (a *Attachment) Detach() { a.detach() }

func newID() string {
	// 时间戳纳秒 + 自增序列，本进程规模下碰撞概率足够低，不引入 uuid 依赖。
	seq := idSeq.Add(1)
	return fmt.Sprintf("%x-%d", time.Now().UnixNano(), seq)
}

var idSeq atomic.Uint64

func NewTerminalService(version string) *TerminalService {
	return &TerminalService{
		version:     version,
		dialer:      remote.Dial,
		connections: map[string]*managedConnection{},
		terminals:   map[string]*terminal{},
	}
}

// SetNotify 注入宿主通知回路（terminal/exited 等）。
func (s *TerminalService) SetNotify(fn func(method string, params any)) {
	s.notify = fn
}

// Connection 返回指定连接（供 Transfer 等能力借用 SFTP 等扩展接口）。
func (s *TerminalService) Connection(connectionID string) (remote.Connection, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	mc, ok := s.connections[connectionID]
	if !ok {
		return nil, fmt.Errorf("连接不存在: %s", connectionID)
	}
	return mc.conn, nil
}

// ConnectionOfTerminal 返回终端所属连接（共享 FilesPanel 按终端会话定位连接）。
func (s *TerminalService) ConnectionOfTerminal(terminalID string) (remote.Connection, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.terminals[terminalID]
	if !ok {
		return nil, fmt.Errorf("终端不存在: %s", terminalID)
	}
	mc, ok := s.connections[t.connectionID]
	if !ok {
		return nil, fmt.Errorf("连接不存在: %s", t.connectionID)
	}
	return mc.conn, nil
}

// Connect 建立到远端的连接。拨号可能耗时数秒，调用方（RPC 层）在独立 goroutine 处理。
func (s *TerminalService) Connect(cfg remote.ConnectConfig) (string, error) {
	conn, err := s.dialer(&cfg)
	if err != nil {
		return "", fmt.Errorf("连接 %s 失败: %w", cfg.Host, err)
	}
	id := "conn-" + newID()
	s.mu.Lock()
	s.connections[id] = &managedConnection{id: id, cfg: cfg, conn: conn, terminals: map[string]struct{}{}}
	s.mu.Unlock()
	return id, nil
}

// Disconnect 断开连接：先关其全部终端，再关连接本体。
func (s *TerminalService) Disconnect(connectionID string) error {
	s.mu.Lock()
	mc, ok := s.connections[connectionID]
	if !ok {
		s.mu.Unlock()
		return fmt.Errorf("连接不存在: %s", connectionID)
	}
	termIDs := make([]string, 0, len(mc.terminals))
	for id := range mc.terminals {
		termIDs = append(termIDs, id)
	}
	s.mu.Unlock()
	for _, id := range termIDs {
		_ = s.CloseTerminal(id)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.connections[connectionID]; !ok {
		return nil // 已被并发回收
	}
	delete(s.connections, connectionID)
	return mc.conn.Close()
}

// OpenTerminal 在指定连接上开交互式终端并启动输出泵。
func (s *TerminalService) OpenTerminal(connectionID string, cols, rows int) (string, error) {
	s.mu.Lock()
	mc, ok := s.connections[connectionID]
	if !ok {
		s.mu.Unlock()
		return "", fmt.Errorf("连接不存在: %s", connectionID)
	}
	s.mu.Unlock()

	stdin, stdout, err := mc.conn.StartShell(cols, rows)
	if err != nil {
		return "", fmt.Errorf("打开终端失败: %w", err)
	}
	id := "term-" + newID()
	t := &terminal{
		id: id, connectionID: connectionID, stdin: stdin,
		ring: newRingBuffer(replayBufferSize), subs: map[*subscriber]struct{}{},
	}
	s.mu.Lock()
	// 连接可能在 StartShell 期间被断开。
	if _, ok := s.connections[connectionID]; !ok {
		s.mu.Unlock()
		_ = stdin.Close()
		return "", fmt.Errorf("连接不存在: %s", connectionID)
	}
	s.terminals[id] = t
	mc.terminals[id] = struct{}{}
	s.mu.Unlock()

	go s.pump(t, stdout)
	return id, nil
}

// pump 把 PTY 输出广播给全部订阅者并写入重放缓冲；读结束即视为终端退出。
func (s *TerminalService) pump(t *terminal, stdout io.Reader) {
	buf := make([]byte, pumpReadBuf)
	for {
		n, err := stdout.Read(buf)
		if n > 0 {
			data := append([]byte(nil), buf[:n]...)
			var overflowed []*subscriber
			t.mu.Lock()
			t.ring.write(data)
			for sub := range t.subs {
				if sub.send(data) {
					overflowed = append(overflowed, sub)
				}
			}
			for _, sub := range overflowed {
				delete(t.subs, sub)
			}
			t.mu.Unlock()
			// 锁外关闭，避免死锁；被踢客户端重连后从重放缓冲续流。
			for _, sub := range overflowed {
				sub.close()
			}
		}
		if err != nil {
			s.finishTerminal(t, err)
			return
		}
	}
}

// finishTerminal 终端退出：摘除、关 stdin、踢掉订阅者、通知宿主。
func (s *TerminalService) finishTerminal(t *terminal, cause error) {
	s.mu.Lock()
	if _, ok := s.terminals[t.id]; !ok {
		s.mu.Unlock()
		return
	}
	delete(s.terminals, t.id)
	if mc, ok := s.connections[t.connectionID]; ok {
		delete(mc.terminals, t.id)
	}
	t.mu.Lock()
	subs := make([]*subscriber, 0, len(t.subs))
	for sub := range t.subs {
		subs = append(subs, sub)
	}
	t.subs = map[*subscriber]struct{}{}
	t.mu.Unlock()
	_ = t.stdin.Close()
	s.mu.Unlock()

	for _, sub := range subs {
		sub.close()
	}
	if s.notify != nil {
		reason := "进程结束"
		if cause != nil && cause != io.EOF {
			reason = cause.Error()
		}
		s.notify("terminal/exited", map[string]string{terminalIDField: t.id, "reason": reason})
	}
}

// Attach 为一条数据面连接挂载终端：在锁内完成"订阅 → 拷贝重放缓冲"，
// 保证不丢不重（泵同样持锁追加+广播）。
func (s *TerminalService) Attach(terminalID string) (*Attachment, error) {
	s.mu.Lock()
	t, ok := s.terminals[terminalID]
	s.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("终端不存在: %s", terminalID)
	}
	sub := &subscriber{ch: make(chan []byte, subscriberBuffer), done: make(chan struct{})}
	t.mu.Lock()
	t.subs[sub] = struct{}{}
	replay := t.ring.snapshot()
	t.mu.Unlock()
	return &Attachment{
		Replay: replay,
		Ch:     sub.ch,
		detach: func() {
			t.mu.Lock()
			if _, ok := t.subs[sub]; ok {
				delete(t.subs, sub)
				t.mu.Unlock()
				sub.close()
				return
			}
			t.mu.Unlock()
		},
	}, nil
}

// WriteInput 把用户键入写入 PTY stdin；同时喂给录制钩子（若已装配）。
func (s *TerminalService) WriteInput(terminalID string, data []byte) error {
	s.mu.Lock()
	t, ok := s.terminals[terminalID]
	recorder := s.inputRecorder
	s.mu.Unlock()
	if !ok {
		return fmt.Errorf("终端不存在: %s", terminalID)
	}
	if recorder != nil {
		recorder(terminalID, data)
	}
	_, err := t.stdin.Write(data)
	return err
}

// SetInputRecorder 装配输入录制钩子（脚本录制服务在启动时注入）。
func (s *TerminalService) SetInputRecorder(fn func(terminalID string, data []byte)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.inputRecorder = fn
}

// SessionInfo 返回终端所属连接的主机与用户（脚本录制元数据用）。
func (s *TerminalService) SessionInfo(terminalID string) (host, user string, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.terminals[terminalID]
	if !ok {
		return "", "", fmt.Errorf("终端不存在: %s", terminalID)
	}
	mc, ok := s.connections[t.connectionID]
	if !ok {
		return "", "", fmt.Errorf("连接不存在: %s", t.connectionID)
	}
	return mc.cfg.Host, mc.cfg.User, nil
}

// Resize 调整远端终端尺寸。
func (s *TerminalService) Resize(terminalID string, cols, rows int) error {
	s.mu.Lock()
	t, ok := s.terminals[terminalID]
	var conn remote.Connection
	if ok {
		conn = s.connections[t.connectionID].conn
	}
	s.mu.Unlock()
	if !ok {
		return fmt.Errorf("终端不存在: %s", terminalID)
	}
	return conn.Resize(cols, rows)
}

// CloseTerminal 主动关闭终端。
func (s *TerminalService) CloseTerminal(terminalID string) error {
	s.mu.Lock()
	t, ok := s.terminals[terminalID]
	s.mu.Unlock()
	if !ok {
		return nil // 幂等：已退出视为成功
	}
	s.finishTerminal(t, nil)
	return nil
}

// Shutdown 关闭全部连接（进程退出时调用）。
func (s *TerminalService) Shutdown() {
	s.mu.Lock()
	ids := make([]string, 0, len(s.connections))
	for id := range s.connections {
		ids = append(ids, id)
	}
	s.mu.Unlock()
	for _, id := range ids {
		_ = s.Disconnect(id)
	}
}
