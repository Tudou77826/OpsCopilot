// ops-sidecar 是 AAW Desktop remoteOps 插件的常驻运维 sidecar：
// 无 GUI 依赖，NDJSON over stdio，复用运维内核（pkg/core/ops），
// 与 OpsCopilot GUI 共享同一份数据目录（sessions.json / 白名单 / 凭据）。
// 数据目录由宿主通过 OPSCOPILOT_DATA_DIR 注入；缺省退回 exe 同目录。
//
// 两种运行形态：缺省 service 模式（NDJSON 协议 + 回环 MCP 桥监听）；
// `--mcp` 为 Agent 侧 stdio 代理（ACP mcpServers 拉起，转发到 service）。
package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"opscopilot/pkg/opssidecar"
	"opscopilot/pkg/projectspike"
	// 空白 import 触发 sshclient/telnetclient 的 init()，向 remote 注册协议
	// dialer。ops 包只依赖 remote 抽象，不直接 import 协议实现。
	_ "opscopilot/pkg/sshclient"
	_ "opscopilot/pkg/telnetclient"
)

func main() {
	if len(os.Args) == 2 && os.Args[1] == "--project-spike" {
		if err := projectspike.Run(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if len(os.Args) == 2 && os.Args[1] == "--mcp" {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
		go func() {
			<-sig
			os.Exit(0)
		}()
		os.Exit(opssidecar.RunMCPProxy(os.Stdin, os.Stdout, opssidecar.ResolveDataDir()))
	}
	if len(os.Args) > 1 {
		fmt.Fprintln(os.Stderr, "[ops-sidecar] 未知参数，仅支持 --mcp、--project-spike 或无参 service 模式")
		os.Exit(2)
	}
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sig
		os.Exit(0)
	}()
	os.Exit(opssidecar.Run())
}
