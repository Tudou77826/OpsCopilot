package knowledge

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"opscopilot/pkg/recorder"
)

func TestExtractSection(t *testing.T) {
	md := `## 关键词
OOM, 内存不足, out of memory, Java堆内存, JVM

## 问题现象
Pod出现OOMKilled，容器频繁重启

## 根本原因
Java堆内存设置过小，导致OOM

## 排查路径
1. ` + "`kubectl describe pod`" + ` → 发现 OOMKilled
2. ` + "`jstat -gc`" + ` → 发现老年代已满`

	tests := []struct {
		heading string
		want    string
	}{
		{"关键词", "OOM, 内存不足, out of memory, Java堆内存, JVM"},
		{"问题现象", "Pod出现OOMKilled，容器频繁重启"},
		{"根本原因", "Java堆内存设置过小，导致OOM"},
		{"排查路径", "1. `kubectl describe pod` → 发现 OOMKilled\n2. `jstat -gc` → 发现老年代已满"},
		{"不存在", ""},
	}

	for _, tt := range tests {
		t.Run(tt.heading, func(t *testing.T) {
			got := extractSection(md, tt.heading)
			if got != tt.want {
				t.Errorf("extractSection(%q) =\n%q\nwant:\n%q", tt.heading, got, tt.want)
			}
		})
	}
}

func TestExtractSectionEmpty(t *testing.T) {
	got := extractSection("", "关键词")
	if got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestExtractKeywords(t *testing.T) {
	md := `## 关键词
OOM, 内存不足, out of memory, JVM`
	keywords := extractKeywords(md)
	if len(keywords) != 4 {
		t.Fatalf("expected 4 keywords, got %d: %v", len(keywords), keywords)
	}
	if keywords[0] != "OOM" {
		t.Errorf("first keyword = %q, want OOM", keywords[0])
	}
}

func TestSanitizeTitle(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Pod出现OOMKilled，容器频繁重启", "Pod出现OOMKilled，容器频繁重启"},
		{"- **现象**: 支付接口返回504", "支付接口返回504"},
		{"", ""},
		{strings.Repeat("a", 100), strings.Repeat("a", 80)[:80]},
	}

	for _, tt := range tests {
		t.Run(tt.input[:min(len(tt.input), 20)], func(t *testing.T) {
			got := sanitizeTitle(tt.input)
			if got != tt.want {
				t.Errorf("sanitizeTitle(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func TestSanitizeFileName(t *testing.T) {
	tests := []struct {
		service string
		module  string
		want    string
	}{
		{"Payment Service", "核心支付模块", "Payment_Service_核心支付模块"},
		{"User Service", "Auth", "User_Service_Auth"},
		{"test/service", "a/b", "test_service_a_b"},
	}

	for _, tt := range tests {
		name := tt.service + "/" + tt.module
		t.Run(name, func(t *testing.T) {
			got := sanitizeFileName(tt.service, tt.module)
			if got != tt.want {
				t.Errorf("sanitizeFileName(%q, %q) = %q, want %q", tt.service, tt.module, got, tt.want)
			}
		})
	}
}

func TestAppendRecordToExistingFile(t *testing.T) {
	dir := t.TempDir()

	// 创建已有文件
	existingContent := "---\nservice: \"Payment Service\"\nmodule: \"核心支付模块\"\ntype: sop\n---\n\n# Payment Service - 核心支付模块 运维文档\n"
	existingFile := filepath.Join(dir, "payment_sop.md")
	os.WriteFile(existingFile, []byte(existingContent), 0644)

	session := &recorder.RecordingSession{
		ID:        "test-session-001",
		Problem:   "支付接口超时",
		StartTime: time.Now(),
	}

	conclusion := `## 关键词
504, timeout, Nginx, upstream, PHP-FPM

## 问题现象
支付接口返回 504 Gateway Timeout，客户端请求超时

## 涉及组件
Nginx, PHP-FPM, MySQL

## 栒本原因
MySQL 慢查询导致 PHP-FPM 阻塞，进程池耗尽

## 排查路径
1. ` + "`grep \"504\" /var/log/nginx/access.log`" + ` → 发现所有请求都超时
2. ` + "`curl http://localhost/status`" + ` → PHP-FPM 进程池 full

## 解决方案
优化慢查询 SQL，PHP-FPM 超时调至 120s`

	input := &ArchiveInput{
		Session:    session,
		Conclusion: conclusion,
		Service:    "Payment Service",
		Module:     "核心支付模块",
		FilePath:   "payment_sop.md",
	}

	relPath, err := AppendRecord(dir, input)
	if err != nil {
		t.Fatalf("AppendRecord error: %v", err)
	}

	if relPath != "payment_sop.md" {
		t.Errorf("relPath = %q, want payment_sop.md", relPath)
	}

	// 验证文件内容
	data, err := os.ReadFile(existingFile)
	if err != nil {
		t.Fatalf("read file error: %v", err)
	}

	content := string(data)

	// 应包含原有内容
	if !strings.Contains(content, "Payment Service - 核心支付模块 运维文档") {
		t.Error("existing content should be preserved")
	}

	// 应包含新增场景
	if !strings.Contains(content, "## 场景：") {
		t.Error("should contain scenario heading")
	}
	if !strings.Contains(content, "504, timeout") {
		t.Error("should contain keywords")
	}
	if !strings.Contains(content, "test-session-001") {
		t.Error("should contain session ID")
	}
}

func TestAppendRecordNewFile(t *testing.T) {
	dir := t.TempDir()

	session := &recorder.RecordingSession{
		ID:        "test-session-002",
		Problem:   "服务启动失败",
		StartTime: time.Now(),
	}

	conclusion := `## 关键词
启动失败, systemd, exit code 1

## 问题现象
服务启动后立即退出，systemd status 显示 exit code 1

## 根本原因
配置文件路径错误导致启动失败

## 排查路径
1. ` + "`systemctl status myservice`" + ` → exit code 1
2. ` + "`journalctl -u myservice`" + ` → 配置文件 not found`

	input := &ArchiveInput{
		Session:    session,
		Conclusion: conclusion,
		Service:    "Order Service",
		Module:     "订单处理",
		FilePath:   "", // 新文件
	}

	relPath, err := AppendRecord(dir, input)
	if err != nil {
		t.Fatalf("AppendRecord error: %v", err)
	}

	if relPath == "" {
		t.Fatal("relPath should not be empty")
	}

	// 验证文件存在
	fullPath := filepath.Join(dir, relPath)
	data, err := os.ReadFile(fullPath)
	if err != nil {
		t.Fatalf("read file error: %v", err)
	}

	content := string(data)

	// 应包含 front matter
	if !strings.Contains(content, "service: \"Order Service\"") {
		t.Error("should contain front matter service")
	}
	if !strings.Contains(content, "module: \"订单处理\"") {
		t.Error("should contain front matter module")
	}

	// 应包含场景
	if !strings.Contains(content, "## 场景：") {
		t.Error("should contain scenario heading")
	}
}

func TestAppendRecordNoService(t *testing.T) {
	dir := t.TempDir()
	input := &ArchiveInput{
		Session:    &recorder.RecordingSession{},
		Conclusion: "test",
		Service:    "",
		Module:     "test",
	}

	_, err := AppendRecord(dir, input)
	if err == nil {
		t.Fatal("expected error for empty service")
	}
}

func TestBuildArchiveRecord(t *testing.T) {
	session := &recorder.RecordingSession{
		ID:      "abc123",
		Problem: "支付接口超时",
	}

	conclusion := `## 关键词
504, timeout

## 问题现象
支付接口返回 504 Gateway Timeout，客户端请求超时
影响范围：所有支付请求

## 涉及组件
Nginx, PHP-FPM, MySQL

## 根本原因
MySQL 慢查询导致 PHP-FPM 阻塞，进程池耗尽

## 排查路径
1. ` + "`grep \"504\" /var/log/nginx/access.log`" + ` → 发现所有请求都超时
2. ` + "`curl http://localhost/status`" + ` → PHP-FPM 进程池 full
3. ` + "`mysqldumpslow -s t /var/log/mysql/slow.log`" + ` → 发现慢查询

## 解决方案
优化慢查询 SQL，PHP-FPM 超时调至 120s`

	input := &ArchiveInput{
		Session:    session,
		Conclusion: conclusion,
		Service:    "Payment",
		Module:     "Core",
	}

	record := BuildArchiveRecord(input, time.Date(2026, 5, 18, 10, 30, 0, 0, time.Local))

	if !strings.Contains(record, "## 场景：") {
		t.Error("record should contain scenario heading")
	}
	if !strings.Contains(record, "**现象**") {
		t.Error("record should contain phenomena bullet for indexer")
	}
	if !strings.Contains(record, "**关键词**") {
		t.Error("record should contain keywords bullet for indexer")
	}
	if !strings.Contains(record, "**涉及组件**") {
		t.Error("record should contain components bullet for indexer")
	}
	if !strings.Contains(record, "abc123") {
		t.Error("record should contain session ID")
	}

	// 关键验证：完整结论原文必须保留
	if !strings.Contains(record, "影响范围：所有支付请求") {
		t.Error("full conclusion body should be preserved — missing phenomena detail")
	}
	if !strings.Contains(record, "mysqldumpslow") {
		t.Error("full conclusion body should be preserved — missing diagnostic command")
	}
	if !strings.Contains(record, "优化慢查询 SQL，PHP-FPM 超时调至 120s") {
		t.Error("full conclusion body should be preserved — missing solution")
	}
	if !strings.Contains(record, "## 解决方案") {
		t.Error("full conclusion body should be preserved — missing solution heading")
	}
}

func TestSingleLine(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"hello\nworld", "hello world"},
		{"hello  world", "hello world"},
		{"  hello  ", "hello"},
		{"a   b", "a b"},
		{"hello\tworld", "hello world"},
	}

	for _, tt := range tests {
		got := singleLine(tt.input)
		if got != tt.want {
			t.Errorf("singleLine(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestAppendRecordPathTraversal(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "safe.md"), []byte("safe content"), 0644)

	tests := []struct {
		name     string
		filePath string
	}{
		{"parent traversal", "../../etc/passwd"},
		{"deep traversal", "sub/../../../etc/crontab"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := &ArchiveInput{
				Session:    &recorder.RecordingSession{ID: "test"},
				Conclusion: "## 问题现象\ntest",
				Service:    "Test",
				Module:     "Mod",
				FilePath:   tt.filePath,
			}

			_, err := AppendRecord(dir, input)
			if err == nil {
				t.Errorf("expected error for path traversal: %q", tt.filePath)
			}
		})
	}

	// Also test that a valid relative path within dir succeeds
	input := &ArchiveInput{
		Session:    &recorder.RecordingSession{ID: "test"},
		Conclusion: "## 问题现象\ntest",
		Service:    "Test",
		Module:     "Mod",
		FilePath:   "safe.md",
	}
	_, err := AppendRecord(dir, input)
	if err != nil {
		t.Errorf("valid file path should succeed: %v", err)
	}
}

func TestAppendRecordNilSession(t *testing.T) {
	dir := t.TempDir()
	input := &ArchiveInput{
		Session:    nil,
		Conclusion: "## 问题现象\ntest",
		Service:    "Test",
		Module:     "Mod",
		FilePath:   "",
	}

	relPath, err := AppendRecord(dir, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should create file with empty session ID
	fullPath := filepath.Join(dir, relPath)
	data, err := os.ReadFile(fullPath)
	if err != nil {
		t.Fatalf("read file error: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "会话ID: ") {
		t.Error("should contain session ID field")
	}
}

func TestSanitizeTitleChineseTruncation(t *testing.T) {
	// 80 runes, each Chinese char is 3 bytes
	longChinese := strings.Repeat("测", 100)
	got := sanitizeTitle(longChinese)
	runes := []rune(got)
	if len(runes) > 80 {
		t.Errorf("title should be at most 80 runes, got %d", len(runes))
	}
}

func TestValidatePathWithinDir(t *testing.T) {
	tests := []struct {
		name    string
		baseDir string
		target  string
		wantErr bool
	}{
		{"valid file", "/tmp/kb", "/tmp/kb/test.md", false},
		{"valid subdir", "/tmp/kb", "/tmp/kb/sub/test.md", false},
		{"traversal", "/tmp/kb", "/tmp/evil.md", true},
		{"parent traversal", "/tmp/kb", "/tmp/kb/../../etc/passwd", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePathWithinDir(tt.baseDir, tt.target)
			if (err != nil) != tt.wantErr {
				t.Errorf("validatePathWithinDir(%q, %q) error = %v, wantErr %v", tt.baseDir, tt.target, err, tt.wantErr)
			}
		})
	}
}
