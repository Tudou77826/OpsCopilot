package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
)

type AppConfig struct {
	LLM                  LLMConfig          `json:"llm"`
	Prompts              map[string]string  `json:"prompts"`
	Log                  LogConfig          `json:"log"`
	Docs                 DocsConfig         `json:"docs"`
	QuickCommands        []QuickCommand     `json:"quick_commands"`
	CompletionDelay      int                `json:"completion_delay"`
	CommandQueryShortcut string             `json:"command_query_shortcut"`
	Experimental         ExperimentalConfig `json:"experimental"`
	Terminal             TerminalConfig     `json:"terminal"`
	HighlightRules       []HighlightRule    `json:"highlight_rules"`
}

// ExperimentalConfig 实验性功能配置（保留结构以便未来扩展）
type ExperimentalConfig struct {
}

type TerminalConfig struct {
	Scrollback       int  `json:"scrollback"`
	SearchEnabled    bool `json:"search_enabled"`
	HighlightEnabled bool `json:"highlight_enabled"`
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
	Dir string `json:"dir"`
}

type DocsConfig struct {
	Dir string `json:"dir"`
}

type Manager struct {
	configPath         string
	promptsPath        string
	quickCommandsPath  string
	highlightRulesPath string
	sessionsPath       string
	Config             *AppConfig
	lastImportMessage  string
	importing          atomic.Bool
}

func NewManager() *Manager {
	// 默认配置
	defaultLLM := LoadLLMConfig()

	cwd, _ := os.Getwd()
	defaultLogDir := filepath.Join(cwd, "logs")

	cfg := &AppConfig{
		LLM: *defaultLLM,
		Prompts: map[string]string{
			"smart_connect":        DefaultSmartConnectPrompt,
			"qa_prompt":            DefaultQAPrompt,
			"conclusion_prompt":    DefaultConclusionPrompt,
			"polish_prompt":        DefaultPolishPrompt,
			"troubleshoot_prompt":  DefaultTroubleshootPrompt,
			"command_query_prompt": DefaultCommandQueryPrompt,
		},
		Log: LogConfig{
			Dir: defaultLogDir,
		},
		Docs: DocsConfig{
			Dir: "", // Default to empty, will be resolved dynamically if empty
		},
		QuickCommands:        []QuickCommand{},
		CompletionDelay:      150, // Default 150ms
		CommandQueryShortcut: "Ctrl+K",
		Experimental:         ExperimentalConfig{},
		Terminal: TerminalConfig{
			Scrollback:       5000,
			SearchEnabled:    true,
			HighlightEnabled: true,
		},
		HighlightRules: []HighlightRule{},
	}

	return &Manager{
		configPath:         "config.json",         // 默认在当前目录
		promptsPath:        "prompts.json",        // prompts 配置文件
		quickCommandsPath:  "quick_commands.json", // quick_commands 配置文件
		highlightRulesPath: "highlight_rules.json",
		sessionsPath:       "sessions.json",
		Config:             cfg,
	}
}

// Load reads config from file, creating it with defaults if not exists
func (m *Manager) Load() error {
	// 加载主配置文件
	data, err := os.ReadFile(m.configPath)
	if os.IsNotExist(err) {
		// 文件不存在，保存默认配置
		return m.Save()
	}
	if err != nil {
		return err
	}

	var raw map[string]any
	_ = json.Unmarshal(data, &raw)
	llmRaw, _ := raw["llm"].(map[string]any)
	_, hasFastModel := llmRaw["FastModel"]
	_, hasComplexModel := llmRaw["ComplexModel"]
	_, hasOldModel := llmRaw["Model"]
	_, hasCommandQueryShortcut := raw["command_query_shortcut"]
	_, hasTerminal := raw["terminal"]

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
	if !hasCommandQueryShortcut || m.Config.CommandQueryShortcut == "" {
		m.Config.CommandQueryShortcut = "Ctrl+K"
		changed = true
	}
	if !hasTerminal {
		m.Config.Terminal = TerminalConfig{
			Scrollback:       5000,
			SearchEnabled:    true,
			HighlightEnabled: true,
		}
		changed = true
	}
	if m.Config.Terminal.Scrollback <= 0 {
		m.Config.Terminal.Scrollback = 5000
		changed = true
	}

	// 加载 prompts 配置
	if err := m.loadPrompts(); err != nil {
		return err
	}

	// 加载 quick_commands 配置
	if err := m.loadQuickCommands(); err != nil {
		return err
	}

	// 加载 highlight_rules 配置
	if err := m.loadHighlightRules(); err != nil {
		return err
	}

	if changed {
		if err := m.Save(); err != nil {
			return err
		}
	}

	return nil
}

// loadPrompts 从独立文件加载提示词配置
func (m *Manager) loadPrompts() error {
	// 确保 Prompts map 初始化
	if m.Config.Prompts == nil {
		m.Config.Prompts = make(map[string]string)
	}

	// 读取 prompts.json 文件
	data, err := os.ReadFile(m.promptsPath)
	if os.IsNotExist(err) {
		// 文件不存在，使用默认值并保存
		m.Config.Prompts = map[string]string{
			"smart_connect":        DefaultSmartConnectPrompt,
			"qa_prompt":            DefaultQAPrompt,
			"conclusion_prompt":    DefaultConclusionPrompt,
			"polish_prompt":        DefaultPolishPrompt,
			"troubleshoot_prompt":  DefaultTroubleshootPrompt,
			"command_query_prompt": DefaultCommandQueryPrompt,
		}
		return m.savePrompts()
	}
	if err != nil {
		return err
	}

	// 解析 JSON 到 Prompts map
	if err := json.Unmarshal(data, &m.Config.Prompts); err != nil {
		return err
	}

	// 确保默认 Prompt 存在 (如果文件中没有)
	if _, ok := m.Config.Prompts["smart_connect"]; !ok {
		m.Config.Prompts["smart_connect"] = DefaultSmartConnectPrompt
	}
	if _, ok := m.Config.Prompts["qa_prompt"]; !ok {
		m.Config.Prompts["qa_prompt"] = DefaultQAPrompt
	}
	if _, ok := m.Config.Prompts["conclusion_prompt"]; !ok {
		m.Config.Prompts["conclusion_prompt"] = DefaultConclusionPrompt
	}
	if _, ok := m.Config.Prompts["polish_prompt"]; !ok {
		m.Config.Prompts["polish_prompt"] = DefaultPolishPrompt
	}
	if _, ok := m.Config.Prompts["troubleshoot_prompt"]; !ok {
		m.Config.Prompts["troubleshoot_prompt"] = DefaultTroubleshootPrompt
	}
	if _, ok := m.Config.Prompts["command_query_prompt"]; !ok {
		m.Config.Prompts["command_query_prompt"] = DefaultCommandQueryPrompt
	}

	return nil
}

// loadQuickCommands 从独立文件加载快捷命令配置
func (m *Manager) loadQuickCommands() error {
	// 读取 quick_commands.json 文件
	data, err := os.ReadFile(m.quickCommandsPath)
	if os.IsNotExist(err) {
		// 文件不存在，初始化为空数组并保存
		m.Config.QuickCommands = []QuickCommand{}
		return m.saveQuickCommands()
	}
	if err != nil {
		return err
	}

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
	return m.saveQuickCommands()
}

func (m *Manager) Save() error {
	// 保存主配置（不包含 prompts 和 quick_commands）
	type ConfigForSave struct {
		LLM                  LLMConfig          `json:"llm"`
		Log                  LogConfig          `json:"log"`
		Docs                 DocsConfig         `json:"docs"`
		CompletionDelay      int                `json:"completion_delay"`
		CommandQueryShortcut string             `json:"command_query_shortcut"`
		Experimental         ExperimentalConfig `json:"experimental"`
		Terminal             TerminalConfig     `json:"terminal"`
	}

	cfg := ConfigForSave{
		LLM:                  m.Config.LLM,
		Log:                  m.Config.Log,
		Docs:                 m.Config.Docs,
		CompletionDelay:      m.Config.CompletionDelay,
		CommandQueryShortcut: m.Config.CommandQueryShortcut,
		Experimental:         m.Config.Experimental,
		Terminal:             m.Config.Terminal,
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(m.configPath, data, 0644); err != nil {
		return err
	}

	// 保存 prompts 到独立文件
	if err := m.savePrompts(); err != nil {
		return err
	}

	// 保存 quick_commands 到独立文件
	if err := m.saveQuickCommands(); err != nil {
		return err
	}

	// 保存 highlight_rules 到独立文件
	if err := m.saveHighlightRules(); err != nil {
		return err
	}

	return nil
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

	if data, err := os.ReadFile(filepath.Join(cleaned, "prompts.json")); err == nil {
		var old map[string]string
		if err := json.Unmarshal(data, &old); err != nil {
			warnings = append(warnings, "prompts.json 格式错误，已跳过")
		} else {
			if updated.Prompts == nil {
				updated.Prompts = map[string]string{}
			}
			for k, v := range old {
				updated.Prompts[k] = v
			}
			imported = append(imported, "prompts.json")
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
		if err := backupFileIfExists(m.sessionsPath, m.sessionsPath+".bak"); err != nil {
			warnings = append(warnings, "备份 sessions.json 失败: "+err.Error())
		}
		if err := os.WriteFile(m.sessionsPath, data, 0644); err != nil {
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
	if err := backupFileIfExists(m.promptsPath, m.promptsPath+".bak"); err != nil {
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

// savePrompts 保存提示词配置到独立文件
func (m *Manager) savePrompts() error {
	data, err := json.MarshalIndent(m.Config.Prompts, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(m.promptsPath, data, 0644)
}

// saveQuickCommands 保存快捷命令配置到独立文件
func (m *Manager) saveQuickCommands() error {
	data, err := json.MarshalIndent(m.Config.QuickCommands, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(m.quickCommandsPath, data, 0644)
}

func (m *Manager) SetLLMConfig(apiKey, baseURL, model string) {
	m.Config.LLM.APIKey = apiKey
	m.Config.LLM.BaseURL = baseURL
	m.Config.LLM.FastModel = model
}

func (m *Manager) SetPrompt(key, content string) {
	if m.Config.Prompts == nil {
		m.Config.Prompts = make(map[string]string)
	}
	m.Config.Prompts[key] = content
	// 立即保存到独立文件
	m.savePrompts()
}

func (m *Manager) SetLogDir(dir string) {
	m.Config.Log.Dir = dir
}

func (m *Manager) SetQuickCommands(cmds []QuickCommand) {
	m.Config.QuickCommands = cmds
	// 立即保存到独立文件
	m.saveQuickCommands()
}

func (m *Manager) loadHighlightRules() error {
	data, err := os.ReadFile(m.highlightRulesPath)
	if os.IsNotExist(err) {
		m.Config.HighlightRules = []HighlightRule{}
		return m.saveHighlightRules()
	}
	if err != nil {
		return err
	}

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
	return os.WriteFile(m.highlightRulesPath, data, 0644)
}

func (m *Manager) SetHighlightRules(rules []HighlightRule) {
	m.Config.HighlightRules = rules
	m.saveHighlightRules()
}
