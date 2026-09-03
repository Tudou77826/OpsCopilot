package shellsidecar

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"opscopilot/pkg/config"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"opscopilot/pkg/completion"
)

func Run(version string, args []string, desktopRoot string) error {
	flags := flag.NewFlagSet("opscopilot-shell", flag.ContinueOnError)
	var (
		wsAddr         = flags.String("ws-addr", "127.0.0.1:0", "数据面监听地址（默认随机端口）")
		dev            = flags.Bool("dev", false, "dev 模式：控制面镜像到 /rpc，供无宿主调试")
		token          = flags.String("token", "", "数据面鉴权 token（默认随机生成）")
		dataDir        = flags.String("data-dir", "", "数据目录（启用已保存连接配置的持久化）")
		workspaceFiles = flags.Bool("workspace-files", false, "本地文件面板仅开放独立 files 文件区")
	)
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected arguments")
	}
	if desktopRoot != "" && *dataDir == "" {
		return fmt.Errorf("插件临时文件区未指定")
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	slog.SetDefault(logger)

	tok := *token
	if tok == "" {
		raw := make([]byte, 16)
		if _, err := rand.Read(raw); err != nil {
			slog.Error("生成 token 失败", "error", err)
			return fmt.Errorf("初始化 Ops 服务失败")
		}
		tok = hex.EncodeToString(raw)
	}

	stdioWriter := &RPCWriter{Out: os.Stdout}
	service := NewTerminalService(version)
	api := &ControlAPI{Service: service, Version: version, Token: tok, LocalInstallation: desktopRoot != ""}
	if desktopRoot != "" {
		paths, err := ResolveDesktopData(desktopRoot)
		if err != nil {
			return err
		}
		api.Configs, err = NewConfigServiceWithPath(filepath.Join(paths.Root, "sessions.json"))
		if err != nil {
			return err
		}
		api.Configs.mgr.PreserveCredentials = true
		inner, err := newJSONListService(filepath.Join(paths.Root, "quick_commands.json"))
		if err != nil {
			return err
		}
		api.QuickCmds = &QuickCmdService{inner: inner}
		api.Settings = &SettingsService{desktop: &desktopSettings{root: paths.Root, snapshots: map[string]*config.Manager{}}}
		api.AI = &AIConfigService{desktopRoot: paths.Root}
		api.Scripts, err = NewStructuredScriptServiceWithPaths(service, paths.Scripts, paths.Recordings)
		if err != nil {
			return err
		}
		defer api.Scripts.Close()
		api.FT, err = NewWorkspaceFT(service, filepath.Join(*dataDir, "files"))
		if err != nil {
			return err
		}
		defer api.FT.Close()
		api.FT.SetNotify(api.Notify)
		*workspaceFiles = true
	} else if *dataDir != "" {
		configs, err := NewConfigService(*dataDir)
		if err != nil {
			slog.Error("初始化连接配置存储失败", "error", err)
			return fmt.Errorf("初始化 Ops 服务失败")
		}
		api.Configs = configs
		quickCmds, err := NewQuickCmdService(*dataDir)
		if err != nil {
			slog.Error("初始化快捷命令存储失败", "error", err)
			return fmt.Errorf("初始化 Ops 服务失败")
		}
		api.QuickCmds = quickCmds
		scripts, err := NewStructuredScriptService(service, *dataDir)
		if err != nil {
			slog.Error("初始化脚本服务失败", "error", err)
			return fmt.Errorf("初始化 Ops 服务失败")
		}
		api.Scripts = scripts
		defer scripts.Close()
		ft := NewFTService(service, *dataDir)
		if *workspaceFiles {
			ft, err = NewWorkspaceFT(service, filepath.Join(*dataDir, "files"))
			if err != nil {
				slog.Error("初始化文件区失败")
				return fmt.Errorf("初始化 Ops 服务失败")
			}
		}
		defer ft.Close()
		ft.SetNotify(api.Notify)
		api.FT = ft
		settingsSvc, err := NewSettingsService(*dataDir)
		if err != nil {
			slog.Error("初始化设置存储失败", "error", err)
			return fmt.Errorf("初始化 Ops 服务失败")
		}
		api.Settings = settingsSvc
		aiCfg, err := NewAIConfigService(*dataDir)
		if err != nil {
			slog.Error("初始化 AI 配置存储失败", "error", err)
			return fmt.Errorf("初始化 Ops 服务失败")
		}
		api.AI = aiCfg
		// 诊断案例与脚本服务共享 recorder（终端键入进入排查时间线）。
		diagnose, err := NewDiagnoseService(aiCfg, *dataDir, scripts.Recorder())
		if err != nil {
			slog.Error("初始化诊断服务失败", "error", err)
			return fmt.Errorf("初始化 Ops 服务失败")
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

	var hub *DevHub
	if *dev {
		hub = NewDevHub(api)
		api.Dev = hub
	}

	var filesHandler http.Handler
	if *workspaceFiles && api.FT != nil {
		filesHandler = api.FT.WorkspaceHTTP(tok)
	}
	server, wsBase, err := ServeDataPlane(*wsAddr, tok, service, hub, filesHandler)
	if err != nil {
		slog.Error("启动数据面失败", "error", err)
		return fmt.Errorf("初始化 Ops 服务失败")
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
		ServeControl(ctx, os.Stdin, stdioWriter, api)
	}
	service.Shutdown()
	_ = server.Close()
	slog.Info("shell-sidecar 退出")
	return nil
}
