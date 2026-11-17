// ops-sidecar 是 AAW Desktop remoteOps 插件的常驻运维 sidecar：
// 无 GUI 依赖，NDJSON over stdio，复用运维内核（pkg/core/ops），
// 与 OpsCopilot GUI 共享同一份数据目录（sessions.json / 白名单 / 凭据）。
// 数据目录由宿主通过 OPSCOPILOT_DATA_DIR 注入；缺省退回 exe 同目录。
package main

import (
	"os"
	"os/signal"
	"syscall"

	"opscopilot/pkg/opssidecar"
	// 空白 import 触发 sshclient/telnetclient 的 init()，向 remote 注册协议
	// dialer。ops 包只依赖 remote 抽象，不直接 import 协议实现。
	_ "opscopilot/pkg/sshclient"
	_ "opscopilot/pkg/telnetclient"
)

func main() {
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sig
		os.Exit(0)
	}()
	os.Exit(opssidecar.Run())
}
