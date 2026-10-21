// Package acp adapts newline-delimited ACP JSON-RPC. It never grants application
// authority or executes terminal commands; reverse requests go to a host handler.
package acp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"sync"
	"sync/atomic"
	"unicode/utf8"
)

var ErrClosed = errors.New("ACP 连接已结束")

type RPCError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func (e *RPCError) Error() string { return e.Message }

type Handler interface {
	Request(context.Context, string, json.RawMessage) (any, error)
	Notify(context.Context, string, json.RawMessage)
}
type envelope struct {
	Version string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
	barrier chan struct{}
	handler Handler
}
type Client struct {
	ctx               context.Context
	cancel            context.CancelFunc
	reader            io.ReadCloser
	writer            io.WriteCloser
	closeTransport    func()
	handler           Handler
	mu                sync.Mutex
	writeMu           sync.Mutex
	waiters           map[string]chan envelope
	id                atomic.Uint64
	once              sync.Once
	done              chan struct{}
	notifications     chan envelope
	requests          chan struct{}
	processDone       <-chan struct{}
	startupDiagnostic *startupDiagnostic
}

func (c *Client) StartupDiagnostic() string {
	if c.startupDiagnostic == nil {
		return ""
	}
	return c.startupDiagnostic.summary()
}

func (c *Client) WaitProcess(ctx context.Context) error {
	if c.processDone == nil {
		return nil
	}
	select {
	case <-c.processDone:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func Connect(ctx context.Context, reader io.ReadCloser, writer io.WriteCloser, handler Handler, closeTransport func()) *Client {
	ctx, cancel := context.WithCancel(ctx)
	c := &Client{ctx: ctx, cancel: cancel, reader: reader, writer: writer, handler: handler, closeTransport: closeTransport, waiters: map[string]chan envelope{}, done: make(chan struct{}), notifications: make(chan envelope, 64), requests: make(chan struct{}, 8)}
	go c.read()
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case message := <-c.notifications:
				if message.barrier != nil {
					close(message.barrier)
					continue
				}
				if message.handler != nil {
					message.handler.Notify(ctx, message.Method, message.Params)
				}
			}
		}
	}()
	go func() { <-ctx.Done(); c.Close() }()
	return c
}
func (c *Client) Done() <-chan struct{} { return c.done }

// FlushNotifications waits for updates received before the prompt response.
// A fast final response must not discard queued answer text during shutdown.
func (c *Client) FlushNotifications(ctx context.Context) error {
	done := make(chan struct{})
	select {
	case c.notifications <- envelope{barrier: done}:
	case <-ctx.Done():
		return ctx.Err()
	case <-c.Done():
		return ErrClosed
	}
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-c.Done():
		return ErrClosed
	}
}
func (c *Client) Close() {
	c.once.Do(func() {
		c.cancel()
		if c.closeTransport != nil {
			c.closeTransport()
		}
		_ = c.reader.Close()
		_ = c.writer.Close()
		close(c.done)
	})
}
func (c *Client) send(message envelope) error {
	data, err := json.Marshal(message)
	if err != nil {
		return errors.New("ACP 消息无法编码")
	}
	if len(data) > 1<<20 {
		return errors.New("ACP 消息超出上限")
	}
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if c.ctx.Err() != nil {
		return ErrClosed
	}
	n, err := c.writer.Write(append(data, '\n'))
	if err != nil || n != len(data)+1 {
		c.Close()
		return ErrClosed
	}
	return nil
}
func (c *Client) Call(ctx context.Context, method string, params any, result any) error {
	return c.call(ctx, method, params, result, nil)
}

// onSent is a host-side admission barrier, never a protocol/model callback.
func (c *Client) call(ctx context.Context, method string, params any, result any, onSent func()) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	stop := context.AfterFunc(ctx, c.Close)
	defer stop()
	data, err := json.Marshal(params)
	if err != nil {
		return errors.New("ACP 参数无法编码")
	}
	id, _ := json.Marshal(c.id.Add(1))
	key := string(id)
	wait := make(chan envelope, 1)
	c.mu.Lock()
	if c.ctx.Err() != nil {
		c.mu.Unlock()
		return ErrClosed
	}
	if len(c.waiters) >= 128 {
		c.mu.Unlock()
		return errors.New("ACP 请求过多")
	}
	c.waiters[key] = wait
	c.mu.Unlock()
	defer func() { c.mu.Lock(); delete(c.waiters, key); c.mu.Unlock() }()
	if err := c.send(envelope{Version: "2.0", ID: id, Method: method, Params: data}); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return err
	}
	if onSent != nil {
		onSent()
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-c.ctx.Done():
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return ErrClosed
	case response := <-wait:
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if response.Error != nil {
			return response.Error
		}
		if result == nil {
			return nil
		}
		if err := json.Unmarshal(response.Result, result); err != nil {
			return errors.New("ACP 响应格式无效")
		}
		return nil
	}
}
func (c *Client) Notify(method string, params any) error {
	data, err := json.Marshal(params)
	if err != nil {
		return err
	}
	return c.send(envelope{Version: "2.0", Method: method, Params: data})
}
func validID(id json.RawMessage) bool {
	if len(id) == 0 {
		return true
	}
	var v any
	if json.Unmarshal(id, &v) != nil {
		return false
	}
	switch v.(type) {
	case string, float64:
		return true
	}
	return false
}
func (c *Client) read() {
	defer c.Close()
	scanner := bufio.NewScanner(c.reader)
	scanner.Buffer(make([]byte, 4096), 1<<20)
	for scanner.Scan() {
		line := scanner.Bytes()
		var message envelope
		if !utf8.Valid(line) || json.Unmarshal(line, &message) != nil || message.Version != "2.0" || !validID(message.ID) {
			return
		}
		if message.Method != "" {
			message.handler = c.handler
			if snapshot, ok := c.handler.(interface{ Capture() Handler }); ok {
				message.handler = snapshot.Capture()
			}
			if len(message.ID) == 0 {
				select {
				case c.notifications <- message:
				default:
					return
				}
				continue
			}
			select {
			case c.requests <- struct{}{}:
				go c.handle(message)
			default:
				return
			}
			continue
		}
		if len(message.ID) == 0 || (len(message.Result) == 0) == (message.Error == nil) {
			return
		}
		c.mu.Lock()
		wait := c.waiters[string(message.ID)]
		c.mu.Unlock()
		if wait != nil {
			select {
			case wait <- message:
			default:
				return
			}
		}
	}
}
func (c *Client) handle(request envelope) {
	defer func() { <-c.requests }()
	response := envelope{Version: "2.0", ID: request.ID}
	var value any
	var err error
	if request.handler == nil {
		err = &RPCError{Code: -32601, Message: "宿主未开放该能力"}
	} else {
		value, err = request.handler.Request(c.ctx, request.Method, request.Params)
	}
	if err != nil {
		var rpc *RPCError
		if errors.As(err, &rpc) {
			response.Error = rpc
		} else {
			response.Error = &RPCError{Code: -32000, Message: "宿主请求已取消或未获允许"}
		}
	} else {
		response.Result, _ = json.Marshal(value)
	}
	_ = c.send(response)
}

type InitializeResult struct {
	ProtocolVersion int `json:"protocolVersion"`
	AgentInfo       struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	} `json:"agentInfo"`
}

func (c *Client) Initialize(ctx context.Context) (InitializeResult, error) {
	var result InitializeResult
	err := c.Call(ctx, "initialize", map[string]any{"protocolVersion": 1, "clientInfo": map[string]string{"name": "OpsCopilot", "version": "dev"}, "clientCapabilities": map[string]any{}}, &result)
	if err == nil && result.ProtocolVersion != 1 {
		err = errors.New("不支持此 ACP 协议版本")
	}
	if err != nil {
		c.Close()
	}
	return result, err
}
