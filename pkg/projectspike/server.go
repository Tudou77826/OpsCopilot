// 远程项目预验证入口，仅允许访问预先准备好的测试目录。
package projectspike

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"regexp"
	"sync"
	"time"

	"opscopilot/pkg/connectiondiag"
	"opscopilot/pkg/sshclient"
)

var fixturePath = regexp.MustCompile("^/tmp/aaw-remote-project-spike\\.[A-Za-z0-9]{8}$")

type config struct {
	Host, User, Password, HostKey, Root string
	Port                                int
}

type tool struct {
	Name, Description string
	Schema            map[string]any
}

func (t tool) wire() map[string]any {
	return map[string]any{"name": t.Name, "description": t.Description, "inputSchema": t.Schema}
}

func object(fields map[string]any, required ...string) map[string]any {
	if fields == nil {
		fields = map[string]any{}
	}
	if required == nil {
		required = []string{}
	}
	return map[string]any{"type": "object", "properties": fields, "required": required, "additionalProperties": false}
}

type service struct {
	conn          *sshclient.Client
	usesRootLogin bool
	root          string
	token         string
	mu            sync.Mutex
	input         *json.Decoder
	output        *json.Encoder
}

func (s *service) approve(name string, input toolInput) error {
	id := rand.Text()
	event := map[string]any{"event": "permission", "id": id, "tool": name, "path": input.Path, "command": input.Command}
	if name == "edit_file" {
		event["edit"] = map[string]any{"kind": "edit", "path": input.Path, "oldString": input.OldText, "newString": input.NewText}
	} else if name == "write_file" {
		event["edit"] = map[string]any{"kind": "write", "path": input.Path, "content": input.Content}
	}
	if err := s.output.Encode(event); err != nil {
		return err
	}
	var decision struct {
		ID    string
		Allow bool
	}
	if err := s.input.Decode(&decision); err != nil || decision.ID != id || !decision.Allow {
		return errors.New("宿主拒绝操作")
	}
	return nil
}

func (s *service) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || r.URL.Path != "/mcp" {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+s.token)) != 1 {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	var req struct {
		ID     json.RawMessage
		Method string
		Params struct {
			Name      string
			Arguments json.RawMessage
		}
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 6*maxTextBytes)).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if len(req.ID) == 0 {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	reply := map[string]any{"jsonrpc": "2.0", "id": req.ID}
	switch req.Method {
	case "initialize":
		reply["result"] = map[string]any{"protocolVersion": "2024-11-05",
			"capabilities": map[string]any{"tools": map[string]any{}},
			"serverInfo":   map[string]string{"name": "aaw_project_spike", "version": "1"}}
	case "ping":
		reply["result"] = map[string]any{}
	case "tools/list":
		var list []map[string]any
		for _, item := range tools {
			list = append(list, item.wire())
		}
		reply["result"] = map[string]any{"tools": list}
	case "tools/call":
		ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
		defer cancel()
		s.mu.Lock()
		value, err := s.call(ctx, req.Params.Name, req.Params.Arguments)
		_ = s.output.Encode(map[string]any{"event": "tool", "tool": req.Params.Name, "ok": err == nil, "result": value})
		s.mu.Unlock()
		body, _ := json.Marshal(value)
		if err != nil {
			body = []byte(err.Error())
		}
		reply["result"] = map[string]any{"isError": err != nil,
			"content": []map[string]string{{"type": "text", "text": string(body)}}}
	default:
		reply["error"] = map[string]any{"code": -32601, "message": "未知 MCP 方法"}
	}
	_ = json.NewEncoder(w).Encode(reply)
}

func Run() error {
	input, output := json.NewDecoder(os.Stdin), json.NewEncoder(os.Stdout)
	var opts config
	if err := input.Decode(&opts); err != nil {
		return err
	}
	if !fixturePath.MatchString(opts.Root) || opts.HostKey == "" || opts.Password == "" {
		return errors.New("需要测试目录、固定主机密钥和 SSH 凭据")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	sshConfig := &sshclient.ConnectConfig{
		Host: opts.Host, Port: opts.Port, User: opts.User, Password: opts.Password, HostKey: opts.HostKey,
	}
	conn, err := sshclient.NewClientWithTrace(ctx, sshConfig)
	if errors.Is(err, io.EOF) {
		// 握手前偶发断开时再拨一次，不重试认证或指纹校验失败。
		traceCtx, trace := connectiondiag.New(ctx)
		conn, err = sshclient.NewClientWithTrace(traceCtx, sshConfig)
		if err != nil {
			fmt.Fprintln(os.Stderr, trace.Finish())
		}
	}
	if err != nil {
		return err
	}
	defer conn.Close()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	defer listener.Close()
	s := &service{conn: conn, usesRootLogin: opts.User == "root", root: opts.Root, token: rand.Text(), input: input, output: output}
	if err := output.Encode(map[string]string{"event": "ready", "url": "http://" + listener.Addr().String() + "/mcp", "token": s.token}); err != nil {
		return err
	}
	server := &http.Server{Handler: s, ReadHeaderTimeout: 5 * time.Second}
	return server.Serve(listener)
}
