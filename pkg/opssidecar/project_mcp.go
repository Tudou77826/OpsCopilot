package opssidecar

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/pkg/sftp"
)

const projectApprovalTimeout = 5 * time.Minute

type projectMCPBinding struct {
	Token        string
	ProjectID    string
	ConnectionID string
	Revision     uint64
	RemoteRoot   string
	SessionHint  string
}

type projectApprovalDecision struct {
	Allowed bool
	Reason  string
}

type pendingProjectApproval struct {
	RequestID    string
	Token        string
	ConnectionID string
	Revision     uint64
	Decided      chan projectApprovalDecision
}

type projectMCPManager struct {
	svc      *Service
	mu       sync.Mutex
	bindings map[string]*projectMCPBinding
	pending  map[string]*pendingProjectApproval
}

func newProjectMCPManager(svc *Service) *projectMCPManager {
	return &projectMCPManager{
		svc:      svc,
		bindings: map[string]*projectMCPBinding{},
		pending:  map[string]*pendingProjectApproval{},
	}
}

func (m *projectMCPManager) activeCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.bindings)
}

func (m *projectMCPManager) invalidateConnection(connectionID string) {
	m.mu.Lock()
	var toCancel []*pendingProjectApproval
	for id, p := range m.pending {
		if p.ConnectionID == connectionID {
			toCancel = append(toCancel, p)
			delete(m.pending, id)
		}
	}
	for tok, b := range m.bindings {
		if b.ConnectionID == connectionID {
			delete(m.bindings, tok)
		}
	}
	m.mu.Unlock()
	for _, p := range toCancel {
		select {
		case p.Decided <- projectApprovalDecision{Allowed: false, Reason: "SSH 连接配置已变更，待审批操作已失效"}:
		default:
		}
	}
}

func (m *projectMCPManager) closeAll() {
	m.mu.Lock()
	all := make([]*pendingProjectApproval, 0, len(m.pending))
	for id, p := range m.pending {
		all = append(all, p)
		delete(m.pending, id)
	}
	m.bindings = map[string]*projectMCPBinding{}
	m.mu.Unlock()
	for _, p := range all {
		select {
		case p.Decided <- projectApprovalDecision{Allowed: false, Reason: "远程项目服务已关闭"}:
		default:
		}
	}
}

func newProjectToken() string {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "ptok-" + newRequestID()
	}
	return "ptok-" + hex.EncodeToString(b)
}

func (s *Service) handleProjectMCPBind(req Request) (Response, bool) {
	var params struct {
		ProjectID    string `json:"projectId"`
		ConnectionID string `json:"connectionId"`
		RemoteRoot   string `json:"remoteRoot"`
		SessionHint  string `json:"sessionHint,omitempty"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil ||
		strings.TrimSpace(params.ProjectID) == "" ||
		strings.TrimSpace(params.ConnectionID) == "" ||
		strings.TrimSpace(params.RemoteRoot) == "" {
		return invalidParams(req.ID, "projectId、connectionId 与 remoteRoot 必填"), false
	}
	addr, _ := s.bridgeInfo()
	if addr == "" {
		return errorResponse(req.ID, CodeInternal, "本地 MCP HTTP 桥尚未就绪", ""), false
	}
	rev := s.projects.revisionOf(params.ConnectionID)
	token := newProjectToken()
	binding := &projectMCPBinding{
		Token:        token,
		ProjectID:    strings.TrimSpace(params.ProjectID),
		ConnectionID: strings.TrimSpace(params.ConnectionID),
		Revision:     rev,
		RemoteRoot:   cleanRemotePath(params.RemoteRoot),
		SessionHint:  strings.TrimSpace(params.SessionHint),
	}
	s.projectMCP.mu.Lock()
	s.projectMCP.bindings[token] = binding
	s.projectMCP.mu.Unlock()
	return okResponse(req.ID, map[string]any{
		"token":    token,
		"url":      "http://" + addr + "/project-mcp",
		"revision": rev,
	}), false
}

func (s *Service) handleProjectMCPUnbind(req Request) (Response, bool) {
	var params struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil || strings.TrimSpace(params.Token) == "" {
		return invalidParams(req.ID, "token 必填"), false
	}
	m := s.projectMCP
	m.mu.Lock()
	delete(m.bindings, params.Token)
	var toCancel []*pendingProjectApproval
	for id, p := range m.pending {
		if p.Token == params.Token {
			toCancel = append(toCancel, p)
			delete(m.pending, id)
		}
	}
	m.mu.Unlock()
	for _, p := range toCancel {
		select {
		case p.Decided <- projectApprovalDecision{Allowed: false, Reason: "会话已关闭，待审批操作已取消"}:
		default:
		}
	}
	return okResponse(req.ID, map[string]any{"unbound": true}), false
}

func (s *Service) handleProjectApprovalReply(req Request) (Response, bool) {
	var params struct {
		RequestID string `json:"requestId"`
		Approved  bool   `json:"approved"`
		Reason    string `json:"reason,omitempty"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil || strings.TrimSpace(params.RequestID) == "" {
		return invalidParams(req.ID, "requestId 必填"), false
	}
	m := s.projectMCP
	m.mu.Lock()
	pending := m.pending[params.RequestID]
	delete(m.pending, params.RequestID)
	m.mu.Unlock()
	if pending == nil {
		return errorResponse(req.ID, CodeNotFound, "没有等待审批的远程项目操作", ""), false
	}
	select {
	case pending.Decided <- projectApprovalDecision{Allowed: params.Approved, Reason: params.Reason}:
	default:
	}
	return okResponse(req.ID, map[string]any{"delivered": true}), false
}

var projectMCPTools = []map[string]any{
	{
		"name":        "read_file",
		"description": "读取远端项目的文本文件，返回内容和版本标识（SHA-256）。使用项目相对路径。",
		"inputSchema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{"type": "string"},
			},
			"required": []string{"path"},
		},
	},
	{
		"name":        "edit_file",
		"description": "按读取版本局部修改远端文本文件。old_text 必须在文件中唯一匹配。",
		"inputSchema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path":     map[string]any{"type": "string"},
				"version":  map[string]any{"type": "string"},
				"old_text": map[string]any{"type": "string"},
				"new_text": map[string]any{"type": "string"},
			},
			"required": []string{"path", "version", "old_text", "new_text"},
		},
	},
	{
		"name":        "write_file",
		"description": "新建或覆盖远端文本文件。新建时省略 version；覆盖已有文件时传入读取版本。",
		"inputSchema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path":    map[string]any{"type": "string"},
				"version": map[string]any{"type": "string"},
				"content": map[string]any{"type": "string"},
			},
			"required": []string{"path", "content"},
		},
	},
	{
		"name":        "run_command",
		"description": "在远端项目目录执行 Linux Shell 命令（可用于检索、Git 查询、运行测试），返回 stdout、stderr 和退出码。",
		"inputSchema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"command": map[string]any{"type": "string"},
				"cwd":     map[string]any{"type": "string"},
			},
			"required": []string{"command"},
		},
	},
}

func (m *projectMCPManager) handleHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "仅支持 POST", http.StatusMethodNotAllowed)
		return
	}
	token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	m.mu.Lock()
	binding := m.bindings[token]
	m.mu.Unlock()
	if binding == nil {
		http.Error(w, "未授权或会话工具已解绑", http.StatusUnauthorized)
		return
	}
	if m.svc.projects.revisionOf(binding.ConnectionID) != binding.Revision {
		http.Error(w, "SSH 连接配置已变更，请重新建立会话", http.StatusUnauthorized)
		return
	}

	raw, err := io.ReadAll(io.LimitReader(r.Body, 2*1024*1024))
	if err != nil {
		http.Error(w, "读取请求失败", http.StatusBadRequest)
		return
	}
	var req mcpRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		http.Error(w, "JSON-RPC 解析失败", http.StatusBadRequest)
		return
	}
	if req.Method == "notifications/initialized" || len(req.ID) == 0 {
		w.WriteHeader(http.StatusAccepted)
		return
	}

	var result any
	var rpcErr error
	switch req.Method {
	case "initialize":
		result = map[string]any{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": "aaw-remote-project", "version": "1.0.0"},
		}
	case "tools/list":
		result = map[string]any{"tools": projectMCPTools}
	case "tools/call":
		var callParams struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &callParams); err != nil {
			rpcErr = errors.New("tools/call 参数无效")
		} else {
			out, callErr := m.executeTool(r.Context(), binding, callParams.Name, callParams.Arguments)
			if callErr != nil {
				result = map[string]any{
					"isError": true,
					"content": []map[string]any{{"type": "text", "text": callErr.Error()}},
				}
			} else {
				encoded, _ := json.Marshal(out)
				result = map[string]any{
					"content": []map[string]any{{"type": "text", "text": string(encoded)}},
				}
			}
		}
	default:
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(mcpError(req.ID, -32601, "未知方法: "+req.Method))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if rpcErr != nil {
		_, _ = w.Write(mcpError(req.ID, -32602, rpcErr.Error()))
		return
	}
	reply, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      req.ID,
		"result":  result,
	})
	_, _ = w.Write(reply)
}

func (m *projectMCPManager) requestApproval(
	ctx context.Context,
	binding *projectMCPBinding,
	tool string,
	summary string,
	input map[string]any,
) error {
	requestID := "pap-" + newRequestID()
	pending := &pendingProjectApproval{
		RequestID:    requestID,
		Token:        binding.Token,
		ConnectionID: binding.ConnectionID,
		Revision:     binding.Revision,
		Decided:      make(chan projectApprovalDecision, 1),
	}
	m.mu.Lock()
	// 二次确认绑定与连接版本在登记审批前仍有效。
	if m.bindings[binding.Token] == nil || m.svc.projects.revisionOf(binding.ConnectionID) != binding.Revision {
		m.mu.Unlock()
		return errors.New("会话工具已失效或 SSH 连接已变更")
	}
	m.pending[requestID] = pending
	m.mu.Unlock()

	m.svc.emitEvent(EventProjectApprovalRequest, map[string]any{
		"requestId":    requestID,
		"token":        binding.Token,
		"projectId":    binding.ProjectID,
		"connectionId": binding.ConnectionID,
		"sessionHint":  binding.SessionHint,
		"tool":         tool,
		"summary":      summary,
		"input":        input,
	})

	select {
	case decision := <-pending.Decided:
		if !decision.Allowed {
			if decision.Reason != "" {
				return errors.New(decision.Reason)
			}
			return errors.New("用户拒绝了该远端项目操作")
		}
		// 批准返回后再校验一次连接修订，防止审批期间切换连接后误提交。
		if m.svc.projects.revisionOf(binding.ConnectionID) != binding.Revision {
			return errors.New("审批期间 SSH 连接配置已变更，操作已取消")
		}
		return nil
	case <-ctx.Done():
		m.mu.Lock()
		delete(m.pending, requestID)
		m.mu.Unlock()
		return errors.New("远端项目操作已取消")
	case <-time.After(projectApprovalTimeout):
		m.mu.Lock()
		delete(m.pending, requestID)
		m.mu.Unlock()
		return errors.New("等待审批超时，操作已取消")
	}
}

func (m *projectMCPManager) executeTool(
	ctx context.Context,
	binding *projectMCPBinding,
	toolName string,
	rawArgs json.RawMessage,
) (any, error) {
	var args struct {
		Path    string  `json:"path"`
		Version *string `json:"version"`
		OldText string  `json:"old_text"`
		NewText *string `json:"new_text"`
		Content *string `json:"content"`
		Command string  `json:"command"`
		Cwd     string  `json:"cwd"`
	}
	if err := json.Unmarshal(rawArgs, &args); err != nil {
		return nil, errors.New("工具参数不是有效的 JSON 对象")
	}

	switch toolName {
	case "read_file":
		target, err := resolveProjectRelPath(binding.RemoteRoot, args.Path)
		if err != nil {
			return nil, err
		}
		if target == binding.RemoteRoot {
			return nil, errors.New("文件路径不能是项目根目录")
		}
		var out projectFileText
		err = m.svc.withProjectSFTP(binding.ConnectionID, func(client *sftp.Client) error {
			var readErr error
			out, readErr = readProjectTextFile(client, target)
			return readErr
		})
		m.svc.audit(auditRecord{
			Time: time.Now().Unix(), Tool: "project.read_file", Server: binding.ConnectionID,
			Command: args.Path, Detail: binding.ProjectID, OK: err == nil,
			Error: errString(err),
		})
		if err != nil {
			return nil, err
		}
		return map[string]any{
			"path":    args.Path,
			"content": out.Content,
			"version": out.Version,
			"bytes":   out.Bytes,
		}, nil

	case "edit_file":
		if args.Version == nil || args.NewText == nil {
			return nil, errors.New("edit_file 需要 version、old_text 与 new_text")
		}
		target, err := resolveProjectRelPath(binding.RemoteRoot, args.Path)
		if err != nil {
			return nil, err
		}
		if err := m.requestApproval(ctx, binding, "edit_file", "修改远端文件 "+args.Path, map[string]any{
			"path":     args.Path,
			"old_text": args.OldText,
			"new_text": *args.NewText,
		}); err != nil {
			m.svc.audit(auditRecord{
				Time: time.Now().Unix(), Tool: "project.edit_file", Server: binding.ConnectionID,
				Command: args.Path, Detail: "denied", OK: false, Error: err.Error(),
			})
			return nil, err
		}
		unlock := m.svc.projectPathLocks.lock(binding.ConnectionID, target)
		defer unlock()
		var out projectFileText
		err = m.svc.withProjectSFTP(binding.ConnectionID, func(client *sftp.Client) error {
			current, readErr := readProjectTextFile(client, target)
			if readErr != nil {
				return readErr
			}
			if current.Version != *args.Version {
				return errProjectConflict
			}
			if args.OldText == "" || strings.Count(current.Content, args.OldText) != 1 {
				return errors.New("old_text 必须在当前文件中恰好匹配一次")
			}
			updated := strings.Replace(current.Content, args.OldText, *args.NewText, 1)
			var writeErr error
			out, writeErr = writeProjectTextFile(client, target, updated, args.Version, false)
			return writeErr
		})
		m.svc.audit(auditRecord{
			Time: time.Now().Unix(), Tool: "project.edit_file", Server: binding.ConnectionID,
			Command: args.Path, Detail: binding.ProjectID, OK: err == nil, Error: errString(err),
		})
		if err != nil {
			return nil, err
		}
		return map[string]any{"path": args.Path, "version": out.Version, "bytes": out.Bytes}, nil

	case "write_file":
		if args.Content == nil {
			return nil, errors.New("write_file 需要 content")
		}
		target, err := resolveProjectRelPath(binding.RemoteRoot, args.Path)
		if err != nil {
			return nil, err
		}
		summary := "新建远端文件 " + args.Path
		if args.Version != nil {
			summary = "写入远端文件 " + args.Path
		}
		if err := m.requestApproval(ctx, binding, "write_file", summary, map[string]any{
			"path":    args.Path,
			"isNew":   args.Version == nil,
			"content": *args.Content,
		}); err != nil {
			m.svc.audit(auditRecord{
				Time: time.Now().Unix(), Tool: "project.write_file", Server: binding.ConnectionID,
				Command: args.Path, Detail: "denied", OK: false, Error: err.Error(),
			})
			return nil, err
		}
		unlock := m.svc.projectPathLocks.lock(binding.ConnectionID, target)
		defer unlock()
		var out projectFileText
		err = m.svc.withProjectSFTP(binding.ConnectionID, func(client *sftp.Client) error {
			var writeErr error
			out, writeErr = writeProjectTextFile(client, target, *args.Content, args.Version, false)
			return writeErr
		})
		m.svc.audit(auditRecord{
			Time: time.Now().Unix(), Tool: "project.write_file", Server: binding.ConnectionID,
			Command: args.Path, Detail: binding.ProjectID, OK: err == nil, Error: errString(err),
		})
		if err != nil {
			return nil, err
		}
		return map[string]any{"path": args.Path, "version": out.Version, "bytes": out.Bytes}, nil

	case "run_command":
		cwd := binding.RemoteRoot
		if strings.TrimSpace(args.Cwd) != "" && strings.TrimSpace(args.Cwd) != "." {
			resolved, err := resolveProjectRelPath(binding.RemoteRoot, args.Cwd)
			if err != nil {
				return nil, err
			}
			cwd = resolved
		}
		if err := m.requestApproval(ctx, binding, "run_command", "运行远端命令: "+args.Command, map[string]any{
			"command": args.Command,
			"cwd":     args.Cwd,
		}); err != nil {
			m.svc.audit(auditRecord{
				Time: time.Now().Unix(), Tool: "project.run_command", Server: binding.ConnectionID,
				Command: args.Command, Detail: "denied", OK: false, Error: err.Error(),
			})
			return nil, err
		}
		res, err := m.svc.runProjectCommand(ctx, "", binding.ConnectionID, cwd, args.Command, defaultProjectExecTimeoutSec, maxProjectExecOutputBytes)
		m.svc.audit(auditRecord{
			Time: time.Now().Unix(), Tool: "project.run_command", Server: binding.ConnectionID,
			Command: args.Command, Detail: fmt.Sprintf("exit=%d", res.ExitCode), OK: err == nil, Error: errString(err),
		})
		if err != nil {
			return nil, err
		}
		return res, nil

	default:
		return nil, errors.New("未知远程项目工具: " + toolName)
	}
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
