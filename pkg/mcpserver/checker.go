package mcpserver

import (
	"fmt"
	"regexp"
	"strings"
)

// allowedCommands 只读命令白名单（向后兼容：当没有配置文件时使用）
// 设计原则：只保留运维排查最常用的命令
var allowedCommands = []string{
	// === 文件查看 ===
	`^ls(\s|$)`,
	`^cat\s`,
	`^head\s`,
	`^tail\s`,
	`^find\s`,
	`^tree\s`,
	`^stat\s`,
	`^wc\s`,

	// === 文本处理 ===
	`^grep\s`,
	`^awk\s`,
	`^sed\s+-n`,
	`^sort\s`,
	`^uniq\s`,
	`^cut\s`,
	`^jq\s`,

	// === 进程 ===
	`^ps\s`,
	`^top(\s|$)`,
	`^pgrep\s`,
	`^pstree\s`,
	`^lsof\s`,

	// === 系统资源 ===
	`^free\s`,
	`^df\s`,
	`^du\s`,
	`^uptime(\s|$)`,
	`^iostat\s`,
	`^vmstat\s`,
	`^uname(\s|$)`,

	// === 网络 ===
	`^netstat\s`,
	`^ss\s`,
	`^ip\s`,
	`^ping\s`,
	`^curl\s+.*-I`,
	`^nslookup\s`,

	// === 服务/日志 ===
	`^systemctl\s+status`,
	`^journalctl\s`,
	`^dmesg(\s|$)`,

	// === 容器 ===
	`^docker\s+(ps|images|logs|inspect|stats)`,
	`^kubectl\s+(get|describe|logs|top)`,
	`^crictl\s+(ps|logs|pods)`,

	// === Java ===
	`^jps(\s|$)`,
	`^jstat\s`,
	`^jinfo\s`,
	`^jstack\s`,
	`^jmap\s+-histo`,

	// === 通用 ===
	`^echo\s`,
	`^which\s`,
	`^hostname(\s|$)`,
	`^whoami(\s|$)`,
	`^id(\s|$)`,
	`^date(\s|$)`,
	`^env(\s|$)`,
	`^pwd(\s|$)`,
}

// SimpleCheckResult 简单检查结果（向后兼容）
type SimpleCheckResult struct {
	Allowed bool
	Reason  string
}

// CommandChecker 命令检查器（向后兼容的简单实现）
// 当 WhitelistManager 不可用时使用
type CommandChecker struct {
	allowedPatterns []*regexp.Regexp
}

// NewCommandChecker 创建命令检查器
func NewCommandChecker() *CommandChecker {
	c := &CommandChecker{}
	for _, pattern := range allowedCommands {
		compiled, err := regexp.Compile(pattern)
		if err != nil {
			fmt.Printf("[MCP] Warning: Invalid pattern %s: %v\n", pattern, err)
			continue
		}
		c.allowedPatterns = append(c.allowedPatterns, compiled)
	}
	return c
}

// Check 检查命令是否允许执行
func (c *CommandChecker) Check(command string) SimpleCheckResult {
	command = strings.TrimSpace(command)

	// 空命令
	if command == "" {
		return SimpleCheckResult{
			Allowed: false,
			Reason:  "命令不能为空",
		}
	}

	// 检查白名单
	for _, pattern := range c.allowedPatterns {
		if pattern.MatchString(command) {
			return SimpleCheckResult{
				Allowed: true,
				Reason:  "",
			}
		}
	}

	// 不在白名单
	return SimpleCheckResult{
		Allowed: false,
		Reason: fmt.Sprintf(
			"命令 '%s' 不在只读白名单中。\n\n"+
				"可用的命令：\n"+
				"- 文件：cat, head, tail, ls, find, grep, awk, jq\n"+
				"- 进程：ps, top, pgrep, pstree\n"+
				"- 资源：free, df, du, uptime, iostat, vmstat\n"+
				"- 网络：netstat, ss, ip, ping, curl -I\n"+
				"- 服务：systemctl status, journalctl, dmesg\n"+
				"- 容器：docker ps/logs, kubectl get/logs\n"+
				"- Java：jps, jstat, jstack, jmap -histo\n\n"+
				"如需其他命令，请联系管理员更新白名单。",
			command,
		),
	}
}
