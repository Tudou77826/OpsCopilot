package mcpserver

// CommandCategory 命令分类
type CommandCategory string

const (
	CategoryReadOnly CommandCategory = "read_only" // 只读命令
	CategoryWrite    CommandCategory = "write"     // 写入命令
)

// WhitelistConfig 白名单配置
type WhitelistConfig struct {
	Version         string   `json:"version"`           // 配置版本
	LLMCheckEnabled bool     `json:"llm_check_enabled"` // 是否启用 LLM 风险检查
	Policies        []Policy `json:"policies"`          // 策略列表
}

// Policy 策略（按 IP 段区分）
type Policy struct {
	ID          string    `json:"id"`           // 策略 ID
	Name        string    `json:"name"`         // 策略名称
	Description string    `json:"description"`  // 策略描述
	IPRanges    []string  `json:"ip_ranges"`    // IP 段（CIDR 或 "*" 表示所有）
	Commands    []Command `json:"commands"`     // 命令规则
}

// Command 命令规则
type Command struct {
	Pattern     string          `json:"pattern"`     // 正则表达式
	Category    CommandCategory `json:"category"`    // 命令分类
	Description string          `json:"description"` // 命令描述
	Enabled     bool            `json:"enabled"`     // 是否启用
}

// CheckResult 检查结果
type CheckResult struct {
	Allowed    bool            `json:"allowed"`     // 是否允许
	Reason     string          `json:"reason"`      // 原因说明
	Category   CommandCategory `json:"category"`    // 命令分类
	PolicyName string          `json:"policy_name"` // 匹配的策略名称
}

// RiskLevel 风险等级
type RiskLevel string

const (
	RiskLow    RiskLevel = "low"    // 低风险
	RiskMedium RiskLevel = "medium" // 中风险
	RiskHigh   RiskLevel = "high"   // 高风险
)

// RiskAssessment LLM 风险评估结果
type RiskAssessment struct {
	IsRisky     bool      `json:"is_risky"`     // 是否有风险
	RiskLevel   RiskLevel `json:"risk_level"`   // 风险等级
	Reason      string    `json:"reason"`       // 原因说明（中文）
	Suggestions string    `json:"suggestions"`  // 安全建议（中文）
}

// DefaultWhitelistConfig 返回默认白名单配置
func DefaultWhitelistConfig() *WhitelistConfig {
	return &WhitelistConfig{
		Version:         "1.0",
		LLMCheckEnabled: true,
		Policies: []Policy{
			{
				ID:          "default",
				Name:        "默认策略",
				Description: "适用于所有服务器的只读命令策略",
				IPRanges:    []string{"*"},
				Commands:    getDefaultCommands(),
			},
		},
	}
}

// getDefaultCommands 返回默认命令列表（从现有的 allowedCommands 迁移）
func getDefaultCommands() []Command {
	return []Command{
		// === 文件查看 ===
		{Pattern: `^ls(\s|$)`, Category: CategoryReadOnly, Description: "列出目录", Enabled: true},
		{Pattern: `^cat\s`, Category: CategoryReadOnly, Description: "查看文件", Enabled: true},
		{Pattern: `^head\s`, Category: CategoryReadOnly, Description: "查看文件头部", Enabled: true},
		{Pattern: `^tail\s`, Category: CategoryReadOnly, Description: "查看文件尾部", Enabled: true},
		{Pattern: `^find\s`, Category: CategoryReadOnly, Description: "查找文件", Enabled: true},
		{Pattern: `^tree\s`, Category: CategoryReadOnly, Description: "目录树", Enabled: true},
		{Pattern: `^stat\s`, Category: CategoryReadOnly, Description: "文件状态", Enabled: true},
		{Pattern: `^wc\s`, Category: CategoryReadOnly, Description: "统计行数", Enabled: true},

		// === 文本处理 ===
		{Pattern: `^grep\s`, Category: CategoryReadOnly, Description: "搜索文本", Enabled: true},
		{Pattern: `^awk\s`, Category: CategoryReadOnly, Description: "文本处理", Enabled: true},
		{Pattern: `^sed\s+-n`, Category: CategoryReadOnly, Description: "流编辑（只读模式）", Enabled: true},
		{Pattern: `^sort\s`, Category: CategoryReadOnly, Description: "排序", Enabled: true},
		{Pattern: `^uniq\s`, Category: CategoryReadOnly, Description: "去重", Enabled: true},
		{Pattern: `^cut\s`, Category: CategoryReadOnly, Description: "剪切文本", Enabled: true},
		{Pattern: `^jq\s`, Category: CategoryReadOnly, Description: "JSON 处理", Enabled: true},

		// === 进程 ===
		{Pattern: `^ps\s`, Category: CategoryReadOnly, Description: "进程列表", Enabled: true},
		{Pattern: `^top(\s|$)`, Category: CategoryReadOnly, Description: "进程监控", Enabled: true},
		{Pattern: `^pgrep\s`, Category: CategoryReadOnly, Description: "进程搜索", Enabled: true},
		{Pattern: `^pstree\s`, Category: CategoryReadOnly, Description: "进程树", Enabled: true},
		{Pattern: `^lsof\s`, Category: CategoryReadOnly, Description: "打开文件列表", Enabled: true},

		// === 系统资源 ===
		{Pattern: `^free\s`, Category: CategoryReadOnly, Description: "内存信息", Enabled: true},
		{Pattern: `^df\s`, Category: CategoryReadOnly, Description: "磁盘使用", Enabled: true},
		{Pattern: `^du\s`, Category: CategoryReadOnly, Description: "目录大小", Enabled: true},
		{Pattern: `^uptime(\s|$)`, Category: CategoryReadOnly, Description: "系统运行时间", Enabled: true},
		{Pattern: `^iostat\s`, Category: CategoryReadOnly, Description: "IO 统计", Enabled: true},
		{Pattern: `^vmstat\s`, Category: CategoryReadOnly, Description: "虚拟内存统计", Enabled: true},
		{Pattern: `^uname(\s|$)`, Category: CategoryReadOnly, Description: "系统信息", Enabled: true},

		// === 网络 ===
		{Pattern: `^netstat\s`, Category: CategoryReadOnly, Description: "网络连接", Enabled: true},
		{Pattern: `^ss\s`, Category: CategoryReadOnly, Description: "Socket 统计", Enabled: true},
		{Pattern: `^ip\s`, Category: CategoryReadOnly, Description: "网络配置", Enabled: true},
		{Pattern: `^ping\s`, Category: CategoryReadOnly, Description: "网络连通性", Enabled: true},
		{Pattern: `^curl\s+.*-I`, Category: CategoryReadOnly, Description: "HTTP 头检查", Enabled: true},
		{Pattern: `^nslookup\s`, Category: CategoryReadOnly, Description: "DNS 查询", Enabled: true},

		// === 服务/日志 ===
		{Pattern: `^systemctl\s+status`, Category: CategoryReadOnly, Description: "服务状态", Enabled: true},
		{Pattern: `^journalctl\s`, Category: CategoryReadOnly, Description: "系统日志", Enabled: true},
		{Pattern: `^dmesg(\s|$)`, Category: CategoryReadOnly, Description: "内核日志", Enabled: true},

		// === 容器 ===
		{Pattern: `^docker\s+(ps|images|logs|inspect|stats)`, Category: CategoryReadOnly, Description: "Docker 查询", Enabled: true},
		{Pattern: `^kubectl\s+(get|describe|logs|top)`, Category: CategoryReadOnly, Description: "Kubernetes 查询", Enabled: true},
		{Pattern: `^crictl\s+(ps|logs|pods)`, Category: CategoryReadOnly, Description: "CRI 查询", Enabled: true},

		// === Java ===
		{Pattern: `^jps(\s|$)`, Category: CategoryReadOnly, Description: "Java 进程", Enabled: true},
		{Pattern: `^jstat\s`, Category: CategoryReadOnly, Description: "JVM 统计", Enabled: true},
		{Pattern: `^jinfo\s`, Category: CategoryReadOnly, Description: "JVM 配置", Enabled: true},
		{Pattern: `^jstack\s`, Category: CategoryReadOnly, Description: "Java 线程栈", Enabled: true},
		{Pattern: `^jmap\s+-histo`, Category: CategoryReadOnly, Description: "Java 堆统计", Enabled: true},

		// === 通用 ===
		{Pattern: `^echo\s`, Category: CategoryReadOnly, Description: "输出文本", Enabled: true},
		{Pattern: `^which\s`, Category: CategoryReadOnly, Description: "查找命令", Enabled: true},
		{Pattern: `^hostname(\s|$)`, Category: CategoryReadOnly, Description: "主机名", Enabled: true},
		{Pattern: `^whoami(\s|$)`, Category: CategoryReadOnly, Description: "当前用户", Enabled: true},
		{Pattern: `^id(\s|$)`, Category: CategoryReadOnly, Description: "用户 ID", Enabled: true},
		{Pattern: `^date(\s|$)`, Category: CategoryReadOnly, Description: "日期时间", Enabled: true},
		{Pattern: `^env(\s|$)`, Category: CategoryReadOnly, Description: "环境变量", Enabled: true},
		{Pattern: `^pwd(\s|$)`, Category: CategoryReadOnly, Description: "当前目录", Enabled: true},
	}
}
