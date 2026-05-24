package patchstore

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

// GitFeedbackStore manages feedback data in a Git repository alongside patches
type GitFeedbackStore struct {
	localDir  string // Same Git working tree as GitPatchStore
	remoteURL string // Remote repository URL
	branch    string // Branch name
}

// NewGitFeedbackStore creates a feedback store using the same Git repo as the patch store
func NewGitFeedbackStore(localDir, remoteURL, branch string) *GitFeedbackStore {
	if branch == "" {
		branch = "main"
	}
	return &GitFeedbackStore{
		localDir:  localDir,
		remoteURL: remoteURL,
		branch:    branch,
	}
}

// feedbackDir returns the feedback directory path
func (s *GitFeedbackStore) feedbackDir() string {
	return filepath.Join(s.localDir, "feedback")
}

// feedbackFilePath returns the path for a specific user's feedback file
// Format: feedback/{service}/{module}/{date}_{patchID}_{user}.json
func (s *GitFeedbackStore) feedbackFilePath(fb UserFeedback) string {
	serviceDir := sanitizeDirName(fb.Service)
	moduleDir := sanitizeDirName(fb.Module)
	fileName := fmt.Sprintf("%s_%s.json", fb.PatchID, sanitizeDirName(fb.User))
	return filepath.Join(s.feedbackDir(), serviceDir, moduleDir, fileName)
}

// GetFeedback retrieves all user feedback for a specific patch
func (s *GitFeedbackStore) GetFeedback(ctx context.Context, patchID string) ([]UserFeedback, error) {
	all, err := s.ListAllFeedback(ctx)
	if err != nil {
		return nil, err
	}

	var result []UserFeedback
	for _, fb := range all {
		if fb.PatchID == patchID {
			result = append(result, fb)
		}
	}
	return result, nil
}

// SaveFeedback saves or updates a user's feedback and pushes to remote.
// Full flow: ensure repo → pull → merge with existing → write file → add → commit → push
func (s *GitFeedbackStore) SaveFeedback(ctx context.Context, feedback UserFeedback) error {
	if err := s.ensureRepo(ctx); err != nil {
		return fmt.Errorf("ensure repo: %w", err)
	}

	if err := s.pull(ctx); err != nil {
		return fmt.Errorf("pull before save: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(s.feedbackFilePath(feedback)), 0755); err != nil {
		return fmt.Errorf("create feedback directory: %w", err)
	}

	filePath := s.feedbackFilePath(feedback)

	// Merge with existing feedback: preserve issues from other calls
	existing := s.readFeedbackFile(filePath)
	if existing != nil {
		if len(feedback.Issues) == 0 && len(existing.Issues) > 0 {
			feedback.Issues = existing.Issues
		}
	}

	data, err := json.MarshalIndent(feedback, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal feedback: %w", err)
	}

	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return fmt.Errorf("write feedback file: %w", err)
	}

	relPath, _ := filepath.Rel(s.localDir, filePath)
	relPath = filepath.ToSlash(relPath)

	if err := s.gitCmd(ctx, "add", relPath); err != nil {
		return fmt.Errorf("git add: %w", err)
	}

	commitMsg := fmt.Sprintf("feedback: %s/%s - %s by %s",
		feedback.Service, feedback.Module, feedback.PatchID, feedback.User)
	if err := s.commit(ctx, commitMsg); err != nil {
		return fmt.Errorf("git commit: %w", err)
	}

	if err := s.push(ctx); err != nil {
		// Pull and retry once on push failure (non-fast-forward)
		if pullErr := s.pull(ctx); pullErr != nil {
			return fmt.Errorf("git push failed and retry pull also failed: %w (push: %w)", pullErr, err)
		}
		if pushErr := s.push(ctx); pushErr != nil {
			return fmt.Errorf("git push failed after retry pull: %w", pushErr)
		}
	}

	return nil
}

// UpdateIssueStatus changes an issue's status within a user's feedback file
func (s *GitFeedbackStore) UpdateIssueStatus(ctx context.Context, patchID, issueID, status string) error {
	all, err := s.ListAllFeedback(ctx)
	if err != nil {
		return err
	}

	found := false
	for _, fb := range all {
		if fb.PatchID != patchID {
			continue
		}
		for i, issue := range fb.Issues {
			if issue.ID == issueID {
				fb.Issues[i].Status = status
				if err := s.SaveFeedback(ctx, fb); err != nil {
					return fmt.Errorf("save updated feedback: %w", err)
				}
				found = true
				break
			}
		}
		if found {
			break
		}
	}

	if !found {
		return fmt.Errorf("issue %s not found in patch %s", issueID, patchID)
	}
	return nil
}

// ListAllFeedback reads all feedback files from the feedback directory
func (s *GitFeedbackStore) ListAllFeedback(ctx context.Context) ([]UserFeedback, error) {
	fbDir := s.feedbackDir()
	if _, err := os.Stat(fbDir); os.IsNotExist(err) {
		return nil, nil
	}

	var allFeedback []UserFeedback
	err := filepath.WalkDir(fbDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			slog.Warn("error walking feedback directory", "path", path, "error", err)
			return nil
		}
		if d.IsDir() || !strings.HasSuffix(strings.ToLower(d.Name()), ".json") {
			return nil
		}

		fb := s.readFeedbackFile(path)
		if fb != nil {
			allFeedback = append(allFeedback, *fb)
		}
		return nil
	})

	return allFeedback, err
}

// readFeedbackFile reads and parses a single feedback JSON file
func (s *GitFeedbackStore) readFeedbackFile(path string) *UserFeedback {
	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			slog.Warn("failed to read feedback file", "path", path, "error", err)
		}
		return nil
	}

	var fb UserFeedback
	if err := json.Unmarshal(data, &fb); err != nil {
		slog.Warn("failed to parse feedback file", "path", path, "error", err)
		return nil
	}
	return &fb
}

// Pull fetches latest feedback from remote (exported for manual sync)
func (s *GitFeedbackStore) Pull(ctx context.Context) error {
	if err := s.ensureRepo(ctx); err != nil {
		return err
	}
	return s.pull(ctx)
}

// --- Git operations ---

// ensureRepo clones the remote repo if it doesn't exist locally yet
func (s *GitFeedbackStore) ensureRepo(ctx context.Context) error {
	gitDir := filepath.Join(s.localDir, ".git")
	if _, err := os.Stat(gitDir); err == nil {
		return nil // Already exists; GitPatchStore handles repo config validation
	}

	parentDir := filepath.Dir(s.localDir)
	if err := os.MkdirAll(parentDir, 0755); err != nil {
		return fmt.Errorf("create parent dir: %w", err)
	}

	return s.gitCmdInDir(ctx, parentDir, "clone", "-b", s.branch, s.remoteURL, s.localDir)
}

// pull executes git pull --rebase
func (s *GitFeedbackStore) pull(ctx context.Context) error {
	return s.gitCmd(ctx, "pull", "--rebase", "origin", s.branch)
}

// push executes git push
func (s *GitFeedbackStore) push(ctx context.Context) error {
	return s.gitCmd(ctx, "push", "origin", s.branch)
}

// commit executes git commit (no-op if nothing to commit)
func (s *GitFeedbackStore) commit(ctx context.Context, message string) error {
	// Check if there's anything staged to commit
	status, err := s.gitOutput(ctx, "diff", "--cached", "--name-only")
	if err != nil {
		return err
	}
	if strings.TrimSpace(status) == "" {
		return nil // Nothing to commit
	}
	return s.gitCmd(ctx, "commit", "-m", message)
}

func (s *GitFeedbackStore) gitCmd(ctx context.Context, args ...string) error {
	return s.gitCmdInDir(ctx, s.localDir, args...)
}

func (s *GitFeedbackStore) gitOutput(ctx context.Context, args ...string) (string, error) {
	return s.gitOutputInDir(ctx, s.localDir, args...)
}

func (s *GitFeedbackStore) gitCmdInDir(ctx context.Context, dir string, args ...string) error {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git %s: %s: %w", strings.Join(args, " "), strings.TrimSpace(stderr.String()), err)
	}
	return nil
}

func (s *GitFeedbackStore) gitOutputInDir(ctx context.Context, dir string, args ...string) (string, error) {
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
