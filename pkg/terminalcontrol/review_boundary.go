package terminalcontrol

import (
	"regexp"
	"strings"
)

var mutatingWords = regexp.MustCompile(`(?i)(^|[^a-z0-9_])(rm|rmdir|unlink|shred|reboot|shutdown|kill|killall|pkill|restart|stop-service|restart-service|remove-item|set-content|add-content|out-file|invoke-webrequest|sudo|su|passwd|chmod|chown|usermod|useradd|mv|cp|tee|touch|mkdir|truncate|dd|mkfs|mount|umount)([^a-z0-9_]|$)`)

// RequiresHumanReview is a conservative floor for AI review, not a complete
// shell parser. Complex unknown effects are still the independent judge's job.
func RequiresHumanReview(action string) bool {
	return sensitivePath(action) || mutatingWords.MatchString(action) || strings.Contains(strings.ToLower(action), "printenv")
}
