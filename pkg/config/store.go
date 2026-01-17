package config

import (
	"encoding/json"
	"os"
	"path/filepath"
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
}

type ExperimentalConfig struct {
	Monitoring bool `json:"monitoring"`
}

type QuickCommand struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Content string `json:"content"`
}

type LogConfig struct {
	Dir string `json:"dir"`
}

type DocsConfig struct {
	Dir string `json:"dir"`
}

type Manager struct {
	configPath        string
	promptsPath       string
	quickCommandsPath string
	Config            *AppConfig
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
		Experimental: ExperimentalConfig{
			Monitoring: false,
		},
	}

	return &Manager{
		configPath:        "config.json",         // 默认在当前目录
		promptsPath:       "prompts.json",        // prompts 配置文件
		quickCommandsPath: "quick_commands.json", // quick_commands 配置文件
		Config:            cfg,
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

	// 加载 prompts 配置
	if err := m.loadPrompts(); err != nil {
		return err
	}

	// 加载 quick_commands 配置
	if err := m.loadQuickCommands(); err != nil {
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

	return nil
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
	}

	cfg := ConfigForSave{
		LLM:                  m.Config.LLM,
		Log:                  m.Config.Log,
		Docs:                 m.Config.Docs,
		CompletionDelay:      m.Config.CompletionDelay,
		CommandQueryShortcut: m.Config.CommandQueryShortcut,
		Experimental:         m.Config.Experimental,
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

	return nil
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
