package sessionshare

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// initBareRepo 创建带 main 分支初始提交的裸仓库，返回其路径。
func initBareRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available, skipping E2E test")
	}

	tmpDir := t.TempDir()
	bareRepo := filepath.Join(tmpDir, "sessions-bare.git")

	runGit(t, tmpDir, "init", "--bare", bareRepo)
	cloneDir := filepath.Join(tmpDir, "init-clone")
	runGit(t, tmpDir, "clone", bareRepo, cloneDir)
	runGit(t, cloneDir, "config", "user.name", "Init User")
	runGit(t, cloneDir, "config", "user.email", "init@test.com")
	os.WriteFile(filepath.Join(cloneDir, ".gitkeep"), []byte(""), 0644)
	runGit(t, cloneDir, "add", ".gitkeep")
	runGit(t, cloneDir, "commit", "-m", "init")
	runGit(t, cloneDir, "branch", "-M", "main")
	runGit(t, cloneDir, "push", "-u", "origin", "main")
	os.RemoveAll(cloneDir)

	return bareRepo
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %s: %v", args, string(out), err)
	}
}

// TestGitSessionStoreE2E 端到端：双用户双工作副本的完整共享流程。
// 重点验证两个合并规则：
//  1. 上传时本人文件内同端点 upsert（旧的登录时间不覆盖新记录）
//  2. 跨用户 merge 时同端点取 LastLoginAt 最新者
func TestGitSessionStoreE2E(t *testing.T) {
	bareRepo := initBareRepo(t)
	tmpDir := filepath.Dir(bareRepo)
	ctx := context.Background()

	storeA := NewGitSessionStore(bareRepo, filepath.Join(tmpDir, "store-a"), "main")
	storeB := NewGitSessionStore(bareRepo, filepath.Join(tmpDir, "store-b"), "main")

	t1 := time.Date(2026, 8, 10, 9, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 8, 15, 9, 0, 0, 0, time.UTC)
	t3 := time.Date(2026, 8, 18, 9, 0, 0, 0, time.UTC)

	// 1. alice 上传两个端点（同一文件累积）
	ep1Alice := SharedSession{Owner: "alice", Name: "web-01", Host: "10.0.0.1", Port: 22, User: "root",
		SecretsEnc: "v1:alice-enc", LastLoginAt: t1, UpdatedAt: t1}
	ep2 := SharedSession{Owner: "alice", Name: "db-01", Host: "10.0.0.2", Port: 22, User: "root",
		LastLoginAt: t1, UpdatedAt: t1}
	if err := storeA.Upload(ctx, ep1Alice); err != nil {
		t.Fatalf("alice upload ep1: %v", err)
	}
	if err := storeA.Upload(ctx, ep2); err != nil {
		t.Fatalf("alice upload ep2: %v", err)
	}

	all, err := storeA.DownloadAll(ctx)
	if err != nil {
		t.Fatalf("DownloadAll: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("expected 2 entries after alice uploads, got %d", len(all))
	}

	// 2. alice 同端点重传更旧的登录时间 → 不覆盖
	ep1Stale := ep1Alice
	ep1Stale.LastLoginAt = t1.Add(-24 * time.Hour)
	ep1Stale.UpdatedAt = t1.Add(-24 * time.Hour)
	if err := storeA.Upload(ctx, ep1Stale); err != nil {
		t.Fatalf("alice upload stale: %v", err)
	}
	all, _ = storeA.DownloadAll(ctx)
	for _, s := range all {
		if s.Host == "10.0.0.1" && !s.LastLoginAt.Equal(t1) {
			t.Errorf("stale upload must not overwrite newer record: %v", s.LastLoginAt)
		}
	}

	// 3. bob（另一工作副本）上传同端点、更新的登录时间 → 独立文件共存
	ep1Bob := SharedSession{Owner: "bob", Name: "web-01", Host: "10.0.0.1", Port: 22, User: "root",
		SecretsEnc: "v1:bob-enc", LastLoginAt: t2, UpdatedAt: t2}
	if err := storeB.Upload(ctx, ep1Bob); err != nil {
		t.Fatalf("bob upload: %v", err)
	}

	// 4. 双方全量拉取后条目数一致（3 条：alice 2 + bob 1）
	allA, err := storeA.DownloadAll(ctx)
	if err != nil {
		t.Fatalf("alice DownloadAll after bob upload: %v", err)
	}
	if len(allA) != 3 {
		t.Fatalf("expected 3 raw entries, got %d", len(allA))
	}

	// 5. 跨用户 merge：同端点取最新登录时间（bob 的 t2 胜 alice 的 t1）
	merged := MergeSharedSessions(allA)
	if len(merged) != 2 {
		t.Fatalf("expected 2 merged entries, got %d", len(merged))
	}
	if merged[0].Owner != "bob" || !merged[0].LastLoginAt.Equal(t2) {
		t.Errorf("newest-login entry should win: got owner=%s at %v", merged[0].Owner, merged[0].LastLoginAt)
	}

	// 6. alice 以更新的时间重传 → merge 后回到 alice
	ep1AliceNew := ep1Alice
	ep1AliceNew.LastLoginAt = t3
	ep1AliceNew.UpdatedAt = t3
	if err := storeA.Upload(ctx, ep1AliceNew); err != nil {
		t.Fatalf("alice re-upload: %v", err)
	}
	allB, _ := storeB.DownloadAll(ctx)
	merged = MergeSharedSessions(allB)
	if len(merged) != 2 {
		t.Fatalf("expected 2 merged entries after re-upload, got %d", len(merged))
	}
	if merged[0].Owner != "alice" || !merged[0].LastLoginAt.Equal(t3) {
		t.Errorf("alice's newest login should win after re-upload: got owner=%s at %v",
			merged[0].Owner, merged[0].LastLoginAt)
	}

	// 7. alice 删除自己的端点 → 其他人不再看到 alice 的该端点
	if err := storeA.Delete(ctx, ep2); err != nil {
		t.Fatalf("alice delete ep2: %v", err)
	}
	allB, _ = storeB.DownloadAll(ctx)
	for _, s := range allB {
		if s.Host == "10.0.0.2" && s.Owner == "alice" {
			t.Errorf("deleted entry must disappear from other users' view")
		}
	}

	// 8. 密文原样存取（store 不解释密文，仅透传）
	found := false
	for _, s := range allB {
		if s.Owner == "alice" && s.Host == "10.0.0.1" {
			found = true
			if s.SecretsEnc != "v1:alice-enc" {
				t.Errorf("secrets ciphertext must roundtrip unchanged, got %q", s.SecretsEnc)
			}
		}
	}
	if !found {
		t.Error("alice's ep1 should be visible to bob")
	}
}

// TestGitSessionStoreRepoMismatch 配置变更（remote URL 变化）时删库重 clone。
func TestGitSessionStoreRepoMismatch(t *testing.T) {
	bareRepo := initBareRepo(t)
	tmpDir := filepath.Dir(bareRepo)
	ctx := context.Background()
	localDir := filepath.Join(tmpDir, "store-a")

	store := NewGitSessionStore(bareRepo, localDir, "main")
	entry := SharedSession{Owner: "alice", Host: "h", Port: 22, User: "u", LastLoginAt: time.Now()}
	if err := store.Upload(ctx, entry); err != nil {
		t.Fatalf("upload: %v", err)
	}

	// 指向另一个仓库 → 自动重 clone，旧数据被丢弃
	bareRepo2 := initBareRepo(t)
	store2 := NewGitSessionStore(bareRepo2, localDir, "main")
	all, err := store2.DownloadAll(ctx)
	if err != nil {
		t.Fatalf("DownloadAll after remote change: %v", err)
	}
	if len(all) != 0 {
		t.Errorf("switched remote should start empty, got %d entries", len(all))
	}
}
