package opssidecar

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
	"opscopilot/pkg/core/ops"
)

const (
	defaultProjectExecTimeoutSec = 120
	maxProjectExecOutputBytes    = 256 * 1024
	maxProjectGitDiffBytes       = 512 * 1024
)

type projectOpTracker struct {
	mu      sync.Mutex
	cancels map[string]context.CancelFunc
}

func newProjectOpTracker() *projectOpTracker {
	return &projectOpTracker{cancels: map[string]context.CancelFunc{}}
}

func (t *projectOpTracker) register(opID string, cancel context.CancelFunc) {
	t.mu.Lock()
	t.cancels[opID] = cancel
	t.mu.Unlock()
}

func (t *projectOpTracker) unregister(opID string) {
	t.mu.Lock()
	delete(t.cancels, opID)
	t.mu.Unlock()
}

func (t *projectOpTracker) cancel(opID string) bool {
	t.mu.Lock()
	fn := t.cancels[opID]
	t.mu.Unlock()
	if fn != nil {
		fn()
		return true
	}
	return false
}

func (t *projectOpTracker) cancelAll() {
	t.mu.Lock()
	all := make([]context.CancelFunc, 0, len(t.cancels))
	for id, fn := range t.cancels {
		all = append(all, fn)
		delete(t.cancels, id)
	}
	t.mu.Unlock()
	for _, fn := range all {
		fn()
	}
}

func (t *projectOpTracker) activeCount() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.cancels)
}

type boundedBuffer struct {
	limit     int
	buf       bytes.Buffer
	truncated bool
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	remaining := b.limit - b.buf.Len()
	if remaining <= 0 {
		b.truncated = true
		return len(p), nil
	}
	if len(p) > remaining {
		_, _ = b.buf.Write(p[:remaining])
		b.truncated = true
		return len(p), nil
	}
	_, _ = b.buf.Write(p)
	return len(p), nil
}

type projectExecResult struct {
	OpID      string `json:"opId"`
	Stdout    string `json:"stdout"`
	Stderr    string `json:"stderr"`
	ExitCode  int    `json:"exitCode"`
	TimedOut  bool   `json:"timedOut"`
	Cancelled bool   `json:"cancelled"`
	Truncated bool   `json:"truncated"`
}

func (s *Service) runProjectCommand(
	parentCtx context.Context,
	opID string,
	connectionID string,
	workDir string,
	command string,
	timeoutSec int,
	maxBytes int,
) (projectExecResult, error) {
	if strings.TrimSpace(command) == "" || len(command) > 8192 || strings.ContainsRune(command, 0) {
		return projectExecResult{}, errors.New("命令为空或超出长度限制")
	}
	if opID == "" {
		opID = "op-" + newRequestID()
	}
	if timeoutSec <= 0 {
		timeoutSec = defaultProjectExecTimeoutSec
	}
	if maxBytes <= 0 {
		maxBytes = maxProjectExecOutputBytes
	}

	ctx, cancel := context.WithTimeout(parentCtx, time.Duration(timeoutSec)*time.Second)
	s.projectOps.register(opID, cancel)
	defer func() {
		s.projectOps.unregister(opID)
		cancel()
	}()

	var res projectExecResult
	res.OpID = opID

	err := s.withProjectSSH(connectionID, func(client *ssh.Client) error {
		session, err := client.NewSession()
		if err != nil {
			return err
		}
		defer session.Close()

		stdout := &boundedBuffer{limit: maxBytes}
		stderr := &boundedBuffer{limit: maxBytes}
		session.Stdout = stdout
		session.Stderr = stderr

		fullCmd := command
		if strings.TrimSpace(workDir) != "" {
			fullCmd = "cd " + ops.QuoteShell(cleanRemotePath(workDir)) + " && (\n" + command + "\n)"
		}

		if err := session.Start(fullCmd); err != nil {
			return err
		}
		done := make(chan error, 1)
		go func() {
			done <- session.Wait()
		}()

		var waitErr error
		select {
		case waitErr = <-done:
		case <-ctx.Done():
			_ = session.Signal(ssh.SIGKILL)
			_ = session.Close()
			waitErr = ctx.Err()
		}

		res.Stdout = stdout.buf.String()
		res.Stderr = stderr.buf.String()
		res.Truncated = stdout.truncated || stderr.truncated

		if errors.Is(waitErr, context.DeadlineExceeded) {
			res.TimedOut = true
			res.ExitCode = -1
			return nil
		}
		if errors.Is(waitErr, context.Canceled) {
			res.Cancelled = true
			res.ExitCode = -1
			return nil
		}
		if waitErr != nil {
			var exitErr *ssh.ExitError
			if errors.As(waitErr, &exitErr) {
				res.ExitCode = exitErr.ExitStatus()
				return nil
			}
			return waitErr
		}
		res.ExitCode = 0
		return nil
	})
	return res, err
}

func (s *Service) handleProjectExec(req Request) (Response, bool) {
	var params struct {
		OpID         string `json:"opId,omitempty"`
		ConnectionID string `json:"connectionId"`
		RemoteRoot   string `json:"remoteRoot"`
		Cwd          string `json:"cwd,omitempty"`
		Command      string `json:"command"`
		TimeoutSec   int    `json:"timeoutSec,omitempty"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil ||
		strings.TrimSpace(params.ConnectionID) == "" ||
		strings.TrimSpace(params.RemoteRoot) == "" ||
		strings.TrimSpace(params.Command) == "" {
		return invalidParams(req.ID, "connectionId、remoteRoot 与 command 必填"), false
	}

	workDir := cleanRemotePath(params.RemoteRoot)
	if strings.TrimSpace(params.Cwd) != "" && strings.TrimSpace(params.Cwd) != "." {
		resolved, err := resolveProjectRelPath(workDir, params.Cwd)
		if err != nil {
			return invalidParams(req.ID, err.Error()), false
		}
		workDir = resolved
	}

	res, err := s.runProjectCommand(
		context.Background(),
		params.OpID,
		params.ConnectionID,
		workDir,
		params.Command,
		params.TimeoutSec,
		maxProjectExecOutputBytes,
	)
	if err != nil {
		return classifyProjectFSError(req.ID, err), false
	}
	return okResponse(req.ID, res), false
}

func (s *Service) handleProjectExecCancel(req Request) (Response, bool) {
	var params struct {
		OpID string `json:"opId"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil || strings.TrimSpace(params.OpID) == "" {
		return invalidParams(req.ID, "opId 必填"), false
	}
	cancelled := s.projectOps.cancel(params.OpID)
	return okResponse(req.ID, map[string]any{"cancelled": cancelled}), false
}

// ---- 远端 Git 查询 ----

type projectGitChange struct {
	Path    string  `json:"path"`
	OldPath *string `json:"oldPath"`
	Source  string  `json:"source"` // staged | unstaged | untracked | conflict
	Status  string  `json:"status"`
}

func mapGitStatusCode(code byte) string {
	switch code {
	case 'A':
		return "added"
	case 'D':
		return "deleted"
	case 'R':
		return "renamed"
	case 'C':
		return "copied"
	case 'U':
		return "conflict"
	case '?':
		return "untracked"
	default:
		return "modified"
	}
}

func parseGitPorcelainZ(raw string) []projectGitChange {
	parts := strings.Split(raw, "\x00")
	changes := make([]projectGitChange, 0)
	for i := 0; i < len(parts); i++ {
		entry := parts[i]
		if len(entry) < 4 {
			continue
		}
		x := entry[0]
		y := entry[1]
		filePath := entry[3:]
		var oldPath *string
		if (x == 'R' || x == 'C' || y == 'R' || y == 'C') && i+1 < len(parts) {
			prev := parts[i+1]
			oldPath = &prev
			i++
		}
		if x == '?' && y == '?' {
			changes = append(changes, projectGitChange{
				Path: filePath, OldPath: nil, Source: "untracked", Status: "untracked",
			})
			continue
		}
		if x == 'U' || y == 'U' || (x == 'A' && y == 'A') || (x == 'D' && y == 'D') {
			changes = append(changes, projectGitChange{
				Path: filePath, OldPath: oldPath, Source: "conflict", Status: "conflict",
			})
			continue
		}
		if x != ' ' && x != '?' {
			changes = append(changes, projectGitChange{
				Path: filePath, OldPath: oldPath, Source: "staged", Status: mapGitStatusCode(x),
			})
		}
		if y != ' ' && y != '?' {
			changes = append(changes, projectGitChange{
				Path: filePath, OldPath: oldPath, Source: "unstaged", Status: mapGitStatusCode(y),
			})
		}
	}
	return changes
}

func (s *Service) handleProjectGitStatus(req Request) (Response, bool) {
	var params struct {
		ConnectionID string `json:"connectionId"`
		RemoteRoot   string `json:"remoteRoot"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil ||
		strings.TrimSpace(params.ConnectionID) == "" ||
		strings.TrimSpace(params.RemoteRoot) == "" {
		return invalidParams(req.ID, "connectionId 与 remoteRoot 必填"), false
	}

	root := cleanRemotePath(params.RemoteRoot)
	// 检查是否 Git 仓库
	topRes, err := s.runProjectCommand(context.Background(), "", params.ConnectionID, root, "git rev-parse --show-toplevel", 15, 4096)
	if err != nil {
		return classifyProjectFSError(req.ID, err), false
	}
	if topRes.ExitCode != 0 {
		return okResponse(req.ID, map[string]any{
			"repoRoot":   nil,
			"branch":     "",
			"changes":    []projectGitChange{},
			"watchPaths": []string{},
		}), false
	}
	repoRoot := strings.TrimSpace(topRes.Stdout)

	branchRes, err := s.runProjectCommand(context.Background(), "", params.ConnectionID, root, "git symbolic-ref --short HEAD 2>/dev/null || git rev-parse --short HEAD 2>/dev/null || echo HEAD", 15, 4096)
	if err != nil {
		return classifyProjectFSError(req.ID, err), false
	}
	branch := strings.TrimSpace(branchRes.Stdout)

	statusRes, err := s.runProjectCommand(context.Background(), "", params.ConnectionID, root, "git status --porcelain=v1 -z -uall", 20, maxProjectGitDiffBytes)
	if err != nil {
		return classifyProjectFSError(req.ID, err), false
	}
	changes := parseGitPorcelainZ(statusRes.Stdout)

	return okResponse(req.ID, map[string]any{
		"repoRoot":   repoRoot,
		"branch":     branch,
		"changes":    changes,
		"watchPaths": []string{},
		"statsWarning": func() any {
			if statusRes.Truncated {
				return "远端 Git 状态输出已截断"
			}
			return nil
		}(),
	}), false
}

func (s *Service) handleProjectGitDiff(req Request) (Response, bool) {
	var params struct {
		ConnectionID string `json:"connectionId"`
		RemoteRoot   string `json:"remoteRoot"`
		Path         string `json:"path"`
		Source       string `json:"source"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil ||
		strings.TrimSpace(params.ConnectionID) == "" ||
		strings.TrimSpace(params.RemoteRoot) == "" ||
		strings.TrimSpace(params.Path) == "" {
		return invalidParams(req.ID, "connectionId、remoteRoot 与 path 必填"), false
	}

	root := cleanRemotePath(params.RemoteRoot)
	relPath := strings.TrimPrefix(path.Clean(strings.ReplaceAll(params.Path, "\\", "/")), "/")
	if relPath == "" || relPath == "." || strings.HasPrefix(relPath, "..") {
		return invalidParams(req.ID, "无效的变更路径"), false
	}

	quoted := ops.QuoteShell(relPath)
	var diffCmd string
	var beforeCmd string
	var afterCmd string

	switch params.Source {
	case "staged":
		diffCmd = "git diff --cached --no-color -- " + quoted
		beforeCmd = "git show HEAD:" + quoted + " 2>/dev/null || true"
		afterCmd = "git show :0:" + quoted + " 2>/dev/null || true"
	case "untracked":
		diffCmd = "git diff --no-index --no-color -- /dev/null " + quoted + " || true"
		beforeCmd = ""
		afterCmd = "cat -- " + quoted + " 2>/dev/null || true"
	default:
		diffCmd = "git diff --no-color -- " + quoted
		beforeCmd = "git show :0:" + quoted + " 2>/dev/null || git show HEAD:" + quoted + " 2>/dev/null || true"
		afterCmd = "cat -- " + quoted + " 2>/dev/null || true"
	}

	diffRes, err := s.runProjectCommand(context.Background(), "", params.ConnectionID, root, diffCmd, 20, maxProjectGitDiffBytes)
	if err != nil {
		return classifyProjectFSError(req.ID, err), false
	}

	var beforeContent *string
	if beforeCmd != "" {
		if bRes, bErr := s.runProjectCommand(context.Background(), "", params.ConnectionID, root, beforeCmd, 15, maxProjectGitDiffBytes); bErr == nil && bRes.ExitCode == 0 {
			text := bRes.Stdout
			beforeContent = &text
		}
	}
	var afterContent *string
	if afterCmd != "" {
		if aRes, aErr := s.runProjectCommand(context.Background(), "", params.ConnectionID, root, afterCmd, 15, maxProjectGitDiffBytes); aErr == nil && aRes.ExitCode == 0 {
			text := aRes.Stdout
			afterContent = &text
		}
	}

	var notice any
	if diffRes.Truncated {
		notice = fmt.Sprintf("远端 diff 输出超过 %d KiB，已截断", maxProjectGitDiffBytes/1024)
	}
	return okResponse(req.ID, map[string]any{
		"patch":         diffRes.Stdout,
		"notice":        notice,
		"beforeContent": beforeContent,
		"afterContent":  afterContent,
	}), false
}
