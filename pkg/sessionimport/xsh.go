package sessionimport

import (
	"fmt"
	"strconv"
	"strings"

	"opscopilot/pkg/remote"
)

// SessionRecord 是从一个 .xsh 解析出的会话。
type SessionRecord struct {
	// GroupPath 是分组层级（目录名，或 .xts 内的成员路径）；空表示落在根。
	GroupPath []string
	// Name 是会话显示名。Xshell 用文件名作为显示名，文件内没有独立的名称字段。
	Name string

	// Protocol 已归一化为 remote.ProtocolSSH / remote.ProtocolTelnet；
	// 不支持的协议会被置空，并带上警告，由导入阶段跳过。
	Protocol string
	Host     string
	Port     int
	User     string

	// Password 仅在解密成功时非空；PasswordEncrypted 表示原文件里确实存了密码。
	Password          string
	PasswordEncrypted bool
	PasswordRule      string

	// Version 取自 [SessionInfo] Version，如 "8.1"。
	Version string
	// SourceFile 是文件路径或 .xts 内的成员路径，用于报告与排错。
	SourceFile string

	Warnings []string
}

// ParseOptions 控制解析行为。
type ParseOptions struct {
	// Credentials 用于解密会话密码。
	Credentials Credentials
	// DecryptPassword 为 false 时只解析非敏感字段，密码留空。
	DecryptPassword bool
}

// ParseXSH 解析一个 .xsh 文件的内容。
//
// name 是会话显示名，groupPath 是它所在的分组层级（由目录结构或 .xts 成员路径决定）。
// 解析失败的单个字段只会产生警告，不会让整条会话作废——导入的原则是尽量把能用的
// 信息带过来，而不是因为一个未知字段丢弃一条连接。
func ParseXSH(content []byte, name string, groupPath []string, opts ParseOptions) *SessionRecord {
	text, _ := DecodeText(content)
	doc := ParseINI(text)

	rec := &SessionRecord{
		GroupPath:  groupPath,
		Name:       name,
		Version:    firstNonEmpty(doc.Get("SessionInfo", "Version"), doc.Get("CONNECTION", "Version")),
		Host:       doc.Get("CONNECTION", "Host"),
		User:       doc.Get("CONNECTION:AUTHENTICATION", "UserName"),
		SourceFile: strings.Join(append(append([]string{}, groupPath...), name+".xsh"), "/"),
	}

	rec.Protocol = normalizeProtocol(doc.Get("CONNECTION", "Protocol"), doc, rec)
	rec.Port = parsePort(doc.Get("CONNECTION", "Port"), rec.Protocol, rec)

	if rec.Host == "" {
		rec.Warnings = append(rec.Warnings, "缺少主机地址，已跳过")
	}
	if rec.Protocol == "" {
		proto := doc.Get("CONNECTION", "Protocol")
		if proto == "" {
			proto = "未知"
		}
		rec.Warnings = append(rec.Warnings, fmt.Sprintf("协议 %s 暂不支持（当前仅支持 SSH/Telnet），已跳过", proto))
	}

	parseAuthentication(doc, rec, opts)
	parseProxy(doc, rec)

	return rec
}

func parseAuthentication(doc *Document, rec *SessionRecord, opts ParseOptions) {
	encoded := doc.Get("CONNECTION:AUTHENTICATION", "Password")
	userKey := doc.Get("CONNECTION:AUTHENTICATION", "UserKey")

	if encoded != "" {
		rec.PasswordEncrypted = true
		if opts.DecryptPassword {
			result, ok := DecryptPassword(encoded, KeyCandidates(opts.Credentials))
			if ok {
				rec.Password = result.Plaintext
				rec.PasswordRule = result.Rule
			} else {
				rec.Warnings = append(rec.Warnings, "密码无法解密：需要导出该会话的 Windows 账户 SID 或 Xshell 主密码")
			}
		}
	}

	if userKey != "" {
		rec.Warnings = append(rec.Warnings, "该会话使用密钥认证（UserKey），OpsCopilot 当前仅支持密码认证，导入后需手工补充密码")
	}
	if rec.User == "" && rec.Protocol == remote.ProtocolSSH {
		rec.Warnings = append(rec.Warnings, "未设置用户名")
	}
}

// parseProxy 处理 Xshell 的代理/跳板引用。
//
// [CONNECTION:PROXY] Proxy=<名> 指向独立代理定义文件，该文件不一定随导出一起提供，
// 因此这里只告警不做映射——静默丢弃跳板配置会让用户以为连接是直连的。
func parseProxy(doc *Document, rec *SessionRecord) {
	if doc.Get("CONNECTION:PROXY", "Proxy") != "" {
		rec.Warnings = append(rec.Warnings, "该会话配置了代理/跳板机，OpsCopilot 暂不自动映射，导入后需手工配置")
	}
}

func normalizeProtocol(raw string, doc *Document, rec *SessionRecord) string {
	switch strings.ToUpper(strings.TrimSpace(raw)) {
	case "SSH", "":
		// 空值按 SSH 处理，与 remote.Dial 的归一化一致。但若存在 TELNET 节且没有
		// SSH 痕迹，更可能是 telnet 会话，交由下面的分支判断。
		if raw == "" && doc.HasSection("CONNECTION:TELNET") && !doc.HasSection("CONNECTION:SSH") {
			return remote.ProtocolTelnet
		}
		return remote.ProtocolSSH
	case "TELNET":
		return remote.ProtocolTelnet
	default:
		return ""
	}
}

func parsePort(raw, protocol string, rec *SessionRecord) int {
	if port, err := strconv.Atoi(strings.TrimSpace(raw)); err == nil && port > 0 && port <= 65535 {
		return port
	}
	if raw != "" {
		rec.Warnings = append(rec.Warnings, fmt.Sprintf("端口 %q 无法识别，已按协议默认端口处理", raw))
	}
	if protocol == remote.ProtocolTelnet {
		return 23
	}
	return 22
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
