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
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"opscopilot/pkg/connectiondiag"
)

const jdbcFrameLimit = 64 << 20

const jdbcDirectMemoryEnv = "OPSCOPILOT_JDBC_DIRECT_MEMORY_MB"

// A process-local diagnostic override, never an automatic retry or a global Java
// setting. Default launches preserve the original JVM automatic direct-memory
// policy; explicit comparisons retain the existing heap arguments and connection cap.
func jdbcJVMOptions() ([]string, error) {
	options := []string{"-Xms16m", "-Xmx96m"}
	if value := strings.TrimSpace(os.Getenv(jdbcDirectMemoryEnv)); value != "" {
		n, err := strconv.Atoi(value)
		if err != nil || n < 16 || n > 1024 {
			return nil, fmt.Errorf("%s 必须为 16–1024 之间的整数（MiB）；未启动 JDBC 组件", jdbcDirectMemoryEnv)
		}
		options = append(options, fmt.Sprintf("-XX:MaxDirectMemorySize=%dm", n))
	}
	return append(options, "-Dfile.encoding=UTF-8", "--add-exports=java.base/sun.nio.ch=ALL-UNNAMED"), nil
}

type jdbcReply struct {
	ID      uint64           `json:"id"`
	Data    json.RawMessage  `json:"data"`
	Error   string           `json:"error"`
	Fatal   bool             `json:"fatal,omitempty"`
	Events  []jdbcPhase      `json:"events,omitempty"`
	Runtime *jdbcRuntimeInfo `json:"runtime,omitempty"`
	Failure *jdbcFailureInfo `json:"failure,omitempty"`
}

type jdbcPhase struct {
	Stage     string `json:"stage"`
	ElapsedMS int64  `json:"elapsedMS"`
}
type jdbcProcess struct {
	cmd          *exec.Cmd
	input        io.WriteCloser
	output       io.ReadCloser
	write        sync.Mutex
	mu           sync.Mutex
	pending      map[uint64]chan jdbcReply
	next         atomic.Uint64
	done         chan struct{}
	exited       chan struct{}
	once         sync.Once
	release      func()
	startup      *jdbcStartupDiagnostic
	exitCode     int
	exitObserved bool
}

func (s *Service) startJDBC(ctx context.Context, d JDBCDriver) (*jdbcProcess, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	java, helper, err := s.jdbcRuntime()
	if err != nil {
		return nil, err
	}
	recordJDBCFiles(ctx, java, helper)
	options, err := jdbcJVMOptions()
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
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
	cmd := exec.Command(java, append(options, "-jar", helper)...)
	var memoryOptions []string
	for _, option := range options {
		if memoryOptionPattern.MatchString(option) {
			memoryOptions = append(memoryOptions, option)
		}
	}
	connectiondiag.Fact(ctx, "JVM请求的内存参数", strings.Join(memoryOptions, " "))
	var optionEnvironment []string
	for _, name := range []string{"JAVA_TOOL_OPTIONS", "JDK_JAVA_OPTIONS", "_JAVA_OPTIONS"} {
		if strings.TrimSpace(os.Getenv(name)) != "" {
			optionEnvironment = append(optionEnvironment, name)
		}
	}
	presence := "未发现"
	if len(optionEnvironment) > 0 {
		presence = strings.Join(optionEnvironment, ", ") + "（仅记录存在，未输出值）"
	}
	connectiondiag.Fact(ctx, "宿主JVM参数环境变量", presence)
	connectiondiag.Event(ctx, "jdbc.runtime.start", "Java程序=%s 辅助程序=%s JVM参数=%s；不使用系统Java和额外网络服务", java, helper, strings.Join(options, " "))
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
	startup := &jdbcStartupDiagnostic{}
	cmd.Stderr = startup
	if err = cmd.Start(); err != nil {
		input.Close()
		output.Close()
		release()
		return nil, fmt.Errorf("启动 JDBC 运行组件失败: %w", err)
	}
	p := &jdbcProcess{cmd: cmd, input: input, output: output, pending: map[uint64]chan jdbcReply{}, done: make(chan struct{}), exited: make(chan struct{}), release: release}
	p.startup = startup
	go p.read()
	go func() {
		_ = cmd.Wait()
		p.mu.Lock()
		p.exitCode, p.exitObserved = cmd.ProcessState.ExitCode(), true
		p.mu.Unlock()
		close(p.exited)
		p.close()
	}()
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
	if err := ctx.Err(); err != nil {
		return err
	}
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
		if err := ctx.Err(); err != nil {
			finished <- err
			return
		}
		err := binary.Write(p.input, binary.BigEndian, uint32(len(b)))
		if err == nil {
			_, err = p.input.Write(b)
		}
		finished <- err
	}()
	select {
	case err = <-finished:
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return p.runtimeFailure(ctx)
		}
	case <-ctx.Done():
		p.close()
		return ctx.Err()
	case <-p.done:
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return p.runtimeFailure(ctx)
	}
	select {
	case reply := <-ch:
		recordJDBCRuntime(ctx, reply.Runtime)
		recordJDBCFailure(ctx, reply.Failure)
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
			if reply.Fatal || strings.Contains(reply.Error, "OutOfMemoryError") {
				connectiondiag.Event(ctx, "jdbc.memory.failed", "JDBC 运行组件报告内存分配失败；保留原错误并回收该辅助进程，不自动扩大内存或重试")
				p.close()
			}
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
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return p.runtimeFailure(ctx)
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
	for index, j := range d.Jars {
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
		connectiondiag.Fact(ctx, fmt.Sprintf("驱动JAR.%d", index+1), j.Name+" SHA256="+j.SHA256)
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
	connectiondiag.Fact(ctx, "Java版本", handshake.JavaVersion)
	connectiondiag.Fact(ctx, "Java厂商", handshake.JavaVendor)
	connectiondiag.Fact(ctx, "Java架构", handshake.OSArch)
	if out != nil {
		return json.Unmarshal(reply, out)
	}
	return nil
}
