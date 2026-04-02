package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
	"golang.org/x/crypto/ssh"

	"opscopilot/pkg/filetransfer"
	"opscopilot/pkg/ftipc"
	"opscopilot/pkg/sessionmanager"
	"opscopilot/pkg/sshclient"
)

// FTPApp is the independent FTP file manager application.
type FTPApp struct {
	ctx          context.Context
	ipcClient    *ftipc.Client
	ipcTokenFile string
	sessionID    string

	mu            sync.RWMutex
	clients       map[string]*ssh.Client
	closeFns      map[string]func()
	rootPasswords map[string]string
	loginUsers    map[string]string

	ftMu      sync.Mutex
	ftCancels map[string]context.CancelFunc
}

// NewFTPApp creates a new FTP manager application.
func NewFTPApp(ipcTokenFile, sessionID string) *FTPApp {
	return &FTPApp{
		ipcTokenFile:  ipcTokenFile,
		sessionID:     sessionID,
		clients:       make(map[string]*ssh.Client),
		closeFns:      make(map[string]func()),
		rootPasswords: make(map[string]string),
		loginUsers:    make(map[string]string),
		ftCancels:     make(map[string]context.CancelFunc),
	}
}

func (a *FTPApp) startup(ctx context.Context) {
	a.ctx = ctx

	// If we have IPC token file, connect via IPC
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

	// Auto-connect if session is specified
	if a.sessionID != "" && a.ipcClient == nil {
		go a.autoConnect(a.sessionID)
	}
}

// autoConnect reads session config and connects.
func (a *FTPApp) autoConnect(sessionID string) {
	mgr := sessionmanager.NewManager()
	if err := mgr.Load(); err != nil {
		log.Printf("[FTP] 加载配置失败: %v", err)
		return
	}

	session := a.findSession(mgr.GetSessions(), sessionID)
	if session == nil || session.Config == nil {
		log.Printf("[FTP] 会话不存在: %s", sessionID)
		return
	}

	cfg := session.Config
	password := cfg.Password

	// Create SSH client
	sshCfg := &sshclient.ConnectConfig{
		Host:         cfg.Host,
		Port:         cfg.Port,
		User:         cfg.User,
		Password:     password,
		RootPassword: cfg.RootPassword,
	}
	if cfg.Bastion != nil {
		sshCfg.Bastion = &sshclient.ConnectConfig{
			Name:     cfg.Bastion.Name,
			Host:     cfg.Bastion.Host,
			Port:     cfg.Bastion.Port,
			User:     cfg.Bastion.User,
			Password: cfg.Bastion.Password,
		}
	}

	client, err := sshclient.NewClient(sshCfg)
	if err != nil {
		log.Printf("[FTP] 连接失败: %v", err)
		return
	}

	a.mu.Lock()
	a.clients[sessionID] = client.SSHClient()
	a.closeFns[sessionID] = func() { _ = client.Close() }
	a.rootPasswords[sessionID] = cfg.RootPassword
	a.loginUsers[sessionID] = cfg.User
	a.mu.Unlock()

	log.Printf("[FTP] 已自动连接会话: %s", sessionID)
}

func (a *FTPApp) findSession(sessions []*sessionmanager.Session, id string) *sessionmanager.Session {
	for _, s := range sessions {
		if s.ID == id {
			return s
		}
		if s.Type == sessionmanager.TypeFolder && len(s.Children) > 0 {
			found := a.findSession(s.Children, id)
			if found != nil {
				return found
			}
		}
	}
	return nil
}

// --- Wails bindings ---

func (a *FTPApp) ListSessions() string {
	if raw, ok := a.callIPCGetSessions(); ok {
		return raw
	}
	mgr := sessionmanager.NewManager()
	if err := mgr.Load(); err != nil {
		return "[]"
	}
	sessions := make([]map[string]string, 0)
	var walk func(nodes []*sessionmanager.Session)
	walk = func(nodes []*sessionmanager.Session) {
		for _, s := range nodes {
			if s == nil {
				continue
			}
			if s.Config != nil {
				title := s.Name
				if strings.TrimSpace(title) == "" {
					title = s.ID
				}
				sessions = append(sessions, map[string]string{
					"id":    s.ID,
					"title": title,
				})
				continue
			}
			if s.Type == sessionmanager.TypeFolder {
				walk(s.Children)
			}
		}
	}
	walk(mgr.GetSessions())
	return mustJSON(sessions)
}

func (a *FTPApp) GetStartupSession() string {
	return a.sessionID
}

func (a *FTPApp) Connect(sessionID string) string {
	if a.ipcClient != nil {
		return mustJSON(map[string]any{"ok": true, "message": "IPC 模式无需本地连接"})
	}
	mgr := sessionmanager.NewManager()
	if err := mgr.Load(); err != nil {
		return mustJSON(map[string]any{"ok": false, "error": "加载配置失败"})
	}

	session := a.findSession(mgr.GetSessions(), sessionID)
	if session == nil || session.Config == nil {
		return mustJSON(map[string]any{"ok": false, "error": "会话不存在"})
	}

	cfg := session.Config
	password := cfg.Password

	sshCfg := &sshclient.ConnectConfig{
		Host:         cfg.Host,
		Port:         cfg.Port,
		User:         cfg.User,
		Password:     password,
		RootPassword: cfg.RootPassword,
	}
	if cfg.Bastion != nil {
		sshCfg.Bastion = &sshclient.ConnectConfig{
			Name:     cfg.Bastion.Name,
			Host:     cfg.Bastion.Host,
			Port:     cfg.Bastion.Port,
			User:     cfg.Bastion.User,
			Password: cfg.Bastion.Password,
		}
	}

	client, err := sshclient.NewClient(sshCfg)
	if err != nil {
		return mustJSON(map[string]any{"ok": false, "error": "连接失败: " + err.Error()})
	}

	a.mu.Lock()
	a.clients[sessionID] = client.SSHClient()
	a.closeFns[sessionID] = func() { _ = client.Close() }
	a.rootPasswords[sessionID] = cfg.RootPassword
	a.loginUsers[sessionID] = cfg.User
	a.mu.Unlock()

	return mustJSON(map[string]any{"ok": true, "message": "已连接"})
}

func (a *FTPApp) Disconnect(sessionID string) string {
	if a.ipcClient != nil {
		return mustJSON(map[string]any{"ok": true})
	}
	a.mu.Lock()
	client := a.clients[sessionID]
	closeFn := a.closeFns[sessionID]
	delete(a.clients, sessionID)
	delete(a.closeFns, sessionID)
	delete(a.rootPasswords, sessionID)
	delete(a.loginUsers, sessionID)
	a.mu.Unlock()

	if client != nil {
		_ = client.Close()
	}
	if closeFn != nil {
		closeFn()
	}
	return mustJSON(map[string]any{"ok": true})
}

func (a *FTPApp) FTList(sessionID, remotePath string) string {
	if raw, ok := a.callIPCAction("ft_list", ftipc.IPCRequest{SessionID: sessionID, Path: remotePath}); ok {
		return raw
	}
	a.mu.RLock()
	client := a.clients[sessionID]
	rootPwd := a.rootPasswords[sessionID]
	loginUser := a.loginUsers[sessionID]
	a.mu.RUnlock()

	if client == nil {
		return mustJSON(ftResponse{OK: false, Error: "未连接"})
	}

	var entries []filetransfer.Entry
	var err error
	if rootPwd != "" {
		relay := filetransfer.NewRootRelayTransport(client, rootPwd, loginUser)
		entries, err = relay.List(context.Background(), remotePath)
	} else {
		tr := filetransfer.NewSFTPTransport(client)
		entries, err = tr.List(context.Background(), remotePath)
	}
	if err != nil {
		return mustJSON(ftResponse{OK: false, Error: err.Error()})
	}
	return mustJSON(ftResponse{OK: true, Entries: entries})
}

func (a *FTPApp) FTStat(sessionID, remotePath string) string {
	if raw, ok := a.callIPCAction("ft_stat", ftipc.IPCRequest{SessionID: sessionID, Path: remotePath}); ok {
		return raw
	}
	a.mu.RLock()
	client := a.clients[sessionID]
	rootPwd := a.rootPasswords[sessionID]
	loginUser := a.loginUsers[sessionID]
	a.mu.RUnlock()

	if client == nil {
		return mustJSON(ftResponse{OK: false, Error: "未连接"})
	}

	var entry filetransfer.Entry
	var err error
	if rootPwd != "" {
		relay := filetransfer.NewRootRelayTransport(client, rootPwd, loginUser)
		entry, err = relay.Stat(context.Background(), remotePath)
	} else {
		tr := filetransfer.NewSFTPTransport(client)
		entry, err = tr.Stat(context.Background(), remotePath)
	}
	if err != nil {
		return mustJSON(ftResponse{OK: false, Error: err.Error()})
	}
	return mustJSON(ftResponse{OK: true, Entry: &entry})
}

func (a *FTPApp) FTUpload(sessionID, localPath, remotePath string) string {
	if raw, ok := a.callIPCAction("ft_upload", ftipc.IPCRequest{SessionID: sessionID, Path: remotePath, LocalPath: localPath}); ok {
		return raw
	}
	return a.startTransfer(sessionID, "upload", localPath, remotePath)
}

func (a *FTPApp) FTDownload(sessionID, remotePath, localPath string) string {
	if raw, ok := a.callIPCAction("ft_download", ftipc.IPCRequest{SessionID: sessionID, Path: remotePath, LocalPath: localPath}); ok {
		return raw
	}
	return a.startTransfer(sessionID, "download", remotePath, localPath)
}

func (a *FTPApp) FTCancel(taskID string) string {
	if raw, ok := a.callIPCAction("ft_cancel", ftipc.IPCRequest{TaskID: taskID}); ok {
		return raw
	}
	a.ftMu.Lock()
	cancel, ok := a.ftCancels[taskID]
	a.ftMu.Unlock()
	if !ok {
		return mustJSON(ftResponse{OK: false, Error: "任务不存在"})
	}
	cancel()
	return mustJSON(ftResponse{OK: true, Message: "已取消"})
}

func (a *FTPApp) FTRemoteMkdir(sessionID, remotePath string) string {
	if raw, ok := a.callIPCAction("ft_remote_mkdir", ftipc.IPCRequest{SessionID: sessionID, Path: remotePath}); ok {
		return raw
	}
	a.mu.RLock()
	client := a.clients[sessionID]
	rootPwd := a.rootPasswords[sessionID]
	loginUser := a.loginUsers[sessionID]
	a.mu.RUnlock()

	if client == nil {
		return mustJSON(ftResponse{OK: false, Error: "未连接"})
	}
	if rootPwd != "" {
		relay := filetransfer.NewRootRelayTransport(client, rootPwd, loginUser)
		if err := relay.Mkdir(context.Background(), remotePath); err != nil {
			return mustJSON(ftResponse{OK: false, Error: err.Error()})
		}
	} else {
		tr := filetransfer.NewSFTPTransport(client)
		if err := tr.Mkdir(context.Background(), remotePath); err != nil {
			return mustJSON(ftResponse{OK: false, Error: err.Error()})
		}
	}
	return mustJSON(ftResponse{OK: true})
}

func (a *FTPApp) FTRemoteRemove(sessionID, remotePath string) string {
	if raw, ok := a.callIPCAction("ft_remote_remove", ftipc.IPCRequest{SessionID: sessionID, Path: remotePath}); ok {
		return raw
	}
	a.mu.RLock()
	client := a.clients[sessionID]
	rootPwd := a.rootPasswords[sessionID]
	loginUser := a.loginUsers[sessionID]
	a.mu.RUnlock()

	if client == nil {
		return mustJSON(ftResponse{OK: false, Error: "未连接"})
	}
	if rootPwd != "" {
		relay := filetransfer.NewRootRelayTransport(client, rootPwd, loginUser)
		if err := relay.Remove(context.Background(), remotePath, true); err != nil {
			return mustJSON(ftResponse{OK: false, Error: err.Error()})
		}
	} else {
		tr := filetransfer.NewSFTPTransport(client)
		if err := tr.Remove(context.Background(), remotePath, true); err != nil {
			return mustJSON(ftResponse{OK: false, Error: err.Error()})
		}
	}
	return mustJSON(ftResponse{OK: true})
}

func (a *FTPApp) FTRemoteRename(sessionID, oldPath, newPath string) string {
	if raw, ok := a.callIPCAction("ft_remote_rename", ftipc.IPCRequest{SessionID: sessionID, Path: oldPath, DstPath: newPath}); ok {
		return raw
	}
	a.mu.RLock()
	client := a.clients[sessionID]
	rootPwd := a.rootPasswords[sessionID]
	loginUser := a.loginUsers[sessionID]
	a.mu.RUnlock()

	if client == nil {
		return mustJSON(ftResponse{OK: false, Error: "未连接"})
	}
	if rootPwd != "" {
		relay := filetransfer.NewRootRelayTransport(client, rootPwd, loginUser)
		if err := relay.Rename(context.Background(), oldPath, newPath); err != nil {
			return mustJSON(ftResponse{OK: false, Error: err.Error()})
		}
	} else {
		tr := filetransfer.NewSFTPTransport(client)
		if err := tr.Rename(context.Background(), oldPath, newPath); err != nil {
			return mustJSON(ftResponse{OK: false, Error: err.Error()})
		}
	}
	return mustJSON(ftResponse{OK: true})
}

// --- Internal ---

type ftResponse struct {
	OK      bool                 `json:"ok"`
	Message string               `json:"message,omitempty"`
	Error   string               `json:"error,omitempty"`
	Entries []filetransfer.Entry `json:"entries,omitempty"`
	Entry   *filetransfer.Entry  `json:"entry,omitempty"`
	TaskID  string               `json:"taskId,omitempty"`
}

func (a *FTPApp) startTransfer(sessionID, op, localPath, remotePath string) string {
	a.mu.RLock()
	client := a.clients[sessionID]
	rootPwd := a.rootPasswords[sessionID]
	loginUser := a.loginUsers[sessionID]
	a.mu.RUnlock()

	if client == nil {
		return mustJSON(ftResponse{OK: false, Error: "未连接"})
	}

	taskID := fmt.Sprintf("ftp-%d", time.Now().UnixMilli())
	ctx, cancel := context.WithCancel(context.Background())

	a.ftMu.Lock()
	a.ftCancels[taskID] = cancel
	a.ftMu.Unlock()

	go func() {
		defer func() {
			a.ftMu.Lock()
			delete(a.ftCancels, taskID)
			a.ftMu.Unlock()
		}()

		progressFn := func(p filetransfer.Progress) {
			if a.ctx == nil {
				return
			}
			payload := map[string]any{
				"taskId":    taskID,
				"sessionId": sessionID,
			}
			if p.Step != "" {
				// Step-only notification: don't overwrite byte progress
				payload["step"] = p.Step
			} else {
				payload["bytesDone"] = p.BytesDone
				payload["bytesTotal"] = p.BytesTotal
				payload["speedBps"] = p.SpeedBps
			}
			runtime.EventsEmit(a.ctx, "file-transfer-progress", payload)
		}

		var (
			res   filetransfer.TransferResult
			opErr error
		)
		if rootPwd != "" {
			relay := filetransfer.NewRootRelayTransport(client, rootPwd, loginUser)
			if op == "upload" {
				res, opErr = relay.Upload(ctx, localPath, remotePath, progressFn)
			} else {
				res, opErr = relay.Download(ctx, remotePath, localPath, progressFn)
			}
		} else {
			sftpTr := filetransfer.NewSFTPTransport(client)
			if op == "upload" {
				res, opErr = sftpTr.Upload(ctx, localPath, remotePath, progressFn)
			} else {
				res, opErr = sftpTr.Download(ctx, remotePath, localPath, progressFn)
			}
		}

		if a.ctx == nil {
			return
		}
		if opErr != nil {
			runtime.EventsEmit(a.ctx, "file-transfer-done", map[string]any{
				"taskId":    taskID,
				"sessionId": sessionID,
				"ok":        false,
				"error":     opErr.Error(),
			})
			return
		}
		runtime.EventsEmit(a.ctx, "file-transfer-done", map[string]any{
			"taskId":    taskID,
			"sessionId": sessionID,
			"ok":        true,
			"bytes":     res.Bytes,
			"message":   "完成",
		})
	}()

	return mustJSON(ftResponse{OK: true, TaskID: taskID})
}

func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return `{"ok":false,"error":"marshal failed"}`
	}
	return string(b)
}

func (a *FTPApp) FTCheck(sessionID string) string {
	if raw, ok := a.callIPCAction("ft_check", ftipc.IPCRequest{SessionID: sessionID}); ok {
		return raw
	}
	a.mu.RLock()
	client := a.clients[sessionID]
	a.mu.RUnlock()
	if client == nil {
		return mustJSON(ftResponse{OK: false, Error: "未连接"})
	}
	sftpTr := filetransfer.NewSFTPTransport(client)
	_, _, sftpErr := sftpTr.Check(context.Background())
	if sftpErr == nil {
		return mustJSON(ftResponse{OK: true, Message: "sftp(login)"})
	}
	scpTr := filetransfer.NewSCPTransport(client)
	ok, _, err := scpTr.Check(context.Background())
	if err != nil {
		return mustJSON(ftResponse{OK: false, Error: err.Error()})
	}
	if ok {
		return mustJSON(ftResponse{OK: true, Message: "scp(login)"})
	}
	return mustJSON(ftResponse{OK: false, Error: sftpErr.Error()})
}

func (a *FTPApp) FTRemoteReadFile(sessionID, remotePath string, maxBytes int64) string {
	if raw, ok := a.callIPCAction("ft_remote_read", ftipc.IPCRequest{SessionID: sessionID, Path: remotePath, MaxBytes: maxBytes}); ok {
		return raw
	}
	a.mu.RLock()
	client := a.clients[sessionID]
	rootPwd := a.rootPasswords[sessionID]
	loginUser := a.loginUsers[sessionID]
	a.mu.RUnlock()
	if client == nil {
		return mustJSON(ftResponse{OK: false, Error: "未连接"})
	}
	if rootPwd != "" {
		relay := filetransfer.NewRootRelayTransport(client, rootPwd, loginUser)
		content, err := relay.ReadFile(context.Background(), remotePath, maxBytes)
		if err != nil {
			return mustJSON(map[string]any{"ok": false, "error": err.Error()})
		}
		return mustJSON(map[string]any{"ok": true, "content": content})
	}
	tr := filetransfer.NewSFTPTransport(client)
	content, err := tr.ReadFile(context.Background(), remotePath, maxBytes)
	if err != nil {
		return mustJSON(map[string]any{"ok": false, "error": err.Error()})
	}
	return mustJSON(map[string]any{"ok": true, "content": content})
}

func (a *FTPApp) FTRemoteWriteFile(sessionID, remotePath, content string) string {
	if raw, ok := a.callIPCAction("ft_remote_write", ftipc.IPCRequest{SessionID: sessionID, Path: remotePath, Content: content}); ok {
		return raw
	}
	a.mu.RLock()
	client := a.clients[sessionID]
	rootPwd := a.rootPasswords[sessionID]
	loginUser := a.loginUsers[sessionID]
	a.mu.RUnlock()
	if client == nil {
		return mustJSON(ftResponse{OK: false, Error: "未连接"})
	}
	if rootPwd != "" {
		relay := filetransfer.NewRootRelayTransport(client, rootPwd, loginUser)
		if err := relay.WriteFile(context.Background(), remotePath, []byte(content)); err != nil {
			return mustJSON(map[string]any{"ok": false, "error": err.Error()})
		}
		return mustJSON(map[string]any{"ok": true})
	}
	tr := filetransfer.NewSFTPTransport(client)
	if err := tr.WriteFile(context.Background(), remotePath, []byte(content)); err != nil {
		return mustJSON(map[string]any{"ok": false, "error": err.Error()})
	}
	return mustJSON(map[string]any{"ok": true})
}

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
	out := make([]filetransfer.Entry, 0, len(entries))
	for _, de := range entries {
		fi, err := de.Info()
		if err != nil {
			continue
		}
		out = append(out, filetransfer.Entry{
			Path:    filepath.Join(p, de.Name()),
			Name:    de.Name(),
			IsDir:   de.IsDir(),
			Size:    fi.Size(),
			Mode:    uint32(fi.Mode()),
			ModTime: fi.ModTime(),
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
