package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"opscopilot/pkg/core/ops"
)

// cmdDiagnose: opscopilot diagnose --problem Y
// 基于知识库的 AI 故障诊断。纯知识检索 + 推理，不碰任何服务器。
// 输出建议命令和排查步骤，由外部 AI/用户自行决定是否调 exec 验证。
func cmdDiagnose(args []string) int {
	fs := flag.NewFlagSet("diagnose", flag.ExitOnError)
	problem := fs.String("problem", "", "故障现象描述（必填）")
	fs.Parse(args)

	if *problem == "" {
		fmt.Fprintln(os.Stderr, "错误: --problem 必填")
		fs.Usage()
		return 1
	}

	env := loadCLIEnv()
	aiSvc, err := newAIService()
	if err != nil {
		fmt.Fprintf(os.Stderr, "初始化 AI 服务失败: %v\n", err)
		return 1
	}

	// 构建知识库目录
	if err := aiSvc.UpdateCatalog(env.knowledgeDir); err != nil {
		fmt.Fprintf(os.Stderr, "[WARN] 知识库目录加载失败（诊断将不基于知识库）: %v\n", err)
	}

	answer, err := aiSvc.AskTroubleshoot(context.Background(), *problem, env.knowledgeDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "诊断失败: %v\n", err)
		return 1
	}

	// 直接输出诊断结果（含 summary/steps/commands，commands 带知识库来源行号）
	// 不替调用方决定执行哪些命令 —— 那是 exec 子命令的职责
	out, _ := json.Marshal(map[string]any{
		"diagnosis": answer,
	})
	fmt.Println(string(out))
	return 0
}

// cmdFile: opscopilot file <download|upload> ...
func cmdFile(args []string) int {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "用法: opscopilot file <download|upload> ...")
		return 1
	}

	env := loadCLIEnv()
	mgr, err := newOpsManager(env)
	if err != nil {
		fmt.Fprintf(os.Stderr, "初始化失败: %v\n", err)
		return 1
	}
	defer mgr.Shutdown()

	switch args[0] {
	case "download":
		fs := flag.NewFlagSet("file download", flag.ExitOnError)
		server := fs.String("server", "", "服务器 IP（必填，需已在 OpsCopilot 中登记）")
		remote := fs.String("remote", "", "远程文件路径（必填）")
		local := fs.String("local", "", "本地落地路径（必填）")
		maxBytes := fs.Int("max-bytes", 10485760, "最大下载字节数")
		fs.Parse(args[1:])

		if *server == "" || *remote == "" || *local == "" {
			fmt.Fprintln(os.Stderr, "错误: --server --remote --local 必填")
			return 1
		}
		// Download 内部自动连接（惰性）
		r, err := mgr.Download(*server, *remote, ops.DownloadOptions{LocalPath: *local, MaxBytes: *maxBytes})
		if err != nil {
			fmt.Fprintf(os.Stderr, "下载失败: %v\n", err)
			return 1
		}
		out, _ := json.Marshal(r)
		fmt.Println(string(out))

	case "upload":
		fs := flag.NewFlagSet("file upload", flag.ExitOnError)
		server := fs.String("server", "", "服务器 IP（必填，需已在 OpsCopilot 中登记）")
		local := fs.String("local", "", "本地源文件路径（必填）")
		remote := fs.String("remote", "", "远程目标路径（必填）")
		backup := fs.Bool("backup", true, "覆盖前备份远程文件")
		mkdir := fs.Bool("mkdir", false, "自动创建远程目录")
		fs.Parse(args[1:])

		if *server == "" || *local == "" || *remote == "" {
			fmt.Fprintln(os.Stderr, "错误: --server --local --remote 必填")
			return 1
		}
		// Upload 内部自动连接（惰性）
		r, err := mgr.Upload(*server, *remote, ops.UploadOptions{LocalPath: *local, Backup: *backup, Mkdir: *mkdir})
		if err != nil {
			fmt.Fprintf(os.Stderr, "上传失败: %v\n", err)
			return 1
		}
		out, _ := json.Marshal(r)
		fmt.Println(string(out))

	default:
		fmt.Fprintf(os.Stderr, "未知 file 子命令: %s（可选 download/upload）\n", args[0])
		return 1
	}
	return 0
}
