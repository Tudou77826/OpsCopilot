// +build windows

package mcp

import (
	"golang.org/x/sys/windows"
)

// setPlatformCmdAttr 设置 Windows 平台特定的进程属性
// 隐藏控制台窗口，避免出现黑屏
func (c *stdioClient) setPlatformCmdAttr() {
	c.cmd.SysProcAttr = &windows.SysProcAttr{
		CreationFlags: windows.CREATE_NO_WINDOW,
	}
}
