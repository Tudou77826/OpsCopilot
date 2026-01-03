package main

import (
	"context"
	"fmt"
	"io"
	"opscopilot/pkg/sshclient"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// App struct
type App struct {
	ctx    context.Context
	client *sshclient.Client
	stdin  io.WriteCloser
}

// NewApp creates a new App application struct
func NewApp() *App {
	return &App{}
}

// startup is called when the app starts. The context is saved
// so we can call the runtime methods
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

type ConnectConfig struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	User     string `json:"user"`
	Password string `json:"password"`
}

func (a *App) Connect(config ConnectConfig) string {
	clientConfig := &sshclient.ConnectConfig{
		Host:     config.Host,
		Port:     config.Port,
		User:     config.User,
		Password: config.Password,
	}

	client, err := sshclient.NewClient(clientConfig)
	if err != nil {
		return fmt.Sprintf("Error connecting: %v", err)
	}
	a.client = client

	// Start shell with default size
	_, stdin, stdout, err := client.StartShell(120, 30)
	if err != nil {
		return fmt.Sprintf("Error starting shell: %v", err)
	}

	a.stdin = stdin

	// Read loop
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := stdout.Read(buf)
			if err != nil {
				if err != io.EOF {
					runtime.EventsEmit(a.ctx, "terminal-data", fmt.Sprintf("\r\nConnection Error: %v\r\n", err))
				} else {
					runtime.EventsEmit(a.ctx, "terminal-data", "\r\nConnection Closed\r\n")
				}
				runtime.EventsEmit(a.ctx, "connection-status", "Disconnected")
				a.stdin = nil
				a.client = nil
				break
			}
			if n > 0 {
				// Convert to base64 if needed, but string is usually fine for utf8
				runtime.EventsEmit(a.ctx, "terminal-data", string(buf[:n]))
			}
		}
	}()

	return "Connected"
}

func (a *App) Write(data string) {
	if a.stdin != nil {
		_, err := a.stdin.Write([]byte(data))
		if err != nil {
			runtime.EventsEmit(a.ctx, "terminal-data", fmt.Sprintf("\r\nWrite Error: %v\r\n", err))
		}
	}
}

// Greet returns a greeting for the given name
func (a *App) Greet(name string) string {
	return fmt.Sprintf("Hello %s, It's show time!", name)
}
