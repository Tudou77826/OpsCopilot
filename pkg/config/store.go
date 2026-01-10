package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type AppConfig struct {
	LLM           LLMConfig         `json:"llm"`
	Prompts       map[string]string `json:"prompts"`
	Log           LogConfig         `json:"log"`
	Docs          DocsConfig        `json:"docs"`
	QuickCommands []QuickCommand    `json:"quick_commands"`
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
	configPath string
	Config     *AppConfig
}

func NewManager() *Manager {
	// 默认配置
	defaultLLM := LoadLLMConfig()

	cwd, _ := os.Getwd()
	defaultLogDir := filepath.Join(cwd, "logs")

	cfg := &AppConfig{
		LLM: *defaultLLM,
		Prompts: map[string]string{
			"smart_connect":     DefaultSmartConnectPrompt,
			"qa_prompt":         DefaultQAPrompt,
			"conclusion_prompt": DefaultConclusionPrompt,
			"polish_prompt":     DefaultPolishPrompt,
		},
		Log: LogConfig{
			Dir: defaultLogDir,
		},
		Docs: DocsConfig{
			Dir: "", // Default to empty, will be resolved dynamically if empty
		},
	}

	return &Manager{
		configPath: "config.json", // 默认在当前目录
		Config:     cfg,
	}
}

// Load reads config from file, creating it with defaults if not exists
func (m *Manager) Load() error {
	data, err := os.ReadFile(m.configPath)
	if os.IsNotExist(err) {
		// 文件不存在，保存默认配置
		return m.Save()
	}
	if err != nil {
		return err
	}

	// 解析配置
	if err := json.Unmarshal(data, m.Config); err != nil {
		return err
	}

	// 确保 Prompts map 初始化
	if m.Config.Prompts == nil {
		m.Config.Prompts = make(map[string]string)
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

	return nil
}

func (m *Manager) Save() error {
	data, err := json.MarshalIndent(m.Config, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(m.configPath, data, 0644)
}

func (m *Manager) SetLLMConfig(apiKey, baseURL, model string) {
	m.Config.LLM.APIKey = apiKey
	m.Config.LLM.BaseURL = baseURL
	m.Config.LLM.Model = model
}

func (m *Manager) SetPrompt(key, content string) {
	if m.Config.Prompts == nil {
		m.Config.Prompts = make(map[string]string)
	}
	m.Config.Prompts[key] = content
}

func (m *Manager) SetLogDir(dir string) {
	m.Config.Log.Dir = dir
}

func (m *Manager) SetQuickCommands(cmds []QuickCommand) {
	m.Config.QuickCommands = cmds
}
