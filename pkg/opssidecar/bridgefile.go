package opssidecar

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"
)

// 服务发现文件：仅作 --mcp 代理的「拉起兜底」——service 空闲退出后，代理
// 读该文件探测（不可连则拉起 service 子进程并重读）。主路径是 Desktop 经
// bridge/info 取凭据、env 直传（见 mcpbridge.go 头注），不经过本文件。
// 每次启动覆写：随机端口 + 新 token，双开实例各自一份（last-writer-wins，
// 同数据目录功能等价）。

const bridgeFileName = "ops-sidecar-service.json"

type BridgeInfo struct {
	Address string `json:"address"`
	Token   string `json:"token"`
	PID     int    `json:"pid"`
}

func bridgeFilePath(dataDir string) string { return filepath.Join(dataDir, bridgeFileName) }

func loadBridgeInfo(dataDir string) (BridgeInfo, error) {
	raw, err := os.ReadFile(bridgeFilePath(dataDir))
	if err != nil {
		return BridgeInfo{}, err
	}
	var info BridgeInfo
	if err := json.Unmarshal(raw, &info); err != nil {
		return BridgeInfo{}, err
	}
	if info.Address == "" || len(info.Token) != 64 {
		return BridgeInfo{}, fmt.Errorf("服务发现文件内容无效")
	}
	return info, nil
}

func writeBridgeInfo(dataDir string, info BridgeInfo) error {
	raw, err := json.Marshal(info)
	if err != nil {
		return err
	}
	return os.WriteFile(bridgeFilePath(dataDir), raw, 0o600)
}

func removeBridgeInfo(dataDir string) { _ = os.Remove(bridgeFilePath(dataDir)) }

// bridgeAlive 探测发现文件指向的 service 是否可连（拉起兜底用）。
func bridgeAlive(info BridgeInfo) bool {
	conn, err := net.DialTimeout("tcp4", info.Address, dialTimeout)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

// bridgeStalePID 发现文件里的进程已死（区分「service 没跑」与「文件陈旧」）。
func bridgeStalePID(info BridgeInfo) bool {
	if info.PID <= 0 {
		return true
	}
	proc, err := os.FindProcess(info.PID)
	if err != nil {
		return true
	}
	// Windows 上 FindProcess 总成功；用信号 0 探活
	return proc.Signal(os.Signal(nil)) != nil
}

var _ = time.Second
