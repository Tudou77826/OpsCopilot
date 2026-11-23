package opssidecar

// MCP stdio 代理（--mcp 模式）：被 Agent 按 ACP mcpServers 约定拉起，
// stdin/stdout 上跑按行分隔的 MCP JSON-RPC；每条消息经 {token,message}
// 信封转发给 service 的回环桥。
//
// 地址与 token 来源（按序）：
//  1. 环境变量 OPSCOPILOT_STDIO_BRIDGE_ADDRESS / _TOKEN（Desktop 经
//     mcpServers env 直传——主路径，双开各自独立）；
//  2. 数据目录发现文件（拉起兜底：service 空闲退出后，代理拉起一个
//     service 子进程并按新发现文件重连）。

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

const (
	proxySpawnWait = 15 * time.Second
	proxyRetryGap  = 500 * time.Millisecond
)

// RunMCPProxy 运行 stdio 代理主循环，返回进程退出码。
func RunMCPProxy(input io.Reader, output io.Writer, dataDir string) int {
	var writeMu sync.Mutex
	respondError := func(id json.RawMessage, message string) {
		if len(id) == 0 {
			return
		}
		payload, _ := json.Marshal(map[string]any{
			"jsonrpc": "2.0", "id": id,
			"error": map[string]any{"code": -32000, "message": message},
		})
		writeMu.Lock()
		defer writeMu.Unlock()
		_, _ = output.Write(append(payload, '\n'))
	}

	resolve := func() (BridgeInfo, error) {
		// 主路径：env 直传
		if addr := os.Getenv("OPSCOPILOT_STDIO_BRIDGE_ADDRESS"); addr != "" {
			token := os.Getenv("OPSCOPILOT_STDIO_BRIDGE_TOKEN")
			if len(token) > 0 {
				return BridgeInfo{Address: addr, Token: token}, nil
			}
		}
		// 兜底：发现文件 + 必要时拉起 service
		info, err := loadBridgeInfo(dataDir)
		if err != nil || !bridgeAlive(info) {
			if err := spawnService(dataDir); err == nil {
				deadline := time.Now().Add(proxySpawnWait)
				for time.Now().Before(deadline) {
					if info, err = loadBridgeInfo(dataDir); err == nil && bridgeAlive(info) {
						return info, nil
					}
					time.Sleep(proxyRetryGap)
				}
			}
			return BridgeInfo{}, errors.New("运维服务未就绪")
		}
		return info, nil
	}

	forward := func(message []byte) ([]byte, error) {
		info, err := resolve()
		if err != nil {
			return nil, err
		}
		conn, err := net.DialTimeout("tcp4", info.Address, dialTimeout)
		if err != nil {
			// service 可能刚空闲退出：重解析（含拉起兜底）后重试一次
			if info, err = resolve(); err != nil {
				return nil, err
			}
			if conn, err = net.DialTimeout("tcp4", info.Address, dialTimeout); err != nil {
				return nil, errors.New("运维服务不可达")
			}
		}
		defer conn.Close()
		envelope, err := json.Marshal(bridgeRequest{Token: info.Token, Message: message})
		if err != nil {
			return nil, err
		}
		_ = conn.SetWriteDeadline(time.Now().Add(dialTimeout))
		if _, err := conn.Write(append(envelope, '\n')); err != nil {
			return nil, errors.New("运维服务写入失败")
		}
		_ = conn.SetReadDeadline(time.Now().Add(5 * time.Minute))
		var reply bridgeReply
		if err := json.NewDecoder(conn).Decode(&reply); err != nil {
			return nil, errors.New("运维服务应答失败")
		}
		if reply.Error != "" {
			return nil, errors.New(reply.Error)
		}
		return reply.Message, nil
	}

	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 0, 4096), mcpMaxMessage)
	for scanner.Scan() {
		message := strings.TrimSpace(scanner.Text())
		if message == "" {
			continue
		}
		if !json.Valid([]byte(message)) {
			fmt.Fprintf(os.Stderr, "[ops-sidecar] MCP 消息无效\n")
			return 1
		}
		var incoming struct {
			ID json.RawMessage `json:"id"`
		}
		_ = json.Unmarshal([]byte(message), &incoming)
		reply, err := forward([]byte(message))
		if err != nil {
			respondError(incoming.ID, err.Error())
			continue
		}
		if len(incoming.ID) == 0 || len(reply) == 0 {
			continue // 通知无应答
		}
		writeMu.Lock()
		_, _ = output.Write(append(reply, '\n'))
		writeMu.Unlock()
	}
	return 0
}

// spawnService 拉起一个 service 子进程（stdin 保持打开保活；退出即随代理回收）。
func spawnService(dataDir string) error {
	self, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.Command(self)
	cmd.Env = append(os.Environ(), "OPSCOPILOT_DATA_DIR="+dataDir)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		return err
	}
	go func() {
		_ = cmd.Wait()
		_ = stdin.Close()
	}()
	return nil
}
