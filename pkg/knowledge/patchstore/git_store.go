package patchstore

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// GitPatchStore 基于 Git 仓库的补丁存储实现
type GitPatchStore struct {
	remoteURL   string // 远程仓库地址
	localDir    string // 本地 clone 路径（patches 仓库的工作目录）
	branch      string // 分支名
	authorName  string
	authorEmail string
}

// NewGitPatchStore 创建 Git 补丁存储
func NewGitPatchStore(remoteURL, localDir, branch, authorName, authorEmail string) *GitPatchStore {
	if branch == "" {
		branch = "main"
	}
	return &GitPatchStore{
		remoteURL:   remoteURL,
		localDir:    localDir,
		branch:      branch,
		authorName:  authorName,
		authorEmail: authorEmail,
	}
}

// Upload 上传单条补丁：pull → 写文件 → add → commit → push
func (s *GitPatchStore) Upload(ctx context.Context, patch Patch) error {
	if err := s.ensureRepo(ctx); err != nil {
		return fmt.Errorf("ensure repo: %w", err)
	}

	if patch.Author == "" {
		authorName, _, err := s.getGitUser(ctx)
		if err != nil {
			return fmt.Errorf("get git user: %w", err)
		}
		patch.Author = authorName
	}

	if err := s.pull(ctx); err != nil {
		return fmt.Errorf("pull before upload: %w", err)
	}

	filePath := s.patchFilePath(patch)
	if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
		return fmt.Errorf("create patch directory: %w", err)
	}

	content := s.formatPatchFile(patch)
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		return fmt.Errorf("write patch file: %w", err)
	}

	relPath, _ := filepath.Rel(s.localDir, filePath)
	relPath = filepath.ToSlash(relPath)

	if err := s.gitCmd(ctx, "add", relPath); err != nil {
		return fmt.Errorf("git add: %w", err)
	}

	commitMsg := fmt.Sprintf("archive: %s/%s - %s", patch.Service, patch.Module, patch.ID)
	if err := s.commit(ctx, commitMsg); err != nil {
		return fmt.Errorf("git commit: %w", err)
	}

	if err := s.push(ctx); err != nil {
		return fmt.Errorf("git push: %w", err)
	}

	return nil
}

// Download 下载指定时间之后的补丁（增量同步）
func (s *GitPatchStore) Download(ctx context.Context, since time.Time) ([]Patch, error) {
	if err := s.ensureRepo(ctx); err != nil {
		return nil, fmt.Errorf("ensure repo: %w", err)
	}

	if err := s.pull(ctx); err != nil {
		return nil, fmt.Errorf("pull: %w", err)
	}

	all, err := s.readAllPatches()
	if err != nil {
		return nil, err
	}

	var result []Patch
	for _, p := range all {
		if !p.Timestamp.Before(since) {
			result = append(result, p)
		}
	}
	return result, nil
}

// DownloadAll 下载全部补丁
func (s *GitPatchStore) DownloadAll(ctx context.Context) ([]Patch, error) {
	if err := s.ensureRepo(ctx); err != nil {
		return nil, fmt.Errorf("ensure repo: %w", err)
	}

	if err := s.pull(ctx); err != nil {
		return nil, fmt.Errorf("pull: %w", err)
	}

	return s.readAllPatches()
}

// LastSyncTime 获取最近一次补丁的时间戳
func (s *GitPatchStore) LastSyncTime(ctx context.Context) (time.Time, error) {
	patches, err := s.DownloadAll(ctx)
	if err != nil {
		return time.Time{}, err
	}

	var latest time.Time
	for _, p := range patches {
		if p.Timestamp.After(latest) {
			latest = p.Timestamp
		}
	}
	return latest, nil
}

// --- 内部方法 ---

// ensureRepo 确保本地仓库存在且配置（remote URL、分支）与当前配置一致
func (s *GitPatchStore) ensureRepo(ctx context.Context) error {
	gitDir := filepath.Join(s.localDir, ".git")
	if _, err := os.Stat(gitDir); err == nil {
		// 已存在：校验 remote URL 和分支是否匹配
		if !s.repoConfigMatches(ctx) {
			_ = os.RemoveAll(s.localDir)
		} else {
			return nil
		}
	}

	// clone 仓库（git clone 会自动创建目标目录）
	parentDir := filepath.Dir(s.localDir)
	if err := os.MkdirAll(parentDir, 0755); err != nil {
		return fmt.Errorf("create parent dir: %w", err)
	}

	return s.gitCmdInDir(ctx, parentDir, "clone", "-b", s.branch, s.remoteURL, s.localDir)
}

// repoConfigMatches 检查本地仓库的 remote URL 和当前分支是否与配置一致
func (s *GitPatchStore) repoConfigMatches(ctx context.Context) bool {
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

// pull 执行 git pull --rebase
func (s *GitPatchStore) pull(ctx context.Context) error {
	return s.gitCmd(ctx, "pull", "--rebase", "origin", s.branch)
}

// push 执行 git push
func (s *GitPatchStore) push(ctx context.Context) error {
	return s.gitCmd(ctx, "push", "origin", s.branch)
}

// commit 执行 git commit
func (s *GitPatchStore) commit(ctx context.Context, message string) error {
	return s.gitCmd(ctx, "commit", "-m", message)
}

// patchFilePath 生成补丁文件路径
func (s *GitPatchStore) patchFilePath(patch Patch) string {
	serviceDir := sanitizeDirName(patch.Service)
	moduleDir := sanitizeDirName(patch.Module)
	dateStr := patch.Timestamp.Format("2006-01-02")
	fileName := fmt.Sprintf("%s_%s.md", dateStr, patch.ID)
	return filepath.Join(s.localDir, "patches", serviceDir, moduleDir, fileName)
}

// formatPatchFile 生成补丁文件内容
func (s *GitPatchStore) formatPatchFile(patch Patch) string {
	return formatPatchFile(patch)
}

func formatPatchFile(patch Patch) string {
	var sb strings.Builder
	sb.WriteString("---\n")
	fmt.Fprintf(&sb, "service: %q\n", patch.Service)
	fmt.Fprintf(&sb, "module: %q\n", patch.Module)
	sb.WriteString("type: archive\n")
	fmt.Fprintf(&sb, "date: %q\n", patch.Timestamp.Format("2006-01-02"))
	fmt.Fprintf(&sb, "timestamp: %q\n", patch.Timestamp.Format(time.RFC3339))
	fmt.Fprintf(&sb, "author: %q\n", patch.Author)
	fmt.Fprintf(&sb, "patch_id: %q\n", patch.ID)
	sb.WriteString("---\n\n")
	sb.WriteString(patch.Content)
	if !strings.HasSuffix(patch.Content, "\n") {
		sb.WriteString("\n")
	}
	return sb.String()
}

// readAllPatches 从本地仓库读取所有补丁文件
func (s *GitPatchStore) readAllPatches() ([]Patch, error) {
	patchesDir := filepath.Join(s.localDir, "patches")
	if _, err := os.Stat(patchesDir); os.IsNotExist(err) {
		return nil, nil
	}

	var patches []Patch
	err := filepath.Walk(patchesDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		patch, err := parsePatchFile(data)
		if err != nil {
			return nil // 跳过无法解析的文件
		}
		patches = append(patches, patch)
		return nil
	})

	return patches, err
}

// gitCmd 在仓库目录执行 git 命令
func (s *GitPatchStore) gitCmd(ctx context.Context, args ...string) error {
	return s.gitCmdInDir(ctx, s.localDir, args...)
}

func (s *GitPatchStore) gitOutput(ctx context.Context, args ...string) (string, error) {
	return s.gitOutputInDir(ctx, s.localDir, args...)
}

// gitCmdInDir 在指定目录执行 git 命令
func (s *GitPatchStore) gitCmdInDir(ctx context.Context, dir string, args ...string) error {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir

	if s.authorName != "" {
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME="+s.authorName,
			"GIT_AUTHOR_EMAIL="+s.authorEmail,
			"GIT_COMMITTER_NAME="+s.authorName,
			"GIT_COMMITTER_EMAIL="+s.authorEmail,
		)
	}

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git %s: %s: %w", strings.Join(args, " "), strings.TrimSpace(stderr.String()), err)
	}
	return nil
}

func (s *GitPatchStore) gitOutputInDir(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %s: %w", strings.Join(args, " "), strings.TrimSpace(stderr.String()), err)
	}

	return strings.TrimSpace(stdout.String()), nil
}

// getGitUser 读取本机 git config 中的用户信息，优先使用仓库可见配置。
func (s *GitPatchStore) getGitUser(ctx context.Context) (string, string, error) {
	name, err := s.gitOutput(ctx, "config", "--get", "user.name")
	if err != nil || name == "" {
		if s.authorName != "" {
			return s.authorName, s.authorEmail, nil
		}
		return "", "", fmt.Errorf("git config user.name not found")
	}

	email, _ := s.gitOutput(ctx, "config", "--get", "user.email")
	return name, email, nil
}

// --- 解析 ---

// parsePatchFile 从 Markdown 文件内容解析补丁
func parsePatchFile(data []byte) (Patch, error) {
	content := string(data)
	frontMatter, body := parseFrontMatter(content)

	ts, err := time.Parse(time.RFC3339, frontMatter["timestamp"])
	if err != nil || ts.IsZero() {
		ts, _ = time.ParseInLocation("2006-01-02", frontMatter["date"], time.Local)
	}

	return Patch{
		ID:        frontMatter["patch_id"],
		Service:   frontMatter["service"],
		Module:    frontMatter["module"],
		Author:    frontMatter["author"],
		Timestamp: ts,
		Content:   strings.TrimSpace(body),
	}, nil
}

// parseFrontMatter 提取 YAML front matter 为 map
func parseFrontMatter(content string) (map[string]string, string) {
	// 统一行尾为 LF
	content = strings.ReplaceAll(content, "\r\n", "\n")

	if !strings.HasPrefix(content, "---\n") {
		return nil, content
	}

	end := strings.Index(content[4:], "\n---\n")
	if end < 0 {
		return nil, content
	}

	fmText := content[4 : 4+end]
	body := content[4+end+5:]

	fm := make(map[string]string)
	for line := range strings.SplitSeq(fmText, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		key, val, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)
		val = strings.Trim(val, "\"")
		fm[key] = val
	}

	return fm, body
}

var unsafeDirChar = regexp.MustCompile(`[^\p{Han}a-zA-Z0-9_\-]`)

func sanitizeDirName(name string) string {
	name = strings.ReplaceAll(name, " ", "_")
	name = unsafeDirChar.ReplaceAllString(name, "_")
	for strings.Contains(name, "__") {
		name = strings.ReplaceAll(name, "__", "_")
	}
	return strings.Trim(name, "_")
}
