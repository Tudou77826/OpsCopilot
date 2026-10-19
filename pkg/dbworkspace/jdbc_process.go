package dbworkspace

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"opscopilot/pkg/connectiondiag"
)

const jdbcFrameLimit = 64 << 20

type jdbcReply struct {
	ID     uint64          `json:"id"`
	Data   json.RawMessage `json:"data"`
	Error  string          `json:"error"`
	Events []jdbcPhase     `json:"events,omitempty"`
}

type jdbcPhase struct {
	Stage     string `json:"stage"`
	ElapsedMS int64  `json:"elapsedMS"`
}
type jdbcProcess struct {
	cmd     *exec.Cmd
	input   io.WriteCloser
	output  io.ReadCloser
	write   sync.Mutex
	mu      sync.Mutex
	pending map[uint64]chan jdbcReply
	next    atomic.Uint64
	done    chan struct{}
	exited  chan struct{}
	once    sync.Once
	release func()
}

func (s *Service) startJDBC(ctx context.Context, d JDBCDriver) (*jdbcProcess, error) {
	java, helper, err := s.jdbcRuntime()
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	if s.closed || s.jdbcProcesses >= 4 {
		s.mu.Unlock()
		return nil, fmt.Errorf("JDBC 最多同时打开 4 个连接；请先断开其他 JDBC 连接")
	}
	s.jdbcProcesses++
	s.mu.Unlock()
	release := func() { s.mu.Lock(); s.jdbcProcesses--; s.mu.Unlock() }
	// Legacy vendor drivers reflect on public NIO buffer APIs. Export only this
	// package to the isolated helper; do not open unrelated/private JDK members.
	cmd := exec.Command(java, "-Xms16m", "-Xmx96m", "-Dfile.encoding=UTF-8",
		"--add-exports=java.base/sun.nio.ch=ALL-UNNAMED", "-jar", helper)
	connectiondiag.Event(ctx, "jdbc.runtime.start", "Java程序=%s 辅助程序=%s JVM参数=-Xms16m -Xmx96m -Dfile.encoding=UTF-8 --add-exports=java.base/sun.nio.ch=ALL-UNNAMED；不使用系统Java和额外网络服务", java, helper)
	hideJDBCWindow(cmd)
	input, err := cmd.StdinPipe()
	if err != nil {
		release()
		return nil, err
	}
	output, err := cmd.StdoutPipe()
	if err != nil {
		input.Close()
		release()
		return nil, err
	}
	cmd.Stderr = io.Discard
	if err = cmd.Start(); err != nil {
		input.Close()
		output.Close()
		release()
		return nil, fmt.Errorf("启动 JDBC 运行组件失败: %w", err)
	}
	p := &jdbcProcess{cmd: cmd, input: input, output: output, pending: map[uint64]chan jdbcReply{}, done: make(chan struct{}), exited: make(chan struct{}), release: release}
	go p.read()
	go func() { _ = cmd.Wait(); close(p.exited); p.close() }()
	if err = ctx.Err(); err != nil {
		p.close()
		return nil, err
	}
	return p, nil
}
func (p *jdbcProcess) close() {
	p.once.Do(func() { close(p.done); p.input.Close(); p.output.Close(); _ = p.cmd.Process.Kill(); p.release() })
	select {
	case <-p.exited:
	case <-time.After(2 * time.Second):
	}
}
func (p *jdbcProcess) read() {
	defer p.close()
	r := bufio.NewReader(p.output)
	for {
		var size uint32
		if err := binary.Read(r, binary.BigEndian, &size); err != nil {
			return
		}
		if size == 0 || size > jdbcFrameLimit {
			return
		}
		b := make([]byte, int(size))
		if _, err := io.ReadFull(r, b); err != nil {
			return
		}
		var reply jdbcReply
		if json.Unmarshal(b, &reply) != nil {
			return
		}
		p.mu.Lock()
		ch := p.pending[reply.ID]
		p.mu.Unlock()
		if ch != nil {
			select {
			case ch <- reply:
			default:
			}
		}
	}
}
func (p *jdbcProcess) call(ctx context.Context, request map[string]any, out any) error {
	id := p.next.Add(1)
	request["id"] = id
	b, err := json.Marshal(request)
	if err != nil {
		return err
	}
	if len(b) > jdbcFrameLimit {
		return fmt.Errorf("JDBC 请求过大")
	}
	ch := make(chan jdbcReply, 1)
	p.mu.Lock()
	p.pending[id] = ch
	p.mu.Unlock()
	defer func() { p.mu.Lock(); delete(p.pending, id); p.mu.Unlock() }()
	// Bound pipe writes as well as response waits. A stuck helper is disposable.
	finished := make(chan error, 1)
	go func() {
		p.write.Lock()
		defer p.write.Unlock()
		err := binary.Write(p.input, binary.BigEndian, uint32(len(b)))
		if err == nil {
			_, err = p.input.Write(b)
		}
		finished <- err
	}()
	select {
	case err = <-finished:
		if err != nil {
			return fmt.Errorf("JDBC 通道已关闭")
		}
	case <-ctx.Done():
		p.close()
		return ctx.Err()
	case <-p.done:
		return fmt.Errorf("JDBC 辅助进程已退出，请重新连接")
	}
	select {
	case reply := <-ch:
		if op, _ := request["op"].(string); op == "open" || op == "load" || op == "version" {
			for _, event := range reply.Events {
				// Accept only fixed helper phase names; never forward arbitrary driver logs.
				switch event.Stage {
				case "driver.load", "driver.loaded", "url.check", "driver.connect", "driver.connected", "metadata.version", "metadata.version.ok":
					connectiondiag.Event(ctx, "jdbc."+event.Stage, "Java阶段=%s Java请求内耗时=%dms", event.Stage, event.ElapsedMS)
				}
			}
		}
		if reply.Error != "" {
			return fmt.Errorf("%s", reply.Error)
		}
		if out != nil {
			if err := json.Unmarshal(reply.Data, out); err != nil {
				return err
			}
			if result, ok := out.(*ResultSet); ok {
				return validateJDBCResult(*result)
			}
			return nil
		}
		return nil
	case <-ctx.Done():
		// Cancellation is read on a separate Java thread; never replay SQL.
		if request["op"] != "cancel" {
			cancelCtx, stop := context.WithTimeout(context.Background(), 500*time.Millisecond)
			_ = p.call(cancelCtx, map[string]any{"op": "cancel"}, nil)
			stop()
		}
		p.close()
		return ctx.Err()
	case <-p.done:
		return fmt.Errorf("JDBC 辅助进程已退出，请重新连接")
	}
}

func validateJDBCResult(r ResultSet) error {
	if len(r.Columns) > 2048 || len(r.Rows) > 10001 {
		return fmt.Errorf("JDBC 结果超出行列上限")
	}
	budget := 8 << 20
	for _, row := range r.Rows {
		if len(row) != len(r.Columns) {
			return fmt.Errorf("JDBC 返回的行列数量不一致")
		}
		budget -= len(row) * 32
		for _, cell := range row {
			if cell != nil {
				if len(*cell) > 1<<20 {
					return fmt.Errorf("JDBC 单元格超过大小上限")
				}
				budget -= len(*cell)
			}
		}
		if budget < 0 {
			return fmt.Errorf("JDBC 结果超过 8 MiB 上限")
		}
	}
	return nil
}
func (p *jdbcProcess) load(ctx context.Context, root string, d JDBCDriver, out any) error {
	paths := []string{}
	for _, j := range d.Jars {
		path := filepath.Join(root, d.ID, j.Name)
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		hash := sha256.New()
		n, err := io.Copy(hash, io.LimitReader(f, 64<<20+1))
		f.Close()
		if err != nil {
			return err
		}
		if n > 64<<20 || hex.EncodeToString(hash.Sum(nil)) != j.SHA256 {
			return fmt.Errorf("JDBC 驱动文件校验失败，请重新导入")
		}
		absolute, err := filepath.Abs(path)
		if err != nil {
			return err
		}
		paths = append(paths, absolute)
	}
	var reply json.RawMessage
	if err := p.call(ctx, map[string]any{"op": "load", "jars": paths, "className": d.ClassName, "dialect": d.Dialect, "checkURL": jdbcURL(d, Profile{Host: "127.0.0.1", Port: 1888, Database: "OPSCOPILOT_URL_CHECK"})}, &reply); err != nil {
		return err
	}
	var handshake struct {
		Protocol    int    `json:"protocol"`
		JavaVersion string `json:"javaVersion"`
		JavaVendor  string `json:"javaVendor"`
		OSArch      string `json:"osArch"`
	}
	if err := json.Unmarshal(reply, &handshake); err != nil {
		return err
	}
	if handshake.Protocol != 1 {
		return fmt.Errorf("JDBC 运行组件版本不兼容，请更新组件包")
	}
	connectiondiag.Event(ctx, "jdbc.runtime.info", "Java版本=%s 厂商=%s 架构=%s；字段为空表示辅助程序较旧，不据此推断运行环境", handshake.JavaVersion, handshake.JavaVendor, handshake.OSArch)
	if out != nil {
		return json.Unmarshal(reply, out)
	}
	return nil
}
