package xshellimport

import (
	"strings"
	"testing"
	"unicode/utf16"

	"golang.org/x/text/encoding/simplifiedchinese"
	"opscopilot/pkg/remote"
)

// utf16leBOM 生成带 BOM 的 UTF-16LE 字节，与 Xshell 8 写出的 .xsh 一致。
func utf16leBOM(t *testing.T, s string) []byte {
	t.Helper()
	out := []byte{0xFF, 0xFE}
	for _, unit := range utf16.Encode([]rune(s)) {
		out = append(out, byte(unit), byte(unit>>8))
	}
	return out
}

func gbkBytes(t *testing.T, s string) []byte {
	t.Helper()
	encoded, err := simplifiedchinese.GBK.NewEncoder().Bytes([]byte(s))
	if err != nil {
		t.Fatalf("GBK 编码失败: %v", err)
	}
	return encoded
}

// ── 编码回退链 ──────────────────────────────────────────────

func TestDecodeText_UTF16LEWithBOM(t *testing.T) {
	raw := utf16leBOM(t, "[CONNECTION]\nHost=生产环境-01\n")
	got, enc := DecodeText(raw)
	if enc != EncodingUTF16LE {
		t.Errorf("应识别为 UTF-16LE，实际 %s", enc)
	}
	if !strings.Contains(got, "生产环境-01") {
		t.Errorf("中文内容解码错误: %q", got)
	}
}

func TestDecodeText_UTF16BEWithBOM(t *testing.T) {
	units := utf16.Encode([]rune("Host=路由器"))
	raw := []byte{0xFE, 0xFF}
	for _, unit := range units {
		raw = append(raw, byte(unit>>8), byte(unit))
	}
	got, enc := DecodeText(raw)
	if enc != EncodingUTF16BE {
		t.Errorf("应识别为 UTF-16BE，实际 %s", enc)
	}
	if !strings.Contains(got, "路由器") {
		t.Errorf("内容解码错误: %q", got)
	}
}

func TestDecodeText_GBKFallback(t *testing.T) {
	raw := gbkBytes(t, "[CONNECTION]\nHost=生产环境-02\n")
	got, enc := DecodeText(raw)
	if enc != EncodingGBK {
		t.Errorf("应回退到 GBK，实际 %s", enc)
	}
	if !strings.Contains(got, "生产环境-02") {
		t.Errorf("GBK 中文解码错误: %q", got)
	}
}

func TestDecodeText_UTF8Passthrough(t *testing.T) {
	raw := []byte("Host=web-1\n")
	got, enc := DecodeText(raw)
	if enc != EncodingUTF8 || !strings.Contains(got, "web-1") {
		t.Errorf("UTF-8 应原样通过，实际 %s %q", enc, got)
	}
}

// 非法字节不能让解码 panic 或返回空——一条会话因个别坏字节整条丢失是不可接受的。
func TestDecodeText_InvalidBytesDoNotPanic(t *testing.T) {
	raw := []byte{0x48, 0x6F, 0x73, 0x74, 0x3D, 0xFF, 0xFE, 0xFD, 0x80}
	if got, _ := DecodeText(raw); got == "" {
		t.Error("非法字节也应尽力返回内容")
	}
}

// ── INI 解析 ────────────────────────────────────────────────

func TestParseINI_SectionsWithColonAndCase(t *testing.T) {
	doc := ParseINI(strings.Join([]string{
		"[CONNECTION]",
		"Host=10.0.0.1",
		"[CONNECTION:AUTHENTICATION]",
		"username=root", // 键名小写，取值应大小写不敏感
		"UserName=root",
	}, "\r\n"))

	if got := doc.Get("CONNECTION", "Host"); got != "10.0.0.1" {
		t.Errorf("Host 取值错误: %q", got)
	}
	if got := doc.Get("connection:authentication", "USERNAME"); got != "root" {
		t.Errorf("带冒号的节名与键名应大小写不敏感，实际 %q", got)
	}
	if doc.HasSection("CONNECTION:PROXY") {
		t.Error("不存在的节不应报告存在")
	}
}

func TestParseINI_CommentsAndBlankLines(t *testing.T) {
	doc := ParseINI(strings.Join([]string{
		"; 这是注释",
		"# 这也是注释",
		"",
		"[CONNECTION]",
		"   ",
		"Host=10.0.0.1",
		";Host=ignored",
		"Port=2222",
	}, "\n"))

	if got := doc.Get("CONNECTION", "Host"); got != "10.0.0.1" {
		t.Errorf("注释行不应被当作键值，实际 %q", got)
	}
	if got := doc.Get("CONNECTION", "Port"); got != "2222" {
		t.Errorf("Port 取值错误: %q", got)
	}
}

func TestParseINI_DuplicateKeyLastWinsAndSplitOnFirstEquals(t *testing.T) {
	doc := ParseINI("[S]\nKey=a=b=c\nKey=second\n")
	if got := doc.Get("S", "Key"); got != "second" {
		t.Errorf("重复键应后者生效，实际 %q", got)
	}

	doc2 := ParseINI("[S]\nKey=a=b=c\n")
	if got := doc2.Get("S", "Key"); got != "a=b=c" {
		t.Errorf("应按第一个 = 切分，实际 %q", got)
	}
}

func TestParseINI_KeysOutsideSectionIgnored(t *testing.T) {
	doc := ParseINI("Orphan=1\n[S]\nReal=2\n")
	if got := doc.Get("", "Orphan"); got != "" {
		t.Errorf("节外的键不应被读取，实际 %q", got)
	}
	if got := doc.Get("S", "Real"); got != "2" {
		t.Errorf("正常键应可读，实际 %q", got)
	}
}

// ── .xsh 解析 ───────────────────────────────────────────────

// buildXSH 组装一个最小的 .xsh 文件内容（UTF-16LE，与 Xshell 8 一致）。
func buildXSH(t *testing.T, protocol, host, port, user, passwordB64, userKey, proxy string) []byte {
	t.Helper()
	var b strings.Builder
	b.WriteString("[SessionInfo]\nVersion=8.1\n")
	if proxy != "" {
		b.WriteString("[CONNECTION:PROXY]\nProxy=" + proxy + "\n")
	}
	b.WriteString("[CONNECTION]\n")
	b.WriteString("Protocol=" + protocol + "\n")
	b.WriteString("Host=" + host + "\n")
	b.WriteString("Port=" + port + "\n")
	b.WriteString("[CONNECTION:AUTHENTICATION]\n")
	b.WriteString("UserName=" + user + "\n")
	b.WriteString("Password=" + passwordB64 + "\n")
	b.WriteString("UserKey=" + userKey + "\n")
	return utf16leBOM(t, b.String())
}

func TestParseXSH_SSHWithDecryptablePassword(t *testing.T) {
	creds := testCredentials()
	cands := KeyCandidates(creds)
	encoded := encryptXshell(t, "s3cret", cands[0].Key)

	rec := ParseXSH(
		buildXSH(t, "SSH", "10.0.0.9", "2222", "root", encoded, "", ""),
		"生产-web-1", []string{"生产", "华东"},
		ParseOptions{Credentials: creds, DecryptPassword: true},
	)

	if rec.Protocol != remote.ProtocolSSH {
		t.Errorf("协议应为 ssh，实际 %q", rec.Protocol)
	}
	if rec.Host != "10.0.0.9" || rec.Port != 2222 || rec.User != "root" {
		t.Errorf("连接字段解析错误: %+v", rec)
	}
	if rec.Name != "生产-web-1" {
		t.Errorf("显示名应取文件名，实际 %q", rec.Name)
	}
	if strings.Join(rec.GroupPath, "/") != "生产/华东" {
		t.Errorf("分组层级错误: %v", rec.GroupPath)
	}
	if rec.Password != "s3cret" {
		t.Errorf("密码未解密，实际 %q", rec.Password)
	}
	if rec.PasswordRule == "" {
		t.Error("应记录命中的密钥规则以便排错")
	}
	if rec.Version != "8.1" {
		t.Errorf("版本解析错误: %q", rec.Version)
	}
}

func TestParseXSH_TelnetAndDefaultPort(t *testing.T) {
	rec := ParseXSH(
		buildXSH(t, "TELNET", "10.0.0.10", "", "admin", "", "", ""),
		"switch-1", nil,
		ParseOptions{},
	)
	if rec.Protocol != remote.ProtocolTelnet {
		t.Errorf("协议应为 telnet，实际 %q", rec.Protocol)
	}
	if rec.Port != 23 {
		t.Errorf("telnet 缺省端口应为 23，实际 %d", rec.Port)
	}
}

func TestParseXSH_UnsupportedProtocolIsMarkedNotImported(t *testing.T) {
	rec := ParseXSH(
		buildXSH(t, "RLOGIN", "10.0.0.11", "513", "ops", "", "", ""),
		"legacy-1", nil, ParseOptions{},
	)
	if rec.Protocol != "" {
		t.Errorf("不支持的协议应置空以标记跳过，实际 %q", rec.Protocol)
	}
	if !hasWarningContaining(rec.Warnings, "暂不支持") {
		t.Errorf("应给出不支持的警告，实际 %v", rec.Warnings)
	}
}

func TestParseXSH_MissingHostIsMarked(t *testing.T) {
	rec := ParseXSH(buildXSH(t, "SSH", "", "", "root", "", "", ""), "broken", nil, ParseOptions{})
	if rec.Host != "" {
		t.Fatalf("构造失败")
	}
	if !hasWarningContaining(rec.Warnings, "缺少主机地址") {
		t.Errorf("应给出缺少主机的警告，实际 %v", rec.Warnings)
	}
}

func TestParseXSH_UserKeyAndProxyProduceWarnings(t *testing.T) {
	rec := ParseXSH(
		buildXSH(t, "SSH", "10.0.0.12", "22", "root", "", `C:\keys\id_rsa.pri`, "corp-proxy"),
		"key-auth", nil, ParseOptions{},
	)
	if !hasWarningContaining(rec.Warnings, "密钥认证") {
		t.Errorf("密钥认证应告警，实际 %v", rec.Warnings)
	}
	if !hasWarningContaining(rec.Warnings, "代理/跳板机") {
		t.Errorf("代理/跳板应告警，实际 %v", rec.Warnings)
	}
}

// 密码解密失败时不能静默：必须保留会话并带上可解释的告警。
func TestParseXSH_UndecryptablePasswordWarnsButKeepsRecord(t *testing.T) {
	encoded := encryptXshell(t, "secret", sha256Sum("some-other-key-material"))
	rec := ParseXSH(
		buildXSH(t, "SSH", "10.0.0.13", "22", "root", encoded, "", ""),
		"foreign", nil,
		ParseOptions{Credentials: testCredentials(), DecryptPassword: true},
	)

	if !rec.PasswordEncrypted {
		t.Error("应标记原文件里存有密码")
	}
	if rec.Password != "" {
		t.Errorf("不应解出密码，实际 %q", rec.Password)
	}
	if rec.Host != "10.0.0.13" {
		t.Error("解密失败不得丢弃会话的非敏感字段")
	}
	if !hasWarningContaining(rec.Warnings, "无法解密") {
		t.Errorf("应给出可解释的解密失败告警，实际 %v", rec.Warnings)
	}
}

func TestParseXSH_NoPasswordStored(t *testing.T) {
	rec := ParseXSH(buildXSH(t, "SSH", "10.0.0.14", "22", "root", "", "", ""), "nopw", nil, ParseOptions{})
	if rec.PasswordEncrypted || rec.Password != "" {
		t.Errorf("未存密码的会话不应报告密码相关状态: %+v", rec)
	}
}

func TestParseXSH_DecryptDisabledLeavesPasswordEmpty(t *testing.T) {
	creds := testCredentials()
	encoded := encryptXshell(t, "s3cret", KeyCandidates(creds)[0].Key)
	rec := ParseXSH(
		buildXSH(t, "SSH", "10.0.0.15", "22", "root", encoded, "", ""),
		"keep-encrypted", nil,
		ParseOptions{Credentials: creds, DecryptPassword: false},
	)
	if !rec.PasswordEncrypted {
		t.Error("关闭解密时仍应标记文件里存有密码")
	}
	if rec.Password != "" {
		t.Errorf("关闭解密时不应尝试解密，实际 %q", rec.Password)
	}
}

func hasWarningContaining(warnings []string, substr string) bool {
	for _, w := range warnings {
		if strings.Contains(w, substr) {
			return true
		}
	}
	return false
}
