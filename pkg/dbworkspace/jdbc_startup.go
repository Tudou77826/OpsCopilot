package dbworkspace

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"opscopilot/pkg/connectiondiag"
)

// Java can echo JAVA_TOOL_OPTIONS, including credentials, before Main redirects
// stderr. Keep only a bounded matching tail and expose fixed classifications.
type jdbcStartupDiagnostic struct {
	mu           sync.Mutex
	tail, reason string
}

func (s *jdbcStartupDiagnostic) Write(data []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	text := strings.ToLower(s.tail + string(data[:min(len(data), 4096)]))
	switch {
	case strings.Contains(text, "unrecognized vm option"), strings.Contains(text, "unrecognized option:"):
		s.reason = "Java 拒绝启动参数；请核对 JVM 参数环境变量及随包运行组件，不输出原始参数值。"
	case strings.Contains(text, "unsupportedclassversionerror"), strings.Contains(text, "unsupported major.minor version"):
		s.reason = "Java 报告辅助程序类版本不兼容；请核对完整 JDBC 运行组件。"
	case strings.Contains(text, "invalid maximum heap size"), strings.Contains(text, "invalid initial heap size"), strings.Contains(text, "could not reserve enough space"):
		s.reason = "Java 启动时内存配置或地址空间预留失败；请核对内存参数和本机资源。"
	case strings.Contains(text, "unable to access jarfile"):
		s.reason = "Java 无法访问辅助程序 JAR；请核对文件是否完整及访问权限。"
	case s.reason == "" && (strings.Contains(text, "could not create the java virtual machine") || strings.Contains(text, "error occurred during initialization of vm")):
		s.reason = "Java 虚拟机初始化失败；请核对随包组件和 JVM 参数环境变量。"
	}
	s.tail = text[max(0, len(text)-256):]
	return len(data), nil
}

func (s *jdbcStartupDiagnostic) summary() string {
	if s == nil {
		return ""
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.reason
}

func (p *jdbcProcess) runtimeFailure(ctx context.Context) error {
	select {
	case <-p.exited:
	case <-time.After(100 * time.Millisecond):
	}
	p.mu.Lock()
	code, observed := p.exitCode, p.exitObserved
	p.mu.Unlock()
	detail := p.startup.summary()
	if detail == "" {
		detail = "辅助进程未返回有效响应；原始标准错误可能包含敏感参数，未直接输出。"
	}
	connectiondiag.Fact(ctx, "失败阶段", "Java辅助进程启动／通信")
	connectiondiag.Fact(ctx, "辅助进程启动诊断", detail)
	if observed {
		connectiondiag.Fact(ctx, "辅助进程结束状态", fmt.Sprint(code)+"（仅为观察状态，不单独判定根因）")
	}
	connectiondiag.Event(ctx, "jdbc.runtime.failed", "%s", detail)
	return fmt.Errorf("JDBC 辅助进程启动或通信失败：%s", detail)
}
