package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"path/filepath"
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

	mu           sync.RWMutex
	clients     map[string]*ssh.Client
	closeFns    map[string]func()
	rootPasswords map[string]string

	ftMu      sync.Mutex
	ftCancels map[string]context.CancelFunc
}

// NewFTPApp creates a new FTP manager application.
func NewFTPApp(ipcTokenFile, sessionID string) *FTPApp {
	return &FTPApp{
		ipcTokenFile: ipcTokenFile,
		sessionID:    sessionID,
		clients:     make(map[string]*ssh.Client),
		closeFns:    make(map[string]func()),
		rootPasswords: make(map[string]string),
		ftCancels: make(map[string]context.CancelFunc),
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
	if a.sessionID != "" {
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
	mgr := sessionmanager.NewManager()
	if err := mgr.Load(); err != nil {
		return "[]"
	}
	sessions := mgr.GetSessions()
	names := make([]string, 0)
	for _, s := range sessions {
		if s.Config != nil {
			names = append(names, s.Name)
		}
	}
	return mustJSON(names)
}

func (a *FTPApp) Connect(sessionName string) string {
	mgr := sessionmanager.NewManager()
	if err := mgr.Load(); err != nil {
		return mustJSON(map[string]any{"ok": false, "error": "加载配置失败"})
	}

	session := a.findSession(mgr.GetSessions(), sessionName)
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
	a.clients[sessionName] = client.SSHClient()
	a.closeFns[sessionName] = func() { _ = client.Close() }
	a.rootPasswords[sessionName] = cfg.RootPassword
	a.mu.Unlock()

	return mustJSON(map[string]any{"ok": true, "message": "已连接"})
}

func (a *FTPApp) Disconnect(sessionName string) string {
	a.mu.Lock()
	client := a.clients[sessionName]
	closeFn := a.closeFns[sessionName]
	delete(a.clients, sessionName)
	delete(a.closeFns, sessionName)
	delete(a.rootPasswords, sessionName)
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
	a.mu.RLock()
	client := a.clients[sessionID]
	rootPwd := a.rootPasswords[sessionID]
	a.mu.RUnlock()

	if client == nil {
		return mustJSON(ftResponse{OK: false, Error: "未连接"})
	}

	var entries []filetransfer.Entry
	var err error
	if rootPwd != "" {
		relay := filetransfer.NewRootRelayTransport(client, rootPwd)
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
	a.mu.RLock()
	client := a.clients[sessionID]
	rootPwd := a.rootPasswords[sessionID]
	a.mu.RUnlock()

	if client == nil {
		return mustJSON(ftResponse{OK: false, Error: "未连接"})
	}

	var entry filetransfer.Entry
	var err error
	if rootPwd != "" {
		relay := filetransfer.NewRootRelayTransport(client, rootPwd)
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
	return a.startTransfer(sessionID, "upload", localPath, remotePath)
}

func (a *FTPApp) FTDownload(sessionID, remotePath, localPath string) string {
	return a.startTransfer(sessionID, "download", remotePath, localPath)
}

func (a *FTPApp) FTCancel(taskID string) string {
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
	a.mu.RLock()
	client := a.clients[sessionID]
	rootPwd := a.rootPasswords[sessionID]
	a.mu.RUnlock()

	if client == nil {
		return mustJSON(ftResponse{OK: false, Error: "未连接"})
	}
	if rootPwd != "" {
		relay := filetransfer.NewRootRelayTransport(client, rootPwd)
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
	a.mu.RLock()
	client := a.clients[sessionID]
	rootPwd := a.rootPasswords[sessionID]
	a.mu.RUnlock()

	if client == nil {
		return mustJSON(ftResponse{OK: false, Error: "未连接"})
	}
	if rootPwd != "" {
		relay := filetransfer.NewRootRelayTransport(client, rootPwd)
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
	a.mu.RLock()
	client := a.clients[sessionID]
	rootPwd := a.rootPasswords[sessionID]
	a.mu.RUnlock()

	if client == nil {
		return mustJSON(ftResponse{OK: false, Error: "未连接"})
	}
	if rootPwd != "" {
		relay := filetransfer.NewRootRelayTransport(client, rootPwd)
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
	OK      bool                  `json:"ok"`
	Message string               `json:"message,omitempty"`
	Error   string               `json:"error,omitempty"`
	Entries []filetransfer.Entry `json:"entries,omitempty"`
	Entry   *filetransfer.Entry `json:"entry,omitempty"`
	TaskID  string               `json:"taskId,omitempty"`
}

func (a *FTPApp) startTransfer(sessionID, op, localPath, remotePath string) string {
	a.mu.RLock()
	client := a.clients[sessionID]
	rootPwd := a.rootPasswords[sessionID]
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
			runtime.EventsEmit(a.ctx, "ftp-transfer-progress", map[string]any{
				"taskId":     taskID,
				"sessionId":  sessionID,
				"bytesDone":  p.BytesDone,
				"bytesTotal": p.BytesTotal,
				"speedBps":   p.SpeedBps,
			})
		}

		var (
			res   filetransfer.TransferResult
			opErr error
		)
		if rootPwd != "" {
			relay := filetransfer.NewRootRelayTransport(client, rootPwd)
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
			runtime.EventsEmit(a.ctx, "ftp-transfer-done", map[string]any{
				"taskId":    taskID,
				"sessionId":  sessionID,
				"ok":      false,
				"error":   opErr.Error(),
			})
			return
		}
		runtime.EventsEmit(a.ctx, "ftp-transfer-done", map[string]any{
			"taskId":    taskID,
			"sessionId":  sessionID,
			"ok":      true,
			"bytes":    res.Bytes,
			"message": "完成",
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
