package sessionshare

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"syscall"
)

// GitSessionStore 基于 Git 仓库的会话共享存储实现（模式对齐 patchstore.GitPatchStore）。
// 仓库布局：sessions/<owner>.json，每用户一个文件（属主写，跨用户零冲突）。
type GitSessionStore struct {
	remoteURL string // 远程仓库地址
	localDir  string // 本地 clone 路径
	branch    string // 分支名

	opsMu sync.Mutex // 串行化 git 工作树操作（pull/write/push 不可交错）

	ownerMu          sync.Mutex
	owner            string // 解析后的当前用户标识（git config user.name，回退 OS 用户名）
	identityResolved bool   // 是否已探测过本机 git 提交身份
	injectIdentity   bool   // 本机无 git 身份时，以 owner 名注入提交身份
}

// NewGitSessionStore 创建 Git 会话共享存储。
func NewGitSessionStore(remoteURL, localDir, branch string) *GitSessionStore {
	if branch == "" {
		branch = "main"
	}
	return &GitSessionStore{
		remoteURL: remoteURL,
		localDir:  localDir,
		branch:    branch,
	}
}

// Owner 返回当前用户标识：优先本机 git config user.name，缺失回退 OS 用户名。
// 结果缓存；本机缺少 git 身份时后续提交以 owner 名注入。
func (s *GitSessionStore) Owner(ctx context.Context) string {
	s.ownerMu.Lock()
	defer s.ownerMu.Unlock()
	if s.owner != "" {
		return s.owner
	}

	if name, err := s.gitOutput(ctx, "config", "--get", "user.name"); err == nil && name != "" {
		s.owner = name
		return s.owner
	}

	if u, err := user.Current(); err == nil && u.Username != "" {
		s.owner = u.Username
		s.injectIdentity = true
		return s.owner
	}
	s.owner = "unknown"
	s.injectIdentity = true
	return s.owner
}

// resolveIdentity 在仓库就绪后探测本机 git 提交身份（缓存结果）。
// 缺失时置 injectIdentity，后续 commit 以 owner 名注入——
// 避免未配置 git user 的机器（常见于 Windows 普通用户）提交失败。
func (s *GitSessionStore) resolveIdentity(ctx context.Context) {
	s.ownerMu.Lock()
	defer s.ownerMu.Unlock()
	if s.identityResolved {
		return
	}
	s.identityResolved = true
	if name, err := s.gitOutput(ctx, "config", "--get", "user.name"); err == nil && name != "" {
		return
	}
	s.injectIdentity = true
}

// Upload 将一条记录 upsert 到本人文件：pull → 读本人文件 → 端点级合并
// （仅当新 LastLoginAt 不早于已有记录时覆盖）→ 写回 → commit → push。
func (s *GitSessionStore) Upload(ctx context.Context, session SharedSession) error {
	s.opsMu.Lock()
	defer s.opsMu.Unlock()

	if err := s.ensureRepo(ctx); err != nil {
		return fmt.Errorf("ensure repo: %w", err)
	}
	s.resolveIdentity(ctx)
	if session.Owner == "" {
		session.Owner = s.Owner(ctx)
	}
	if err := s.pull(ctx); err != nil {
		return fmt.Errorf("pull before upload: %w", err)
	}

	changed, err := s.upsertInOwnerFile(session)
	if err != nil {
		return err
	}
	if !changed {
		// 远端已有更新或相同的记录，无需推送
		slog.Debug("session share: remote record is newer or equal, skip upload",
			"host", session.Host, "owner", session.Owner)
		return nil
	}

	return s.commitAndPush(ctx, s.ownerFilePath(session.Owner), "sessions: %s -> %s:%d",
		session.Owner, session.Host, session.Port)
}

// Delete 从本人文件中移除指定端点条目。
func (s *GitSessionStore) Delete(ctx context.Context, endpoint SharedSession) error {
	s.opsMu.Lock()
	defer s.opsMu.Unlock()

	if err := s.ensureRepo(ctx); err != nil {
		return fmt.Errorf("ensure repo: %w", err)
	}
	s.resolveIdentity(ctx)
	if endpoint.Owner == "" {
		endpoint.Owner = s.Owner(ctx)
	}
	if err := s.pull(ctx); err != nil {
		return fmt.Errorf("pull before delete: %w", err)
	}

	filePath := s.ownerFilePath(endpoint.Owner)
	sessions := s.readOwnerFile(filePath)
	kept := make([]SharedSession, 0, len(sessions))
	for _, existing := range sessions {
		if !SameEndpoint(existing, endpoint) {
			kept = append(kept, existing)
		}
	}
	if len(kept) == len(sessions) {
		return nil // 目标条目不存在，视为成功
	}

	if err := s.writeOwnerFile(filePath, kept); err != nil {
		return err
	}
	return s.commitAndPush(ctx, filePath, "sessions: remove %s:%d by %s",
		endpoint.Host, endpoint.Port, endpoint.Owner)
}

// DownloadAll 拉取并返回仓库中全部用户的条目。
func (s *GitSessionStore) DownloadAll(ctx context.Context) ([]SharedSession, error) {
	s.opsMu.Lock()
	defer s.opsMu.Unlock()

	if err := s.ensureRepo(ctx); err != nil {
		return nil, fmt.Errorf("ensure repo: %w", err)
	}
	s.resolveIdentity(ctx)
	if err := s.pull(ctx); err != nil {
		return nil, fmt.Errorf("pull: %w", err)
	}

	sessionsDir := filepath.Join(s.localDir, "sessions")
	entries, err := os.ReadDir(sessionsDir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read sessions dir: %w", err)
	}

	var all []SharedSession
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".json") {
			continue
		}
		all = append(all, s.readOwnerFile(filepath.Join(sessionsDir, entry.Name()))...)
	}
	return all, nil
}

// --- 内部方法 ---

// upsertInOwnerFile 读取本人文件并按端点合并写入。
// 返回 changed=false 表示远端记录更新或相同、无需写入。
func (s *GitSessionStore) upsertInOwnerFile(session SharedSession) (bool, error) {
	filePath := s.ownerFilePath(session.Owner)
	sessions := s.readOwnerFile(filePath)

	replaced := false
	for i, existing := range sessions {
		if SameEndpoint(existing, session) {
			if session.LastLoginAt.Before(existing.LastLoginAt) {
				return false, nil // 远端记录更新，保留远端
			}
			sessions[i] = session
			replaced = true
			break
		}
	}
	if !replaced {
		sessions = append(sessions, session)
	}

	if err := s.writeOwnerFile(filePath, sessions); err != nil {
		return false, err
	}
	return true, nil
}

// ownerFilePath 本人文件的完整路径：sessions/<sanitized-owner>.json
func (s *GitSessionStore) ownerFilePath(owner string) string {
	return filepath.Join(s.localDir, "sessions", sanitizeFileName(owner)+".json")
}

func (s *GitSessionStore) readOwnerFile(path string) []SharedSession {
	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			slog.Warn("session share: failed to read owner file", "path", path, "error", err)
		}
		return nil
	}
	var sessions []SharedSession
	if err := json.Unmarshal(data, &sessions); err != nil {
		slog.Warn("session share: failed to parse owner file", "path", path, "error", err)
		return nil
	}
	return sessions
}

func (s *GitSessionStore) writeOwnerFile(path string, sessions []SharedSession) error {
	if len(sessions) == 0 {
		sessions = []SharedSession{} // 序列化为 [] 而非 null
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("create sessions dir: %w", err)
	}
	data, err := json.MarshalIndent(sessions, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal sessions: %w", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write owner file: %w", err)
	}
	return nil
}

// commitAndPush 对指定文件执行 add → commit → push（push 失败时 pull 后重试一次）。
func (s *GitSessionStore) commitAndPush(ctx context.Context, filePath string, msgFmt string, args ...interface{}) error {
	relPath, _ := filepath.Rel(s.localDir, filePath)
	relPath = filepath.ToSlash(relPath)

	if err := s.gitCmd(ctx, "add", relPath); err != nil {
		return fmt.Errorf("git add: %w", err)
	}

	// 无暂存变更时 commit 跳过（与 GitFeedbackStore 一致）
	if staged, err := s.gitOutput(ctx, "diff", "--cached", "--name-only"); err != nil {
		return fmt.Errorf("git diff --cached: %w", err)
	} else if strings.TrimSpace(staged) == "" {
		return nil
	}

	if err := s.gitCmd(ctx, "commit", "-m", fmt.Sprintf(msgFmt, args...)); err != nil {
		return fmt.Errorf("git commit: %w", err)
	}

	if err := s.push(ctx); err != nil {
		// 非 fast-forward（同用户双机并发）：pull 后重试一次
		if pullErr := s.pull(ctx); pullErr != nil {
			return fmt.Errorf("git push failed and retry pull also failed: %w (push: %w)", pullErr, err)
		}
		if pushErr := s.push(ctx); pushErr != nil {
			return fmt.Errorf("git push failed after retry pull: %w", pushErr)
		}
	}
	return nil
}

// ensureRepo 确保本地仓库存在且配置（remote URL、分支）与当前配置一致，
// 不一致则删库重 clone（与 GitPatchStore 相同策略）。
func (s *GitSessionStore) ensureRepo(ctx context.Context) error {
	gitDir := filepath.Join(s.localDir, ".git")
	if _, err := os.Stat(gitDir); err == nil {
		if !s.repoConfigMatches(ctx) {
			_ = os.RemoveAll(s.localDir)
		} else {
			return nil
		}
	}

	parentDir := filepath.Dir(s.localDir)
	if err := os.MkdirAll(parentDir, 0755); err != nil {
		return fmt.Errorf("create parent dir: %w", err)
	}
	return s.gitCmdInDir(ctx, parentDir, "clone", "-b", s.branch, s.remoteURL, s.localDir)
}

func (s *GitSessionStore) repoConfigMatches(ctx context.Context) bool {
	currentRemote, err := s.gitOutput(ctx, "remote", "get-url", "origin")
	if err != nil || currentRemote != s.remoteURL {
		return false
	}
	currentBranch, err := s.gitOutput(ctx, "branch", "--show-current")
	if err != nil || currentBranch != s.branch {
		return false
	}
	return true
}

// pull 执行 git pull --rebase，带工作树恢复：
//   - 拉取前清理中断的 rebase / 脏工作树（上次进程崩溃的残留），否则 pull 永久失败；
//   - rebase 冲突（同用户双机改同一文件）时，丢弃本地未推送提交并对齐远端。
//     丢弃是安全的：本地簿记以 last_pushed_at 驱动重传，未推送数据不丢失。
func (s *GitSessionStore) pull(ctx context.Context) error {
	_ = s.gitCmd(ctx, "rebase", "--abort")
	_ = s.gitCmd(ctx, "checkout", "--", ".")

	if err := s.gitCmd(ctx, "pull", "--rebase", "origin", s.branch); err != nil {
		_ = s.gitCmd(ctx, "rebase", "--abort")
		_ = s.gitCmd(ctx, "fetch", "origin")
		if resetErr := s.gitCmd(ctx, "reset", "--hard", "origin/"+s.branch); resetErr != nil {
			return fmt.Errorf("pull: %w (recovery reset also failed: %w)", err, resetErr)
		}
	}
	return nil
}

func (s *GitSessionStore) push(ctx context.Context) error {
	return s.gitCmd(ctx, "push", "origin", s.branch)
}

func (s *GitSessionStore) gitCmd(ctx context.Context, args ...string) error {
	return s.gitCmdInDir(ctx, s.localDir, args...)
}

func (s *GitSessionStore) gitOutput(ctx context.Context, args ...string) (string, error) {
	return s.gitOutputInDir(ctx, s.localDir, args...)
}

func (s *GitSessionStore) gitCmdInDir(ctx context.Context, dir string, args ...string) error {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}

	s.ownerMu.Lock()
	inject, name := s.injectIdentity, s.owner
	s.ownerMu.Unlock()
	if inject {
		if name == "" {
			name = "opscopilot-user"
		}
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME="+name,
			"GIT_AUTHOR_EMAIL="+"opscopilot@users.noreply",
			"GIT_COMMITTER_NAME="+name,
			"GIT_COMMITTER_EMAIL="+"opscopilot@users.noreply",
		)
	}

	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git %s: %s: %w", strings.Join(args, " "), strings.TrimSpace(stderr.String()), err)
	}
	return nil
}

func (s *GitSessionStore) gitOutputInDir(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %s: %w", strings.Join(args, " "), strings.TrimSpace(stderr.String()), err)
	}
	return strings.TrimSpace(stdout.String()), nil
}

var unsafeFileChar = regexp.MustCompile(`[^\p{Han}a-zA-Z0-9_\-]`)
var multiUnderscore = regexp.MustCompile(`_{2,}`)

// sanitizeFileName 清洗 owner 名作为文件名（保留中文/字母/数字/下划线/连字符）。
func sanitizeFileName(name string) string {
	name = strings.ReplaceAll(name, " ", "_")
	name = unsafeFileChar.ReplaceAllString(name, "_")
	name = multiUnderscore.ReplaceAllString(name, "_")
	return strings.Trim(name, "_")
}
