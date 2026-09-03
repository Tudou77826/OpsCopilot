package shellsidecar

import (
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"opscopilot/pkg/config"
	"opscopilot/pkg/filetxn"
	"path/filepath"
	"strconv"
	"sync"
)

// DesktopData resolves all user data inside Ops, never in the Teams adapter.
// Relative custom locations retain desktop semantics (relative to the exe).
type DesktopData struct{ Root, Scripts, Recordings string }

func ResolveDesktopData(root string) (DesktopData, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return DesktopData{}, err
	}
	m := config.NewManagerWithDir(root)
	m.SetReadOnly(true)
	if err = m.Load(); err != nil {
		return DesktopData{}, fmt.Errorf("读取本地 Ops 配置失败: %w", err)
	}
	absolute := func(p string) string {
		if filepath.IsAbs(p) {
			return filepath.Clean(p)
		}
		return filepath.Join(root, p)
	}
	logDir := absolute(m.Config.Log.Dir)
	scripts := m.Config.Scripts.Dir
	if scripts == "" {
		scripts = filepath.Join(filepath.Dir(logDir), "scripts")
	}
	return DesktopData{root, absolute(scripts), filepath.Join(logDir, "recordings")}, nil
}

type desktopSettings struct {
	root      string
	mu        sync.Mutex
	snapshots map[string]*config.Manager
}

func (d *desktopSettings) get() (ShellSettingsJSON, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	m := config.NewManagerWithDir(d.root)
	m.SetReadOnly(true)
	if err := m.Load(); err != nil {
		return ShellSettingsJSON{}, err
	}
	c := m.Config
	out := defaultShellSettings()
	out.Theme = c.Appearance.Theme
	out.CompletionDelay = c.CompletionDelay
	out.CommandQueryShortcut = c.CommandQueryShortcut
	b, _ := json.Marshal(c.Terminal)
	if err := json.Unmarshal(b, &out.Terminal); err != nil {
		return out, err
	}
	for _, r := range c.HighlightRules {
		out.HighlightRules = append(out.HighlightRules, HighlightRuleJSON{ID: r.ID, Name: r.Name, Pattern: r.Pattern, IsEnabled: r.IsEnabled, Priority: r.Priority, Style: HighlightStyleJSON{BackgroundColor: r.Style.BackgroundColor, Color: r.Style.Color, FontWeight: r.Style.FontWeight, TextDecoration: r.Style.TextDecoration, Opacity: strconv.FormatFloat(r.Style.Opacity, 'f', -1, 64)}})
	}
	if len(d.snapshots) >= 128 {
		d.snapshots = map[string]*config.Manager{}
	}
	out.Revision = uuid.NewString()
	d.snapshots[out.Revision] = m
	return out, nil
}
func (d *desktopSettings) save(next ShellSettingsJSON) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	m := d.snapshots[next.Revision]
	if m == nil {
		return filetxn.ErrConflict
	}
	m.Config.Appearance.Theme = next.Theme
	m.Config.CompletionDelay = next.CompletionDelay
	m.Config.CommandQueryShortcut = next.CommandQueryShortcut
	b, _ := json.Marshal(next.Terminal)
	if err := json.Unmarshal(b, &m.Config.Terminal); err != nil {
		return err
	}
	rules := []config.HighlightRule{}
	for _, r := range next.HighlightRules {
		opacity := 0.0
		if r.Style.Opacity != "" {
			v, e := strconv.ParseFloat(r.Style.Opacity, 64)
			if e != nil {
				return fmt.Errorf("无效的高亮透明度")
			}
			opacity = v
		}
		rules = append(rules, config.HighlightRule{ID: r.ID, Name: r.Name, Pattern: r.Pattern, IsEnabled: r.IsEnabled, Priority: r.Priority, Style: config.HighlightStyle{BackgroundColor: r.Style.BackgroundColor, Color: r.Style.Color, FontWeight: r.Style.FontWeight, TextDecoration: r.Style.TextDecoration, Opacity: opacity}})
	}
	if err := m.SetHighlightRules(rules); err != nil {
		return err
	}
	return m.Save()
}
