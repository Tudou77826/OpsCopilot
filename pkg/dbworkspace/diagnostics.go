package dbworkspace

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"opscopilot/pkg/connectiondiag"
)

type connectionDiagnostic struct {
	trace *connectiondiag.Trace
	id    string
}

func (s *Service) beginDiagnostic(ctx context.Context, operation string, p Profile, password string) (context.Context, connectionDiagnostic) {
	ctx, trace := connectiondiag.New(ctx)
	connectiondiag.AddSecrets(ctx, password, p.User)
	d := connectionDiagnostic{trace: trace, id: time.Now().UTC().Format("20060102T150405.000000000Z") + "-" + uuid.NewString()}
	connectiondiag.Event(ctx, "attempt.start", "编号=%s 操作=%s 应用版本=%s 平台=%s/%s Go=%s", d.id, operation, s.BuildVersion, runtime.GOOS, runtime.GOARCH, runtime.Version())
	connectiondiag.Event(ctx, "database.config", "引擎=%s 数据库目标=%s 数据库=%q TLS=%s 程序只读限制=%t SSH节点ID=%q 数据库账号不记录", p.Engine, net.JoinHostPort(p.Host, strconv.Itoa(p.Port)), p.Database, p.TLS, p.ReadOnly, p.GatewayID)
	mode := "直接连接"
	if p.GatewayID != "" {
		mode = "SSH 隧道"
	}
	for key, value := range map[string]string{"应用版本": s.BuildVersion, "宿主平台": runtime.GOOS + "/" + runtime.GOARCH, "连接方式": mode, "数据库目标": net.JoinHostPort(p.Host, strconv.Itoa(p.Port)), "数据库或模式": p.Database, "TLS设置": p.TLS} {
		connectiondiag.Fact(ctx, key, value)
	}
	connectiondiag.Event(ctx, "ssh.mode", "SSH通道策略=%s；默认先请求标准转发，仅在 SSH 明确拒绝时尝试同账号同目标的兼容通道", p.GatewayMode)
	connectiondiag.Event(ctx, "diagnostic.scope", "仅观察本次真实连接；不改地址、不切账号、不额外认证、不执行诊断 SQL 或远程探测命令。SSH 跳转原有的命令备用路径如被触发会单独记录。")
	return ctx, d
}

type diagnosticError struct {
	cause  error
	report string
}

func (e *diagnosticError) Error() string { return e.report }
func (e *diagnosticError) Unwrap() error { return e.cause }

func (s *Service) finishDiagnostic(ctx context.Context, d connectionDiagnostic, cause error) error {
	if cause == nil {
		connectiondiag.Event(ctx, "attempt.ok", "连接及版本读取完成；不代表已验证查询、事务或写入功能。")
	} else {
		connectiondiag.Event(ctx, "attempt.failed", "%v", cause)
	}
	// Freeze first so comparison fields, evidence and the timeline describe the
	// same attempt; late cleanup cannot turn an intentional close into a cause.
	timeline := d.trace.Finish()
	facts := d.trace.Facts()
	conclusion := ""
	switch {
	case d.trace.Has("database.version.ok"):
		conclusion += "\n已确认：数据库连接和版本读取均已成功。"
	case d.trace.Has("jdbc.driver.connected") || d.trace.Has("jdbc.open.ok"):
		conclusion += "\n已确认：JDBC driver.connect 已返回连接；后续错误需按元数据或版本读取阶段分析。"
	case d.trace.Has("ssh.database.command.ok"):
		conclusion += "\n已确认：最终 SSH 节点上的临时 Python 进程已确认数据库 TCP 连接；数据库握手、认证仍需结合 JDBC 阶段判断。"
	case d.trace.Has("ssh.database.command.failed"):
		conclusion += "\n已确认：兼容通道未完成就绪确认；以 python3/SSH exec 启动状态及远端 connect 阶段错误为准，标准转发尝试见时间线。"
	case d.trace.Has("ssh.database.channel.ok"):
		conclusion += "\n已确认：SSH 服务至少一次确认数据库 TCP 通道；数据库握手、认证或主动断开仍需结合 JDBC 阶段和字节统计。"
		if d.trace.Has("ssh.database.channel.rejected") || d.trace.Has("ssh.database.channel.failed") {
			conclusion += " 同次操作也有失败的通道请求，需按时间线分别判断，不能将其归为全部通道均失败。"
		}
	case d.trace.Has("ssh.database.channel.rejected"):
		conclusion += "\n已确认：最终 SSH 服务拒绝 direct-tcpip 请求，数据库通道未建立。待验证：具体服务端规则、账号/认证方式或目标限制；未读取服务端配置，不能断言某个配置项已关闭。"
	case d.trace.Has("ssh.database.channel.failed"):
		conclusion += "\n已确认：最终 SSH 到数据库的通道建立失败；以对应 SSH 原始错误及原因码为准。"
	default:
		conclusion += "\n当前证据不足以确认数据库目标 TCP 已建立；从时间线中最后成功的阶段继续定位。"
	}
	if cause == nil {
		conclusion += "\n本次连接成功。"
	}
	if d.trace.Has("jdbc.memory.failed") {
		conclusion += "\n已确认：本地 JDBC 运行组件报告内存分配失败，相关辅助进程已回收。尚未确认申请量是否合理，需对照成功客户端的驱动、URL、目标端口及 JVM／缓冲配置；不将其直接归因为数据库服务器内存不足。"
	}
	var original string
	if cause != nil {
		original = "错误详情：\n" + diagnosticText(d.trace.Redact(cause.Error()), 8<<10) + "\n\n"
	}
	status, phase := "失败", facts["失败阶段"]
	if cause == nil {
		status, phase = "成功", "数据库连接及版本读取完成"
	}
	if phase == "" {
		phase = "未取得辅助程序失败阶段；请结合最后完成事件和原始错误判断"
	}
	category := diagnosticCategory(facts["异常分类"])
	if value := facts["辅助进程启动诊断"]; value != "" {
		category = value
	}
	if cause == nil {
		category = "连接与版本读取成功"
	}
	report := "数据库连接诊断 v4\n编号：" + d.id + "\n结果：" + status + "\n失败或完成阶段：" + phase + "\n诊断摘要：" + category + "\n\n" +
		original + "环境与连接对照（成功／失败日志按同名字段比较）：\n" + diagnosticFacts(facts) +
		"\n证据判断：\n" + diagnosticText(strings.TrimSpace(conclusion), 1024) +
		"\n\n下一步核对：\n" + diagnosticNext(facts, d.trace, cause) +
		"\n\n时间线（Go 为事件观察时间；Java 阶段在 RPC 返回时回传，另附请求内耗时）：\n" + diagnosticText(timeline, 48<<10) + "\n"
	report = diagnosticText(report, connectiondiag.MaxReportBytes-1024)
	path, saveErr := s.saveDiagnostic(d.id, report)
	if saveErr != nil {
		report += "\n诊断文件保存失败（不改变连接结果）：" + d.trace.Redact(saveErr.Error())
	} else {
		report += "\n诊断日志：" + path
	}
	if cause == nil {
		return nil
	}
	return &diagnosticError{cause: cause, report: report}
}

func diagnosticCategory(category string) string {
	if value := map[string]string{
		"memory.direct":        "本地 JDBC 直接内存分配失败；申请量来源尚未确定，不能据此判断数据库服务器内存不足。",
		"memory.other":         "本地 JDBC 运行组件发生内存分配失败；操作不自动重试。",
		"driver.compatibility": "异常指向 Java／驱动模块、类版本或依赖兼容问题；详见原始异常链。",
		"network.dns":          "驱动报告数据库地址解析失败。", "network.timeout": "驱动报告连接或读取超时；具体阶段见原始异常链。",
		"network.refused": "驱动报告连接被拒绝；核对实际目标及监听端口。", "tls": "异常指向 TLS／证书验证阶段。",
		"authentication": "驱动报告认证错误；数据库账号和密码不记录。",
	}[category]; value != "" {
		return value
	}
	return "现有证据尚不能自动确定根因；优先查看失败阶段、原始异常链和调用位置。"
}

func diagnosticFacts(facts map[string]string) string {
	keys := []string{
		"应用版本", "宿主平台", "连接方式", "数据库目标", "SSH实际路径", "JDBC实际URL",
		"JDBC驱动类", "JDBC驱动版本", "JDBC方言", "JDBC连接属性策略", "数据库版本",
		"Java版本", "Java构建", "Java厂商", "Java虚拟机", "Java架构",
		"实际Java堆上限", "直接内存限额证据", "直接内存失败申请", "JVM请求的内存参数", "JVM接收的内存参数",
		"宿主JVM参数环境变量", "JVM参数环境变量", "辅助进程启动诊断", "辅助进程结束状态",
		"辅助程序SHA256", "Java启动器SHA256", "JVM本体SHA256", "Java发行信息SHA256", "TLS设置", "数据库或模式",
	}
	for i := 1; i <= 8; i++ {
		keys = append(keys, fmt.Sprintf("驱动JAR.%d", i))
	}
	var text strings.Builder
	for _, key := range keys {
		value := facts[key]
		if strings.HasPrefix(key, "驱动JAR.") && value == "" {
			continue
		}
		if value == "" {
			value = "未采集／未提供（不据此推断一致或不同）"
		}
		fmt.Fprintf(&text, "%s：%s\n", key, value)
	}
	return diagnosticText(text.String(), 4<<10)
}

func diagnosticNext(facts map[string]string, trace *connectiondiag.Trace, cause error) string {
	var result string
	switch {
	case cause == nil:
		result = "保留本报告，与失败环境的同名对照字段及实际连接路径比较。仅证明本次目标已连通，不代表其他目标或查询能力已验证。"
	case trace.Has("jdbc.memory.failed"):
		result = "先比较成功／失败环境的 JVM接收参数、实际堆上限、直接内存限额证据与环境变量存在情况；再对照驱动／辅助程序指纹、实际目标和分配调用位置。若堆栈指向报文读取，重点核对目标服务协议和版本；若指向初始化，核对驱动缓冲配置。提高上限后成功仅证明预算影响，不证明申请量合理。"
	case trace.Has("jdbc.runtime.failed"):
		result = "先核对辅助进程启动诊断、组件指纹和已发现的 JVM 参数环境变量。环境变量可能含密钥，仅在本机核对设置，不直接导出其原文；本报告只保留固定分类及内存参数。"
	case trace.Has("jdbc.driver.connected"):
		result = "连接已返回；优先查看后续元数据／版本读取的异常和调用位置，不把该错误误判为 SSH 无法连通。"
	case trace.Has("ssh.database.channel.rejected") && !trace.Has("ssh.database.channel.ok") && !trace.Has("ssh.database.command.ok"):
		result = "先对照最后 SSH 节点、身份、目标地址字符串和端口以及通道拒绝原因；尚未证明是哪条服务端策略生效。"
	case facts["异常分类"] == "driver.compatibility":
		result = "先对照 Java 构建、JVM参数、辅助程序指纹和每个依赖 JAR 的 SHA256；同名 JAR 或同一主版本号不能代替文件一致性。"
	default:
		result = "先比较实际目标、连接路径、驱动／辅助程序指纹及 Java 环境，再按原始异常链的失败阶段排查网络、TLS、认证或元数据。未读取服务器配置，不自动判定具体规则。"
	}
	return diagnosticText(result, 1024)
}

func diagnosticText(value string, limit int) string {
	value = strings.ToValidUTF8(value, "�")
	if len(value) <= limit {
		return value
	}
	marker := "\n[内容已截断]\n"
	end := max(0, limit-len(marker))
	for end > 0 && !utf8.RuneStart(value[end]) {
		end--
	}
	return value[:end] + marker
}

// Only files generated by this feature are rotated. Each attempt has an isolated
// file; concurrent connections never mix their routes, identities or credentials.
func (s *Service) saveDiagnostic(id, report string) (string, error) {
	dir, err := filepath.Abs(filepath.Join(filepath.Dir(s.path), "database-diagnostics"))
	if err != nil {
		return "", err
	}
	if err = os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	path := filepath.Join(dir, "connection-"+id+".log")
	if len(report) > connectiondiag.MaxReportBytes {
		return "", fmt.Errorf("诊断报告超过大小上限")
	}
	if err = os.WriteFile(path, []byte(report), 0600); err != nil {
		return "", err
	}
	entries, err := os.ReadDir(dir)
	if err == nil {
		var generated []string
		for _, entry := range entries {
			name := entry.Name()
			if entry.IsDir() || !strings.HasPrefix(name, "connection-") || !strings.HasSuffix(name, ".log") {
				continue
			}
			stem := strings.TrimSuffix(strings.TrimPrefix(name, "connection-"), ".log")
			if len(stem) < 38 || stem[len(stem)-37] != '-' {
				continue
			}
			if _, err := uuid.Parse(stem[len(stem)-36:]); err != nil {
				continue
			}
			if _, err := time.Parse("20060102T150405.000000000Z", stem[:len(stem)-37]); err == nil {
				generated = append(generated, name)
			}
		}
		sort.Strings(generated)
		for len(generated) > 20 {
			_ = os.Remove(filepath.Join(dir, generated[0]))
			generated = generated[1:]
		}
	}
	return path, nil
}
