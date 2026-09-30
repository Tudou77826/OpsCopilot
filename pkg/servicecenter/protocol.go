// Package servicecenter contains the intranet service protocol shared by desktop and server.
package servicecenter

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"
)

func reportingEnabled(choice string) bool { return choice == "standard" || choice == "minimal" }

const PolicyVersion = "3"
const MinimalPolicyVersion = "2"

func consentVersion(choice string) string {
	if choice == "minimal" {
		return MinimalPolicyVersion
	}
	return PolicyVersion
}

const Notice = "同意上报用于了解功能使用与升级质量：每日活跃标记、连接发起及成功次数、Ctrl+K 打开次数、快捷命令执行次数；脚本创建与执行、AI定位、归档、知识查询与打开、文件传输、命令生成与发送的次数及固定结果；CLI exec、file、diagnose、knowledge各入口的次数及结果、策略拒绝与连接恢复次数；随机安装标识、统计日期、客户端版本、系统类型与架构、说明版本及同意时间；升级当前/目标版本、阶段、结果、固定错误分类及关联编号。拒绝后可另行同意最简方案：仅每日活跃标记、客户端版本、随机安装标识、统计日期、记录编号、说明版本、同意时间及上报模式，不包含系统类型、架构、功能计数或升级结果。两种模式均不收集姓名、账号、主机名、服务器地址、连接信息、脚本名称或内容、文件名和路径、搜索词、命令名称或正文或参数、输入、会话、知识文档、密码、令牌、日志或原始错误。数据发送至内网服务器，不涉及任何形式的外发。实例记录保存90天，无安装标识的日汇总保存1年。设置页可随时关闭；关闭后停止采集和发送、清空本地队列，已送达数据按期限处理。共享知识库、共享连接信息由各自业务服务独立管理，不受此选择影响。"

type Policy struct {
	Version string `json:"version"`
	Notice  string `json:"notice"`
}

// ReleaseInfo keeps the GitHub-compatible wire shape while using mirrored asset URLs.
type ReleaseInfo struct {
	TagName     string    `json:"tag_name"`
	Name        string    `json:"name"`
	Body        string    `json:"body"`
	HTMLURL     string    `json:"html_url"`
	PublishedAt time.Time `json:"published_at"`
	Assets      []Asset   `json:"assets"`
}
type Asset struct {
	SHA256             string `json:"sha256,omitempty"`
	Digest             string `json:"digest,omitempty"`
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
}
type Announcement struct {
	ID       string    `json:"id"`
	Text     string    `json:"text"`
	URL      string    `json:"url"`
	Order    int       `json:"order"`
	StartsAt time.Time `json:"startsAt"`
	EndsAt   time.Time `json:"endsAt"`
}

func (a Announcement) Active(now time.Time) bool {
	return !now.Before(a.StartsAt) && now.Before(a.EndsAt)
}

// Record has no free-form payload or per-action timestamp.
type Record struct {
	ReportingMode  string         `json:"reportingMode,omitempty"`
	ID             string         `json:"id"`
	InstallationID string         `json:"installationId"`
	Date           string         `json:"date"`
	Version        string         `json:"version"`
	OS             string         `json:"os,omitempty"`
	Arch           string         `json:"arch,omitempty"`
	PolicyVersion  string         `json:"policyVersion"`
	ConsentAt      time.Time      `json:"consentAt"`
	Kind           string         `json:"kind"`
	Active         int            `json:"active"`
	Connections    int            `json:"connections,omitempty"`
	Connected      int            `json:"connected,omitempty"`
	CtrlK          int            `json:"ctrlK,omitempty"`
	QuickCommands  int            `json:"quickCommands,omitempty"`
	Usage          map[string]int `json:"usage,omitempty"`
	TargetVersion  string         `json:"targetVersion,omitempty"`
	Phase          string         `json:"phase,omitempty"`
	Result         string         `json:"result,omitempty"`
	ErrorCode      string         `json:"errorCode,omitempty"`
	UpgradeID      string         `json:"upgradeId,omitempty"`
}

var identifier = regexp.MustCompile(`^[a-zA-Z0-9._:-]{1,150}$`)
var version = regexp.MustCompile(`^[vV]?[0-9]+(\.[0-9]+){1,3}([+-][a-zA-Z0-9.-]+)?$|^dev$`)

func ValidVersion(v string) bool { return version.MatchString(v) }
func (r Record) Validate(now time.Time) error {
	day, err := time.Parse("2006-01-02", r.Date)
	if err != nil || day.Before(now.UTC().Truncate(24*time.Hour).AddDate(0, 0, -7)) || day.After(now.Add(24*time.Hour)) {
		return fmt.Errorf("invalid date")
	}
	if !identifier.MatchString(r.ID) || !identifier.MatchString(r.InstallationID) || !ValidVersion(r.Version) || r.PolicyVersion != consentVersion(r.ReportingMode) || r.ConsentAt.IsZero() || r.ConsentAt.After(now.Add(time.Minute)) || r.ConsentAt.After(day.Add(48*time.Hour)) {
		return fmt.Errorf("invalid authorization")
	}
	if !reportingEnabled(r.ReportingMode) {
		return fmt.Errorf("invalid reporting mode")
	}
	if r.ReportingMode == "minimal" {
		if len(r.Usage) != 0 || r.Kind != "daily" || r.OS != "" || r.Arch != "" || r.Connections+r.Connected+r.CtrlK+r.QuickCommands != 0 {
			return fmt.Errorf("fields exceed minimal consent")
		}
	} else if !oneOf(r.OS, "windows", "linux", "darwin") || !oneOf(r.Arch, "amd64", "arm64", "386", "arm") {
		return fmt.Errorf("invalid environment")
	}
	for _, n := range []int{r.Active, r.Connections, r.Connected, r.CtrlK, r.QuickCommands} {
		if n < 0 || n > 1000000 {
			return fmt.Errorf("invalid counter")
		}
	}
	for key, n := range r.Usage {
		if !ValidUsage(key) || n < 0 || n > 1000000 {
			return fmt.Errorf("invalid usage counter")
		}
	}
	if r.Kind == "daily" {
		if r.Active > 1 || r.Connected > r.Connections || r.ID != r.InstallationID+":"+r.Date+":"+r.Version || r.TargetVersion != "" || r.Phase != "" || r.Result != "" || r.ErrorCode != "" || r.UpgradeID != "" {
			return fmt.Errorf("invalid daily record")
		}
		return nil
	}
	if len(r.Usage) != 0 || r.Kind != "upgrade" || !ValidVersion(r.TargetVersion) || !identifier.MatchString(r.UpgradeID) || !oneOf(r.Phase, "check", "download", "install", "startup") || !oneOf(r.Result, "success", "failure", "no_update") || !oneOf(r.ErrorCode, "none", "network", "checksum", "install", "local") || r.ID != r.UpgradeID+":"+r.Phase || r.Active+r.Connections+r.Connected+r.CtrlK+r.QuickCommands != 0 {
		return fmt.Errorf("invalid upgrade record")
	}
	if (r.Result == "no_update" && r.Phase != "check") || (r.Result == "failure" && r.ErrorCode == "none") || (r.Result != "failure" && r.ErrorCode != "none") {
		return fmt.Errorf("invalid upgrade outcome")
	}
	return nil
}
func oneOf(v string, allowed ...string) bool {
	for _, s := range allowed {
		if v == s {
			return true
		}
	}
	return false
}
func ValidateBase(raw string) (string, error) {
	if raw == "" {
		return "", nil
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return "", fmt.Errorf("服务地址必须是 HTTP 或 HTTPS 根地址，例如 http://88.45.4.2:8090")
	}
	return u.Scheme + "://" + u.Host, nil
}
func InternalURL(base, raw string) bool {
	b, e := url.Parse(base)
	if e != nil {
		return false
	}
	u, e := url.Parse(raw)
	return e == nil && u.Scheme == b.Scheme && u.Host == b.Host && u.User == nil
}

// DownloadURL additionally disallows query parameters and fragments on artifact URLs.
func DownloadURL(base, raw string) bool {
	if !InternalURL(base, raw) {
		return false
	}
	u, e := url.Parse(raw)
	return e == nil && (u.Scheme == "https" || u.Scheme == "http") && strings.HasPrefix(u.Path, "/downloads/") && u.RawQuery == "" && u.Fragment == ""
}

// Fixed keys only. No operation names, arguments, paths or error text are accepted.
func ValidUsage(key string) bool {
	if oneOf(key, "gui_script_created", "gui_knowledge_opened", "gui_knowledge_search", "gui_knowledge_found", "gui_knowledge_empty", "gui_command_typed", "cli_policy_command", "cli_policy_target", "cli_policy_path", "cli_policy_size", "cli_recovery_started", "cli_recovery_success", "cli_recovery_failure") {
		return true
	}
	for _, prefix := range []string{"gui_script", "gui_diagnose", "gui_archive", "gui_upload", "gui_download", "gui_generate", "cli_exec", "cli_upload", "cli_download", "cli_diagnose", "cli_knowledge_list", "cli_knowledge_search", "cli_knowledge_read"} {
		for _, outcome := range []string{"started", "success", "failure", "cancelled"} {
			if key == prefix+"_"+outcome && !(strings.HasPrefix(prefix, "cli_") && outcome == "cancelled") {
				return true
			}
		}
	}
	return false
}
