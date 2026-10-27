package terminalcontrol

import (
	"encoding/json"
	"regexp"
	"strings"
)

var effectCommand = regexp.MustCompile(`(?i)(^|[;&|()\n])\s*(?:[\w./\\:-]+[/\\])?(rm|rmdir|unlink|shred|reboot|shutdown|kill|killall|pkill|stop-service|restart-service|remove-item|sudo|su|passwd|chmod|chown|usermod|useradd|truncate|dd|mkfs|mount|umount|cp|mv|tee)(\s|$)`)
var serviceChange = regexp.MustCompile(`(?i)(^|[;&|()\n])\s*(systemctl\s+(restart|stop|start|reload|enable|disable|mask)|service\s+\S+\s+(restart|stop|start|reload))(\s|$)`)
var inplaceEdit = regexp.MustCompile(`(?i)(^|[;&|()\n])\s*(?:[\w./\\:-]+[/\\])?sed\s+(?:[^;|\n]*\s)?-i(?:\s|=|$)`)
var overwriteRedirect = regexp.MustCompile(`(^|[^>])>([^>&]|$)`)
var pathOperand = regexp.MustCompile("(?:[A-Za-z]:[\\\\/]|~?/|\\.env\\b)[^\\s\"'`;,|()]+")

// RequiresHumanReview is a narrow high-consequence floor, not a shell parser
// or a proof of safety. Other effects are reviewed by iCode.
func RequiresHumanReview(action string) bool {
	var payload map[string]json.RawMessage
	if json.Unmarshal([]byte(action), &payload) == nil && payload != nil {
		for key, raw := range payload {
			var text string
			if json.Unmarshal(raw, &text) != nil {
				continue
			}
			switch strings.ToLower(key) {
			case "command", "script", "code":
				if RequiresHumanReview(text) {
					return true
				}
			case "path", "file_path", "filepath", "destination":
				if sensitivePath(text) {
					return true
				}
			}
		}
		return false
	}
	for _, p := range pathOperand.FindAllString(action, -1) {
		if sensitivePath(p) {
			return true
		}
	}
	var outside strings.Builder
	var quote rune
	escaped := false
	for _, r := range action {
		if escaped {
			escaped = false
			continue
		}
		if r == '\\' && quote != '\'' {
			escaped = true
			continue
		}
		if quote != 0 {
			if r == quote {
				quote = 0
				outside.WriteRune(' ')
			}
			continue
		}
		if r == '\'' || r == '"' || r == '`' {
			quote = r
			outside.WriteRune(' ')
			continue
		}
		outside.WriteRune(r)
	}
	text := outside.String()
	return effectCommand.MatchString(text) || serviceChange.MatchString(text) || inplaceEdit.MatchString(text) || overwriteRedirect.MatchString(text) || strings.HasPrefix(strings.TrimSpace(strings.ToLower(text)), "printenv")
}
