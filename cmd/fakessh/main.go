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
	// FAKESSH_ADDR 可固定监听地址（断链/重连类验证需要两次启动同端口）。
	addr := os.Getenv("FAKESSH_ADDR")
	banner := os.Getenv("FAKESSH_BANNER")
	if banner == "" {
		banner = "== fakessh ready ==\r\n"
	}
	var server *fakessh.Server
	var err error
	if addr != "" {
		server, err = fakessh.StartAddr(banner, os.Getenv("FAKESSH_SFTP_ROOT"), addr)
	} else {
		server, err = fakessh.Start(banner, os.Getenv("FAKESSH_SFTP_ROOT"))
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "启动失败:", err)
		os.Exit(1)
	}
	fmt.Printf("fakessh 监听 %s（账号 test/test，全量回显）\n", server.Host())
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	<-sig
	_ = server.Close()
}
