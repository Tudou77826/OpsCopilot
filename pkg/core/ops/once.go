package ops

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"opscopilot/pkg/core/security"
	"opscopilot/pkg/remote"
)

var ErrPolicy = errors.New("操作被现有策略拒绝")
var executionUser = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.-]{0,63}$`)

type OnceOptions struct {
	Directory, User, Elevation string
	Timeout                    time.Duration
	OutputBytes, LineBytes     int
}
type OnceResult struct {
	remote.CommandResult
	Status        string     `json:"status"`
	DurationMS    int64      `json:"durationMs"`
	LoginUser     string     `json:"loginUser"`
	RequestedUser string     `json:"requestedUser"`
	OutputMeta    OutputMeta `json:"outputMeta"`
}

func QuoteShell(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'" }

// PrepareOnce checks the user's actual command before any network access. The
// fixed cd/su/sudo wrapper is constructed by the host, never an approval bypass.
func PrepareOnce(cfg *remote.ConnectConfig, command string, options OnceOptions, policy *security.WhitelistManager) (remote.CommandRequest, error) {
	if cfg == nil || strings.TrimSpace(command) == "" || len(command) > 64<<10 || strings.ContainsRune(command, 0) {
		return remote.CommandRequest{}, errors.New("远程命令无效")
	}
	if policy == nil {
		return remote.CommandRequest{}, errors.New("命令策略不可用")
	}
	check := policy.Check(command, cfg.Host)
	if !check.Allowed || check.Category != security.CategoryReadOnly {
		if check.Reason == "" {
			check.Reason = "服务端定位仅允许只读命令"
		}
		return remote.CommandRequest{}, fmt.Errorf("%w：%s", ErrPolicy, check.Reason)
	}
	if strings.ContainsAny(options.Directory, "\x00\r\n") || len(options.Directory) > 4096 {
		return remote.CommandRequest{}, errors.New("远程目录无效")
	}
	wrapped := command
	if options.Directory != "" {
		if !strings.HasPrefix(options.Directory, "/") {
			return remote.CommandRequest{}, errors.New("远程目录必须为绝对路径")
		}
		// Group the complete command so `;` and `||` cannot escape a failed cd.
		wrapped = "cd " + QuoteShell(options.Directory) + " && (\n" + command + "\n)"
	}
	request := remote.CommandRequest{Command: wrapped, CaptureBytes: 64 << 10}
	if options.User == "" || options.User == cfg.User {
		if options.Elevation != "" {
			return request, errors.New("提权必须指定不同于登录账号的目标账号")
		}
		return request, nil
	}
	if !executionUser.MatchString(options.User) {
		return request, errors.New("目标账号无效")
	}
	if options.Elevation != "su" && options.Elevation != "sudo" {
		return request, errors.New("切换账号时必须明确指定 su 或 sudo")
	}
	password := cfg.RootPassword
	if options.Elevation == "sudo" {
		password = cfg.Password
	}
	if password == "" || strings.ContainsAny(password, "\r\n\x00") {
		return request, errors.New("未配置可用的提权凭据")
	}
	request.Input = password + "\n"
	if options.Elevation == "su" {
		request.Command = "su - " + QuoteShell(options.User) + " -c " + QuoteShell(wrapped)
	} else {
		request.Command = "sudo -S -p '' -u " + QuoteShell(options.User) + " -- sh -c " + QuoteShell(wrapped)
	}
	return request, nil
}
func ExecuteOnce(ctx context.Context, conn remote.Connection, cfg *remote.ConnectConfig, request remote.CommandRequest, options OnceOptions) (OnceResult, error) {
	out := OnceResult{LoginUser: cfg.User, RequestedUser: cfg.User}
	if options.User != "" {
		out.RequestedUser = options.User
	}
	runner, ok := conn.(remote.CommandRunner)
	if !ok {
		return out, errors.New("协议不支持有界的单次执行")
	}
	deadline := options.Timeout
	if deadline <= 0 {
		deadline = 120 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, deadline)
	defer cancel()
	start := time.Now()
	result, err := runner.RunCommand(ctx, request)
	out.CommandResult = result
	out.DurationMS = time.Since(start).Milliseconds()
	out.Status = "success"
	if err != nil {
		out.Status = "failed"
		if errors.Is(err, context.DeadlineExceeded) {
			out.Status = "timeout"
		} else if errors.Is(err, context.Canceled) {
			out.Status = "cancelled"
		} else if (result.StartAttempted || result.Started) && !result.CompletionKnown {
			out.Status = "unknown"
		}
	}
	budget := options.OutputBytes
	if budget <= 0 {
		budget = 10240
	}
	line := options.LineBytes
	if line < 400 {
		line = 500
	}
	processed := NewOutputController(budget, line, 5).Process(result.Stdout)
	out.Stdout = processed.Output
	out.OutputMeta = processed.Meta
	remaining := budget - len(out.Stdout)
	if remaining < 512 {
		remaining = 512
	}
	errorOutput := NewOutputController(remaining, line, 5).Process(result.Stderr)
	out.Stderr = errorOutput.Output
	out.Truncated = result.Truncated || processed.Meta.TruncatedLines > 0 || processed.Meta.LongLinesTruncated > 0 || errorOutput.Meta.TruncatedLines > 0 || errorOutput.Meta.LongLinesTruncated > 0
	return out, err
}
