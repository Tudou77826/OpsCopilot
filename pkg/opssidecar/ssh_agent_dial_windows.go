//go:build windows

package opssidecar

import (
	"fmt"
	"io"
	"os"
)

func dialLocalSSHAgent() (io.ReadWriteCloser, error) {
	pipe, err := os.OpenFile(`\\.\pipe\openssh-ssh-agent`, os.O_RDWR, 0)
	if err != nil {
		return nil, fmt.Errorf("无法连接 Windows ssh-agent（请确认 OpenSSH Authentication Agent 服务已启动）: %w", err)
	}
	return pipe, nil
}
