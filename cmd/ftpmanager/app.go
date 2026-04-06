package main

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"strings"

	"opscopilot/pkg/ftipc"
)

// FTPApp is the independent FTP file manager application.
// It operates purely as an IPC client to the main OpsCopilot process.
// All remote operations are delegated to the main app via IPC.
// Only local filesystem operations are handled locally.
type FTPApp struct {
	ctx          context.Context
	ipcClient    *ftipc.Client
	ipcTokenFile string
	sessionID    string
}

// NewFTPApp creates a new FTP manager application.
func NewFTPApp(ipcTokenFile, sessionID string) *FTPApp {
	return &FTPApp{
		ipcTokenFile: ipcTokenFile,
		sessionID:    sessionID,
	}
}

func (a *FTPApp) startup(ctx context.Context) {
	a.ctx = ctx

	if a.ipcTokenFile != "" {
		dir := filepath.Dir(a.ipcTokenFile)
		client, err := ftipc.NewClientFromTokenFile(dir)
		if err != nil {
			log.Printf("[FTP] IPC 连接失败: %v", err)
		} else {
			a.ipcClient = client
			log.Println("[FTP] 已通过 IPC 连接主应用")
		}
	}
}

// --- Wails bindings ---

func (a *FTPApp) ListSessions() string {
	if raw, ok := a.callIPCGetSessions(); ok {
		return raw
	}
	return `{"ok":false,"error":"IPC 未连接，请从主程序启动"}`
}

func (a *FTPApp) GetStartupSession() string {
	return a.sessionID
}

func (a *FTPApp) Connect(sessionID string) string {
	if a.ipcClient != nil {
		return mustJSON(map[string]any{"ok": true, "message": "IPC 模式无需本地连接"})
	}
	return `{"ok":false,"error":"IPC 未连接，请从主程序启动"}`
}

func (a *FTPApp) Disconnect(sessionID string) string {
	if a.ipcClient != nil {
		return mustJSON(map[string]any{"ok": true})
	}
	return `{"ok":false,"error":"IPC 未连接，请从主程序启动"}`
}

func (a *FTPApp) FTCheck(sessionID string) string {
	if raw, ok := a.callIPCAction("ft_check", ftipc.IPCRequest{SessionID: sessionID}); ok {
		return raw
	}
	return `{"ok":false,"error":"IPC 未连接，请从主程序启动"}`
}

func (a *FTPApp) FTList(sessionID, remotePath string) string {
	if raw, ok := a.callIPCAction("ft_list", ftipc.IPCRequest{SessionID: sessionID, Path: remotePath}); ok {
		return raw
	}
	return `{"ok":false,"error":"IPC 未连接，请从主程序启动"}`
}

func (a *FTPApp) FTStat(sessionID, remotePath string) string {
	if raw, ok := a.callIPCAction("ft_stat", ftipc.IPCRequest{SessionID: sessionID, Path: remotePath}); ok {
		return raw
	}
	return `{"ok":false,"error":"IPC 未连接，请从主程序启动"}`
}

func (a *FTPApp) FTUpload(sessionID, localPath, remotePath string) string {
	if raw, ok := a.callIPCAction("ft_upload", ftipc.IPCRequest{SessionID: sessionID, Path: remotePath, LocalPath: localPath}); ok {
		return raw
	}
	return `{"ok":false,"error":"IPC 未连接，请从主程序启动"}`
}

func (a *FTPApp) FTDownload(sessionID, remotePath, localPath string) string {
	if raw, ok := a.callIPCAction("ft_download", ftipc.IPCRequest{SessionID: sessionID, Path: remotePath, LocalPath: localPath}); ok {
		return raw
	}
	return `{"ok":false,"error":"IPC 未连接，请从主程序启动"}`
}

func (a *FTPApp) FTCancel(taskID string) string {
	if raw, ok := a.callIPCAction("ft_cancel", ftipc.IPCRequest{TaskID: taskID}); ok {
		return raw
	}
	return `{"ok":false,"error":"IPC 未连接，请从主程序启动"}`
}

func (a *FTPApp) FTRemoteMkdir(sessionID, remotePath string) string {
	if raw, ok := a.callIPCAction("ft_remote_mkdir", ftipc.IPCRequest{SessionID: sessionID, Path: remotePath}); ok {
		return raw
	}
	return `{"ok":false,"error":"IPC 未连接，请从主程序启动"}`
}

func (a *FTPApp) FTRemoteRemove(sessionID, remotePath string) string {
	if raw, ok := a.callIPCAction("ft_remote_remove", ftipc.IPCRequest{SessionID: sessionID, Path: remotePath}); ok {
		return raw
	}
	return `{"ok":false,"error":"IPC 未连接，请从主程序启动"}`
}

func (a *FTPApp) FTRemoteRename(sessionID, oldPath, newPath string) string {
	if raw, ok := a.callIPCAction("ft_remote_rename", ftipc.IPCRequest{SessionID: sessionID, Path: oldPath, DstPath: newPath}); ok {
		return raw
	}
	return `{"ok":false,"error":"IPC 未连接，请从主程序启动"}`
}

func (a *FTPApp) FTRemoteReadFile(sessionID, remotePath string, maxBytes int64) string {
	if raw, ok := a.callIPCAction("ft_remote_read", ftipc.IPCRequest{SessionID: sessionID, Path: remotePath, MaxBytes: maxBytes}); ok {
		return raw
	}
	return `{"ok":false,"error":"IPC 未连接，请从主程序启动"}`
}

func (a *FTPApp) FTRemoteWriteFile(sessionID, remotePath, content string) string {
	if raw, ok := a.callIPCAction("ft_remote_write", ftipc.IPCRequest{SessionID: sessionID, Path: remotePath, Content: content}); ok {
		return raw
	}
	return `{"ok":false,"error":"IPC 未连接，请从主程序启动"}`
}

// --- Local filesystem operations (no IPC needed) ---

func (a *FTPApp) LocalList(localPath string) string {
	if raw, ok := a.callIPCAction("local_list", ftipc.IPCRequest{Path: localPath}); ok {
		return raw
	}
	p := strings.TrimSpace(localPath)
	if p == "" || p == "." {
		if home, err := os.UserHomeDir(); err == nil && home != "" {
			p = home
		} else if wd, err := os.Getwd(); err == nil && wd != "" {
			p = wd
		} else {
			p = "."
		}
	}
	p = filepath.Clean(p)
	entries, err := os.ReadDir(p)
	if err != nil {
		return mustJSON(map[string]any{"ok": false, "error": err.Error()})
	}
	type localEntry struct {
		Path    string `json:"path"`
		Name    string `json:"name"`
		IsDir   bool   `json:"isDir"`
		Size    int64  `json:"size"`
		Mode    uint32 `json:"mode"`
		ModTime string `json:"modTime"`
	}
	out := make([]localEntry, 0, len(entries))
	for _, de := range entries {
		fi, err := de.Info()
		if err != nil {
			continue
		}
		out = append(out, localEntry{
			Path:    filepath.Join(p, de.Name()),
			Name:    de.Name(),
			IsDir:   de.IsDir(),
			Size:    fi.Size(),
			Mode:    uint32(fi.Mode()),
			ModTime: fi.ModTime().Format("2006-01-02 15:04:05"),
		})
	}
	return mustJSON(map[string]any{"ok": true, "entries": out})
}

func (a *FTPApp) LocalMkdir(localPath string) string {
	if raw, ok := a.callIPCAction("local_mkdir", ftipc.IPCRequest{Path: localPath}); ok {
		return raw
	}
	p := filepath.Clean(strings.TrimSpace(localPath))
	if p == "" {
		return mustJSON(map[string]any{"ok": false, "error": "路径为空"})
	}
	if err := os.MkdirAll(p, 0755); err != nil {
		return mustJSON(map[string]any{"ok": false, "error": err.Error()})
	}
	return mustJSON(map[string]any{"ok": true})
}

func (a *FTPApp) LocalRemove(localPath string) string {
	if raw, ok := a.callIPCAction("local_remove", ftipc.IPCRequest{Path: localPath}); ok {
		return raw
	}
	p := filepath.Clean(strings.TrimSpace(localPath))
	if p == "" {
		return mustJSON(map[string]any{"ok": false, "error": "路径为空"})
	}
	if err := os.RemoveAll(p); err != nil {
		return mustJSON(map[string]any{"ok": false, "error": err.Error()})
	}
	return mustJSON(map[string]any{"ok": true})
}

func (a *FTPApp) LocalRename(oldPath, newPath string) string {
	if raw, ok := a.callIPCAction("local_rename", ftipc.IPCRequest{Path: oldPath, DstPath: newPath}); ok {
		return raw
	}
	oldP := filepath.Clean(strings.TrimSpace(oldPath))
	newP := filepath.Clean(strings.TrimSpace(newPath))
	if oldP == "" || newP == "" {
		return mustJSON(map[string]any{"ok": false, "error": "路径为空"})
	}
	if err := os.Rename(oldP, newP); err != nil {
		return mustJSON(map[string]any{"ok": false, "error": err.Error()})
	}
	return mustJSON(map[string]any{"ok": true})
}

// --- IPC helpers ---

func (a *FTPApp) callIPCGetSessions() (string, bool) {
	if a.ipcClient == nil {
		return "", false
	}
	resp, err := a.ipcClient.GetSessions()
	if err != nil {
		log.Printf("[FTP] IPC 获取会话失败: %v", err)
		return "", false
	}
	return normalizeIPCResponse(resp), true
}

func (a *FTPApp) callIPCAction(action string, req ftipc.IPCRequest) (string, bool) {
	if a.ipcClient == nil {
		return "", false
	}
	req.Action = action
	resp, err := a.ipcClient.DoAction(req)
	if err != nil {
		log.Printf("[FTP] IPC 动作失败 action=%s err=%v", action, err)
		return "", false
	}
	raw := normalizeIPCResponse(resp)
	if action == "ft_upload" || action == "ft_download" {
		var payload map[string]any
		if err := json.Unmarshal([]byte(raw), &payload); err == nil {
			if okFlag, ok := payload["ok"].(bool); ok && okFlag {
				if _, exists := payload["message"]; !exists {
					payload["message"] = "任务已提交到主程序执行"
				}
				raw = mustJSON(payload)
			}
		}
	}
	return raw, true
}

func normalizeIPCResponse(resp ftipc.IPCResponse) string {
	if resp.Data != nil {
		return mustJSON(resp.Data)
	}
	if !resp.OK {
		code := "IPC_ERROR"
		message := "IPC 请求失败"
		if resp.Error != nil {
			if resp.Error.Code != "" {
				code = resp.Error.Code
			}
			if resp.Error.Message != "" {
				message = resp.Error.Message
			}
		}
		return mustJSON(map[string]any{
			"ok": false,
			"error": map[string]string{
				"code":    code,
				"message": message,
			},
		})
	}
	if resp.Message != "" {
		return mustJSON(map[string]any{"ok": true, "message": resp.Message})
	}
	return mustJSON(map[string]any{"ok": true})
}

func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return `{"ok":false,"error":"marshal failed"}`
	}
	return string(b)
}
