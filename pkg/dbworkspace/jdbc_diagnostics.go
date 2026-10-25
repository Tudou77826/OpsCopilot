package dbworkspace

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"opscopilot/pkg/connectiondiag"
)

type jdbcRuntimeInfo struct {
	JavaVersion            string   `json:"javaVersion"`
	JavaRuntimeVersion     string   `json:"javaRuntimeVersion"`
	JavaVendor             string   `json:"javaVendor"`
	VMName                 string   `json:"vmName"`
	OSArch                 string   `json:"osArch"`
	MaxHeapBytes           int64    `json:"maxHeapBytes"`
	DirectMemoryLimitBytes int64    `json:"directMemoryLimitBytes"`
	DirectMemorySource     string   `json:"directMemorySource"`
	MemoryOptions          []string `json:"memoryOptions"`
	OptionEnvironment      []string `json:"optionEnvironment"`
	OtherOptionCount       int      `json:"otherOptionCount"`
}

type jdbcFailureInfo struct {
	Phase        string             `json:"phase"`
	Category     string             `json:"category"`
	DirectMemory *jdbcMemoryFailure `json:"directMemory,omitempty"`
}

type jdbcMemoryFailure struct {
	Requested int64 `json:"requested"`
	Allocated int64 `json:"allocated"`
	Limit     int64 `json:"limit"`
}

var memoryOptionPattern = regexp.MustCompile(`^(?:-Xm[sx]|-XX:(?:InitialHeapSize|MaxHeapSize|MaxDirectMemorySize)=)[0-9]{1,19}[kKmMgGtT]?$`)

func recordJDBCRuntime(ctx context.Context, info *jdbcRuntimeInfo) {
	if info == nil {
		return
	}
	for key, value := range map[string]string{
		"Java版本": info.JavaVersion, "Java构建": info.JavaRuntimeVersion,
		"Java厂商": info.JavaVendor, "Java虚拟机": info.VMName, "Java架构": info.OSArch,
	} {
		connectiondiag.Fact(ctx, key, value)
	}
	if info.MaxHeapBytes > 0 {
		connectiondiag.Fact(ctx, "实际Java堆上限", memorySize(info.MaxHeapBytes))
	}
	source := map[string]string{"vm-option": "JVM选项读取", "startup-argument": "从JVM接收的参数解析", "vm-default": "JVM自动决定，未确定数值", "unavailable": "未取得"}[info.DirectMemorySource]
	if source != "" {
		value := source
		if info.DirectMemoryLimitBytes > 0 {
			value = memorySize(info.DirectMemoryLimitBytes) + "；" + source
		}
		connectiondiag.Fact(ctx, "直接内存限额证据", value)
	}
	var options []string
	for _, value := range info.MemoryOptions {
		if len(options) < 16 && memoryOptionPattern.MatchString(value) {
			options = append(options, value)
		}
	}
	connectiondiag.Fact(ctx, "JVM接收的内存参数", strings.Join(options, " "))
	var environment []string
	for _, name := range []string{"JAVA_TOOL_OPTIONS", "JDK_JAVA_OPTIONS", "_JAVA_OPTIONS"} {
		for _, present := range info.OptionEnvironment {
			if present == name {
				environment = append(environment, name)
				break
			}
		}
	}
	value := "未发现"
	if len(environment) > 0 {
		value = strings.Join(environment, ", ") + "（仅记录存在，未输出值）"
	}
	connectiondiag.Fact(ctx, "JVM参数环境变量", value)
	connectiondiag.Event(ctx, "jdbc.runtime.effective", "Java实际堆上限=%s 直接内存限额来源=%s；内存参数=%s；其他JVM参数内容省略", memorySize(info.MaxHeapBytes), source, strings.Join(options, " "))
}

func recordJDBCFailure(ctx context.Context, info *jdbcFailureInfo) {
	if info == nil {
		return
	}
	for _, phase := range []string{"处理请求", "加载驱动类", "校验 URL 模板", "校验连接 URL", "建立连接／握手认证", "读取数据库版本", "执行 SQL"} {
		if info.Phase == phase {
			connectiondiag.Fact(ctx, "失败阶段", phase)
			break
		}
	}
	for _, category := range []string{"memory.direct", "memory.other", "driver.compatibility", "tls", "network.dns", "network.timeout", "network.refused", "authentication", "driver.other"} {
		if info.Category == category {
			connectiondiag.Fact(ctx, "异常分类", category)
			break
		}
	}
	if memory := info.DirectMemory; info.Category == "memory.direct" && memory != nil && memory.Requested > 0 && memory.Allocated >= 0 && memory.Limit > 0 {
		connectiondiag.Fact(ctx, "直接内存限额证据", memorySize(memory.Limit)+"；本次JVM异常报告")
		connectiondiag.Fact(ctx, "直接内存失败申请", fmt.Sprintf("申请 %s；此前已分配 %s", memorySize(memory.Requested), memorySize(memory.Allocated)))
	}
	connectiondiag.Event(ctx, "jdbc.failure.classified", "辅助程序已返回受限的失败阶段与异常分类；原始异常链见错误详情")
}

func memorySize(bytes int64) string {
	if bytes < 0 {
		return "未取得"
	}
	return fmt.Sprintf("%.2f MiB (%d bytes)", float64(bytes)/(1<<20), bytes)
}

// These hashes describe the files actually selected for this attempt. Reading
// diagnostics must not turn a readable/launchable component into a failed connection.
func recordJDBCFiles(ctx context.Context, java, helper string) {
	if connectiondiag.From(ctx) == nil {
		return
	}
	runtimeRoot := filepath.Dir(filepath.Dir(java))
	files := []struct {
		label, path string
		limit       int64
	}{
		{"Java启动器SHA256", java, 16 << 20}, {"辅助程序SHA256", helper, 64 << 20},
		{"Java发行信息SHA256", filepath.Join(runtimeRoot, "release"), 1 << 20},
	}
	native := filepath.Join(runtimeRoot, "lib", "server", "libjvm.so")
	if runtime.GOOS == "windows" {
		native = filepath.Join(runtimeRoot, "bin", "server", "jvm.dll")
	}
	if runtime.GOOS == "darwin" {
		native = filepath.Join(runtimeRoot, "lib", "server", "libjvm.dylib")
	}
	files = append(files, struct {
		label, path string
		limit       int64
	}{"JVM本体SHA256", native, 128 << 20})
	for _, file := range files {
		value, err := diagnosticFileHash(ctx, file.path, file.limit)
		if err != nil {
			value = "未取得（文件不可读、超过诊断上限或操作已取消）"
		}
		connectiondiag.Fact(ctx, file.label, value)
	}
}

func diagnosticFileHash(ctx context.Context, path string, limit int64) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() || info.Size() > limit {
		return "", fmt.Errorf("file exceeds diagnostic bound")
	}
	hash := sha256.New()
	buffer := make([]byte, 64<<10)
	var read int64
	for {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		n, readErr := f.Read(buffer)
		read += int64(n)
		if read > limit {
			return "", fmt.Errorf("file exceeds diagnostic bound")
		}
		_, _ = hash.Write(buffer[:n])
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return "", readErr
		}
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}
