package mcp

// The agent launches a small stdio process. Its authenticated, private local
// connection forwards each MCP message to the owning conversation's existing
// tool boundary; it never exposes tool authority outside that conversation.
import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"time"
)

const StdioAddressEnv = "OPSCOPILOT_STDIO_BRIDGE_ADDRESS"
const StdioTokenEnv = "OPSCOPILOT_STDIO_BRIDGE_TOKEN"
const maxStdioMessage = 32768

type StdioBridge struct {
	Address     string
	Token       string
	listener    net.Listener
	handler     *Server
	cancel      context.CancelFunc
	once        sync.Once
	connections chan struct{}
}

type bridgeRequest struct {
	Token   string          `json:"token"`
	Message json.RawMessage `json:"message"`
}
type bridgeReply struct {
	Message json.RawMessage `json:"message,omitempty"`
	Error   string          `json:"error,omitempty"`
}

func StartStdioBridge(ctx context.Context, caller Caller) (*StdioBridge, error) {
	if caller == nil || ctx.Err() != nil {
		return nil, errors.New("MCP 调用范围无效")
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(ctx)
	token := hex.EncodeToString(key)
	b := &StdioBridge{Address: listener.Addr().String(), Token: token, listener: listener, cancel: cancel, connections: make(chan struct{}, 16),
		handler: &Server{Token: token, ctx: ctx, caller: caller, busy: make(chan struct{}, 1)}}
	go func() { <-ctx.Done(); b.Close() }()
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			select {
			case b.connections <- struct{}{}:
				go func() { defer func() { <-b.connections }(); b.handle(ctx, conn) }()
			default:
				_ = conn.Close()
			}
		}
	}()
	return b, nil
}

func (b *StdioBridge) Close() { b.once.Do(func() { b.cancel(); _ = b.listener.Close() }) }

func (b *StdioBridge) handle(ctx context.Context, conn net.Conn) {
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	reader := bufio.NewReader(io.LimitReader(conn, maxStdioMessage+1024))
	var request bridgeRequest
	if json.NewDecoder(reader).Decode(&request) != nil || len(request.Message) > maxStdioMessage || subtle.ConstantTimeCompare([]byte(request.Token), []byte(b.Token)) != 1 {
		return
	}
	// Read timeout covers the initial frame, not an operation awaiting approval.
	_ = conn.SetReadDeadline(time.Time{})
	req := httptest.NewRequestWithContext(ctx, "POST", "http://127.0.0.1/mcp", bytes.NewReader(request.Message))
	req.Header.Set("Authorization", "Bearer "+b.Token)
	response := httptest.NewRecorder()
	b.handler.ServeHTTP(response, req)
	result := bridgeReply{}
	if response.Code == http.StatusAccepted {
		return
	}
	if response.Code != http.StatusOK || !json.Valid(response.Body.Bytes()) {
		result.Error = "MCP 请求失败"
	} else {
		result.Message = response.Body.Bytes()
	}
	_ = json.NewEncoder(conn).Encode(result)
}

// RunStdioProxy serves newline-delimited MCP on stdio. Credentials come from
// the MCP process environment, never command-line arguments or logs.
func RunStdioProxy(ctx context.Context, input io.Reader, output io.Writer, address, token string) error {
	if !strings.HasPrefix(address, "127.0.0.1:") || len(token) != 64 {
		return errors.New("MCP 桥接配置无效")
	}
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 4096), maxStdioMessage)
	var writeMu sync.Mutex
	var workers sync.WaitGroup
	workSlots := make(chan struct{}, 16)
	for scanner.Scan() {
		message := append([]byte(nil), scanner.Bytes()...)
		if !json.Valid(message) {
			return errors.New("MCP 消息无效")
		}
		select {
		case workSlots <- struct{}{}:
		case <-ctx.Done():
			return ctx.Err()
		}
		workers.Add(1)
		go func() {
			defer workers.Done()
			defer func() { <-workSlots }()
			respondError := func() {
				var incoming struct {
					ID json.RawMessage `json:"id"`
				}
				if json.Unmarshal(message, &incoming) != nil || len(incoming.ID) == 0 {
					return
				}
				payload, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": incoming.ID, "error": map[string]any{"code": -32000, "message": "Ops 工具桥接不可用"}})
				writeMu.Lock()
				defer writeMu.Unlock()
				_, _ = output.Write(append(payload, '\n'))
			}
			dialer := net.Dialer{Timeout: 5 * time.Second}
			conn, err := dialer.DialContext(ctx, "tcp4", address)
			if err != nil {
				respondError()
				return
			}
			defer conn.Close()
			stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
			defer stop()
			if json.NewEncoder(conn).Encode(bridgeRequest{Token: token, Message: message}) != nil {
				respondError()
				return
			}
			var reply bridgeReply
			if json.NewDecoder(io.LimitReader(conn, 8<<20)).Decode(&reply) != nil || len(reply.Message) == 0 {
				respondError()
				return
			}
			writeMu.Lock()
			defer writeMu.Unlock()
			_, _ = output.Write(append(reply.Message, '\n'))
		}()
	}
	workers.Wait()
	return scanner.Err()
}
