package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"opscopilot/pkg/ai"
	"opscopilot/pkg/config"
	"opscopilot/pkg/core/ops"
	"opscopilot/pkg/llm"
	// blank import 触发 sshclient/telnetclient 的 init(),向 remote 注册
	// 协议 dialer。ops 包只依赖 remote 抽象,不直接 import 协议实现,
	// 故在 CLI 入口显式引入,确保 CLI exec 能分派到对应协议。
	_ "opscopilot/pkg/sshclient"
	_ "opscopilot/pkg/telnetclient"
)

// cliEnv 聚合 CLI 子命令需要的环境/路径配置
// 所有路径默认从可执行文件所在目录推导，支持环境变量覆盖
type cliEnv struct {
	binDir         string
	sessionsFile   string
	whitelistPath  string
	fileAccessPath string
	knowledgeDir   string
}

func loadCLIEnv() cliEnv {
	execPath, _ := os.Executable()
	binDir := filepath.Dir(execPath)

	resolve := func(envVar, defaultName string) string {
		if v := os.Getenv(envVar); v != "" {
			return v
		}
		return filepath.Join(binDir, defaultName)
	}

	return cliEnv{
		binDir:         binDir,
		sessionsFile:   resolve("OPSCOPILOT_SESSIONS_FILE", "sessions.json"),
		whitelistPath:  resolve("OPSCOPILOT_WHITELIST_PATH", "command_whitelist.json"),
		fileAccessPath: resolve("OPSCOPILOT_FILE_ACCESS_PATH", "file_access.json"),
		knowledgeDir:   resolve("OPSCOPILOT_KNOWLEDGE_DIR", "docs"),
	}
}

func loadCLIExecTimeoutSec(env cliEnv) int {
	data, err := os.ReadFile(filepath.Join(env.binDir, "config.json"))
	if err != nil {
		return config.DefaultCLIExecTimeoutSec
	}

	var cfg struct {
		CLI config.CLIConfig `json:"cli"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil || cfg.CLI.ExecTimeoutSec <= 0 {
		return config.DefaultCLIExecTimeoutSec
	}
	return cfg.CLI.ExecTimeoutSec
}

// newOpsManager 构造运维内核管理器（复用 core/ops）
func newOpsManager(env cliEnv) (*ops.Manager, error) {
	return ops.NewManager(&ops.Config{
		SessionsFile:   env.sessionsFile,
		WhitelistPath:  env.whitelistPath,
		FilePath:       env.fileAccessPath,
		MaxTotalBytes:  10240,
		MaxLineLength:  500,
		HeadLines:      5,
		IdleTimeoutMin: 30,
	})
}

// newAIService 构造 AI 诊断服务，复用 GUI 的 LLM 配置。
// configPath 从 env.binDir 推导（与 exe 同目录），而非 cwd——
// 用户可能在任意目录调用 CLI，cwd 下不一定有 config.json。
// 前提：用户已在 OpsCopilot GUI 中配置过 LLM（API key/BaseURL/Model）
func newAIService(env cliEnv) (*ai.AIService, error) {
	configMgr := config.NewManagerWithDir(env.binDir)
	if err := configMgr.Load(); err != nil {
		return nil, fmt.Errorf("加载配置失败（请先在 OpsCopilot GUI 中配置 LLM）: %w", err)
	}
	llmConfig := configMgr.Config.LLM
	if llmConfig.APIKey == "" {
		return nil, fmt.Errorf("LLM API key 未配置，请先在 OpsCopilot GUI 中配置")
	}

	fastModel := llmConfig.FastModel
	if fastModel == "" {
		fastModel = llmConfig.Model
	}
	if fastModel == "" {
		fastModel = "deepseek-chat"
	}
	complexModel := llmConfig.ComplexModel
	if complexModel == "" {
		complexModel = "glm46"
	}

	fastProvider := llm.NewOpenAIProvider(llmConfig.APIKey, llmConfig.BaseURL, fastModel)
	complexProvider := llm.NewOpenAIProvider(llmConfig.APIKey, llmConfig.BaseURL, complexModel)
	// CLI 模式不推送事件到 UI
	ai.SetEventEmitter(nil)
	return ai.NewAIService(fastProvider, complexProvider, configMgr), nil
}

// runCLI 是 CLI 模式入口，解析子命令并分发
func runCLI(args []string) int {
	if len(args) < 1 {
		printCLIUsage()
		return 1
	}

	switch args[0] {
	case "exec":
		return cmdExec(args[1:])
	case "diagnose":
		return cmdDiagnose(args[1:])
	case "file":
		return cmdFile(args[1:])
	case "-h", "--help", "help":
		printCLIUsage()
		return 0
	default:
		fmt.Fprintf(os.Stderr, "未知子命令: %s\n\n", args[0])
		printCLIUsage()
		return 1
	}
}

func printCLIUsage() {
	fmt.Fprintln(os.Stderr, `OpsCopilot CLI —— 运维能力命令行入口

用法:
  opscopilot <子命令> [参数]

子命令:
  exec       在远程服务器上执行命令（受白名单约束）
  diagnose   基于知识库的 AI 故障诊断（输出建议命令和排查步骤，不碰服务器）
  file       文件传输（上传/下载，受文件访问控制约束）

不带子命令启动时进入图形界面。

环境变量（可选，默认从可执行文件所在目录读取）:
  OPSCOPILOT_SESSIONS_FILE    sessions.json 路径
  OPSCOPILOT_WHITELIST_PATH   命令白名单配置路径
  OPSCOPILOT_FILE_ACCESS_PATH 文件访问控制配置路径
  OPSCOPILOT_KNOWLEDGE_DIR    知识库目录路径`)
}

// cmdExec: opscopilot exec --server X --command Y
func cmdExec(args []string) int {
	env := loadCLIEnv()
	defaultTimeoutSec := loadCLIExecTimeoutSec(env)

	fs := flag.NewFlagSet("exec", flag.ExitOnError)
	server := fs.String("server", "", "服务器 IP（必填，需已在 OpsCopilot 中登记）")
	command := fs.String("command", "", "要执行的命令（必填）")
	intent := fs.String("intent", "", "执行这条命令的简要意图")
	maxLineLen := fs.Int("max-line-length", 500, "单行最大长度")
	timeoutSec := fs.Int("timeout-sec", defaultTimeoutSec, "单条命令超时秒数")
	fs.Parse(args)

	if *server == "" || *command == "" {
		fmt.Fprintln(os.Stderr, "错误: --server 和 --command 必填")
		fs.Usage()
		return 1
	}
	if *timeoutSec <= 0 {
		fmt.Fprintln(os.Stderr, "Error: --timeout-sec must be greater than 0")
		return 1
	}

	mgr, err := newOpsManager(env)
	if err != nil {
		fmt.Fprintf(os.Stderr, "初始化失败: %v\n", err)
		return 1
	}
	defer mgr.Shutdown()

	// Exec 内部自动连接（惰性）+ 白名单优先校验 + 命令级超时
	result, err := mgr.Exec(context.Background(), *server, *command, ops.ExecOptions{
		MaxLineLength: *maxLineLen,
		Timeout:       time.Duration(*timeoutSec) * time.Second,
		Intent:        *intent,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "执行失败: %v\n", err)
		return 1
	}

	out, _ := json.Marshal(result)
	fmt.Println(string(out))
	return 0
}
