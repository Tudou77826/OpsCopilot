package opssidecar

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
)

// 服务发现文件：MCP 桥模式（--mcp）据此找到 service 进程的回环监听。
// token 首次生成后持久化（0600），service 重启不换——代理进程的凭据
// 跨重启有效；端口固定（可被 OPSCOPILOT_SIDECAR_PORT 覆盖），同一数据
// 目录同时只允许一个 service 实例（端口占用即互斥）。

const (
	bridgeFileName    = "ops-sidecar-service.json"
	defaultBridgePort = 47319
	bridgePortEnv     = "OPSCOPILOT_SIDECAR_PORT"
)

type BridgeInfo struct {
	Port  int    `json:"port"`
	Token string `json:"token"`
	PID   int    `json:"pid"`
}

func (b BridgeInfo) address() string {
	return net.JoinHostPort("127.0.0.1", strconv.Itoa(b.Port))
}

func bridgeFilePath(dataDir string) string {
	return filepath.Join(dataDir, bridgeFileName)
}

// resolveBridgePort 端口来源：环境变量 > 固定默认值。
func resolveBridgePort() int {
	if raw := os.Getenv(bridgePortEnv); raw != "" {
		if port, err := strconv.Atoi(raw); err == nil && port > 0 && port < 65536 {
			return port
		}
	}
	return defaultBridgePort
}

// loadBridgeInfo 读取发现文件；缺失或损坏返回错误。
func loadBridgeInfo(dataDir string) (BridgeInfo, error) {
	raw, err := os.ReadFile(bridgeFilePath(dataDir))
	if err != nil {
		return BridgeInfo{}, err
	}
	var info BridgeInfo
	if err := json.Unmarshal(raw, &info); err != nil {
		return BridgeInfo{}, err
	}
	if info.Port <= 0 || len(info.Token) != 64 {
		return BridgeInfo{}, fmt.Errorf("服务发现文件内容无效")
	}
	return info, nil
}

// writeBridgeInfo 落盘发现文件（数据目录内，与 sessions.json 同级；
// 目录权限沿用现有约定，不额外收紧）。
func writeBridgeInfo(dataDir string, info BridgeInfo) error {
	raw, err := json.Marshal(info)
	if err != nil {
		return err
	}
	return os.WriteFile(bridgeFilePath(dataDir), raw, 0o600)
}

// removeBridgeInfo service 退出时清理，避免陈旧 PID 误导代理。
func removeBridgeInfo(dataDir string) {
	_ = os.Remove(bridgeFilePath(dataDir))
}

// bridgeAlive 探测发现文件指向的 service 是否可连。
func bridgeAlive(info BridgeInfo) bool {
	conn, err := net.DialTimeout("tcp4", info.address(), dialTimeout)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

// ensureBridgeToken 读取或首次生成 64 位十六进制 token（持久化，跨重启稳定）。
func ensureBridgeToken(dataDir string) (string, error) {
	if existing, err := loadBridgeInfo(dataDir); err == nil {
		return existing.Token, nil
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return "", err
	}
	return hex.EncodeToString(key), nil
}
