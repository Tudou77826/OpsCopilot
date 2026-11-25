package main

import (
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode"

	"opscopilot/pkg/achievements"
	"opscopilot/pkg/config"
	"opscopilot/pkg/filetxn"
)

func (a *App) installQuickCommandExamples() error {
	marker := filepath.Join(a.configMgr.Directory(), "quick-command-examples-v1")
	unlock, err := filetxn.Lock(marker)
	if err != nil {
		return err
	}
	defer unlock()
	existing, err := filetxn.Read(marker)
	if err != nil || len(existing) != 0 {
		return err
	}
	examples := []config.QuickCommand{
		{ID: "example-local-ip", Name: "粘贴本地 IP", Group: "default", Content: "{{local_ip}}", UseVariables: true},
		{ID: "example-remote-ip", Name: "粘贴远程 IP", Group: "default", Content: "{{remote_ip}}", UseVariables: true},
	}
	err = a.configMgr.MutateQuickCommands(func(items []config.QuickCommand) ([]config.QuickCommand, error) {
		for _, example := range examples {
			if !slices.ContainsFunc(items, func(item config.QuickCommand) bool { return item.ID == example.ID }) {
				items = append(items, example)
			}
		}
		return items, nil
	})
	if err != nil {
		return err
	}
	return filetxn.Write(marker, []byte("1\n"))
}

func (a *App) quickCommand(id string) (config.QuickCommand, error) {
	for _, cmd := range a.configMgr.QuickCommandsSnapshot() {
		if cmd.ID == id && strings.TrimSpace(cmd.Content) != "" {
			return cmd, nil
		}
	}
	return config.QuickCommand{}, fmt.Errorf("快捷指令不存在或内容为空")
}

var quickCommandVariable = regexp.MustCompile(`\{\{(?:local_ip|remote_ip|selected_text|date)\}\}`)

func renderQuickCommand(content, localIP, remoteIP, selectedText string, now time.Time) (string, error) {
	var missing error
	result := quickCommandVariable.ReplaceAllStringFunc(content, func(variable string) string {
		if missing != nil {
			return ""
		}
		switch variable {
		case "{{local_ip}}":
			if localIP == "" {
				missing = fmt.Errorf("当前会话无法获取本地 IP")
			}
			return localIP
		case "{{remote_ip}}":
			if remoteIP == "" {
				missing = fmt.Errorf("当前会话无法获取远程 IP")
			}
			return remoteIP
		case "{{selected_text}}":
			if strings.TrimSpace(selectedText) == "" {
				missing = fmt.Errorf("请先在当前终端选中一行文本")
			} else if strings.ContainsFunc(selectedText, unicode.IsControl) {
				missing = fmt.Errorf("请选中一行不含控制字符的文本")
			}
			return selectedText
		default:
			return now.Format("2006-01-02")
		}
	})
	if missing != nil {
		return "", missing
	}
	if strings.ContainsFunc(result, unicode.IsControl) {
		return "", fmt.Errorf("含变量的快捷指令须为单行，且不能包含控制字符")
	}
	return result, nil
}

// DispatchQuickCommandTemplate resolves every input before writing to the
// specific connection that supplied the terminal selection.
func (a *App) DispatchQuickCommandTemplate(commandID, sessionID, connectionID, selectedText string) (achievements.DispatchResult, error) {
	cmd, err := a.quickCommand(commandID)
	if err != nil {
		return achievements.DispatchResult{}, err
	}
	if !cmd.UseVariables {
		return achievements.DispatchResult{}, fmt.Errorf("快捷指令已更新，请重新点击")
	}
	sess, ok := a.sessionMgr.Get(sessionID)
	if !ok || sess.Stdin == nil || connectionID == "" || sess.ConnectionID != connectionID {
		return achievements.DispatchResult{}, fmt.Errorf("当前终端连接已改变，请重新点击")
	}
	var localIP, remoteIP string
	if conn, ok := sess.Conn.(interface{ ConnectionIPs() (string, string) }); ok {
		localIP, remoteIP = conn.ConnectionIPs()
	}
	content, err := renderQuickCommand(cmd.Content, localIP, remoteIP, selectedText, time.Now())
	if err != nil {
		return achievements.DispatchResult{}, err
	}
	return achievements.Dispatch(a.achievements, commandID, []string{sessionID}, func(id string) error {
		if current, ok := a.sessionMgr.Get(id); !ok || current != sess {
			return fmt.Errorf("当前终端连接已改变")
		}
		n, err := io.WriteString(sess.Stdin, content)
		if err == nil && n != len(content) {
			err = io.ErrShortWrite
		}
		if err == nil {
			a.terminalLifecycles.ApplyCurrent(id, func() {
				if current, ok := a.sessionMgr.Get(id); ok && current == sess {
					a.recordInput(id, content)
				}
			})
		}
		return err
	})
}
