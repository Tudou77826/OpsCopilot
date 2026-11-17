package opssidecar

// MCP stdio 代理（--mcp 模式）：被 Agent 按 ACP mcpServers 约定拉起，
// stdin/stdout 上跑按行分隔的 MCP JSON-RPC；每条消息经 {token,message}
// 信封转发给 service 的回环桥。凭据来自发现文件（数据目录内，0600），
// 不出现在命令行与日志。
//
// 生命周期：service 未运行或已空闲退出时，代理将其拉起（子进程，stdin
// 管道保持打开保活），实现「Agent 需要时零到一的懒启动」；代理退出
// （会话结束）回收自己拉起的子进程。地址与 token 都读自发现文件，
// service 重启（同端口、同持久化 token）不破坏代理。

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
	proxySpawnWait    = 15 * time.Second
	proxySpawnAttempt = 500 * time.Millisecond
)

// RunMCPProxy 运行 stdio 代理主循环。返回进程退出码。
func RunMCPProxy(input io.Reader, output io.Writer, dataDir string) int {
	self, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "[ops-sidecar] 无法定位自身可执行文件: %v\n", err)
		return 1
	}

	var childMu sync.Mutex
	var child *exec.Cmd
	spawnChild := func() {
		childMu.Lock()
		defer childMu.Unlock()
		if child != nil && child.Process != nil {
			return // 已拉起，等待端口就绪
		}
		cmd := exec.Command(self)
		cmd.Env = os.Environ()
		stdin, err := cmd.StdinPipe()
		if err != nil {
			return
		}
		// stdout/stderr 丢弃：service 的协议行属于它的客户端（Desktop），
		// 诊断走 stderr 落盘；子进程 stdout 不可混入本代理的输出。
		cmd.Stdout = io.Discard
		cmd.Stderr = io.Discard
		if err := cmd.Start(); err != nil {
			_ = stdin.Close()
			return
		}
		child = cmd
		// stdin 管道保持打开（EOF 会让 service 主循环退出）；子进程退出后
		// 由 Wait 回收并关闭管道
		go func() {
			_ = cmd.Wait()
			_ = stdin.Close()
		}()
	}
	killChild := func() {
		childMu.Lock()
		defer childMu.Unlock()
		if child != nil && child.Process != nil {
			_ = child.Process.Kill()
			child = nil
		}
	}

	// ensureAlive 确认 service 可连；不可连则拉起并轮询等待。
	ensureAlive := func(info BridgeInfo) bool {
		if bridgeAlive(info) {
			return true
		}
		spawnChild()
		deadline := time.Now().Add(proxySpawnWait)
		for time.Now().Before(deadline) {
			if bridgeAlive(info) {
				return true
			}
			time.Sleep(proxySpawnAttempt)
		}
		return false
	}

	var writeMu sync.Mutex
	respondError := func(id json.RawMessage, message string) {
		if len(id) == 0 {
			return
		}
		payload, _ := json.Marshal(map[string]any{
			"jsonrpc": "2.0",
			"id":      id,
			"error":   map[string]any{"code": -32000, "message": message},
		})
		writeMu.Lock()
		defer writeMu.Unlock()
		_, _ = output.Write(append(payload, '\n'))
	}

	forward := func(message []byte) ([]byte, bool, error) {
		var incoming struct {
			ID json.RawMessage `json:"id"`
		}
		_ = json.Unmarshal(message, &incoming)
		info, err := loadBridgeInfo(dataDir)
		if err != nil {
			return nil, false, errors.New("运维服务未就绪（发现文件缺失）")
		}
		if !ensureAlive(info) {
			return nil, false, errors.New("运维服务无法启动")
		}
		conn, err := net.DialTimeout("tcp4", info.address(), dialTimeout)
		if err != nil {
			// service 可能刚退出：再拉起一次后重试一
			if !ensureAlive(info) {
				return nil, false, errors.New("运维服务不可达")
			}
			conn, err = net.DialTimeout("tcp4", info.address(), dialTimeout)
			if err != nil {
				return nil, false, errors.New("运维服务不可达")
			}
		}
		defer conn.Close()
		envelope, err := json.Marshal(bridgeRequest{Token: info.Token, Message: message})
		if err != nil {
			return nil, false, err
		}
		_ = conn.SetWriteDeadline(time.Now().Add(dialTimeout))
		if _, err := conn.Write(append(envelope, '\n')); err != nil {
			return nil, false, errors.New("运维服务写入失败")
		}
		_ = conn.SetReadDeadline(time.Now().Add(5 * time.Minute))
		var reply bridgeReply
		if err := json.NewDecoder(conn).Decode(&reply); err != nil {
			return nil, false, errors.New("运维服务应答失败")
		}
		if reply.Error != "" {
			return nil, false, errors.New(reply.Error)
		}
		return reply.Message, len(incoming.ID) == 0, nil
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
		reply, notification, err := forward([]byte(message))
		if err != nil {
			respondError(incoming.ID, err.Error())
			continue
		}
		if notification || len(reply) == 0 {
			continue // 通知无应答
		}
		writeMu.Lock()
		_, _ = output.Write(append(reply, '\n'))
		writeMu.Unlock()
	}
	killChild()
	return 0
}
