package opssidecar

import (
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"

	"opscopilot/pkg/core/ops"
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
	errTerminalBusy    = errors.New("该服务器已有活动终端会话，请先关闭现有终端")
)

// terminalSession 一个交互终端会话。
//
// 与 OpsCopilot GUI 的「每页签独占连接」不同，sidecar 的终端与 exec/传输共享
// Manager 连接池（凭据解析、跳板机、空闲回收全部单点复用）。代价是同一服务器
// 同一时刻只允许一个活动终端：底层 sshclient 以 lastSession 归属 Resize 与
// CloseShell，多个并发 PTY 会互相错伤。会话独立于 Desktop 侧连接存活——
// webview 重载后按 sessionId 重放 ring 恢复现场；SSH 侧断开后会话标记 dead，
// 同 sessionId 重开即重连服务器（先写模式复位序列再重放）。
type terminalSession struct {
	id   string
	host string

	mu     sync.Mutex
	stdin  io.WriteCloser
	conn   *ops.Connection
	alive  bool
	deadAt time.Time
	gate   *outputGate
	ring   *ringBuffer
}

// terminalManager 终端会话注册表与字节流泵。
type terminalManager struct {
	emit func(event string, params any)

	mu       sync.Mutex
	sessions map[string]*terminalSession
}

func newTerminalManager(emit func(event string, params any)) *terminalManager {
	return &terminalManager{emit: emit, sessions: map[string]*terminalSession{}}
}

// open 打开或重连终端会话。conn 由调用方保证已连接（Manager.Connect +
// GetConnection）。sessionID 非空时按语义分流：存活会话重挂（重放 + 换新
// 闸门），死会话同 id 重连服务器；空 id 新开。终端模式复位序列不经过
// sidecar——Desktop 侧直接写本地 xterm 实例（OpsCopilot Issue #65/#67 的
// 语义：复位的是本地实例的残留模式，与远端无关）。
func (tm *terminalManager) open(conn *ops.Connection, sessionID string, cols, rows int) (map[string]any, error) {
	if cols <= 0 {
		cols = 80
	}
	if rows <= 0 {
		rows = 24
	}

	tm.mu.Lock()
	tm.gcLocked()
	tm.mu.Unlock()

	if sessionID != "" {
		tm.mu.Lock()
		existing := tm.sessions[sessionID]
		tm.mu.Unlock()
		if existing != nil && existing.host == conn.Host {
			return tm.reattach(existing, conn, sessionID, cols, rows)
		}
	}
	if alive := tm.findAlive(conn.Host); alive != nil {
		return nil, errTerminalBusy
	}
	if sessionID == "" {
		sessionID = uuid.NewString()
	}

	stdin, stdout, err := conn.Conn.StartShell(cols, rows)
	if err != nil {
		return nil, fmt.Errorf("建立 PTY 会话失败: %w", err)
	}
	sess := &terminalSession{
		id:    sessionID,
		host:  conn.Host,
		stdin: stdin,
		conn:  conn,
		alive: true,
		gate:  newOutputGate(),
		ring:  newRingBuffer(terminalRingCap),
	}
	tm.mu.Lock()
	tm.sessions[sessionID] = sess
	tm.mu.Unlock()
	slog.Debug("terminal opened", "sessionId", sessionID, "host", conn.Host)
	go tm.pump(sess, stdout)
	return map[string]any{"sessionId": sessionID}, nil
}

// reattach 重挂会话：存活 → 换新闸门等 terminal/ready，环形缓冲整段重放；
// 已死 → 同 sessionId 重连服务器并重放旧现场（模式复位由 Desktop 写本地
// xterm，在 ready 前完成，顺序由闸门保证）。
func (tm *terminalManager) reattach(sess *terminalSession, conn *ops.Connection, sessionID string, cols, rows int) (map[string]any, error) {
	sess.mu.Lock()
	defer sess.mu.Unlock()

	if sess.alive {
		old := sess.gate
		sess.gate = newOutputGate()
		replay := sess.ring.snapshot()
		// 旧闸门内尚未冲刷的输出并入 ring 与重放，保证零丢失
		for _, chunk := range old.markReady() {
			sess.ring.append(chunk)
			replay = append(replay, chunk...)
		}
		return map[string]any{
			"sessionId": sessionID,
			"replay":    base64.StdEncoding.EncodeToString(replay),
		}, nil
	}

	stdin, stdout, err := conn.Conn.StartShell(cols, rows)
	if err != nil {
		return nil, fmt.Errorf("重连 PTY 会话失败: %w", err)
	}
	sess.stdin = stdin
	sess.conn = conn
	sess.gate = newOutputGate()
	sess.alive = true
	sess.deadAt = time.Time{}
	replay := sess.ring.snapshot()
	slog.Debug("terminal reconnected", "sessionId", sessionID, "host", conn.Host)
	go tm.pump(sess, stdout)
	return map[string]any{
		"sessionId": sessionID,
		"replay":    base64.StdEncoding.EncodeToString(replay),
	}, nil
}

// pump 终端输出泵：32KB 读 → 256KB 合帧（或 40ms 延迟冲刷）→ 闸门/环形缓冲/
// 事件。所有 frame 操作收敛在 pump goroutine 内，无需额外加锁。
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

// deliver 单段输出的一致出口：环形缓冲与事件按同序落盘；前端未就绪先入闸门。
func (tm *terminalManager) deliver(sess *terminalSession, data []byte) {
	sess.gate.deliver(data, func(d []byte) {
		sess.ring.append(d)
		tm.emit(EventTerminalOutput, map[string]any{
			"sessionId": sess.id,
			"data":      base64.StdEncoding.EncodeToString(d),
		})
	})
}

// finish SSH 流结束（EOF 或错误）后标记会话死亡并通知前端；会话记录保留
// ring，供同 sessionId 重连重放。
func (tm *terminalManager) finish(sess *terminalSession, err error) {
	sess.mu.Lock()
	if !sess.alive {
		sess.mu.Unlock()
		return
	}
	sess.alive = false
	sess.stdin = nil
	sess.deadAt = time.Now()
	sess.mu.Unlock()

	reason := "远程主机关闭了连接"
	if err != nil && err != io.EOF {
		reason = fmt.Sprintf("连接错误: %v", err)
	}
	tm.emit(EventTerminalClosed, map[string]any{
		"sessionId": sess.id,
		"reason":    reason,
	})
}

// write 向终端写入按键数据。
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

// ready 前端监听就绪：冲刷闸门暂存的首段输出（ring 与事件同序补记）。
func (tm *terminalManager) ready(sessionID string) error {
	sess := tm.get(sessionID)
	if sess == nil {
		return errTerminalUnknown
	}
	for _, chunk := range sess.gate.markReady() {
		sess.ring.append(chunk)
		tm.emit(EventTerminalOutput, map[string]any{
			"sessionId": sess.id,
			"data":      base64.StdEncoding.EncodeToString(chunk),
		})
	}
	return nil
}

// resize 同步终端尺寸到远端（SSH WindowChange / telnet NAWS）。
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

// close 关闭并移除会话：会话级关闭不殃及共享连接（exec/传输继续可用）。
func (tm *terminalManager) close(sessionID string) error {
	tm.mu.Lock()
	sess := tm.sessions[sessionID]
	delete(tm.sessions, sessionID)
	tm.mu.Unlock()
	if sess == nil {
		return errTerminalUnknown
	}
	sess.mu.Lock()
	stdin, conn := sess.stdin, sess.conn
	sess.alive = false
	sess.stdin = nil
	sess.mu.Unlock()
	sess.gate.close()
	if stdin != nil {
		_ = stdin.Close()
	}
	if conn != nil {
		_ = conn.Conn.CloseShell()
	}
	return nil
}

// closeAll 服务退出时关闭全部会话。
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

// cancelAll 预留对称出口（当前无异步任务，保持与服务 Close 的一致形态）。
func (tm *terminalManager) cancelAll() {}

func (tm *terminalManager) get(sessionID string) *terminalSession {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	return tm.sessions[sessionID]
}

func (tm *terminalManager) findAlive(host string) *terminalSession {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	for _, sess := range tm.sessions {
		sess.mu.Lock()
		alive := sess.alive
		sess.mu.Unlock()
		if alive && sess.host == host {
			return sess
		}
	}
	return nil
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
