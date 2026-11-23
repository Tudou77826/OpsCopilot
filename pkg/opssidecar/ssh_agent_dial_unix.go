//go:build !windows

package opssidecar

import (
	"errors"
	"fmt"
	"io"
	"net"
	"os"
)

func dialLocalSSHAgent() (io.ReadWriteCloser, error) {
	sock := os.Getenv("SSH_AUTH_SOCK")
	if sock == "" {
		return nil, errors.New("未设置 SSH_AUTH_SOCK，无法连接 ssh-agent")
	}
	conn, err := net.Dial("unix", sock)
	if err != nil {
		return nil, fmt.Errorf("无法连接 ssh-agent: %w", err)
	}
	return conn, nil
}
