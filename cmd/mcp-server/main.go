package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"opscopilot/pkg/mcpserver"
)

func main() {
	// 获取可执行文件所在目录作为配置文件基础路径
	execPath, _ := os.Executable()
	binDir := filepath.Dir(execPath)

	// 获取 sessions.json 路径
	// 优先使用环境变量，否则使用可执行文件所在目录
	sessionsFile := os.Getenv("OPSCOPILOT_SESSIONS_FILE")
	if sessionsFile == "" {
		sessionsFile = filepath.Join(binDir, "sessions.json")
	}

	// 获取录制目录
	recordingsDir := os.Getenv("OPSCOPILOT_RECORDINGS_DIR")
	if recordingsDir == "" {
		recordingsDir = filepath.Join(binDir, "recordings")
	}

	// 获取知识库目录（用于归档排查经验）
	knowledgeDir := os.Getenv("OPSCOPILOT_KNOWLEDGE_DIR")
	if knowledgeDir == "" {
		knowledgeDir = filepath.Join(binDir, "docs")
	}

	// 获取白名单配置文件路径
	whitelistPath := os.Getenv("OPSCOPILOT_WHITELIST_PATH")
	if whitelistPath == "" {
		whitelistPath = filepath.Join(binDir, "command_whitelist.json")
	}

	// 获取文件访问控制配置文件路径
	fileAccessPath := os.Getenv("OPSCOPILOT_FILE_ACCESS_PATH")
	if fileAccessPath == "" {
		fileAccessPath = filepath.Join(binDir, "file_access.json")
	}

	// 创建服务器配置
	serverConfig := &mcpserver.Config{
		ConfigDir:      binDir,
		SessionsFile:   sessionsFile,
		RecordingsDir:  recordingsDir,
		KnowledgeDir:   knowledgeDir,
		WhitelistPath:  whitelistPath,
		FilePath:       fileAccessPath,
		MaxTotalBytes:  10240,
		MaxLineLength:  500,
		HeadLines:      5,
		IdleTimeoutMin: 30,
	}

	// 创建 MCP Server
	server, err := mcpserver.NewServer(serverConfig)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create MCP server: %v\n", err)
		os.Exit(1)
	}
	defer server.Shutdown()

	// 设置信号处理
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		server.Shutdown()
		os.Exit(0)
	}()

	// 开始处理 MCP 协议（通过 stdin/stdout）
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line := scanner.Bytes()

		var req mcpserver.JSONRPCRequest
		if err := json.Unmarshal(line, &req); err != nil {
			server.SendError(nil, -32700, "Parse error")
			continue
		}

		server.HandleRequest(&req)
	}

	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "Scanner error: %v\n", err)
	}
}
