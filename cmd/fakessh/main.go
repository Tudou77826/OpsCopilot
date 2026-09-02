// fakessh：开发/冒烟用的进程内回显 SSH 服务器（test/test）。
// 用法：go run ./cmd/fakessh 后按提示连接 127.0.0.1:<port>。
package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"opscopilot/internal/shellsidecar/fakessh"
)

func main() {
	server, err := fakessh.Start("== fakessh ready ==\r\n", os.Getenv("FAKESSH_SFTP_ROOT"))
	if err != nil {
		fmt.Fprintln(os.Stderr, "启动失败:", err)
		os.Exit(1)
	}
	fmt.Printf("fakessh 监听 127.0.0.1:%d（账号 test/test，全量回显）\n", server.Port())
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	<-sig
	_ = server.Close()
}
