package acp

import (
	"regexp"
	"strings"
	"sync"
)

var missingModuleLine = regexp.MustCompile(`^ModuleNotFoundError: No module named '([A-Za-z0-9_.-]{1,128})'$`)

// startupDiagnostic consumes stderr but retains only an allowlisted missing
// module name. Arbitrary traceback text, credentials and command output are
// never exposed through the UI or persisted to a diagnostic log.
type startupDiagnostic struct {
	mu        sync.Mutex
	line      []byte
	oversized bool
	module    string
}

func (d *startupDiagnostic) parseLine() {
	if !d.oversized {
		if match := missingModuleLine.FindSubmatch([]byte(strings.TrimSpace(string(d.line)))); len(match) == 2 {
			d.module = string(match[1])
		}
	}
}
func (d *startupDiagnostic) Write(data []byte) (int, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, b := range data {
		if b == '\n' {
			d.parseLine()
			d.line = nil
			d.oversized = false
			continue
		}
		if d.oversized {
			continue
		}
		if len(d.line) >= 4096 {
			d.line = nil
			d.oversized = true
			continue
		}
		d.line = append(d.line, b)
	}
	return len(data), nil
}
func (d *startupDiagnostic) summary() string {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.parseLine()
	if d.module == "" {
		return ""
	}
	return "Chrys 运行环境缺少 Python 模块「" + d.module + "」，尚未进入 ACP 握手。请补齐 Chrys 安装依赖。"
}
