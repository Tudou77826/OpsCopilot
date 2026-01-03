package main

import (
	"context"
	"fmt"
	"io"
	"opscopilot/pkg/secretstore"
	"opscopilot/pkg/session"
	"opscopilot/pkg/sshclient"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// App struct
type App struct {
	ctx         context.Context
	sessionMgr  *session.Manager
	secretStore secretstore.SecretStore
}

// NewApp creates a new App application struct
func NewApp() *App {
	return &App{
		sessionMgr:  session.NewManager(),
		secretStore: secretstore.NewKeyringStore(),
	}
}

// startup is called when the app starts. The context is saved
// so we can call the runtime methods
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

type ConnectConfig struct {
	Host         string         `json:"host"`
	Port         int            `json:"port"`
	User         string         `json:"user"`
	Password     string         `json:"password"`
	RootPassword string         `json:"rootPassword"`
	Bastion      *ConnectConfig `json:"bastion"`
}

type ConnectResult struct {
    Success   bool   `json:"success"`
    SessionID string `json:"sessionId"`
    Message   string `json:"message"`
}

func (a *App) Connect(config ConnectConfig) ConnectResult {
	// 尝试从 SecretStore 保存密码（如果提供了）
	if config.Password != "" {
		_ = a.secretStore.Set("OpsCopilot-SSH", config.Host+":"+config.User, config.Password)
	}

	clientConfig := &sshclient.ConnectConfig{
		Host:     config.Host,
		Port:     config.Port,
		User:     config.User,
		Password: config.Password,
	}

	// 递归构建 Bastion 配置
	if config.Bastion != nil {
		clientConfig.Bastion = &sshclient.ConnectConfig{
			Host:     config.Bastion.Host,
			Port:     config.Bastion.Port,
			User:     config.Bastion.User,
			Password: config.Bastion.Password,
		}
		// 保存 Bastion 密码
		if config.Bastion.Password != "" {
			_ = a.secretStore.Set("OpsCopilot-SSH", config.Bastion.Host+":"+config.Bastion.User, config.Bastion.Password)
		}
	}

	client, err := sshclient.NewClient(clientConfig)
	if err != nil {
		return ConnectResult{Success: false, Message: fmt.Sprintf("Error connecting: %v", err)}
	}

	// Start shell with default size
	var stdin io.WriteCloser
	var stdout io.Reader

	if config.RootPassword != "" {
		_, stdin, stdout, err = client.StartShellWithSudo(120, 30, config.RootPassword)
	} else {
		_, stdin, stdout, err = client.StartShell(120, 30)
	}

	if err != nil {
        client.Close()
		return ConnectResult{Success: false, Message: fmt.Sprintf("Error starting shell: %v", err)}
	}

    // Add to session manager
    sessionID := a.sessionMgr.Add(client, stdin)

	// Read loop
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := stdout.Read(buf)
			if err != nil {
				if err != io.EOF {
					runtime.EventsEmit(a.ctx, "terminal-data:"+sessionID, fmt.Sprintf("\r\nConnection Error: %v\r\n", err))
				} else {
					runtime.EventsEmit(a.ctx, "terminal-data:"+sessionID, "\r\nConnection Closed\r\n")
				}
                // Notify session closed
				runtime.EventsEmit(a.ctx, "session-closed", sessionID)
                a.sessionMgr.Remove(sessionID)
				break
			}
			if n > 0 {
				runtime.EventsEmit(a.ctx, "terminal-data:"+sessionID, string(buf[:n]))
			}
		}
	}()

	return ConnectResult{Success: true, SessionID: sessionID, Message: "Connected"}
}

func (a *App) Write(sessionID string, data string) {
    sess, ok := a.sessionMgr.Get(sessionID)
	if ok && sess.Stdin != nil {
		_, err := sess.Stdin.Write([]byte(data))
		if err != nil {
			runtime.EventsEmit(a.ctx, "terminal-data:"+sessionID, fmt.Sprintf("\r\nWrite Error: %v\r\n", err))
		}
	}
}

func (a *App) Broadcast(sessionIDs []string, data string) {
    a.sessionMgr.Broadcast(sessionIDs, data)
}

func (a *App) CloseSession(sessionID string) {
    a.sessionMgr.Remove(sessionID)
}

// Greet returns a greeting for the given name
func (a *App) Greet(name string) string {
	return fmt.Sprintf("Hello %s, It's show time!", name)
}
