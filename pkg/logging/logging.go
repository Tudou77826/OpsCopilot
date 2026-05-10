package logging

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/natefinch/lumberjack.v2"
)

// Config holds logging configuration.
type Config struct {
	Dir        string // log file directory
	Level      string // debug, info, warn, error (case-insensitive)
	DevMode    bool   // true: console + file; false: file only
	MaxSizeMB  int    // max size per log file before rotation (default 10)
	MaxBackups int    // max number of old log files to keep (default 5)
	Compress   bool   // compress rotated log files (default true)
}

// Setup initializes the global slog logger with lumberjack rotation.
// It must be called once at startup, before any slog calls.
func Setup(cfg Config) {
	if cfg.MaxSizeMB <= 0 {
		cfg.MaxSizeMB = 10
	}
	if cfg.MaxBackups <= 0 {
		cfg.MaxBackups = 5
	}

	// resolve log directory
	logDir := cfg.Dir
	if logDir == "" {
		logDir = "logs"
	}
	if !filepath.IsAbs(logDir) {
		execPath, err := os.Executable()
		var baseDir string
		if err == nil {
			baseDir = filepath.Dir(execPath)
		} else {
			baseDir, _ = os.Getwd()
		}
		logDir = filepath.Join(baseDir, logDir)
	}
	cfg.Dir = logDir

	if err := os.MkdirAll(logDir, 0755); err != nil {
		slog.Error("failed to create log directory", "dir", logDir, "error", err)
		return
	}

	logFile := filepath.Join(logDir, "opscopilot.log")

	lj := &lumberjack.Logger{
		Filename:   logFile,
		MaxSize:    cfg.MaxSizeMB,
		MaxBackups: cfg.MaxBackups,
		Compress:   cfg.Compress,
	}

	// determine log level
	level := parseLevel(cfg.Level)

	// allow env override
	if envLevel := os.Getenv("OPSCOPILOT_LOG_LEVEL"); envLevel != "" {
		level = parseLevel(envLevel)
	}

	// Use PrettyHandler for both console and file
	prettyHandler := newPrettyHandler(lj, level)

	if cfg.DevMode {
		// DevMode: console + file, both use pretty format
		consoleHandler := newPrettyHandler(os.Stdout, level)
		slog.SetDefault(slog.New(newFanOutHandler(prettyHandler, consoleHandler)))
	} else {
		// Production: file only
		slog.SetDefault(slog.New(prettyHandler))
	}

	slog.Info("logging initialized",
		"dir", logDir,
		"level", level.String(),
		"devMode", cfg.DevMode,
		"maxSizeMB", cfg.MaxSizeMB,
		"maxBackups", cfg.MaxBackups,
	)
}

// prettyHandler outputs logs in traditional format: [timestamp] [LEVEL] message key=value ...
type prettyHandler struct {
	w     io.Writer
	level slog.Level
}

func newPrettyHandler(w io.Writer, level slog.Level) *prettyHandler {
	return &prettyHandler{w: w, level: level}
}

func (h *prettyHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level
}

func (h *prettyHandler) Handle(_ context.Context, r slog.Record) error {
	// Format: [2026-05-11 01:08:33] [INFO] message key=value key=value
	ts := r.Time.Format("2006-01-02 15:04:05")
	levelStr := r.Level.String()

	// Build output
	var sb strings.Builder
	sb.WriteString("[")
	sb.WriteString(ts)
	sb.WriteString("] [")
	sb.WriteString(levelStr)
	sb.WriteString("] ")
	sb.WriteString(r.Message)

	// Append attributes as key=value
	r.Attrs(func(a slog.Attr) bool {
		sb.WriteString(" ")
		sb.WriteString(a.Key)
		sb.WriteString("=")
		sb.WriteString(formatAttrValue(a.Value))
		return true
	})

	sb.WriteString("\n")
	_, err := h.w.Write([]byte(sb.String()))
	return err
}

func (h *prettyHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	// For simplicity, we don't support WithAttrs in prettyHandler
	return h
}

func (h *prettyHandler) WithGroup(name string) slog.Handler {
	return h
}

// formatAttrValue formats a slog.Value for human-readable output
func formatAttrValue(v slog.Value) string {
	switch v.Kind() {
	case slog.KindString:
		s := v.String()
		// Quote strings with spaces, newlines, or special chars
		if strings.ContainsAny(s, " \t\n\r\"") {
			// Escape special characters
			s = strings.ReplaceAll(s, "\n", "\\n")
			s = strings.ReplaceAll(s, "\r", "\\r")
			s = strings.ReplaceAll(s, "\t", "\\t")
			s = strings.ReplaceAll(s, "\"", "\\\"")
			return fmt.Sprintf("\"%s\"", s)
		}
		return s
	case slog.KindInt64:
		return fmt.Sprintf("%d", v.Int64())
	case slog.KindFloat64:
		return fmt.Sprintf("%.2f", v.Float64())
	case slog.KindBool:
		return fmt.Sprintf("%v", v.Bool())
	case slog.KindTime:
		return v.Time().Format("2006-01-02 15:04:05")
	case slog.KindDuration:
		return Cost(v.Duration())
	default:
		return fmt.Sprintf("%v", v.Any())
	}
}

// fanOutHandler writes to multiple handlers
type fanOutHandler struct {
	handlers []slog.Handler
}

func newFanOutHandler(handlers ...slog.Handler) *fanOutHandler {
	return &fanOutHandler{handlers: handlers}
}

func (h *fanOutHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, handler := range h.handlers {
		if handler.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

func (h *fanOutHandler) Handle(ctx context.Context, r slog.Record) error {
	for _, handler := range h.handlers {
		if handler.Enabled(ctx, r.Level) {
			if err := handler.Handle(ctx, r); err != nil {
				return err
			}
		}
	}
	return nil
}

func (h *fanOutHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	newHandlers := make([]slog.Handler, len(h.handlers))
	for i, handler := range h.handlers {
		newHandlers[i] = handler.WithAttrs(attrs)
	}
	return newFanOutHandler(newHandlers...)
}

func (h *fanOutHandler) WithGroup(name string) slog.Handler {
	newHandlers := make([]slog.Handler, len(h.handlers))
	for i, handler := range h.handlers {
		newHandlers[i] = handler.WithGroup(name)
	}
	return newFanOutHandler(newHandlers...)
}

// parseLevel converts a level string to slog.Level.
// Defaults to slog.LevelInfo.
func parseLevel(s string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// Truncate shortens a string to max characters, appending "..." if truncated.
// Useful for safely logging large content (LLM responses, tool args, etc.).
func Truncate(s string, max int) string {
	if max <= 0 {
		max = 200
	}
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

// Cost formats a duration as human-readable string.
func Cost(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	return fmt.Sprintf("%.1fs", d.Seconds())
}