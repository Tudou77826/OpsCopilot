// Package connectiondiag collects bounded, credential-filtered connection evidence.
// It never records SQL, wire payloads, RPC bodies or remote command output.
package connectiondiag

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
)

const MaxReportBytes = 64 << 10

type contextKey struct{}

type Trace struct {
	mu      sync.Mutex
	started time.Time
	lines   []string
	secrets []string
	seen    map[string]bool
	facts   map[string]string
	bytes   int
	closed  bool
	dropped bool
}

func New(ctx context.Context) (context.Context, *Trace) {
	t := &Trace{started: time.Now(), seen: map[string]bool{}, facts: map[string]string{}}
	return context.WithValue(ctx, contextKey{}, t), t
}

// Fact stores a small comparison field, not an instruction or a diagnosis.
// Callers supply fixed field names. Values are filtered again at rendering so
// credentials discovered later in the attempt are still removed.
func Fact(ctx context.Context, key, value string) {
	if t := From(ctx); t != nil {
		t.mu.Lock()
		defer t.mu.Unlock()
		if t.closed || len(key) > 80 || (len(t.facts) >= 48 && t.facts[key] == "") {
			return
		}
		value = strings.Map(func(r rune) rune {
			if unicode.IsControl(r) {
				return ' '
			}
			return r
		}, t.redact(value))
		if len(value) > 1024 {
			value = string([]rune(value)[:min(256, len([]rune(value)))]) + " [字段已截断]"
		}
		t.facts[key] = value
	}
}

func (t *Trace) Facts() map[string]string {
	t.mu.Lock()
	defer t.mu.Unlock()
	result := make(map[string]string, len(t.facts))
	for key, value := range t.facts {
		result[key] = t.redact(value)
	}
	return result
}

func From(ctx context.Context) *Trace {
	t, _ := ctx.Value(contextKey{}).(*Trace)
	return t
}

// WithTrace carries the same evidence collector across an independent operation
// context, preserving that operation's cancellation and deadline.
func WithTrace(ctx context.Context, trace *Trace) context.Context {
	if trace == nil {
		return ctx
	}
	return context.WithValue(ctx, contextKey{}, trace)
}

func AddSecrets(ctx context.Context, values ...string) {
	if t := From(ctx); t != nil {
		t.mu.Lock()
		defer t.mu.Unlock()
		for _, value := range values {
			if value == "" {
				continue
			}
			t.secrets = append(t.secrets, value, url.QueryEscape(value), url.PathEscape(value))
		}
		// Longest first prevents partial disclosure for overlapping credentials.
		sort.Slice(t.secrets, func(i, j int) bool { return len(t.secrets[i]) > len(t.secrets[j]) })
	}
}

// Event uses fixed application-owned codes. Untrusted text is filtered before storage.
func Event(ctx context.Context, code, format string, args ...any) {
	if t := From(ctx); t != nil {
		t.mu.Lock()
		defer t.mu.Unlock()
		if t.closed {
			return
		}
		t.seen[code] = true
		message := t.redact(fmt.Sprintf(format, args...))
		message = strings.Map(func(r rune) rune {
			if unicode.IsControl(r) {
				return ' '
			}
			return r
		}, message)
		if len(message) > 4096 {
			message = string([]rune(message)[:min(1024, len([]rune(message)))]) + " [事件已截断]"
		}
		line := fmt.Sprintf("%03d +%dms [%s] %s", len(t.lines)+1, time.Since(t.started).Milliseconds(), code, message)
		if t.bytes+len(line) > 48<<10 || len(t.lines) >= 160 {
			t.dropped = true
			return
		}
		t.lines = append(t.lines, line)
		t.bytes += len(line)
	}
}

func (t *Trace) Has(code string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.seen[code]
}

// Finish freezes the attempt. Late cleanup/session events cannot change its evidence.
func (t *Trace) Finish() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.closed = true
	result := "开始时间：" + t.started.Format(time.RFC3339Nano) + "\n" + strings.Join(t.lines, "\n")
	if t.dropped {
		result += "\n[达到日志上限，部分事件已省略]"
	}
	return t.redact(result)
}

func (t *Trace) Redact(value string) string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.redact(value)
}

var assignment = regexp.MustCompile(`(?i)\b(password|passwd|pwd|token|secret|access_token|user|username)\s*[=:]\s*(?:"[^"]*"|'[^']*'|[^\s;&,]+)`)
var urlUserInfo = regexp.MustCompile(`(//)[^/\s?;@]+@`)

func (t *Trace) redact(value string) string {
	for _, secret := range t.secrets {
		value = strings.ReplaceAll(value, secret, "[redacted]")
	}
	value = assignment.ReplaceAllString(value, "$1=[redacted]")
	return urlUserInfo.ReplaceAllString(value, "${1}[redacted]@")
}
