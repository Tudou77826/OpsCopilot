package dbworkspace

import (
	"fmt"
	"strings"
	"unicode"
)

// Statement offsets are byte positions in the original editor text, including
// its leading comments/space. Only the execution text excludes client delimiters.
type sqlStatement struct {
	text     string
	from, to int
}

func executionSQL(req QueryRequest, dialect string) ([]string, error) {
	if req.Mode != "" && req.Mode != "statement" && req.Mode != "selection" && req.Mode != "script" {
		return nil, fmt.Errorf("未知 SQL 执行方式")
	}
	statements, err := scanSQL(req.SQL, dialect)
	if req.Mode == "statement" {
		position, valid := utf16Position(req.SQL, req.CursorOffset)
		if !valid {
			return nil, fmt.Errorf("SQL 光标位置无效")
		}
		for _, stmt := range statements {
			if position >= stmt.from && position <= stmt.to {
				return []string{stmt.text}, nil
			}
		}
		if err == nil && len(statements) > 0 && position >= statements[len(statements)-1].to {
			return []string{statements[len(statements)-1].text}, nil
		}
	}
	if err != nil {
		return nil, err
	}
	if len(statements) == 0 {
		return nil, fmt.Errorf("请输入要执行的 SQL")
	}
	if req.Mode == "statement" {
		return nil, fmt.Errorf("光标所在位置没有可执行的 SQL")
	}
	if len(statements) > 32 {
		return nil, fmt.Errorf("一次最多执行 32 条 SQL")
	}
	out := make([]string, len(statements))
	for i, stmt := range statements {
		out[i] = stmt.text
	}
	return out, nil
}

// Browser/CodeMirror offsets count UTF-16 code units, Go strings count bytes.
func utf16Position(text string, offset int) (int, bool) {
	if offset < 0 {
		return 0, false
	}
	units := 0
	for i, char := range text {
		if units == offset {
			return i, true
		}
		units++
		if char > 0xffff {
			units++
		}
		if units > offset {
			return 0, false
		}
	}
	return len(text), units == offset
}

// This is a statement-boundary scanner, not a SQL validator. It preserves common
// BEGIN/END routines, anonymous PL/SQL, quoted bodies and MySQL DELIMITER blocks.
// Incomplete/unsupported blocks are rejected before any script statement runs.
func scanSQL(text, dialect string) ([]sqlStatement, error) {
	if len(text) > 1<<20 {
		return nil, fmt.Errorf("SQL 不能超过 1 MiB")
	}
	mysql := dialect == "mysql"
	out := []sqlStatement{}
	start, delimiter := 0, ";"
	words := []string{}
	stack := []string{}
	creating, objectFound, routine, waitingBody := false, false, false, false
	boundary, expression, ended := true, false, false
	reset := func(next int) {
		start = next
		words, stack = nil, nil
		creating, objectFound, routine, waitingBody = false, false, false, false
		boundary, expression, ended = true, false, false
	}
	emit := func(end, next int, keepSemicolon bool) {
		if len(words) > 0 {
			body := strings.TrimSpace(text[start:end])
			if keepSemicolon {
				body += ";"
			}
			out = append(out, sqlStatement{body, start, end})
		}
		reset(next)
	}
	for i := 0; i < len(text); {
		c := text[i]
		// Client commands are recognized only on otherwise empty physical lines.
		if i == 0 || text[i-1] == '\n' {
			lineEnd := strings.IndexByte(text[i:], '\n')
			if lineEnd < 0 {
				lineEnd = len(text)
			} else {
				lineEnd += i
			}
			line := strings.TrimSpace(text[i:lineEnd])
			fields := strings.Fields(line)
			if len(fields) > 0 && strings.EqualFold(fields[0], "DELIMITER") {
				if !mysql || len(words) > 0 || len(fields) != 2 || len(fields[1]) > 16 || strings.ContainsAny(fields[1], "'\"`\\") {
					return out, fmt.Errorf("DELIMITER 仅用于 MySQL 脚本，需独占一行并指定有效分隔符")
				}
				delimiter = fields[1]
				reset(lineEnd)
				i = lineEnd
				continue
			}
			if !mysql && line == "/" {
				if len(stack) > 0 || waitingBody {
					return out, fmt.Errorf("SQL 语法块尚未闭合，不能在 / 处分割")
				}
				emit(i, lineEnd, false)
				i = lineEnd
				continue
			}
		}
		if strings.HasPrefix(text[i:], delimiter) && (delimiter != ";" || (len(stack) == 0 && !waitingBody)) {
			if len(stack) > 0 || waitingBody {
				return out, fmt.Errorf("SQL 语法块尚未闭合")
			}
			emit(i, i+len(delimiter), routine && !mysql && delimiter == ";")
			i += len(delimiter)
			continue
		}
		if unicode.IsSpace(rune(c)) {
			i++
			continue
		}
		if (mysql && c == '#') || (c == '-' && i+1 < len(text) && text[i+1] == '-' && (!mysql || i+2 == len(text) || unicode.IsSpace(rune(text[i+2])))) {
			end := strings.IndexByte(text[i:], '\n')
			if end < 0 {
				break
			}
			i += end
			continue
		}
		if c == '/' && i+1 < len(text) && text[i+1] == '*' {
			if i+2 < len(text) && text[i+2] == '!' {
				return out, fmt.Errorf("请直接书写 SQL，不使用 MySQL 可执行注释")
			}
			end := strings.Index(text[i+2:], "*/")
			if end < 0 {
				return out, fmt.Errorf("SQL 的注释未闭合")
			}
			i += end + 4
			continue
		}
		// Oracle-compatible alternative literals, e.g. q'[a;b]'.
		if !mysql && (c == 'q' || c == 'Q') && i+2 < len(text) && text[i+1] == '\'' {
			close := text[i+2]
			if paired, ok := map[byte]byte{'[': ']', '(': ')', '{': '}', '<': '>'}[close]; ok {
				close = paired
			}
			end := strings.Index(text[i+3:], string([]byte{close, '\''}))
			if end < 0 {
				return out, fmt.Errorf("SQL 的替代引号未闭合")
			}
			i += end + 5
			continue
		}
		// Dollar-quoted JDBC SQL bodies are opaque, including their semicolons.
		if !mysql && c == '$' {
			j := i + 1
			for j < len(text) && (sqlWord(text[j]) || text[j] >= '0' && text[j] <= '9') {
				j++
			}
			if j < len(text) && text[j] == '$' {
				tag := text[i : j+1]
				end := strings.Index(text[j+1:], tag)
				if end < 0 {
					return out, fmt.Errorf("SQL 的美元引号未闭合")
				}
				if routine && len(stack) == 0 {
					waitingBody = false
				}
				i = j + 1 + end + len(tag)
				continue
			}
		}
		if c == '\'' || c == '"' || c == '`' {
			quote, closed := c, false
			for i++; i < len(text); i++ {
				if mysql && text[i] == '\\' {
					i++
					continue
				}
				if text[i] == quote {
					if i+1 < len(text) && text[i+1] == quote {
						i++
						continue
					}
					i++
					closed = true
					break
				}
			}
			if !closed {
				return out, fmt.Errorf("SQL 的引号未闭合")
			}
			if routine && len(stack) == 0 && len(words) > 0 && (words[len(words)-1] == "AS" || words[len(words)-1] == "IS") {
				waitingBody = false
			}
			continue
		}
		if sqlWord(c) {
			j := i + 1
			for j < len(text) && !strings.HasPrefix(text[j:], delimiter) && (sqlWord(text[j]) || text[j] >= '0' && text[j] <= '9' || text[j] == '$') {
				j++
			}
			word := strings.ToUpper(text[i:j])
			if routine && waitingBody && (word == "PROCEDURE" || word == "FUNCTION") {
				return out, fmt.Errorf("暂不支持声明区中的嵌套子程序，请使用数据库专用客户端")
			}
			if len(words) == 0 {
				creating = word == "CREATE"
				if !mysql && (word == "DECLARE" || word == "BEGIN" && !transactionBegin(text[j:])) {
					routine, waitingBody = true, true
				}
			}
			if creating && !objectFound {
				switch word {
				case "PROCEDURE", "FUNCTION", "TRIGGER", "EVENT":
					routine, waitingBody, objectFound = true, !mysql, true
				case "PACKAGE", "TYPE":
					return out, fmt.Errorf("暂不支持包或类型定义脚本，请使用数据库专用客户端")
				case "TABLE", "VIEW", "INDEX", "DATABASE", "SCHEMA", "USER":
					objectFound = true
				}
			}
			if routine {
				if ended && (word == "IF" || word == "LOOP" || word == "CASE" || word == "WHILE" || word == "REPEAT") {
					ended = false
				} else {
					ended = false
					switch word {
					case "BEGIN":
						stack = append(stack, word)
						waitingBody = false
						boundary, expression = true, false
					case "END":
						if len(stack) == 0 {
							return out, fmt.Errorf("SQL 语法块中的 END 没有对应的起始语句")
						}
						stack = stack[:len(stack)-1]
						ended = true
						boundary = false
					case "CASE":
						if len(stack) > 0 {
							stack = append(stack, word)
						}
						boundary = false
					case "IF", "REPEAT", "WHILE":
						if len(stack) > 0 && boundary && (word != "WHILE" || mysql) {
							stack = append(stack, word)
						}
						boundary = false
					case "LOOP":
						if len(stack) > 0 {
							stack = append(stack, word)
						}
						boundary = true
					case "THEN", "ELSE":
						if !expression {
							boundary = true
						}
					default:
						if boundary && (word == "SELECT" || word == "INSERT" || word == "UPDATE" || word == "DELETE" || word == "SET" || word == "RETURN") {
							expression = true
						}
						boundary = false
					}
				}
			}
			words = append(words, word)
			i = j
			continue
		}
		if c == ';' {
			boundary, expression, ended = true, false, false
		}
		i++
	}
	if len(stack) > 0 || waitingBody {
		return out, fmt.Errorf("SQL 语法块未闭合，请检查 BEGIN/END、IF、LOOP 等结构")
	}
	emit(len(text), len(text), false)
	return out, nil
}

func sqlWord(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c == '_' || c >= 0x80
}

func transactionBegin(rest string) bool {
	rest = strings.TrimSpace(rest)
	return strings.HasPrefix(rest, ";") || firstKeyword(rest) == "WORK" || firstKeyword(rest) == "TRANSACTION"
}
