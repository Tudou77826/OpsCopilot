package main

import (
	"errors"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"opscopilot/pkg/connectionstore"
	"opscopilot/pkg/remote"
	"opscopilot/pkg/xshellimport"
)

// 本文件承载 Xshell 导入的 App 层门面。
//
// 与 app_sessionshare.go 一样是独立子文件：按架构治理约定，app.go 只保留生命周期
// 与依赖组装，导入这类新领域能力不往 app.go 主体堆。真正的解析与合并逻辑在
// pkg/xshellimport，本层只做 Wails 边界转换、弹原生对话框、读写会话树。

// ImportOptions 是导入入参（Wails 边界，驼峰命名）。
type ImportOptions struct {
	// DecryptPassword 关闭时只导入主机/端口/用户名等非敏感字段。
	DecryptPassword bool `json:"decryptPassword"`
	// SourceSID 用于跨机器导入：Xshell 的密码密钥依赖导出机器的 Windows SID，
	// 换机器后必须由用户提供源机器 SID 才可能解密。
	SourceSID string `json:"sourceSid,omitempty"`
	// MasterPassword 用于 Xshell 启用了主密码的情况。
	MasterPassword string `json:"masterPassword,omitempty"`
}

// XshellCredentialStatus 是"能否自动解密"的探测结果，用于在导入前给用户明确预期。
type XshellCredentialStatus struct {
	Available   bool   `json:"available"`
	MaskedSID   string `json:"maskedSid,omitempty"`
	WindowsUser string `json:"windowsUser,omitempty"`
	// Message 是面向用户的一句话说明（成功或失败原因）。
	Message string `json:"message"`
}

// XshellSessionDir 描述探测到的一个本机 Xshell 会话目录。
type XshellSessionDir struct {
	Path     string `json:"path"`
	Version  int    `json:"version"`
	Sessions int    `json:"sessions"`
}

// ImportAnalysis 是写入前的影响分析。
type ImportAnalysis struct {
	Total             int            `json:"total"`
	Supported         int            `json:"supported"`
	Unsupported       int            `json:"unsupported"`
	Existing          int            `json:"existing"`
	WithPassword      int            `json:"withPassword"`
	PasswordDecrypted int            `json:"passwordDecrypted"`
	PasswordFailed    int            `json:"passwordFailed"`
	Groups            int            `json:"groups"`
	Protocols         map[string]int `json:"protocols"`
	Warnings          []string       `json:"warnings"`
}

// ImportReport 是导入结果。
type ImportReport struct {
	Imported           int      `json:"imported"`
	SkippedExisting    int      `json:"skippedExisting"`
	SkippedUnsupported int      `json:"skippedUnsupported"`
	PasswordDecrypted  int      `json:"passwordDecrypted"`
	PasswordFailed     int      `json:"passwordFailed"`
	Warnings           []string `json:"warnings"`
}

// importTreeWriter 把 connectionstore 适配成 xshellimport 需要的写入端口。
// 适配层很薄，是为了让 xshellimport 不依赖 connectionstore 而能独立测试。
type importTreeWriter struct {
	store *connectionstore.Store
}

func (w importTreeWriter) EnsureFolderPath(names []string) (string, error) {
	if len(names) == 0 {
		return "", nil
	}
	// 分组段不会含 "/"：Windows 文件名不允许，ZIP 成员路径也按 "/" 切分过了。
	return w.store.EnsureFolderByNamePath(strings.Join(names, "/"))
}

func (w importTreeWriter) HasEndpoint(protocol, host string, port int) bool {
	return w.store.FindByEndpoint(protocol, host, port) != nil
}

func (w importTreeWriter) AddConnection(record *xshellimport.SessionRecord, parentID string) error {
	cfg := remote.ConnectConfig{
		Name:     record.Name,
		Protocol: record.Protocol,
		Host:     record.Host,
		Port:     record.Port,
		User:     record.User,
		Password: record.Password,
	}
	_, err := w.store.CreateConnection(cfg, parentID)
	return err
}

// GetXshellImportStatus 探测本机 Xshell 凭据，用于在导入界面上说明
// "密码将自动解密"还是"需要补充凭据"。
func (a *App) GetXshellImportStatus() XshellCredentialStatus {
	creds, err := xshellimport.LocalCredentials()
	if err != nil || creds.SID == "" {
		return XshellCredentialStatus{
			Message: "未能读取本机 Windows 账户标识，导入后需要手工填写密码",
		}
	}
	return XshellCredentialStatus{
		Available:   true,
		MaskedSID:   xshellimport.MaskSID(creds.SID),
		WindowsUser: creds.WindowsUser,
		Message:     "已检测到本机 Xshell 凭据，导入的密码将自动解密",
	}
}

// DetectXshellSessionDirs 探测本机 Xshell 的会话目录。
//
// 这是同机场景下最省事的导入入口：不需要用户去 Xshell 里做任何导出操作，
// 直接读会话目录即可（也绕开了 .xts 可能被主密码加密的问题）。
func (a *App) DetectXshellSessionDirs() ([]XshellSessionDir, error) {
	dirs := xshellimport.DiscoverXshellSessionDirs()
	out := make([]XshellSessionDir, 0, len(dirs))
	for _, dir := range dirs {
		out = append(out, XshellSessionDir{
			Path:     dir,
			Version:  xshellDirVersion(dir),
			Sessions: countXshFiles(dir),
		})
	}
	return out, nil
}

// SelectSessionImportFile 弹出系统文件选择框，返回选中的 .xts/.xsh；取消时返回空串。
func (a *App) SelectSessionImportFile() (string, error) {
	if a.ctx == nil {
		return "", nil
	}
	path, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "选择 Xshell 导出文件",
		Filters: []runtime.FileFilter{
			{DisplayName: "Xshell 备份包 (*.xts)", Pattern: "*.xts"},
			{DisplayName: "Xshell 会话文件 (*.xsh)", Pattern: "*.xsh"},
			{DisplayName: "所有文件", Pattern: "*.*"},
		},
	})
	if err != nil {
		return "", err
	}
	return path, nil
}

// SelectSessionImportDirectory 弹出系统目录选择框；取消时返回空串。
func (a *App) SelectSessionImportDirectory() (string, error) {
	if a.ctx == nil {
		return "", nil
	}
	path, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "选择 Xshell 会话目录",
	})
	if err != nil {
		return "", err
	}
	return path, nil
}

// AnalyzeXshellImport 只读解析并统计导入影响，不写入任何数据。
//
// 拆出这一步的目的就是密码：解密失败时用户还有机会补上源机器 SID 或主密码，
// 而不是导入完才发现密码是空的。
func (a *App) AnalyzeXshellImport(path string, opts ImportOptions) (*ImportAnalysis, error) {
	records, parseWarnings, err := a.parseXshellImport(path, opts)
	if err != nil {
		return nil, err
	}

	analysis := xshellimport.Analyze(records, importTreeWriter{a.savedSessionMgr})

	// 契约：集合字段一律给空集合而不是 nil。Go 的 nil slice/map 会序列化成 null，
	// 前端按数组/对象处理就会抛异常，而 React 的渲染异常会把整棵组件树卸载成黑屏。
	// 前端另有容错，但边界本身不该产出这种形状。
	protocols := analysis.Protocols
	if protocols == nil {
		protocols = map[string]int{}
	}

	return &ImportAnalysis{
		Total:             analysis.Total,
		Supported:         analysis.Supported,
		Unsupported:       analysis.Unsupported,
		Existing:          analysis.Existing,
		WithPassword:      analysis.WithPassword,
		PasswordDecrypted: analysis.PasswordDecrypted,
		PasswordFailed:    analysis.PasswordFailed,
		Groups:            analysis.Groups,
		Protocols:         protocols,
		Warnings:          mergeWarnings(analysis.Warnings, parseWarnings),
	}, nil
}

// ApplyXshellImport 解析并并入会话树。
//
// 冲突策略固定为"跳过已存在端点"：导入是补齐而不是覆盖，绝不动用户已有会话的字段。
func (a *App) ApplyXshellImport(path string, opts ImportOptions) (*ImportReport, error) {
	records, parseWarnings, err := a.parseXshellImport(path, opts)
	if err != nil {
		return nil, err
	}

	report, err := xshellimport.Apply(records, importTreeWriter{a.savedSessionMgr}, xshellimport.ConflictSkip)
	if err != nil {
		return nil, err
	}

	return &ImportReport{
		Imported:           report.Imported,
		SkippedExisting:    report.SkippedExisting,
		SkippedUnsupported: report.SkippedUnsupported,
		PasswordDecrypted:  report.PasswordDecrypted,
		PasswordFailed:     report.PasswordFailed,
		Warnings:           mergeWarnings(report.Warnings, parseWarnings),
	}, nil
}

func (a *App) parseXshellImport(path string, opts ImportOptions) ([]*xshellimport.SessionRecord, []string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, nil, errors.New("请先选择要导入的 Xshell 导出文件或会话目录")
	}

	records, warnings, err := xshellimport.ParsePath(path, xshellimport.ParseOptions{
		Credentials:     resolveImportCredentials(opts),
		DecryptPassword: opts.DecryptPassword,
	})
	if err != nil {
		return nil, nil, describeImportError(err)
	}
	return records, warnings, nil
}

// resolveImportCredentials 组装解密凭据：默认用本机凭据，用户显式提供的覆盖之。
func resolveImportCredentials(opts ImportOptions) xshellimport.Credentials {
	creds, err := xshellimport.LocalCredentials()
	if err != nil {
		creds = xshellimport.Credentials{}
	}
	if opts.SourceSID != "" {
		creds.SID = strings.TrimSpace(opts.SourceSID)
	}
	if opts.MasterPassword != "" {
		creds.MasterPassword = opts.MasterPassword
	}
	return creds
}

// describeImportError 把底层错误翻译成用户能照着做的提示。
func describeImportError(err error) error {
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return errors.New("路径不存在，请重新选择 Xshell 导出文件或会话目录")
	case errors.Is(err, fs.ErrInvalid):
		return errors.New("不支持的文件类型，请选择 .xsh、.xts 文件或 Xshell 会话目录")
	}
	return err
}

// mergeWarnings 合并去重并排序。始终返回非 nil 切片——见 AnalyzeXshellImport 里的契约说明。
func mergeWarnings(groups ...[]string) []string {
	seen := make(map[string]bool)
	out := []string{}
	for _, group := range groups {
		for _, w := range group {
			w = strings.TrimSpace(w)
			if w == "" || seen[w] {
				continue
			}
			seen[w] = true
			out = append(out, w)
		}
	}
	sort.Strings(out)
	return out
}

func countXshFiles(dir string) int {
	count := 0
	_ = filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.EqualFold(filepath.Ext(d.Name()), ".xsh") {
			count++
		}
		return nil
	})
	return count
}

// xshellDirVersion 从 .../NetSarang Computer/<版本>/Xshell/Sessions 取出 <版本>。
func xshellDirVersion(dir string) int {
	versionDir := filepath.Base(filepath.Dir(filepath.Dir(dir)))
	n := 0
	for _, r := range versionDir {
		if r < '0' || r > '9' {
			return 0
		}
		n = n*10 + int(r-'0')
	}
	return n
}
