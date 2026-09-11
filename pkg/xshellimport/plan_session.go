package xshellimport

import (
	"fmt"
	"sort"
	"strings"
)

// TreeWriter 是导入器写入会话树所需的最小能力，由宿主实现（桌面端用
// connectionstore 适配）。用端口而不是直接依赖 connectionstore，是为了让本包
// 保持可独立测试的纯叶子包，也便于 sidecar 等其它宿主复用同一套导入逻辑。
type TreeWriter interface {
	// EnsureFolderPath 按分组层级逐层查找或创建文件夹，返回最末一层文件夹 ID；
	// 空路径表示根，返回空 ID。
	EnsureFolderPath(names []string) (string, error)
	// HasEndpoint 判断会话树中是否已存在该端点（协议/主机/端口）。
	HasEndpoint(protocol, host string, port int) bool
	// AddConnection 在 parentID 下新建一条连接。
	AddConnection(record *SessionRecord, parentID string) error
}

// ConflictPolicy 决定遇到已存在端点时的行为。
type ConflictPolicy string

const (
	// ConflictSkip 跳过已存在的会话。这是当前唯一支持的策略：导入是"补齐"
	// 而不是"覆盖"，不碰用户已有会话的任何字段最安全。
	ConflictSkip ConflictPolicy = "skip"
)

// Analysis 是导入前的只读分析结果。
//
// 它存在的意义是让"密码解密"这件事在写入前就可见：如果解密失败，用户还有机会
// 补上源机器的 SID 或主密码再导入，而不是导完才发现密码是空的。
type Analysis struct {
	Total             int
	Supported         int // 协议受支持且主机非空，可导入
	Unsupported       int // 协议不支持或缺主机，将被跳过
	Existing          int // 树中已存在同端点，将被跳过
	WithPassword      int // 原文件里存了密码的条数
	PasswordDecrypted int
	PasswordFailed    int
	Groups            int // 涉及的不同分组路径数
	Protocols         map[string]int
	Warnings          []string
}

// CanDecryptAll 表示所有带密码的会话都已成功解密。
func (a *Analysis) CanDecryptAll() bool {
	return a.WithPassword > 0 && a.PasswordFailed == 0
}

// Analyze 统计导入将会产生的结果，不写入任何数据。
func Analyze(records []*SessionRecord, writer TreeWriter) *Analysis {
	a := &Analysis{Protocols: make(map[string]int)}
	groups := make(map[string]bool)

	for _, rec := range records {
		a.Total++
		a.Warnings = append(a.Warnings, rec.Warnings...)

		if rec.PasswordEncrypted {
			a.WithPassword++
			if rec.Password != "" {
				a.PasswordDecrypted++
			} else {
				a.PasswordFailed++
			}
		}

		if len(rec.GroupPath) > 0 {
			groups[strings.Join(rec.GroupPath, "/")] = true
		}

		if rec.Protocol == "" || rec.Host == "" {
			a.Unsupported++
			continue
		}
		a.Protocols[rec.Protocol]++
		if writer != nil && writer.HasEndpoint(rec.Protocol, rec.Host, rec.Port) {
			a.Existing++
			continue
		}
		a.Supported++
	}

	a.Groups = len(groups)
	return a
}

// Report 是导入结果。
type Report struct {
	Imported           int
	SkippedExisting    int
	SkippedUnsupported int
	PasswordDecrypted  int
	PasswordFailed     int
	Warnings           []string
}

// Apply 按计划把记录并入会话树。
//
// 当前只实现"跳过已存在端点"的策略，因此不会改动用户已有会话的任何字段。
// 同一次导入内出现的重复端点也会被去重（writer 的视图在写入过程中会变化，
// 但不能依赖这一点）。
func Apply(records []*SessionRecord, writer TreeWriter, policy ConflictPolicy) (*Report, error) {
	if writer == nil {
		return nil, fmt.Errorf("缺少会话树写入端口")
	}
	if policy != ConflictSkip {
		return nil, fmt.Errorf("不支持的冲突策略: %s", policy)
	}

	report := &Report{}
	added := make(map[string]bool)

	for _, rec := range records {
		report.Warnings = append(report.Warnings, rec.Warnings...)

		if rec.PasswordEncrypted {
			if rec.Password != "" {
				report.PasswordDecrypted++
			} else {
				report.PasswordFailed++
			}
		}

		if rec.Protocol == "" || rec.Host == "" {
			report.SkippedUnsupported++
			continue
		}

		key := endpointKey(rec.Protocol, rec.Host, rec.Port)
		if added[key] || writer.HasEndpoint(rec.Protocol, rec.Host, rec.Port) {
			report.SkippedExisting++
			continue
		}

		parentID, err := writer.EnsureFolderPath(rec.GroupPath)
		if err != nil {
			report.Warnings = append(report.Warnings, fmt.Sprintf("%s: 创建分组失败: %v", rec.Name, err))
			continue
		}
		if err := writer.AddConnection(rec, parentID); err != nil {
			report.Warnings = append(report.Warnings, fmt.Sprintf("%s: 写入失败: %v", rec.Name, err))
			continue
		}
		added[key] = true
		report.Imported++
	}

	sort.Strings(report.Warnings)
	return report, nil
}

func endpointKey(protocol, host string, port int) string {
	return fmt.Sprintf("%s|%s|%d", strings.ToLower(protocol), strings.ToLower(host), port)
}
