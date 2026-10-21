package acp

import (
	"context"
	"errors"
	"os/exec"
	"path/filepath"
)

type ProcessConfig struct {
	Executable  string
	Args        []string
	Directory   string
	Environment []string
}

func Start(ctx context.Context, config ProcessConfig, handler Handler) (*Client, error) {
	if !filepath.IsAbs(config.Executable) || !filepath.IsAbs(config.Directory) {
		return nil, errors.New("ACP 程序和工作目录必须为绝对路径")
	}
	command := exec.CommandContext(ctx, config.Executable, config.Args...)
	command.Dir = config.Directory
	command.Env = config.Environment
	hideWindow(command)
	stdout, err := command.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stdin, err := command.StdinPipe()
	if err != nil {
		_ = stdout.Close()
		return nil, err
	}
	// Consume stderr without forwarding arbitrary third-party output. Only a
	// recognized dependency name can be returned as a startup diagnostic.
	diagnostic := &startupDiagnostic{}
	command.Stderr = diagnostic
	if err := command.Start(); err != nil {
		_ = stdout.Close()
		_ = stdin.Close()
		return nil, errors.New("无法启动 ACP 程序，请检查运行依赖")
	}
	client := Connect(ctx, stdout, stdin, handler, func() { _ = command.Process.Kill() })
	client.startupDiagnostic = diagnostic
	done := make(chan struct{})
	client.processDone = done
	go func() { defer close(done); <-client.Done(); _ = command.Wait() }()
	return client, nil
}
