package dbworkspace

import (
	"errors"
	"regexp"
	"strings"
)

// AssistantSQLRequiresHuman is only the explicit high-risk floor. Unknown
// read-oriented expressions (including IN and dialect functions) go to review.
// This does not authorize or execute SQL and is separate from server read-only policy.
func AssistantSQLRequiresHuman(text string) bool {
	lower := strings.ToLower(strings.TrimSpace(text))
	for strings.HasPrefix(lower, "--") || strings.HasPrefix(lower, "#") {
		i := strings.IndexByte(lower, '\n')
		if i < 0 {
			return true
		}
		lower = strings.TrimSpace(lower[i+1:])
	}
	words := strings.Fields(lower)
	if len(words) == 0 {
		return true
	}
	switch words[0] {
	case "select", "with", "show", "describe", "desc", "explain":
	default:
		return true
	}
	// Quoted string values are data, not mutation verbs. Unrecognized SQL still
	// goes to semantic review; this helper only identifies definite risk markers.
	lower = sqlLiteral.ReplaceAllString(lower, " ")
	return sqlRiskMarker.MatchString(lower)
}

var sqlLiteral = regexp.MustCompile(`'(?:[^']|'')*'`)
var sqlRiskMarker = regexp.MustCompile(`\b(insert|update|delete|drop|alter|truncate|grant|revoke|into|outfile|dumpfile|pg_authid|pg_shadow|credential|password|passwd|secret|token|pg_read_file|lo_export|lo_import|load_file|dblink_exec|pg_terminate_backend|pg_cancel_backend|set_config)\b`)

// AssistantSQL validates boundaries, not SQL semantics. Unknown/dialect-specific
// effects require exact approval; result-returning SQL is not necessarily read-only.
func AssistantSQL(text, dialect string) (readOnly bool, reason string, err error) {
	if len(text) > 8192 {
		return false, "", errors.New("AI 单次 SQL 不能超过 8 KiB")
	}
	statements, err := executionSQL(QueryRequest{SQL: text, Mode: "selection"}, dialect)
	if err != nil {
		return false, "", err
	}
	if len(statements) != 1 {
		return false, "", errors.New("AI 每个操作块只允许一条 SQL，请分别提交")
	}
	lower := strings.ToLower(strings.TrimSpace(text))
	// Models sometimes include purpose comments as well as the host-generated
	// header. Ordinary leading line comments do not change SELECT semantics.
	// Executable/block comments and comments inside SQL remain approval-only.
	for strings.HasPrefix(lower, "-- ") || strings.HasPrefix(lower, "--\t") || strings.HasPrefix(lower, "--\r") || strings.HasPrefix(lower, "--\n") || dialect == "mysql" && strings.HasPrefix(lower, "#") {
		end := strings.IndexByte(lower, '\n')
		if end < 0 {
			break
		}
		lower = strings.TrimSpace(lower[end+1:])
	}
	if !strings.HasPrefix(lower, "select ") && !strings.HasPrefix(lower, "select\n") {
		return false, "查询以外的操作或未知语法需要确认", nil
	}
	for _, word := range []string{"/*", "--", "#", "@", "password", "passwd", "secret", "token", "credential", "pg_authid", "pg_shadow", "mysql.user", " into ", "outfile", "dumpfile", "for update", "lock in share", ";"} {
		if strings.Contains(strings.TrimSuffix(lower, ";"), word) {
			return false, "包含敏感标识、锁定、导出或需要进一步判断的语法", nil
		}
	}
	// A deliberately small allowlist; unknown calls can mutate or access files.
	if regexp.MustCompile(`\.\s*[a-z0-9_"\x60]+\s*\(`).MatchString(lower) {
		return false, "限定名称的函数需要确认", nil
	}
	withoutAggregates := regexp.MustCompile(`\b(count|sum|min|max|avg)\s*\(`).ReplaceAllString(lower, "")
	if strings.Contains(withoutAggregates, "(") {
		return false, "包含尚未确认用途的函数或表达式", nil
	}
	return true, "常规 SELECT 查询，仍使用有上限的结果读取", nil
}

func (s *Service) Dialect(id string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v := s.sessions[id]
	if v == nil {
		return "", errors.New("数据库会话已断开")
	}
	if jdbc, ok := v.connector.(JDBC); ok {
		return jdbc.definition.Dialect, nil
	}
	return "mysql", nil
}
