// shell-sidecar：Shell 的本地 Go 服务（docs/shared-shell-integration.md）。
//
// 运行形态：
//   - 平台模式（默认）：控制面 stdio JSON-RPC（宿主插件 spawn 本进程并对话）；
//   - dev 模式（--dev）：控制面额外镜像到 ws://…/rpc，浏览器开发页可直连调试，
//     不需要宿主存在。
//
// 数据面两种模式都在：ws://127.0.0.1:<port>/terminals/{id}?token=…（二进制 PTY 字节流）。
// stdout 只允许 JSON-RPC 消息；一切日志走 stderr。
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"opscopilot/internal/shellsidecar"
	"opscopilot/pkg/completion"
)

var version = "dev"

func main() {
	var (
		wsAddr  = flag.String("ws-addr", "127.0.0.1:0", "数据面监听地址（默认随机端口）")
		dev     = flag.Bool("dev", false, "dev 模式：控制面镜像到 /rpc，供无宿主调试")
		token   = flag.String("token", "", "数据面鉴权 token（默认随机生成）")
		dataDir = flag.String("data-dir", "", "数据目录（启用已保存连接配置的持久化）")
	)
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	slog.SetDefault(logger)

	tok := *token
	if tok == "" {
		raw := make([]byte, 16)
		if _, err := rand.Read(raw); err != nil {
			slog.Error("生成 token 失败", "error", err)
			os.Exit(1)
		}
		tok = hex.EncodeToString(raw)
	}

	stdioWriter := &shellsidecar.RPCWriter{Out: os.Stdout}
	service := shellsidecar.NewTerminalService(version)
	api := &shellsidecar.ControlAPI{Service: service, Version: version, Token: tok}
	if *dataDir != "" {
		configs, err := shellsidecar.NewConfigService(*dataDir)
		if err != nil {
			slog.Error("初始化连接配置存储失败", "error", err)
			os.Exit(1)
		}
		api.Configs = configs
		quickCmds, err := shellsidecar.NewQuickCmdService(*dataDir)
		if err != nil {
			slog.Error("初始化快捷命令存储失败", "error", err)
			os.Exit(1)
		}
		api.QuickCmds = quickCmds
		scripts, err := shellsidecar.NewStructuredScriptService(service, *dataDir)
		if err != nil {
			slog.Error("初始化脚本服务失败", "error", err)
			os.Exit(1)
		}
		api.Scripts = scripts
		ft := shellsidecar.NewFTService(service, *dataDir)
		ft.SetNotify(api.Notify)
		api.FT = ft
		settingsSvc, err := shellsidecar.NewSettingsService(*dataDir)
		if err != nil {
			slog.Error("初始化设置存储失败", "error", err)
			os.Exit(1)
		}
		api.Settings = settingsSvc
		aiCfg, err := shellsidecar.NewAIConfigService(*dataDir)
		if err != nil {
			slog.Error("初始化 AI 配置存储失败", "error", err)
			os.Exit(1)
		}
		api.AI = aiCfg
		// 诊断案例与脚本服务共享 recorder（终端键入进入排查时间线）。
		diagnose, err := shellsidecar.NewDiagnoseService(aiCfg, *dataDir, scripts.Recorder())
		if err != nil {
			slog.Error("初始化诊断服务失败", "error", err)
			os.Exit(1)
		}
		diagnose.SetNotify(api.Notify)
		api.Diagnose = diagnose
	}
	// 补全服务：静态命令库（内嵌），无持久化依赖，始终可用；失败仅降级。
	if compDB, err := completion.NewDatabase(); err != nil {
		slog.Warn("初始化补全数据库失败，补全不可用", "error", err)
	} else {
		api.Completion = completion.NewService(compDB)
	}
	// 通知回路：service 事件 → stdio（+ dev 广播）。
	service.SetNotify(api.Notify)

	var hub *shellsidecar.DevHub
	if *dev {
		hub = shellsidecar.NewDevHub(api)
		api.Dev = hub
	}

	server, wsBase, err := shellsidecar.ServeDataPlane(*wsAddr, tok, service, hub)
	if err != nil {
		slog.Error("启动数据面失败", "error", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// 信号退出：关全部 SSH 连接 → 停 HTTP。
	go func() {
		<-ctx.Done()
		slog.Info("收到退出信号，正在断开全部连接")
		service.Shutdown()
		_ = server.Close()
	}()

	api.Ready(stdioWriter, wsBase)
	slog.Info("shell-sidecar 就绪", "ws", wsBase, "dev", *dev, "version", version)

	if *dev {
		// dev 模式无宿主：stdin 是空/关闭的，不跑 stdio 主循环，等信号退出。
		<-ctx.Done()
		slog.Info("收到退出信号")
	} else {
		// 平台模式：控制面主循环在 stdio；stdin 关闭 = 宿主离开，主动收尾。
		shellsidecar.ServeControl(ctx, os.Stdin, stdioWriter, api)
	}
	service.Shutdown()
	_ = server.Close()
	slog.Info("shell-sidecar 退出")
}
