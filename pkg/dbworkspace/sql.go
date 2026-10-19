package dbworkspace

import (
	"strings"
	"unicode"
)

// SplitSQL preserves the legacy MySQL script API for existing callers.
func SplitSQL(text string) ([]string, error) {
	return executionSQL(QueryRequest{SQL: text, Mode: "script"}, "mysql")
}

func firstKeyword(text string) string {
	return firstKeywordFor(text, "mysql")
}

func firstKeywordFor(text, dialect string) string {
	for {
		text = strings.TrimSpace(text)
		if strings.HasPrefix(text, "/*") {
			end := strings.Index(text, "*/")
			if end < 0 {
				return ""
			}
			text = text[end+2:]
			continue
		}
		if dialect == "mysql" && strings.HasPrefix(text, "#") || (strings.HasPrefix(text, "--") && (dialect != "mysql" || len(text) == 2 || unicode.IsSpace(rune(text[2])))) {
			end := strings.IndexByte(text, '\n')
			if end < 0 {
				return ""
			}
			text = text[end+1:]
			continue
		}
		break
	}
	end := 0
	for end < len(text) && ((text[end] >= 'a' && text[end] <= 'z') || (text[end] >= 'A' && text[end] <= 'Z')) {
		end++
	}
	if end == 0 && text != "" {
		return "OTHER"
	}
	return strings.ToUpper(text[:end])
}

func returnsRows(stmt string) bool {
	return returnsRowsFor(stmt, "mysql")
}

func returnsRowsFor(stmt, dialect string) bool {
	switch firstKeywordFor(stmt, dialect) {
	case "SELECT", "SHOW", "DESCRIBE", "DESC", "EXPLAIN", "WITH":
		return true
	}
	return false
}
