package main

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"opscopilot/pkg/filetransfer"
	"opscopilot/pkg/remote"
	"opscopilot/pkg/session"
	"opscopilot/pkg/terminal"
	"opscopilot/pkg/terminalcontrol"
)

// terminalConnection binds all callbacks to one actual connection, not merely
// the tab ID reused by reconnect. Publication and invalidation share its lock.
type terminalConnection struct {
	connectionID string
	lifetime     *session.Lifetime
	registered   *session.Session
	handoff      *terminalcontrol.Controller
	gate         *terminalOutputGate
	extractor    *terminal.CommandExtractor
}

// A connected transport can still be blocked starting its shell. Explicit tab
// close disposes it immediately; the late start result cannot publish afterward.
type pendingTerminal struct{ close func() }

func (a *App) writeTerminal(id, data string, emit func(string, interface{})) {
	sess, ok := a.sessionMgr.Get(id)
	if !ok || sess.Stdin == nil {
		return
	}
	n, err := sess.Stdin.Write([]byte(data))
	if err == nil && n != len(data) {
		err = io.ErrShortWrite
	}
	// A blocked old write can return after reconnect. Its error and recording
	// must not attach to the replacement's UI or command extractor.
	a.terminalLifecycles.ApplyCurrent(id, func() {
		if current, ok := a.sessionMgr.Get(id); !ok || current != sess {
			return
		}
		if err != nil {
			emit("terminal-data:"+id, fmt.Sprintf("\r\nWrite Error: %v\r\n", err))
			return
		}
		a.recordInput(id, data)
	})
}

func (a *App) installTerminal(lifetime *session.Lifetime, id string, config ConnectConfig, conn remote.Connection, stdin io.WriteCloser) (*terminalConnection, error) {
	target := &terminalConnection{connectionID: uuid.NewString(), lifetime: lifetime, extractor: terminal.NewCommandExtractor()}
	var err error
	accepted := lifetime.Apply(func() {
		if config.Protocol == "" || config.Protocol == "ssh" {
			target.handoff, stdin, err = a.terminalControlService().AttachConnection(id, target.connectionID, stdin)
			if err != nil {
				return
			}
		}
		a.pendingTerminals.Delete(id)
		target.registered = a.sessionMgr.AddWithID(id, conn, stdin)
		a.setConfig(id, config)
		target.gate = a.storeSessionState(id, config).Output
		a.extractorMu.Lock()
		if a.commandExtractors == nil {
			a.commandExtractors = make(map[string]*terminal.CommandExtractor)
		}
		a.commandExtractors[id] = target.extractor
		a.extractorMu.Unlock()
	})
	if !accepted {
		return nil, errors.New("页签已关闭，本次连接已取消")
	}
	if err != nil {
		return nil, fmt.Errorf("初始化终端输入控制失败: %w", err)
	}
	return target, nil
}

func (a *App) readTerminalSession(target *terminalConnection, stdout io.Reader, emit func(string, interface{})) {
	id := target.registered.ID
	deliver := func(data string) {
		target.gate.deliver(data, func(s string) { emit("terminal-data:"+id, s) })
	}
	buf := make([]byte, 32768)
	for {
		n, err := stdout.Read(buf)
		data := ""
		if n > 0 {
			data = readTerminalChunk(stdout, buf, n)
		}
		// Reads may block indefinitely. Only the bounded processing/publication
		// section holds this tab's lifetime lock, never the transport read.
		if !target.lifetime.Apply(func() {
			if n == 0 {
				return
			}
			if target.handoff != nil {
				data = string(target.handoff.Feed([]byte(data)))
			}
			if a.coreRecorder != nil {
				a.coreRecorder.AddEvent("terminal_output", data, map[string]interface{}{"session_id": id})
			}
			if cmd, found := target.extractor.ProcessOutput(data); found && a.coreRecorder != nil {
				if a.coreRecorder.UpdateLastCommand(id, cmd) {
					slog.Debug("recorder corrected command via output (tab completion)", "session", id)
				}
			}
			deliver(data)
		}) {
			return
		}
		if err == nil {
			continue
		}
		var removed *session.Session
		var transports []*filetransfer.RootRelayTransport
		target.lifetime.Finish(func() {
			a.cancelAssistantTarget(id, target.connectionID)
			reason, message := DisconnectEOF, "远程主机关闭了连接"
			disconnectMsg := "\r\n[断开] 连接已关闭\r\n"
			if err != io.EOF {
				reason, message = DisconnectError, fmt.Sprintf("连接错误: %v", err)
				disconnectMsg = fmt.Sprintf("\r\n[断开] %s\r\n", message)
			}
			// Revoke control before notifying the UI. The next reconnect is
			// admitted only after state and registries agree on disconnection.
			if target.handoff != nil {
				a.terminalControlService().Detach(id, target.handoff)
			}
			a.updateSessionState(id, "disconnected", string(reason))
			a.extractorMu.Lock()
			if a.commandExtractors[id] == target.extractor {
				delete(a.commandExtractors, id)
			}
			a.extractorMu.Unlock()
			removed = a.sessionMgr.TakeIf(id, target.registered)
			transports = a.takeRelayTransports(id)
			deliver(disconnectMsg)
			emit("session-disconnected", map[string]interface{}{
				"sessionId": id, "connectionId": target.connectionID, "reason": string(reason), "message": message, "timestamp": time.Now().Unix(),
			})
		})
		for _, transport := range transports {
			transport.Close()
		}
		if removed != nil && removed.Conn != nil {
			_ = removed.Conn.Close()
		}
		return
	}
}
