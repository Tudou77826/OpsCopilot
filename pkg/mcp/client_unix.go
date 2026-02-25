// +build !windows

package mcp

// setPlatformCmdAttr 设置 Unix 平台特定的进程属性
// 在 Unix 平台上不需要特殊处理
func (c *stdioClient) setPlatformCmdAttr() {
	// No-op on Unix platforms
}
