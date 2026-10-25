package assistantcontrol

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Display excerpts are bounded visible evidence, separate from hash-only audit.
// Filter common credential forms before clipping, including multiline PEM keys.
var displayPEM = regexp.MustCompile(`(?s)-----BEGIN [^-]*PRIVATE KEY-----.*?-----END [^-]*PRIVATE KEY-----`)
var displayCredential = regexp.MustCompile(`(?i)\b(password|passwd|token|secret|api[_-]?key|access[_-]?token|authorization)["']?\s*[=:]\s*(?:"[^"]*"|'[^']*'|[^\s;&,]+)`)
var displayBearer = regexp.MustCompile(`(?i)\bBearer\s+[^\s"']+`)
var displayURL = regexp.MustCompile(`(//)[^/\s?;@]+@`)
var displayANSI = regexp.MustCompile(`\x1b\[[0-?]*[ -/]*[@-~]`)

func DisplayExcerpt(text string, limit int) string {
	text = displayPEM.ReplaceAllString(text, "[私钥内容已隐藏]")
	text = displayBearer.ReplaceAllString(text, "Bearer [redacted]")
	text = displayCredential.ReplaceAllString(text, "$1=[redacted]")
	text = displayURL.ReplaceAllString(text, "${1}[redacted]@")
	text = displayANSI.ReplaceAllString(text, "")
	text = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			return -1
		}
		return r
	}, text)
	if len(text) > limit {
		text = text[:limit]
		for !utf8.ValidString(text) {
			text = text[:len(text)-1]
		}
		text += "\n[片段已截断]"
	}
	return text
}
