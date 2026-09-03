package shellsidecar

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// 数据面：/terminals/{terminalId}?token=...（二进制帧）；
// dev 模式额外暴露 /rpc（JSON-RPC 镜像，文本帧一帧一请求）。
// 两者仅监听 127.0.0.1 且必须携带 token。

const (
	wsWriteTimeout = 10 * time.Second
	wsPongWait     = 60 * time.Second
	wsPingPeriod   = 45 * time.Second
)

type dataPlane struct {
	service  *TerminalService
	token    string
	upgrader websocket.Upgrader
}

// ServeDataPlane 启动数据面监听。返回 http.Server 由调用方 Shutdown。
func ServeDataPlane(addr, token string, service *TerminalService, dev *DevHub, workspace ...http.Handler) (*http.Server, string, error) {
	// 本地 sidecar 场景：前端 origin 与 sidecar 端口必然不同源（浏览器 WS 带
	// Origin，Go 探针不带——零值 CheckOrigin 会把浏览器全拒掉）。鉴权由
	// token + 仅绑定 127.0.0.1 承担，Origin 一律放行。
	dp := &dataPlane{service: service, token: token, upgrader: websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}}
	mux := http.NewServeMux()
	mux.HandleFunc("/terminals/", dp.handleTerminal)
	if len(workspace) > 0 && workspace[0] != nil {
		mux.Handle("/workspace", workspace[0])
	}
	if dev != nil {
		mux.HandleFunc("/rpc", dev.handleRPC)
	}
	server := &http.Server{Addr: addr, Handler: mux}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, "", err
	}
	go func() { _ = server.Serve(ln) }()
	return server, "ws://" + ln.Addr().String(), nil
}

func (dp *dataPlane) handleTerminal(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("token") != dp.token {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	terminalID := strings.TrimPrefix(r.URL.Path, "/terminals/")
	if terminalID == "" {
		http.Error(w, "bad path", http.StatusBadRequest)
		return
	}
	attachment, err := dp.service.Attach(terminalID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	ws, err := dp.upgrader.Upgrade(w, r, nil)
	if err != nil {
		attachment.Detach()
		return
	}
	defer ws.Close()
	defer attachment.Detach()

	// 先重放，再续流（Attach 在锁内同时完成订阅，保证不丢不重）。
	if len(attachment.Replay) > 0 {
		_ = ws.SetWriteDeadline(time.Now().Add(wsWriteTimeout))
		if err := ws.WriteMessage(websocket.BinaryMessage, attachment.Replay); err != nil {
			return
		}
	}
	_ = ws.SetReadDeadline(time.Now().Add(wsPongWait))
	ws.SetPongHandler(func(string) error { return ws.SetReadDeadline(time.Now().Add(wsPongWait)) })

	// 读方向：用户键入 → PTY stdin。读失败（含 pong 超时）即断开本客户端。
	done := make(chan struct{})
	defer close(done)
	go func() {
		defer attachment.Detach()
		defer ws.Close()
		for {
			kind, data, err := ws.ReadMessage()
			if err != nil {
				return
			}
			if kind != websocket.BinaryMessage && kind != websocket.TextMessage {
				continue
			}
			if err := dp.service.WriteInput(terminalID, data); err != nil {
				return
			}
		}
	}()

	// 写方向：PTY 输出 → 客户端，选路串行化（gorilla 单写者约束）。
	// 心跳必须在这里发：浏览器自动回 pong 保活读超时；不 ping 则空闲 60s
	// 被读超时断开（真机冒烟暴露：切走标签再回来连接已死）。
	ping := time.NewTicker(wsPingPeriod)
	defer ping.Stop()
	_ = ws.SetReadDeadline(time.Now().Add(wsPongWait))
	ws.SetPongHandler(func(string) error { return ws.SetReadDeadline(time.Now().Add(wsPongWait)) })
	for {
		select {
		case data, ok := <-attachment.Ch:
			if !ok {
				_ = ws.SetWriteDeadline(time.Now().Add(wsWriteTimeout))
				_ = ws.WriteMessage(websocket.CloseMessage, nil)
				return
			}
			_ = ws.SetWriteDeadline(time.Now().Add(wsWriteTimeout))
			if err := ws.WriteMessage(websocket.BinaryMessage, data); err != nil {
				return
			}
		case <-ping.C:
			_ = ws.SetWriteDeadline(time.Now().Add(wsWriteTimeout))
			if err := ws.WriteControl(websocket.PingMessage, nil, time.Now().Add(wsWriteTimeout)); err != nil {
				return
			}
		case <-done:
			return
		}
	}
}

// DevHub 是 --dev 模式的控制面镜像：浏览器开发页直连 sidecar 调 RPC，
// 无需平台宿主。仅用于本地开发，平台模式为 nil。
type DevHub struct {
	api      *ControlAPI
	upgrader websocket.Upgrader
	mu       sync.Mutex
	conns    map[*devConn]struct{}
}

type devConn struct {
	send func(v any) error
	mu   sync.Mutex
}

func (c *devConn) push(v any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.send(v)
}

// NewDevHub 构造 dev 控制面镜像。
func NewDevHub(api *ControlAPI) *DevHub {
	return &DevHub{
		api:      api,
		upgrader: websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }},
		conns:    map[*devConn]struct{}{},
	}
}

// broadcast 把 sidecar 通知推给所有 dev 控制连接。
func (h *DevHub) broadcast(method string, params any) {
	frame := map[string]any{"jsonrpc": "2.0", "method": method, "params": params}
	h.mu.Lock()
	conns := make([]*devConn, 0, len(h.conns))
	for c := range h.conns {
		conns = append(conns, c)
	}
	h.mu.Unlock()
	for _, c := range conns {
		_ = c.push(frame)
	}
}

func (h *DevHub) handleRPC(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("token") != h.api.Token {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	ws, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer ws.Close()
	conn := &devConn{send: func(v any) error {
		_ = ws.SetWriteDeadline(time.Now().Add(wsWriteTimeout))
		return ws.WriteJSON(v)
	}}
	h.mu.Lock()
	h.conns[conn] = struct{}{}
	h.mu.Unlock()
	defer func() {
		h.mu.Lock()
		delete(h.conns, conn)
		h.mu.Unlock()
	}()

	_ = ws.SetReadDeadline(time.Now().Add(wsPongWait))
	ws.SetPongHandler(func(string) error { return ws.SetReadDeadline(time.Now().Add(wsPongWait)) })
	// ping 保活（经 conn 锁串行化，避免与应答写并发）
	go func() {
		ticker := time.NewTicker(wsPingPeriod)
		defer ticker.Stop()
		for range ticker.C {
			_ = ws.SetWriteDeadline(time.Now().Add(wsWriteTimeout))
			conn.mu.Lock()
			err := ws.WriteControl(websocket.PingMessage, nil, time.Now().Add(wsWriteTimeout))
			conn.mu.Unlock()
			if err != nil {
				return
			}
		}
	}()

	for {
		var req rpcRequest
		if err := ws.ReadJSON(&req); err != nil {
			return
		}
		reqCopy := req
		go func() {
			result, rpcErr := h.api.dispatch(r.Context(), &reqCopy)
			if reqCopy.ID == nil {
				return
			}
			resp := rpcResponse{JSONRPC: "2.0", ID: reqCopy.ID}
			if rpcErr != nil {
				resp.Error = rpcErr
			} else {
				resp.Result = result
			}
			_ = conn.push(resp)
		}()
	}
}
