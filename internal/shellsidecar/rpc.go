package shellsidecar

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"

	"opscopilot/pkg/completion"
	"opscopilot/pkg/remote"
	"opscopilot/pkg/script"
)

// JSON-RPC 2.0 over 行分隔 JSON（stdio；dev 模式镜像到 /rpc 文本帧）。
// 请求可乱序应答（SSH 拨号耗时数秒，每请求独立 goroutine），应答按 id 配对，
// 通知随时下行。stdout 只承载协议消息。

const (
	protocolVersion = 1
	rpcMaxLine      = 1 << 20
)

const terminalIDField = "terminalId"

// ControlAPI 是控制面方法集：stdio（平台模式）与 /rpc 镜像（dev 模式）共用。
type ControlAPI struct {
	Service *TerminalService
	Version string
	// Token 是数据面鉴权 token，initialize 时随能力信息返回给宿主。
	Token string
	// Dev 非 nil 时（--dev 模式），通知同时广播到 dev 控制连接。
	Dev *DevHub
	// Configs 管理已保存连接（sidecar 自有持久化）；未初始化时方法报错。
	Configs *ConfigService
	// QuickCmds：S5 能力；未初始化时报错。
	QuickCmds *QuickCmdService
	// Scripts：结构化脚本（pkg/script 引擎）；未初始化时报错。
	Scripts *StructuredScriptService
	// FT：文件传输（远端 SFTP 操作 + 数据目录沙箱本地面板 + 异步任务）；未初始化时报错。
	FT *FTService
	// Settings：Shell 设置切片持久化（shell-settings.json）；未初始化时报错。
	Settings *SettingsService
	// AI：AI 接入配置持久化（ai-config.json，读取脱敏）；未初始化时报错。
	AI *AIConfigService
	// Diagnose：AI 故障诊断长任务（流式事件 + 取消）；未初始化时报错。
	Diagnose *DiagnoseService
	// Completion：命令补全（pkg/completion 静态库，无持久化依赖）；未初始化时报错。
	Completion *completion.Service

	wsBase string
	out    *RPCWriter
}

// Ready 声明数据面就绪：注入下行写出端与 wsBase，并向宿主发 sidecar/ready。
func (a *ControlAPI) Ready(writer *RPCWriter, wsBase string) {
	a.out = writer
	a.wsBase = wsBase
	a.Notify("sidecar/ready", map[string]string{"wsBase": wsBase})
}

// RPCWriter 是行分隔 JSON 下行写出端（并发安全）。
type RPCWriter struct {
	mu  sync.Mutex
	Out io.Writer
}

func (w *RPCWriter) writeMessage(v any) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = w.Out.Write(append(data, '\n'))
	return err
}

// Notify 把 sidecar 事件推给宿主（stdio），dev 模式下同时广播到 dev 控制端。
func (a *ControlAPI) Notify(method string, params any) {
	if a.out != nil {
		_ = a.out.writeMessage(map[string]any{"jsonrpc": "2.0", "method": method, "params": params})
	}
	if a.Dev != nil {
		a.Dev.broadcast(method, params)
	}
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

// ServeControl 在 r 上逐行读取请求并分发；每请求独立 goroutine，阻塞方法
// （如 SSH 拨号）不拖累通知与后续请求。返回条件：r EOF 或 ctx 取消。
func ServeControl(ctx context.Context, r io.Reader, w *RPCWriter, api *ControlAPI) {
	if api.out == nil {
		api.out = w
	}
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64<<10), rpcMaxLine)
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		if !scanner.Scan() {
			return
		}
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var req rpcRequest
		if err := json.Unmarshal(line, &req); err != nil || req.Method == "" {
			_ = w.writeMessage(rpcResponse{JSONRPC: "2.0", ID: req.ID, Error: &rpcError{Code: -32700, Message: "解析失败"}})
			continue
		}
		reqCopy := req
		go func() {
			result, rpcErr := api.dispatch(ctx, &reqCopy)
			if reqCopy.ID == nil {
				return // 纯通知无应答
			}
			resp := rpcResponse{JSONRPC: "2.0", ID: reqCopy.ID}
			if rpcErr != nil {
				resp.Error = rpcErr
			} else {
				resp.Result = result
			}
			_ = w.writeMessage(resp)
		}()
	}
}

func (a *ControlAPI) dispatch(ctx context.Context, req *rpcRequest) (any, *rpcError) {
	switch req.Method {
	case "initialize":
		return map[string]any{"protocol": protocolVersion, "version": a.Version, "wsBase": a.wsBase, "token": a.Token}, nil
	case "shell.connect":
		var params struct {
			Config remote.ConnectConfig `json:"config"`
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return nil, &rpcError{Code: -32602, Message: "参数错误: " + err.Error()}
		}
		id, err := a.Service.Connect(params.Config)
		if err != nil {
			return nil, &rpcError{Code: -32000, Message: err.Error()}
		}
		return map[string]string{"connectionId": id}, nil
	case "shell.disconnect":
		var params struct {
			ConnectionID string `json:"connectionId"`
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return nil, &rpcError{Code: -32602, Message: "参数错误: " + err.Error()}
		}
		if err := a.Service.Disconnect(params.ConnectionID); err != nil {
			return nil, &rpcError{Code: -32000, Message: err.Error()}
		}
		return map[string]any{}, nil
	case "shell.openTerminal":
		var params struct {
			ConnectionID string `json:"connectionId"`
			Cols         int    `json:"cols"`
			Rows         int    `json:"rows"`
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return nil, &rpcError{Code: -32602, Message: "参数错误: " + err.Error()}
		}
		if params.Cols <= 0 {
			params.Cols = 80
		}
		if params.Rows <= 0 {
			params.Rows = 24
		}
		id, err := a.Service.OpenTerminal(params.ConnectionID, params.Cols, params.Rows)
		if err != nil {
			return nil, &rpcError{Code: -32000, Message: err.Error()}
		}
		return map[string]string{"terminalId": id}, nil
	case "shell.resize":
		var params struct {
			TerminalID string `json:"terminalId"`
			Cols       int    `json:"cols"`
			Rows       int    `json:"rows"`
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return nil, &rpcError{Code: -32602, Message: "参数错误: " + err.Error()}
		}
		if err := a.Service.Resize(params.TerminalID, params.Cols, params.Rows); err != nil {
			return nil, &rpcError{Code: -32000, Message: err.Error()}
		}
		return map[string]any{}, nil
	case "shell.closeTerminal":
		var params struct {
			TerminalID string `json:"terminalId"`
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return nil, &rpcError{Code: -32602, Message: "参数错误: " + err.Error()}
		}
		if err := a.Service.CloseTerminal(params.TerminalID); err != nil {
			return nil, &rpcError{Code: -32000, Message: err.Error()}
		}
		return map[string]any{}, nil
	case "shell.configs.list":
		if a.Configs == nil {
			return nil, &rpcError{Code: -32000, Message: "配置服务未启用（缺少 --data-dir）"}
		}
		sessions, err := a.Configs.List()
		if err != nil {
			return nil, &rpcError{Code: -32000, Message: err.Error()}
		}
		return map[string]any{"sessions": sessions}, nil
	case "shell.configs.save":
		if a.Configs == nil {
			return nil, &rpcError{Code: -32000, Message: "配置服务未启用（缺少 --data-dir）"}
		}
		var params SaveInput
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return nil, &rpcError{Code: -32602, Message: "参数错误: " + err.Error()}
		}
		id, err := a.Configs.Save(params)
		if err != nil {
			return nil, &rpcError{Code: -32000, Message: err.Error()}
		}
		return map[string]string{"id": id}, nil
	case "shell.configs.delete":
		if a.Configs == nil {
			return nil, &rpcError{Code: -32000, Message: "配置服务未启用（缺少 --data-dir）"}
		}
		var params struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(req.Params, &params); err != nil || params.ID == "" {
			return nil, &rpcError{Code: -32602, Message: "参数错误: 需要 id"}
		}
		if err := a.Configs.Delete(params.ID); err != nil {
			return nil, &rpcError{Code: -32000, Message: err.Error()}
		}
		return map[string]any{}, nil
	case "shell.configs.rename":
		if a.Configs == nil {
			return nil, &rpcError{Code: -32000, Message: "配置服务未启用（缺少 --data-dir）"}
		}
		var params struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		}
		if err := json.Unmarshal(req.Params, &params); err != nil || params.ID == "" {
			return nil, &rpcError{Code: -32602, Message: "参数错误: 需要 id"}
		}
		if err := a.Configs.Rename(params.ID, params.Name); err != nil {
			return nil, &rpcError{Code: -32000, Message: err.Error()}
		}
		return map[string]any{}, nil
	case "shell.configs.update":
		if a.Configs == nil {
			return nil, &rpcError{Code: -32000, Message: "配置服务未启用（缺少 --data-dir）"}
		}
		var params struct {
			ID     string               `json:"id"`
			Config remote.ConnectConfig `json:"config"`
			Group  string               `json:"group"`
		}
		if err := json.Unmarshal(req.Params, &params); err != nil || params.ID == "" {
			return nil, &rpcError{Code: -32602, Message: "参数错误: 需要 id 与 config"}
		}
		if err := a.Configs.Update(params.ID, params.Config, params.Group); err != nil {
			return nil, &rpcError{Code: -32000, Message: err.Error()}
		}
		return map[string]any{}, nil
	case "shell.configs.createFolder":
		if a.Configs == nil {
			return nil, &rpcError{Code: -32000, Message: "配置服务未启用（缺少 --data-dir）"}
		}
		var params struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(req.Params, &params); err != nil || params.Name == "" {
			return nil, &rpcError{Code: -32602, Message: "参数错误: 需要 name"}
		}
		if err := a.Configs.CreateFolder(params.Name); err != nil {
			return nil, &rpcError{Code: -32000, Message: err.Error()}
		}
		return map[string]any{}, nil
	case "shell.quickcmds.list":
		if a.QuickCmds == nil {
			return nil, notEnabled()
		}
		return map[string]any{"commands": a.QuickCmds.List()}, nil
	case "shell.quickcmds.save":
		if a.QuickCmds == nil {
			return nil, notEnabled()
		}
		var params QuickCommand
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return nil, badParams(err)
		}
		id, err := a.QuickCmds.Save(params)
		if err != nil {
			return nil, srvErr(err)
		}
		return map[string]string{"id": id}, nil
	case "shell.quickcmds.reorder":
		if a.QuickCmds == nil {
			return nil, notEnabled()
		}
		var params struct {
			IDs []string `json:"ids"`
		}
		if err := json.Unmarshal(req.Params, &params); err != nil || params.IDs == nil {
			return nil, badParams(fmt.Errorf("需要 ids 数组"))
		}
		if err := a.QuickCmds.Reorder(params.IDs); err != nil {
			return nil, srvErr(err)
		}
		return map[string]any{}, nil
	case "shell.quickcmds.delete":
		if a.QuickCmds == nil {
			return nil, notEnabled()
		}
		var params struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(req.Params, &params); err != nil || params.ID == "" {
			return nil, badParams(fmt.Errorf("需要 id"))
		}
		if err := a.QuickCmds.Delete(params.ID); err != nil {
			return nil, srvErr(err)
		}
		return map[string]any{}, nil
	case "shell.script.list":
		if a.Scripts == nil {
			return nil, notEnabled()
		}
		scripts, err := a.Scripts.List()
		if err != nil {
			return nil, srvErr(err)
		}
		return map[string]any{"scripts": scripts}, nil
	case "shell.script.load":
		if a.Scripts == nil {
			return nil, notEnabled()
		}
		var p struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil || p.ID == "" {
			return nil, badParams(fmt.Errorf("需要 id"))
		}
		sc, err := a.Scripts.Load(p.ID)
		if err != nil {
			return nil, srvErr(err)
		}
		return sc, nil
	case "shell.script.update":
		if a.Scripts == nil {
			return nil, notEnabled()
		}
		var sc script.Script
		if err := json.Unmarshal(req.Params, &sc); err != nil || sc.ID == "" {
			return nil, badParams(fmt.Errorf("需要脚本对象"))
		}
		if err := a.Scripts.Update(&sc); err != nil {
			return nil, srvErr(err)
		}
		return map[string]any{}, nil
	case "shell.script.delete":
		if a.Scripts == nil {
			return nil, notEnabled()
		}
		var p struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil || p.ID == "" {
			return nil, badParams(fmt.Errorf("需要 id"))
		}
		if err := a.Scripts.Delete(p.ID); err != nil {
			return nil, srvErr(err)
		}
		return map[string]any{}, nil
	case "shell.script.create":
		if a.Scripts == nil {
			return nil, notEnabled()
		}
		var p struct {
			Name        string `json:"name"`
			Description string `json:"description"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil || p.Name == "" {
			return nil, badParams(fmt.Errorf("需要 name"))
		}
		sc, err := a.Scripts.Create(p.Name, p.Description)
		if err != nil {
			return nil, srvErr(err)
		}
		return sc, nil
	case "shell.script.replay":
		if a.Scripts == nil {
			return nil, notEnabled()
		}
		var p struct {
			ID         string `json:"id"`
			TerminalID string `json:"terminalId"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil || p.ID == "" || p.TerminalID == "" {
			return nil, badParams(fmt.Errorf("需要 id 与 terminalId"))
		}
		if err := a.Scripts.Replay(p.ID, p.TerminalID); err != nil {
			return nil, srvErr(err)
		}
		return map[string]any{}, nil
	case "shell.script.replayVars":
		if a.Scripts == nil {
			return nil, notEnabled()
		}
		var p struct {
			ID         string            `json:"id"`
			TerminalID string            `json:"terminalId"`
			Values     map[string]string `json:"values"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil || p.ID == "" || p.TerminalID == "" {
			return nil, badParams(fmt.Errorf("需要 id 与 terminalId"))
		}
		if err := a.Scripts.ReplayWithVars(p.ID, p.TerminalID, p.Values); err != nil {
			return nil, srvErr(err)
		}
		return map[string]any{}, nil
	case "shell.script.startRecording":
		if a.Scripts == nil {
			return nil, notEnabled()
		}
		var p struct {
			Name        string `json:"name"`
			Description string `json:"description"`
			TerminalID  string `json:"terminalId"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil || p.Name == "" || p.TerminalID == "" {
			return nil, badParams(fmt.Errorf("需要 name 与 terminalId"))
		}
		sc, err := a.Scripts.StartRecording(p.Name, p.Description, p.TerminalID)
		if err != nil {
			return nil, srvErr(err)
		}
		return sc, nil
	case "shell.script.stopRecording":
		if a.Scripts == nil {
			return nil, notEnabled()
		}
		sc, err := a.Scripts.StopRecording()
		if err != nil {
			return nil, srvErr(err)
		}
		return sc, nil
	case "shell.script.status":
		if a.Scripts == nil {
			return nil, notEnabled()
		}
		return a.Scripts.RecordingStatus(), nil
	// ---- 文件传输（共享 FilesPanel 端口；统一返回 ftEnvelope 信封） ----
	case "shell.ft.check":
		if a.FT == nil {
			return nil, notEnabled()
		}
		var p struct {
			TerminalID string `json:"terminalId"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil || p.TerminalID == "" {
			return nil, badParams(fmt.Errorf("需要 terminalId"))
		}
		return a.FT.Check(p.TerminalID), nil
	case "shell.ft.list":
		if a.FT == nil {
			return nil, notEnabled()
		}
		var p struct {
			TerminalID string `json:"terminalId"`
			RemotePath string `json:"remotePath"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil || p.TerminalID == "" {
			return nil, badParams(fmt.Errorf("需要 terminalId"))
		}
		return a.FT.List(p.TerminalID, p.RemotePath), nil
	case "shell.ft.stat":
		if a.FT == nil {
			return nil, notEnabled()
		}
		var p struct {
			TerminalID string `json:"terminalId"`
			RemotePath string `json:"remotePath"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil || p.TerminalID == "" || p.RemotePath == "" {
			return nil, badParams(fmt.Errorf("需要 terminalId 与 remotePath"))
		}
		return a.FT.Stat(p.TerminalID, p.RemotePath), nil
	case "shell.ft.upload":
		if a.FT == nil {
			return nil, notEnabled()
		}
		var p struct {
			TerminalID string `json:"terminalId"`
			LocalPath  string `json:"localPath"`
			RemotePath string `json:"remotePath"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil || p.TerminalID == "" || p.LocalPath == "" || p.RemotePath == "" {
			return nil, badParams(fmt.Errorf("需要 terminalId、localPath 与 remotePath"))
		}
		return a.FT.Upload(p.TerminalID, p.LocalPath, p.RemotePath), nil
	case "shell.ft.download":
		if a.FT == nil {
			return nil, notEnabled()
		}
		var p struct {
			TerminalID string `json:"terminalId"`
			RemotePath string `json:"remotePath"`
			LocalPath  string `json:"localPath"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil || p.TerminalID == "" || p.RemotePath == "" || p.LocalPath == "" {
			return nil, badParams(fmt.Errorf("需要 terminalId、remotePath 与 localPath"))
		}
		return a.FT.Download(p.TerminalID, p.RemotePath, p.LocalPath), nil
	case "shell.ft.cancel":
		if a.FT == nil {
			return nil, notEnabled()
		}
		var p struct {
			TaskID string `json:"taskId"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil || p.TaskID == "" {
			return nil, badParams(fmt.Errorf("需要 taskId"))
		}
		return a.FT.Cancel(p.TaskID), nil
	case "shell.ft.mkdir":
		if a.FT == nil {
			return nil, notEnabled()
		}
		var p struct {
			TerminalID string `json:"terminalId"`
			RemotePath string `json:"remotePath"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil || p.TerminalID == "" || p.RemotePath == "" {
			return nil, badParams(fmt.Errorf("需要 terminalId 与 remotePath"))
		}
		return a.FT.RemoteMkdir(p.TerminalID, p.RemotePath), nil
	case "shell.ft.remove":
		if a.FT == nil {
			return nil, notEnabled()
		}
		var p struct {
			TerminalID string `json:"terminalId"`
			RemotePath string `json:"remotePath"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil || p.TerminalID == "" || p.RemotePath == "" {
			return nil, badParams(fmt.Errorf("需要 terminalId 与 remotePath"))
		}
		return a.FT.RemoteRemove(p.TerminalID, p.RemotePath), nil
	case "shell.ft.rename":
		if a.FT == nil {
			return nil, notEnabled()
		}
		var p struct {
			TerminalID string `json:"terminalId"`
			OldPath    string `json:"oldPath"`
			NewPath    string `json:"newPath"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil || p.TerminalID == "" || p.OldPath == "" || p.NewPath == "" {
			return nil, badParams(fmt.Errorf("需要 terminalId、oldPath 与 newPath"))
		}
		return a.FT.RemoteRename(p.TerminalID, p.OldPath, p.NewPath), nil
	case "shell.ft.readFile":
		if a.FT == nil {
			return nil, notEnabled()
		}
		var p struct {
			TerminalID string `json:"terminalId"`
			RemotePath string `json:"remotePath"`
			MaxBytes   int64  `json:"maxBytes"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil || p.TerminalID == "" || p.RemotePath == "" {
			return nil, badParams(fmt.Errorf("需要 terminalId 与 remotePath"))
		}
		return a.FT.RemoteReadFile(p.TerminalID, p.RemotePath, p.MaxBytes), nil
	case "shell.ft.writeFile":
		if a.FT == nil {
			return nil, notEnabled()
		}
		var p struct {
			TerminalID string `json:"terminalId"`
			RemotePath string `json:"remotePath"`
			Content    string `json:"content"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil || p.TerminalID == "" || p.RemotePath == "" {
			return nil, badParams(fmt.Errorf("需要 terminalId 与 remotePath"))
		}
		return a.FT.RemoteWriteFile(p.TerminalID, p.RemotePath, p.Content), nil
	// ---- Shell 设置 ----
	case "shell.settings.get":
		if a.Settings == nil {
			return nil, notEnabled()
		}
		settings, err := a.Settings.Get()
		if err != nil {
			return nil, srvErr(err)
		}
		return settings, nil
	case "shell.settings.save":
		if a.Settings == nil {
			return nil, notEnabled()
		}
		var next ShellSettingsJSON
		if err := json.Unmarshal(req.Params, &next); err != nil {
			return nil, badParams(err)
		}
		if err := a.Settings.Save(next); err != nil {
			return nil, srvErr(err)
		}
		return map[string]any{}, nil
	// ---- AI 接入配置（密钥只在 sidecar 后台感知，读取脱敏） ----
	case "shell.ai.getConfig":
		if a.AI == nil {
			return nil, notEnabled()
		}
		return a.AI.Status(), nil
	case "shell.ai.saveConfig":
		if a.AI == nil {
			return nil, notEnabled()
		}
		var update AIConfigUpdate
		if err := json.Unmarshal(req.Params, &update); err != nil {
			return nil, badParams(err)
		}
		status, err := a.AI.Save(update)
		if err != nil {
			return nil, srvErr(err)
		}
		return status, nil
	case "shell.ai.generateCommand":
		if a.AI == nil {
			return nil, notEnabled()
		}
		var p struct {
			Request string `json:"request"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil {
			return nil, badParams(err)
		}
		if strings.TrimSpace(p.Request) == "" {
			return nil, badParams(errors.New("request 不能为空"))
		}
		result, err := a.AI.GenerateCommand(p.Request)
		if err != nil {
			return nil, srvErr(err)
		}
		return result, nil
	case "shell.ai.parseIntent":
		if a.AI == nil {
			return nil, notEnabled()
		}
		var p struct {
			Input string `json:"input"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil {
			return nil, badParams(err)
		}
		if strings.TrimSpace(p.Input) == "" {
			return nil, badParams(errors.New("input 不能为空"))
		}
		configs, err := a.AI.ParseConnectIntent(p.Input)
		if err != nil {
			return nil, srvErr(err)
		}
		return map[string]any{"configs": configs}, nil
	// ---- AI 诊断（迭代 C：异步长任务 + shell.diagnose.event 流式事件） ----
	case "shell.diagnose.start":
		if a.Diagnose == nil {
			return nil, notEnabled()
		}
		var input StartInput
		if err := json.Unmarshal(req.Params, &input); err != nil {
			return nil, badParams(err)
		}
		if strings.TrimSpace(input.Problem) == "" {
			return nil, badParams(errors.New("problem 不能为空"))
		}
		result, err := a.Diagnose.Start(input)
		if err != nil {
			return nil, srvErr(err)
		}
		return result, nil
	case "shell.diagnose.cancel":
		if a.Diagnose == nil {
			return nil, notEnabled()
		}
		var p struct {
			RunID string `json:"runId"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil {
			return nil, badParams(err)
		}
		a.Diagnose.Cancel(p.RunID)
		return map[string]any{}, nil
	case "shell.diagnose.stop":
		if a.Diagnose == nil {
			return nil, notEnabled()
		}
		var p struct {
			RootCause  string `json:"rootCause"`
			Conclusion string `json:"conclusion"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil {
			return nil, badParams(err)
		}
		cs, err := a.Diagnose.StopCase(p.RootCause, p.Conclusion)
		if err != nil {
			return nil, srvErr(err)
		}
		return map[string]any{"caseId": cs.ID, "commands": len(cs.Commands)}, nil
	case "shell.diagnose.status":
		if a.Diagnose == nil {
			return nil, notEnabled()
		}
		return a.Diagnose.CaseStatus(), nil
	case "shell.diagnose.conclusion":
		if a.Diagnose == nil {
			return nil, notEnabled()
		}
		var p struct {
			RootCause string `json:"rootCause"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil {
			return nil, badParams(err)
		}
		if strings.TrimSpace(p.RootCause) == "" {
			return nil, badParams(errors.New("rootCause 不能为空"))
		}
		runId, err := a.Diagnose.Conclusion(p.RootCause)
		if err != nil {
			return nil, srvErr(err)
		}
		return map[string]any{"runId": runId}, nil
	case "shell.diagnose.archive":
		if a.Diagnose == nil {
			return nil, notEnabled()
		}
		var input ArchiveInputJSON
		if err := json.Unmarshal(req.Params, &input); err != nil {
			return nil, badParams(err)
		}
		path, err := a.Diagnose.Archive(input)
		if err != nil {
			return nil, srvErr(err)
		}
		return map[string]any{"filePath": path}, nil
	// ---- 命令补全（pkg/completion 静态库） ----
	case "shell.completion":
		if a.Completion == nil {
			return nil, notEnabled()
		}
		var p struct {
			Input  string `json:"input"`
			Cursor int    `json:"cursor"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil {
			return nil, badParams(err)
		}
		resp, err := a.Completion.GetCompletions(completion.CompletionRequest{Input: p.Input, Cursor: p.Cursor})
		if err != nil {
			return nil, srvErr(err)
		}
		return resp, nil
	// ---- 本地面板（sidecar 数据目录沙箱） ----
	case "shell.fs.list":
		if a.FT == nil {
			return nil, notEnabled()
		}
		var p struct {
			Path string `json:"path"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil {
			return nil, badParams(err)
		}
		return a.FT.LocalList(p.Path), nil
	case "shell.fs.stat":
		if a.FT == nil {
			return nil, notEnabled()
		}
		var p struct {
			Path string `json:"path"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil || p.Path == "" {
			return nil, badParams(fmt.Errorf("需要 path"))
		}
		return a.FT.LocalStat(p.Path), nil
	case "shell.fs.mkdir":
		if a.FT == nil {
			return nil, notEnabled()
		}
		var p struct {
			Path string `json:"path"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil || p.Path == "" {
			return nil, badParams(fmt.Errorf("需要 path"))
		}
		return a.FT.LocalMkdir(p.Path), nil
	case "shell.fs.remove":
		if a.FT == nil {
			return nil, notEnabled()
		}
		var p struct {
			Path string `json:"path"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil || p.Path == "" {
			return nil, badParams(fmt.Errorf("需要 path"))
		}
		return a.FT.LocalRemove(p.Path), nil
	case "shell.fs.rename":
		if a.FT == nil {
			return nil, notEnabled()
		}
		var p struct {
			OldPath string `json:"oldPath"`
			NewPath string `json:"newPath"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil || p.OldPath == "" || p.NewPath == "" {
			return nil, badParams(fmt.Errorf("需要 oldPath 与 newPath"))
		}
		return a.FT.LocalRename(p.OldPath, p.NewPath), nil
	case "shell.monitor.sample":
		var params struct {
			ConnectionID string `json:"connectionId"`
		}
		if err := json.Unmarshal(req.Params, &params); err != nil || params.ConnectionID == "" {
			return nil, badParams(fmt.Errorf("需要 connectionId"))
		}
		sample, err := a.Service.SampleMonitor(params.ConnectionID)
		if err != nil {
			return nil, srvErr(err)
		}
		return sample, nil
	default:
		return nil, &rpcError{Code: -32601, Message: "未知方法: " + req.Method}
	}
}

func notEnabled() *rpcError {
	return &rpcError{Code: -32000, Message: "该能力未启用（缺少 --data-dir）"}
}

func badParams(err error) *rpcError {
	return &rpcError{Code: -32602, Message: "参数错误: " + err.Error()}
}

func srvErr(err error) *rpcError {
	return &rpcError{Code: -32000, Message: err.Error()}
}
