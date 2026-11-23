package opssidecar

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"

	"opscopilot/pkg/core/ops"
	"opscopilot/pkg/terminalcontrol"
)

const (
	terminalReadChunk       = 32 << 10              // 单次读块大小
	terminalMaxFrame        = 256 << 10             // 合帧上限：洪泛输出按此尺寸成批下发，不丢帧
	terminalFrameFlushDelay = 40 * time.Millisecond // 半帧挂起时的延迟冲刷
	terminalRingCap         = 256 << 10             // 重放环形缓冲容量
	terminalDeadSessionTTL  = 30 * time.Minute      // 死会话保留时长（供同会话重连重放）
)

var (
	errTerminalUnknown = errors.New("终端会话不存在")
	errTerminalDead    = errors.New("终端会话已断开")
)

// terminalSession 一个交互终端会话（= 一条独立 SSH/telnet 连接，对齐
// OpsCopilot「每页签一条连接」）。连接由 Manager.ConnectIndependent 拨出，
// 不进共享池、不受空闲回收；关会话即关连接。同服务器多会话互不影响。
type terminalSession struct {
	id       string
	serverID string // 树节点 ID（重连时再拨号用）
	host     string

	mu     sync.Mutex
	stdin  io.WriteCloser
	conn   *ops.Connection
	alive  bool
	deadAt time.Time
	gate   *outputGate
	ring   *ringBuffer
	ctrl   *terminalcontrol.Controller
}

// terminalManager 终端会话注册表与字节流泵。
type terminalManager struct {
	emit  func(event string, params any)
	tcSvc *terminalcontrol.Service
	dial  func(serverID string) (*ops.Connection, error) // ConnectIndependent 包装

	mu       sync.Mutex
	sessions map[string]*terminalSession
}

func newTerminalManager(emit func(event string, params any), tcSvc *terminalcontrol.Service, dial func(string) (*ops.Connection, error)) *terminalManager {
	return &terminalManager{emit: emit, tcSvc: tcSvc, dial: dial, sessions: map[string]*terminalSession{}}
}

// open 打开（或按 sessionId 重连）终端会话。sessionID 非空且存在时为重挂/
// 重连语义；空 id 新建。返回 sessionId 与重放内容（明文，可空）。
func (tm *terminalManager) open(serverID, sessionID string, cols, rows int) (map[string]any, error) {
	if cols <= 0 {
		cols = 80
	}
	if rows <= 0 {
		rows = 24
	}
	tm.mu.Lock()
	tm.gcLocked()
	existing := tm.sessions[sessionID]
	tm.mu.Unlock()

	if sessionID != "" && existing != nil {
		return tm.reattach(existing, sessionID, cols, rows)
	}
	if sessionID == "" {
		sessionID = uuid.NewString()
	}
	conn, err := tm.dial(serverID)
	if err != nil {
		return nil, err
	}
	sess, err := tm.startSession(sessionID, serverID, conn, cols, rows, "")
	if err != nil {
		_ = conn.Conn.Close()
		return nil, err
	}
	tm.mu.Lock()
	tm.sessions[sessionID] = sess
	tm.mu.Unlock()
	slog.Debug("terminal opened", "sessionId", sessionID, "server", serverID)
	return map[string]any{"sessionId": sessionID}, nil
}

// startSession 在给定连接上启动 PTY 会话与读取泵；reset 为重连时的本地
// 模式复位序列占位（实际复位由 Desktop 写本地 xterm，见 design）。
func (tm *terminalManager) startSession(sessionID, serverID string, conn *ops.Connection, cols, rows int, _ string) (*terminalSession, error) {
	stdin, stdout, err := conn.Conn.StartShell(cols, rows)
	if err != nil {
		return nil, fmt.Errorf("建立 PTY 会话失败: %w", err)
	}
	sess := &terminalSession{
		id:       sessionID,
		serverID: serverID,
		host:     conn.Host,
		stdin:    stdin,
		conn:     conn,
		alive:    true,
		gate:     newOutputGate(),
		ring:     newRingBuffer(terminalRingCap),
	}
	// 接入终端控制通道（AI 读屏/审批写入的记账点）。失败降级为普通终端。
	if tm.tcSvc != nil {
		if ctrl, wrapped, attachErr := tm.tcSvc.AttachConnection(sessionID, uuid.NewString(), stdin); attachErr == nil {
			sess.stdin = wrapped
			sess.ctrl = ctrl
		} else {
			slog.Warn("terminal control attach failed", "sessionId", sessionID, "error", attachErr)
		}
	}
	go tm.pump(sess, stdout)
	return sess, nil
}

// reattach 重挂会话：存活 → 换新闸门等 terminal/ready，环形缓冲整段重放；
// 已死 → 同 sessionId 重新独立拨号并重放旧现场（模式复位由 Desktop 写本地 xterm）。
func (tm *terminalManager) reattach(sess *terminalSession, sessionID string, cols, rows int) (map[string]any, error) {
	sess.mu.Lock()
	if sess.alive {
		old := sess.gate
		sess.gate = newOutputGate()
		replay := sess.ring.snapshot()
		for _, chunk := range old.markReady() {
			sess.ring.append(chunk)
			replay = append(replay, chunk...)
		}
		sess.mu.Unlock()
		return map[string]any{"sessionId": sessionID, "replay": string(replay)}, nil
	}
	serverID := sess.serverID
	sess.mu.Unlock()

	conn, err := tm.dial(serverID)
	if err != nil {
		return nil, fmt.Errorf("重连失败: %w", err)
	}
	sess.mu.Lock()
	replay := sess.ring.snapshot()
	sess.mu.Unlock()
	// 旧连接已死，直接整条替换；ring 保留继续累计
	fresh, err := tm.startSession(sessionID, serverID, conn, cols, rows, "")
	if err != nil {
		_ = conn.Conn.Close()
		return nil, err
	}
	sess.mu.Lock()
	oldRing := sess.ring
	sess.mu.Unlock()
	fresh.ring = oldRing
	tm.mu.Lock()
	tm.sessions[sessionID] = fresh
	tm.mu.Unlock()
	slog.Debug("terminal reconnected", "sessionId", sessionID)
	return map[string]any{"sessionId": sessionID, "replay": string(replay)}, nil
}

// pump 终端输出泵：32KB 读 → 256KB 合帧（或 40ms 延迟冲刷）→ 闸门/环形缓冲/
// terminal-data:{id} 事件（明文）。
func (tm *terminalManager) pump(sess *terminalSession, stdout io.Reader) {
	chunks := make(chan []byte)
	errCh := make(chan error, 1)
	go func() {
		defer close(chunks)
		buf := make([]byte, terminalReadChunk)
		for {
			n, err := stdout.Read(buf)
			if n > 0 {
				chunk := make([]byte, n)
				copy(chunk, buf[:n])
				chunks <- chunk
			}
			if err != nil {
				errCh <- err
				return
			}
		}
	}()

	flushCh := make(chan struct{}, 1)
	var timer *time.Timer
	frame := make([]byte, 0, 64*1024)
	flush := func() {
		if timer != nil {
			timer.Stop()
			timer = nil
		}
		if len(frame) == 0 {
			return
		}
		data := frame
		frame = make([]byte, 0, cap(data))
		tm.deliver(sess, data)
	}

	for {
		select {
		case chunk, ok := <-chunks:
			if !ok {
				flush()
				tm.finish(sess, <-errCh)
				return
			}
			frame = append(frame, chunk...)
			if len(frame) >= terminalMaxFrame {
				flush()
				continue
			}
			if timer == nil {
				timer = time.AfterFunc(terminalFrameFlushDelay, func() {
					select {
					case flushCh <- struct{}{}:
					default:
					}
				})
			}
		case <-flushCh:
			timer = nil
			flush()
		}
	}
}

// deliver 单段输出的一致出口：控制通道 Feed 过滤 → 环形缓冲与事件按同序
// 落盘；前端未就绪先入闸门。事件名与载荷对齐 ops：terminal-data:{id} 明文。
func (tm *terminalManager) deliver(sess *terminalSession, data []byte) {
	if sess.ctrl != nil {
		data = sess.ctrl.Feed(data)
	}
	if len(data) == 0 {
		return
	}
	sess.gate.deliver(data, func(d []byte) {
		sess.ring.append(d)
		tm.emit(EventTerminalDataPrefix+sess.id, string(d))
	})
}

// finish SSH 流结束（EOF 或错误）：关闭整条独立连接，标记死会话（ring 保留
// 供重连重放），下发 session-disconnected（载荷对齐 ops）。
func (tm *terminalManager) finish(sess *terminalSession, err error) {
	sess.mu.Lock()
	if !sess.alive {
		sess.mu.Unlock()
		return
	}
	sess.alive = false
	sess.stdin = nil
	conn := sess.conn
	sess.conn = nil
	sess.deadAt = time.Now()
	sess.mu.Unlock()

	if conn != nil {
		_ = conn.Conn.Close()
	}
	reason, message := "eof", "远程主机关闭了连接"
	if err != nil && err != io.EOF {
		reason, message = "error", fmt.Sprintf("连接错误: %v", err)
	}
	tm.emit(EventSessionDisconnected, map[string]any{
		"sessionId":   sess.id,
		"reason":      reason,
		"message":     message,
		"timestamp":   time.Now().Unix(),
		"recoverable": true,
	})
}

func (tm *terminalManager) write(sessionID, data string) error {
	sess := tm.get(sessionID)
	if sess == nil {
		return errTerminalUnknown
	}
	sess.mu.Lock()
	stdin, alive := sess.stdin, sess.alive
	sess.mu.Unlock()
	if !alive || stdin == nil {
		return errTerminalDead
	}
	_, err := stdin.Write([]byte(data))
	return err
}

// ready 前端监听就绪：冲刷闸门暂存的首段输出。
func (tm *terminalManager) ready(sessionID string) error {
	sess := tm.get(sessionID)
	if sess == nil {
		return errTerminalUnknown
	}
	for _, chunk := range sess.gate.markReady() {
		sess.ring.append(chunk)
		tm.emit(EventTerminalDataPrefix+sess.id, string(chunk))
	}
	return nil
}

func (tm *terminalManager) resize(sessionID string, cols, rows int) error {
	sess := tm.get(sessionID)
	if sess == nil {
		return errTerminalUnknown
	}
	sess.mu.Lock()
	conn, alive := sess.conn, sess.alive
	sess.mu.Unlock()
	if !alive || conn == nil {
		return errTerminalDead
	}
	return conn.Conn.Resize(cols, rows)
}

// close 关闭并移除会话（整条独立连接随之断开）。
func (tm *terminalManager) close(sessionID string) error {
	tm.mu.Lock()
	sess := tm.sessions[sessionID]
	delete(tm.sessions, sessionID)
	tm.mu.Unlock()
	if sess == nil {
		return errTerminalUnknown
	}
	sess.mu.Lock()
	stdin, conn, ctrl := sess.stdin, sess.conn, sess.ctrl
	sess.alive = false
	sess.stdin = nil
	sess.conn = nil
	sess.ctrl = nil
	sess.mu.Unlock()
	sess.gate.close()
	if stdin != nil {
		_ = stdin.Close()
	}
	if conn != nil {
		_ = conn.Conn.Close()
	}
	if ctrl != nil && tm.tcSvc != nil {
		tm.tcSvc.Detach(sessionID, ctrl)
	}
	return nil
}

func (tm *terminalManager) closeAll() {
	tm.mu.Lock()
	ids := make([]string, 0, len(tm.sessions))
	for id := range tm.sessions {
		ids = append(ids, id)
	}
	tm.mu.Unlock()
	for _, id := range ids {
		_ = tm.close(id)
	}
}

// ---- Agent 工具（terminal_list / terminal_observe） ----

// activeCount 存活会话数（空闲判定用）。
func (tm *terminalManager) activeCount() int {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	count := 0
	for _, sess := range tm.sessions {
		sess.mu.Lock()
		alive := sess.alive
		sess.mu.Unlock()
		if alive {
			count++
		}
	}
	return count
}

// listForAgent 当前终端会话清单（含死会话：可重连重放）。
func (tm *terminalManager) listForAgent() ([]map[string]any, error) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	result := []map[string]any{}
	for _, sess := range tm.sessions {
		sess.mu.Lock()
		entry := map[string]any{"session_id": sess.id, "host": sess.host, "alive": sess.alive}
		sess.mu.Unlock()
		result = append(result, entry)
	}
	return result, nil
}

const observeLimitBytes = 16 << 10

// observeForAgent 读屏：增量读控制器输出环（after = 上次 Sequence）。
func (tm *terminalManager) observeForAgent(sessionID string, after int) (map[string]any, error) {
	sess := tm.get(sessionID)
	if sess == nil {
		return nil, errTerminalUnknown
	}
	sess.mu.Lock()
	ctrl := sess.ctrl
	sess.mu.Unlock()
	if ctrl == nil {
		return nil, errors.New("该终端未接入控制通道，请重开终端")
	}
	if after < 0 {
		after = 0
	}
	snapshot, observation, err := ctrl.Observe(uint64(after), observeLimitBytes)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"session_id": sessionID,
		"sequence":   snapshot.Sequence,
		"revision":   snapshot.Revision,
		"ready":      snapshot.Ready,
		"owned":      snapshot.Owned,
		"from":       observation.From,
		"to":         observation.To,
		"truncated":  observation.Truncated,
		"text":       observation.Text,
	}, nil
}

func (tm *terminalManager) get(sessionID string) *terminalSession {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	return tm.sessions[sessionID]
}

// gcLocked 丢弃超时的死会话（重放窗口有限，避免注册表无限增长）。
func (tm *terminalManager) gcLocked() {
	cutoff := time.Now().Add(-terminalDeadSessionTTL)
	for id, sess := range tm.sessions {
		sess.mu.Lock()
		stale := !sess.alive && !sess.deadAt.IsZero() && sess.deadAt.Before(cutoff)
		sess.mu.Unlock()
		if stale {
			sess.gate.close()
			delete(tm.sessions, id)
		}
	}
}
