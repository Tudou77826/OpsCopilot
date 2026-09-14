package shellsidecar

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// SettingsService：Shell 设置切片的 sidecar 持久化（数据目录内 shell-settings.json）。
// 字段形状与共享 TS 类型对齐（TerminalConfig/HighlightRule 见
// frontend-shell/src/ui/Terminal/highlightTypes.ts），重启后保持。
type SettingsService struct {
	path    string
	desktop *desktopSettings
}

// TerminalConfigJSON 与共享 TerminalConfig 同构（snake_case）。
type TerminalConfigJSON struct {
	Scrollback      int    `json:"scrollback"`
	SearchEnabled   bool   `json:"search_enabled"`
	HighlightEnable bool   `json:"highlight_enabled"`
	FontFamily      string `json:"font_family,omitempty"`
	FontSize        int    `json:"font_size,omitempty"`
}

// HighlightStyleJSON 与共享 HighlightStyle 同构。
type HighlightStyleJSON struct {
	BackgroundColor string `json:"background_color,omitempty"`
	Color           string `json:"color,omitempty"`
	FontWeight      string `json:"font_weight,omitempty"`
	TextDecoration  string `json:"text_decoration,omitempty"`
	Opacity         string `json:"opacity,omitempty"`
}

// HighlightRuleJSON 与共享 HighlightRule 同构。
type HighlightRuleJSON struct {
	ID        string             `json:"id"`
	Name      string             `json:"name"`
	Pattern   string             `json:"pattern"`
	IsEnabled bool               `json:"is_enabled"`
	Priority  int                `json:"priority"`
	Style     HighlightStyleJSON `json:"style"`
}

// ShellSettingsJSON 与共享 ShellSettings 同构。
type ShellSettingsJSON struct {
	Revision        string              `json:"revision,omitempty"`
	Theme           string              `json:"theme"`
	Terminal        TerminalConfigJSON  `json:"terminal"`
	CompletionDelay int                 `json:"completionDelay"`
	HighlightRules  []HighlightRuleJSON `json:"highlightRules"`
	// CommandQueryShortcut 命令查询（Ctrl+K）快捷键，如 "Ctrl+K"；空值取默认。
	CommandQueryShortcut string `json:"commandQueryShortcut,omitempty"`
}

func defaultShellSettings() ShellSettingsJSON {
	return ShellSettingsJSON{
		Theme: "dark",
		Terminal: TerminalConfigJSON{
			Scrollback:      5000,
			SearchEnabled:   true,
			HighlightEnable: true,
			FontFamily:      "JetBrains Mono",
			FontSize:        14,
		},
		CompletionDelay:      150,
		HighlightRules:       []HighlightRuleJSON{},
		CommandQueryShortcut: "Ctrl+K",
	}
}

func NewSettingsService(dataDir string) (*SettingsService, error) {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, err
	}
	return &SettingsService{path: filepath.Join(dataDir, "shell-settings.json")}, nil
}

// Get 返回设置（缺失/损坏时回退默认并落盘）。
func (s *SettingsService) Get() (ShellSettingsJSON, error) {
	if s.desktop != nil {
		return s.desktop.get()
	}
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			def := defaultShellSettings()
			_ = s.write(def)
			return def, nil
		}
		return defaultShellSettings(), err
	}
	var out ShellSettingsJSON
	if err := json.Unmarshal(data, &out); err != nil {
		return defaultShellSettings(), nil
	}
	// 阶段 5 的早期构建曾把这两个顶层字段写成 snake_case。
	// 读取时兼容一次，后续 Save 会统一写回共享 TS 类型使用的 camelCase。
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err == nil {
		if _, ok := fields["completionDelay"]; !ok {
			if legacy, exists := fields["completion_delay"]; exists {
				_ = json.Unmarshal(legacy, &out.CompletionDelay)
			}
		}
		if _, ok := fields["highlightRules"]; !ok {
			if legacy, exists := fields["highlight_rules"]; exists {
				_ = json.Unmarshal(legacy, &out.HighlightRules)
			}
		}
	}
	// 补齐零值字段，避免旧文件缺项时前端拿到空 terminal
	def := defaultShellSettings()
	if out.Theme != "light" && out.Theme != "dark" {
		out.Theme = def.Theme
	}
	if out.Terminal.Scrollback <= 0 {
		out.Terminal.Scrollback = def.Terminal.Scrollback
	}
	if out.Terminal.FontFamily == "" {
		out.Terminal.FontFamily = def.Terminal.FontFamily
	}
	if out.Terminal.FontSize <= 0 {
		out.Terminal.FontSize = def.Terminal.FontSize
	}
	if out.CompletionDelay < 0 || out.CompletionDelay > 2000 {
		out.CompletionDelay = def.CompletionDelay
	}
	if strings.TrimSpace(out.CommandQueryShortcut) == "" {
		out.CommandQueryShortcut = def.CommandQueryShortcut
	}
	if out.HighlightRules == nil {
		out.HighlightRules = []HighlightRuleJSON{}
	}
	return out, nil
}

// Save 全量保存。
func (s *SettingsService) Save(next ShellSettingsJSON) error {
	if s.desktop != nil {
		return s.desktop.save(next)
	}
	if next.HighlightRules == nil {
		next.HighlightRules = []HighlightRuleJSON{}
	}
	return s.write(next)
}

func (s *SettingsService) write(v ShellSettingsJSON) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0o644)
}
