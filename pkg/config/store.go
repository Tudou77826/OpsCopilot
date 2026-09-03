package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"opscopilot/pkg/filetxn"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type AppConfig struct {
	LLM                  LLMConfig          `json:"llm"`
	Log                  LogConfig          `json:"log"`
	Docs                 DocsConfig         `json:"docs"`
	Scripts              ScriptsConfig      `json:"scripts"`
	CLI                  CLIConfig          `json:"cli"`
	QuickCommands        []QuickCommand     `json:"quick_commands"`
	CompletionDelay      int                `json:"completion_delay"`
	CommandQueryShortcut string             `json:"command_query_shortcut"`
	Experimental         ExperimentalConfig `json:"experimental"`
	Terminal             TerminalConfig     `json:"terminal"`
	Appearance           AppearanceConfig   `json:"appearance"`
	HighlightRules       []HighlightRule    `json:"highlight_rules"`
	PatchStore           PatchStoreConfig   `json:"patch_store"`
	SessionShare         SessionShareConfig `json:"session_share"`
}

const DefaultCLIExecTimeoutSec = 120

const (
	DefaultTerminalFontFamily = "JetBrains Mono"
	DefaultTerminalFontSize   = 14
	MinTerminalFontSize       = 10
	MaxTerminalFontSize       = 32
)

// AppearanceConfig 外观模式配置（亮色/暗色主题）
type AppearanceConfig struct {
	Theme string `json:"theme"` // "dark"（默认）或 "light"
}

const (
	DefaultAppearanceTheme = "dark"
	ThemeDark              = "dark"
	ThemeLight             = "light"
)

// NormalizeAppearanceConfig 归一化外观配置：仅接受 dark/light，否则回退默认 dark
func NormalizeAppearanceConfig(cfg AppearanceConfig) AppearanceConfig {
	switch strings.ToLower(strings.TrimSpace(cfg.Theme)) {
	case ThemeLight:
		cfg.Theme = ThemeLight
	case ThemeDark:
		cfg.Theme = ThemeDark
	default:
		cfg.Theme = DefaultAppearanceTheme
	}
	return cfg
}

type CLIConfig struct {
	ExecTimeoutSec int `json:"exec_timeout_sec"`
}

// PatchStoreConfig 补丁存储配置
type PatchStoreConfig struct {
	Enabled   bool   `json:"enabled"`
	Type      string `json:"type"`       // "git"（未来: "http", "sftp"）
	RemoteURL string `json:"remote_url"` // Git 仓库地址
	Branch    string `json:"branch"`     // 分支名，默认 "main"
}

// SessionShareConfig 会话连接信息共享配置。
// 密钥用于加密仓库中的密码（AES-256-GCM + scrypt），
// 团队成员配置相同仓库地址/分支/密钥即可互看共享会话。
type SessionShareConfig struct {
	Enabled   bool   `json:"enabled"`
	RemoteURL string `json:"remote_url"` // Git 仓库地址
	Branch    string `json:"branch"`     // 分支名，默认 "main"
	SecretKey string `json:"secret_key"` // 团队共享加密密钥
}

// ExperimentalConfig 实验性功能配置（保留结构以便未来扩展）
type ExperimentalConfig struct {
}

type TerminalConfig struct {
	Scrollback       int    `json:"scrollback"`
	SearchEnabled    bool   `json:"search_enabled"`
	HighlightEnabled bool   `json:"highlight_enabled"`
	FontFamily       string `json:"font_family"`
	FontSize         int    `json:"font_size"`
}

func NormalizeTerminalConfig(cfg TerminalConfig) TerminalConfig {
	if cfg.Scrollback <= 0 {
		cfg.Scrollback = 5000
	}
	if strings.TrimSpace(cfg.FontFamily) == "" {
		cfg.FontFamily = DefaultTerminalFontFamily
	} else {
		cfg.FontFamily = strings.TrimSpace(cfg.FontFamily)
	}
	switch cfg.FontFamily {
	case "JetBrains Mono", "Fira Code", "Source Code Pro", "IBM Plex Mono", "Inconsolata":
	default:
		cfg.FontFamily = DefaultTerminalFontFamily
	}
	if cfg.FontSize <= 0 {
		cfg.FontSize = DefaultTerminalFontSize
	} else if cfg.FontSize < MinTerminalFontSize {
		cfg.FontSize = MinTerminalFontSize
	}
	if cfg.FontSize > MaxTerminalFontSize {
		cfg.FontSize = MaxTerminalFontSize
	}
	return cfg
}

type HighlightRule struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	Pattern   string         `json:"pattern"`
	IsEnabled bool           `json:"is_enabled"`
	Priority  int            `json:"priority"`
	Style     HighlightStyle `json:"style"`
}

type HighlightStyle struct {
	BackgroundColor string  `json:"background_color,omitempty"`
	Color           string  `json:"color,omitempty"`
	FontWeight      string  `json:"font_weight,omitempty"`
	TextDecoration  string  `json:"text_decoration,omitempty"`
	Opacity         float64 `json:"opacity,omitempty"`
}

type QuickCommand struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Content string `json:"content"`
	Group   string `json:"group,omitempty"` // 所属分组，默认为 "default"
}

type LogConfig struct {
	Dir   string `json:"dir"`
	Level string `json:"level,omitempty"` // debug, info, warn, error
}

type DocsConfig struct {
	Dir string `json:"dir"`
}

type ScriptsConfig struct {
	Dir string `json:"dir"`
}

type Manager struct {
	configPath         string
	quickCommandsPath  string
	highlightRulesPath string
	sessionsPath       string
	Config             *AppConfig
	lastImportMessage  string
	importing          atomic.Bool
	configBase         []byte
	quickBase          []byte
	highlightBase      []byte
	readOnly           bool // 只读模式：Load 时不自动创建文件
	// quickCmdMu 保护快捷命令的读改写；多窗口（多进程）场景下
	// 各进程通过文件变化检测热加载，操作均为单条意图化增删改。
	quickCmdMu sync.Mutex
	// quickCmdMod/quickCmdSize 记录上次同步过的文件状态，用于热加载变化检测
	quickCmdMod  time.Time
	quickCmdSize int64
}

func NewManager() *Manager {
	return newManagerWithDir("")
}

// NewManagerWithDir 创建基于指定目录的配置管理器
// 所有配置文件路径都相对于 dir 解析
func NewManagerWithDir(dir string) *Manager {
	return newManagerWithDir(dir)
}

// SetReadOnly 设置只读模式，Load 时不会自动创建文件
func (m *Manager) SetReadOnly(ro bool) {
	m.readOnly = ro
}

func newManagerWithDir(dir string) *Manager {
	// 默认配置
	defaultLLM := LoadLLMConfig()

	var defaultLogDir string
	if dir != "" {
		defaultLogDir = filepath.Join(dir, "logs")
	} else {
		cwd, _ := os.Getwd()
		defaultLogDir = filepath.Join(cwd, "logs")
	}

	cfg := &AppConfig{
		LLM: *defaultLLM,
		Log: LogConfig{
			Dir: defaultLogDir,
		},
		Docs: DocsConfig{
			Dir: "", // Default to empty, will be resolved dynamically if empty
		},
		Scripts: ScriptsConfig{
			Dir: "", // Default to empty, resolved to {logDir}/../scripts at runtime
		},
		CLI: CLIConfig{
			ExecTimeoutSec: DefaultCLIExecTimeoutSec,
		},
		QuickCommands:        []QuickCommand{},
		CompletionDelay:      150, // Default 150ms
		CommandQueryShortcut: "Ctrl+K",
		Experimental:         ExperimentalConfig{},
		Terminal: TerminalConfig{
			Scrollback:       5000,
			SearchEnabled:    true,
			HighlightEnabled: true,
			FontFamily:       DefaultTerminalFontFamily,
			FontSize:         DefaultTerminalFontSize,
		},
		Appearance: AppearanceConfig{
			Theme: DefaultAppearanceTheme,
		},
		HighlightRules: []HighlightRule{},
	}

	// 如果指定了目录，所有路径基于该目录；否则基于当前目录
	resolvePath := func(name string) string {
		if dir != "" {
			return filepath.Join(dir, name)
		}
		return name
	}

	return &Manager{
		configPath:         resolvePath("config.json"),
		quickCommandsPath:  resolvePath("quick_commands.json"),
		highlightRulesPath: resolvePath("highlight_rules.json"),
		sessionsPath:       resolvePath("sessions.json"),
		Config:             cfg,
	}
}

// Load reads config from file, creating it with defaults if not exists
func (m *Manager) Load() error {
	// 加载主配置文件
	data, err := os.ReadFile(m.configPath)
	if os.IsNotExist(err) {
		if err := m.loadQuickCommands(); err != nil {
			return err
		}
		if err := m.loadHighlightRules(); err != nil {
			return err
		}
		if m.readOnly {
			// 只读模式下不创建文件，使用内存中的默认值
			return nil
		}
		// 文件不存在，保存默认配置
		return m.Save()
	}
	if err != nil {
		return err
	}

	m.configBase = append([]byte(nil), data...)
	var raw map[string]any
	_ = json.Unmarshal(data, &raw)
	llmRaw, _ := raw["llm"].(map[string]any)
	_, hasFastModel := llmRaw["FastModel"]
	_, hasComplexModel := llmRaw["ComplexModel"]
	_, hasOldModel := llmRaw["Model"]
	_, hasCLI := raw["cli"]
	_, hasCommandQueryShortcut := raw["command_query_shortcut"]
	_, hasTerminal := raw["terminal"]
	_, hasAppearance := raw["appearance"]

	// 解析配置
	if err := json.Unmarshal(data, m.Config); err != nil {
		return err
	}

	changed := false
	if !hasFastModel && hasOldModel && m.Config.LLM.Model != "" {
		m.Config.LLM.FastModel = m.Config.LLM.Model
		changed = true
	}
	if !hasComplexModel {
		m.Config.LLM.ComplexModel = "glm46"
		changed = true
	}
	if hasOldModel && m.Config.LLM.Model != "" {
		m.Config.LLM.Model = ""
		changed = true
	}
	if !hasCLI || m.Config.CLI.ExecTimeoutSec <= 0 {
		m.Config.CLI.ExecTimeoutSec = DefaultCLIExecTimeoutSec
		changed = true
	}
	if !hasCommandQueryShortcut || m.Config.CommandQueryShortcut == "" {
		m.Config.CommandQueryShortcut = "Ctrl+K"
		changed = true
	}
	if !hasTerminal {
		m.Config.Terminal = TerminalConfig{
			Scrollback:       5000,
			SearchEnabled:    true,
			HighlightEnabled: true,
			FontFamily:       DefaultTerminalFontFamily,
			FontSize:         DefaultTerminalFontSize,
		}
		changed = true
	}
	normalizedTerminal := NormalizeTerminalConfig(m.Config.Terminal)
	if normalizedTerminal != m.Config.Terminal {
		m.Config.Terminal = normalizedTerminal
		changed = true
	}
	if !hasAppearance {
		m.Config.Appearance = AppearanceConfig{Theme: DefaultAppearanceTheme}
		changed = true
	}
	normalizedAppearance := NormalizeAppearanceConfig(m.Config.Appearance)
	if normalizedAppearance != m.Config.Appearance {
		m.Config.Appearance = normalizedAppearance
		changed = true
	}

	// 加载 quick_commands 配置
	if err := m.loadQuickCommands(); err != nil {
		return err
	}

	// 加载 highlight_rules 配置
	if err := m.loadHighlightRules(); err != nil {
		return err
	}

	// 只读模式下不加载辅助配置文件（prompts/quick_commands/highlight_rules）
	// 也不自动保存迁移变更
	if m.readOnly {
		return nil
	}

	if changed {
		if err := m.Save(); err != nil {
			return err
		}
	}

	return nil
}

// loadQuickCommands 从独立文件加载快捷命令配置
func (m *Manager) loadQuickCommands() error {
	m.quickCmdMu.Lock()
	defer m.quickCmdMu.Unlock()
	// 读取 quick_commands.json 文件
	data, err := os.ReadFile(m.quickCommandsPath)
	if os.IsNotExist(err) {
		// 文件不存在，初始化为空数组并保存
		m.Config.QuickCommands = []QuickCommand{}
		if m.readOnly {
			return nil
		}
		return m.saveQuickCommands()
	}
	if err != nil {
		return err
	}

	m.quickBase = append([]byte(nil), data...)
	// 解析 JSON 到 QuickCommands
	if err := json.Unmarshal(data, &m.Config.QuickCommands); err != nil {
		return err
	}

	// 确保不为 nil
	if m.Config.QuickCommands == nil {
		m.Config.QuickCommands = []QuickCommand{}
	}

	// 迁移旧格式命令（没有 Group 字段的命令归入 "default" 组）
	if err := m.migrateQuickCommands(); err != nil {
		return err
	}

	m.recordQuickCmdStat()
	return nil
}

// migrateQuickCommands 迁移旧格式的快捷命令
func (m *Manager) migrateQuickCommands() error {
	needsMigration := false
	for _, cmd := range m.Config.QuickCommands {
		if cmd.Group == "" {
			needsMigration = true
			break
		}
	}

	if !needsMigration {
		return nil
	}

	// 为没有分组的命令设置默认分组
	for i := range m.Config.QuickCommands {
		if m.Config.QuickCommands[i].Group == "" {
			m.Config.QuickCommands[i].Group = "default"
		}
	}

	// 保存迁移后的数据
	if m.readOnly {
		return nil
	}
	return m.saveQuickCommands()
}

func (m *Manager) Save() error {
	// 保存主配置（不包含 prompts 和 quick_commands）
	type ConfigForSave struct {
		LLM                  LLMConfig          `json:"llm"`
		Log                  LogConfig          `json:"log"`
		Docs                 DocsConfig         `json:"docs"`
		Scripts              ScriptsConfig      `json:"scripts"`
		CLI                  CLIConfig          `json:"cli"`
		CompletionDelay      int                `json:"completion_delay"`
		CommandQueryShortcut string             `json:"command_query_shortcut"`
		Experimental         ExperimentalConfig `json:"experimental"`
		Terminal             TerminalConfig     `json:"terminal"`
		Appearance           AppearanceConfig   `json:"appearance"`
		PatchStore           PatchStoreConfig   `json:"patch_store"`
		SessionShare         SessionShareConfig `json:"session_share"`
	}

	cfg := ConfigForSave{
		LLM:                  m.Config.LLM,
		Log:                  m.Config.Log,
		Docs:                 m.Config.Docs,
		Scripts:              m.Config.Scripts,
		CLI:                  m.Config.CLI,
		CompletionDelay:      m.Config.CompletionDelay,
		CommandQueryShortcut: m.Config.CommandQueryShortcut,
		Experimental:         m.Config.Experimental,
		Terminal:             m.Config.Terminal,
		Appearance:           m.Config.Appearance,
		PatchStore:           m.Config.PatchStore,
		SessionShare:         m.Config.SessionShare,
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	// Clearing the known legacy Model field is an explicit migration, unlike
	// unknown keys which must survive a shared settings save.
	var oldFields, nextFields map[string]any
	_ = json.Unmarshal(m.configBase, &oldFields)
	if llm, ok := oldFields["llm"].(map[string]any); ok && llm["Model"] != nil && cfg.LLM.Model == "" {
		_ = json.Unmarshal(data, &nextFields)
		nextFields["llm"].(map[string]any)["Model"] = ""
		data, err = json.MarshalIndent(nextFields, "", "  ")
		if err != nil {
			return err
		}
	}
	// 内容未变化时跳过写盘，避免无谓刷新 mtime
	if old, rerr := os.ReadFile(m.configPath); rerr == nil && bytes.Equal(old, data) {
		return nil
	}
	merged, err := filetxn.Merge(m.configPath, m.configBase, data)
	if err != nil {
		return err
	}
	m.configBase = merged
	return json.Unmarshal(merged, m.Config)
}

func (m *Manager) LastImportMessage() string {
	return m.lastImportMessage
}

func (m *Manager) ImportFromDirectory(dirPath string) error {
	if !m.importing.CompareAndSwap(false, true) {
		m.lastImportMessage = "正在导入配置..."
		return fmt.Errorf("import in progress")
	}
	defer m.importing.Store(false)

	cleaned := filepath.Clean(strings.TrimSpace(dirPath))
	if cleaned == "" {
		m.lastImportMessage = "目录不存在"
		return fmt.Errorf("empty directory path")
	}

	st, err := os.Stat(cleaned)
	if err != nil {
		if os.IsNotExist(err) {
			m.lastImportMessage = "目录不存在"
		} else {
			m.lastImportMessage = fmt.Sprintf("读取目录失败: %v", err)
		}
		return err
	}
	if !st.IsDir() {
		m.lastImportMessage = "目录不存在"
		return fmt.Errorf("not a directory: %s", cleaned)
	}

	original, err := cloneAppConfig(m.Config)
	if err != nil {
		return err
	}
	updated, err := cloneAppConfig(m.Config)
	if err != nil {
		return err
	}

	var imported []string
	var warnings []string
	usedDefaults := false

	if data, err := os.ReadFile(filepath.Join(cleaned, "config.json")); err == nil {
		type oldConfig struct {
			LLM          LLMConfig          `json:"llm"`
			Log          LogConfig          `json:"log"`
			Docs         DocsConfig         `json:"docs"`
			Experimental ExperimentalConfig `json:"experimental"`
		}
		var old oldConfig
		if err := json.Unmarshal(data, &old); err != nil {
			warnings = append(warnings, "config.json 格式错误，已跳过")
		} else {
			oldLLM := old.LLM
			if oldLLM.APIKey != "" {
				updated.LLM.APIKey = oldLLM.APIKey
			}
			if oldLLM.BaseURL != "" {
				updated.LLM.BaseURL = oldLLM.BaseURL
			}
			fastModel := oldLLM.FastModel
			if fastModel == "" && oldLLM.Model != "" {
				fastModel = oldLLM.Model
			}
			if fastModel != "" {
				updated.LLM.FastModel = fastModel
			}
			if oldLLM.ComplexModel != "" {
				updated.LLM.ComplexModel = oldLLM.ComplexModel
			} else {
				usedDefaults = true
			}
			updated.LLM.Model = ""

			if old.Log.Dir != "" {
				updated.Log.Dir = old.Log.Dir
			}
			if old.Docs.Dir != "" {
				updated.Docs.Dir = old.Docs.Dir
			}

			imported = append(imported, "config.json")
		}
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}

	if data, err := os.ReadFile(filepath.Join(cleaned, "quick_commands.json")); err == nil {
		var old []QuickCommand
		if err := json.Unmarshal(data, &old); err != nil {
			warnings = append(warnings, "quick_commands.json 格式错误，已跳过")
		} else {
			if old == nil {
				old = []QuickCommand{}
			}
			updated.QuickCommands = old
			imported = append(imported, "quick_commands.json")
		}
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}

	if data, err := os.ReadFile(filepath.Join(cleaned, "highlight_rules.json")); err == nil {
		var old []HighlightRule
		if err := json.Unmarshal(data, &old); err != nil {
			warnings = append(warnings, "highlight_rules.json 格式错误，已跳过")
		} else {
			if old == nil {
				old = []HighlightRule{}
			}
			updated.HighlightRules = old
			imported = append(imported, "highlight_rules.json")
		}
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}

	if data, err := os.ReadFile(filepath.Join(cleaned, "sessions.json")); err == nil {
		if err := m.replaceImportedSessions(data); err != nil {
			warnings = append(warnings, "写入 sessions.json 失败: "+err.Error())
		} else {
			imported = append(imported, "sessions.json")
		}
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}

	if len(imported) == 0 {
		if len(warnings) > 0 {
			m.lastImportMessage = "未找到任何可用的配置文件，配置保持不变"
		} else {
			m.lastImportMessage = "未找到任何配置文件，配置保持不变"
		}
		return nil
	}

	if err := backupFileIfExists(m.configPath, m.configPath+".bak"); err != nil {
		return err
	}
	if err := backupFileIfExists(m.quickCommandsPath, m.quickCommandsPath+".bak"); err != nil {
		return err
	}
	if err := backupFileIfExists(m.highlightRulesPath, m.highlightRulesPath+".bak"); err != nil {
		return err
	}

	*m.Config = *updated

	if err := m.Save(); err != nil {
		*m.Config = *original
		return err
	}
	// Save() 不再捆绑写入独立文件，这里显式落盘导入的 quick_commands/highlight_rules
	m.quickCmdMu.Lock()
	if err := m.saveQuickCommands(); err != nil {
		m.quickCmdMu.Unlock()
		*m.Config = *original
		return err
	}
	m.quickCmdMu.Unlock()
	if err := m.saveHighlightRules(); err != nil {
		*m.Config = *original
		return err
	}

	msg := fmt.Sprintf("已成功导入 %d 个配置文件", len(imported))
	if len(warnings) > 0 {
		msg = msg + "。" + strings.Join(warnings, "；")
	}
	if usedDefaults {
		msg += "。部分字段已使用新版本默认值"
	}
	if updated.Log.Dir != "" && original.Log.Dir != updated.Log.Dir {
		msg += `。请前往"系统设置-系统选项"确认日志目录配置`
	}
	if updated.Docs.Dir != "" && original.Docs.Dir != updated.Docs.Dir {
		msg += `。请前往"系统设置-系统选项"确认文档目录配置`
	}
	m.lastImportMessage = msg
	return nil
}

func cloneAppConfig(cfg *AppConfig) (*AppConfig, error) {
	b, err := json.Marshal(cfg)
	if err != nil {
		return nil, err
	}
	var out AppConfig
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func backupFileIfExists(srcPath, dstPath string) error {
	data, err := os.ReadFile(srcPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return os.WriteFile(dstPath, data, 0644)
}

// saveQuickCommands 保存快捷命令配置到独立文件（调用方须持有 quickCmdMu）
func (m *Manager) saveQuickCommands() error {
	data, err := json.MarshalIndent(m.Config.QuickCommands, "", "  ")
	if err != nil {
		return err
	}
	// 内容未变化时跳过写盘，避免无谓刷新 mtime
	if old, rerr := os.ReadFile(m.quickCommandsPath); rerr == nil && bytes.Equal(old, data) {
		return nil
	}
	merged, err := filetxn.Merge(m.quickCommandsPath, m.quickBase, data)
	if err != nil {
		return err
	}
	m.quickBase = merged
	if err = json.Unmarshal(merged, &m.Config.QuickCommands); err != nil {
		return err
	}
	m.recordQuickCmdStat()
	return nil
}

// recordQuickCmdStat 记录文件当前状态，供热加载变化检测（调用方须持有 quickCmdMu）
func (m *Manager) recordQuickCmdStat() {
	if st, err := os.Stat(m.quickCommandsPath); err == nil {
		m.quickCmdMod = st.ModTime()
		m.quickCmdSize = st.Size()
	}
}

func (m *Manager) SetLLMConfig(apiKey, baseURL, model string) {
	m.Config.LLM.APIKey = apiKey
	m.Config.LLM.BaseURL = baseURL
	m.Config.LLM.FastModel = model
}

func (m *Manager) SetLogDir(dir string) {
	m.Config.Log.Dir = dir
}

// AddQuickCommand 追加一条快捷命令并立即写盘。
// 单条意图化操作取代旧的全量覆盖保存，避免多窗口互相用旧快照覆盖。
func (m *Manager) AddQuickCommand(cmd QuickCommand) error {
	return m.MutateQuickCommands(func(items []QuickCommand) ([]QuickCommand, error) {
		for _, item := range items {
			if item.ID == cmd.ID {
				return nil, fmt.Errorf("命令 ID 已存在")
			}
		}
		return append(items, cmd), nil
	})
}
func (m *Manager) UpdateQuickCommand(id string, updates QuickCommand) bool {
	return m.MutateQuickCommands(func(items []QuickCommand) ([]QuickCommand, error) {
		for i, item := range items {
			if item.ID == id {
				updates.ID = id
				items[i] = updates
				return items, nil
			}
		}
		return nil, fmt.Errorf("命令不存在")
	}) == nil
}
func (m *Manager) DeleteQuickCommand(id string) bool {
	return m.MutateQuickCommands(func(items []QuickCommand) ([]QuickCommand, error) {
		for i, item := range items {
			if item.ID == id {
				return append(items[:i:i], items[i+1:]...), nil
			}
		}
		return nil, fmt.Errorf("命令不存在")
	}) == nil
}
func (m *Manager) ReorderQuickCommands(ids []string) bool {
	if len(ids) < 2 {
		return false
	}
	return m.MutateQuickCommands(func(items []QuickCommand) ([]QuickCommand, error) {
		selected := map[string]bool{}
		byID := map[string]QuickCommand{}
		for _, item := range items {
			byID[item.ID] = item
		}
		for _, id := range ids {
			if selected[id] || byID[id].ID == "" {
				return nil, fmt.Errorf("命令排序已过期")
			}
			selected[id] = true
		}
		n := 0
		for i, item := range items {
			if selected[item.ID] {
				items[i] = byID[ids[n]]
				n++
			}
		}
		return items, nil
	}) == nil
}

// MutateQuickCommands is shared by desktop and plugin: reload and apply the
// user's intent under one interprocess lock, without replacing stale arrays.
func (m *Manager) MutateQuickCommands(fn func([]QuickCommand) ([]QuickCommand, error)) error {
	m.quickCmdMu.Lock()
	defer m.quickCmdMu.Unlock()
	release, err := filetxn.Lock(m.quickCommandsPath)
	if err != nil {
		return err
	}
	defer release()
	data, err := filetxn.Read(m.quickCommandsPath)
	if err != nil {
		return err
	}
	items := []QuickCommand{}
	if len(data) > 0 {
		if err = json.Unmarshal(data, &items); err != nil {
			return err
		}
	}
	next, err := fn(items)
	if err != nil {
		return err
	}
	encoded, err := json.MarshalIndent(next, "", "  ")
	if err != nil {
		return err
	}
	if err = filetxn.Write(m.quickCommandsPath, encoded); err != nil {
		return err
	}
	m.Config.QuickCommands = next
	m.quickBase = encoded
	m.recordQuickCmdStat()
	return nil
}

// CheckQuickCommandsChanged 检测 quick_commands.json 是否被外部修改
// （多窗口场景下其他进程写入）。变化时重载进内存并返回最新列表。
func (m *Manager) CheckQuickCommandsChanged() (bool, []QuickCommand) {
	st, err := os.Stat(m.quickCommandsPath)
	if err != nil {
		return false, nil
	}
	m.quickCmdMu.Lock()
	defer m.quickCmdMu.Unlock()
	if st.ModTime().Equal(m.quickCmdMod) && st.Size() == m.quickCmdSize {
		return false, nil
	}
	data, err := os.ReadFile(m.quickCommandsPath)
	if err != nil {
		return false, nil
	}
	var cmds []QuickCommand
	if err := json.Unmarshal(data, &cmds); err != nil {
		return false, nil // 损坏的写入（对端写一半），跳过本轮，下轮再试
	}
	m.Config.QuickCommands = cmds
	m.quickBase = append([]byte(nil), data...)
	m.quickCmdMod = st.ModTime()
	m.quickCmdSize = st.Size()
	return true, cmds
}

func (m *Manager) loadHighlightRules() error {
	data, err := os.ReadFile(m.highlightRulesPath)
	if os.IsNotExist(err) {
		m.Config.HighlightRules = []HighlightRule{}
		if m.readOnly {
			return nil
		}
		return m.saveHighlightRules()
	}
	if err != nil {
		return err
	}

	m.highlightBase = append([]byte(nil), data...)
	var rules []HighlightRule
	if err := json.Unmarshal(data, &rules); err != nil {
		return err
	}
	if rules == nil {
		rules = []HighlightRule{}
	}
	m.Config.HighlightRules = rules
	return nil
}

func (m *Manager) saveHighlightRules() error {
	data, err := json.MarshalIndent(m.Config.HighlightRules, "", "  ")
	if err != nil {
		return err
	}
	// 内容未变化时跳过写盘，避免无谓刷新 mtime
	if old, rerr := os.ReadFile(m.highlightRulesPath); rerr == nil && bytes.Equal(old, data) {
		return nil
	}
	merged, err := filetxn.Merge(m.highlightRulesPath, m.highlightBase, data)
	if err != nil {
		return err
	}
	m.highlightBase = merged
	return json.Unmarshal(merged, &m.Config.HighlightRules)
}

func (m *Manager) SetHighlightRules(rules []HighlightRule) error {
	m.Config.HighlightRules = rules
	return m.saveHighlightRules()
}

// Import is an explicit whole-tree replacement, but still participates in the
// same lock and atomic-write protocol as individual connection edits.
func (m *Manager) replaceImportedSessions(data []byte) error {
	var nodes []json.RawMessage
	if err := json.Unmarshal(data, &nodes); err != nil {
		return err
	}
	release, err := filetxn.Lock(m.sessionsPath)
	if err != nil {
		return err
	}
	defer release()
	if err := backupFileIfExists(m.sessionsPath, m.sessionsPath+".bak"); err != nil {
		return err
	}
	return filetxn.Write(m.sessionsPath, data)
}
