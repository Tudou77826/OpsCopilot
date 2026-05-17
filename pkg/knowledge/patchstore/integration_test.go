package patchstore

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"testing"
	"time"
)

// TestGitPatchStoreE2E 端到端测试：真实 Git 仓库的完整流程
// 需要 git 命令可用
func TestGitPatchStoreE2E(t *testing.T) {
	// 检查 git 是否可用
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available, skipping E2E test")
	}

	tmpDir := t.TempDir()
	bareRepo := filepath.Join(tmpDir, "knowledge-bare.git")
	localDir := filepath.Join(tmpDir, "patchstore")

	// 配置 git 用户（CI 环境可能没有全局配置）
	setupGitUser := func(dir string) {
		runGit(t, dir, "config", "user.name", "Test User")
		runGit(t, dir, "config", "user.email", "test@example.com")
	}

	// 1. 创建 bare 仓库
	t.Log("Step 1: Creating bare Git repo")
	runGit(t, tmpDir, "init", "--bare", bareRepo)

	// 初始化 bare 仓库的默认分支
	cloneDir := filepath.Join(tmpDir, "init-clone")
	runGit(t, tmpDir, "clone", bareRepo, cloneDir)
	setupGitUser(cloneDir)
	// 创建初始 .gitkeep 以确保分支存在
	os.WriteFile(filepath.Join(cloneDir, ".gitkeep"), []byte(""), 0644)
	runGit(t, cloneDir, "add", ".gitkeep")
	runGit(t, cloneDir, "commit", "-m", "init")
	runGit(t, cloneDir, "branch", "-M", "main")
	runGit(t, cloneDir, "push", "-u", "origin", "main")
	os.RemoveAll(cloneDir)

	// 2. 创建 GitPatchStore（模拟用户 A）
	t.Log("Step 2: Creating GitPatchStore (User A)")
	store := NewGitPatchStore(bareRepo, localDir, "main", "alice", "alice@test.com")
	ctx := context.Background()

	// 3. Upload 第一个补丁（ensureRepo 会 clone 仓库，之后再设置 git user）
	t.Log("Step 3: Uploading first patch")
	patch1 := Patch{
		ID:        "a1b2c3d4",
		Service:   "Payment Service",
		Module:    "核心支付模块",
		Author:    "alice",
		Timestamp: time.Date(2026, 5, 17, 10, 0, 0, 0, time.Local),
		Content:   "## 场景：支付接口超时 - 2026-05-17 排查记录\n\n- **现象**: 支付接口返回 504\n- **关键词**: 504, timeout\n\n## 根本原因\nMySQL 慢查询导致线程池耗尽\n\n---\n*归档时间: 2026-05-17 10:00:00 | 作者: alice*",
	}

	if err := store.Upload(ctx, patch1); err != nil {
		t.Fatalf("Upload patch1 failed: %v", err)
	}
	t.Log("  Patch 1 uploaded successfully")

	// 4. Upload 第二个补丁（同一 service+module）
	t.Log("Step 4: Uploading second patch (same service+module)")
	patch2 := Patch{
		ID:        "e5f6g7h8",
		Service:   "Payment Service",
		Module:    "核心支付模块",
		Author:    "bob",
		Timestamp: time.Date(2026, 5, 17, 14, 30, 0, 0, time.Local),
		Content:   "## 场景：支付回调失败 - 2026-05-17 排查记录\n\n- **现象**: 支付回调返回 500\n- **关键词**: 500, callback\n\n## 根本原因\n回调签名验证失败\n\n---\n*归档时间: 2026-05-17 14:30:00 | 作者: bob*",
	}

	if err := store.Upload(ctx, patch2); err != nil {
		t.Fatalf("Upload patch2 failed: %v", err)
	}
	t.Log("  Patch 2 uploaded successfully")

	// 5. Upload 第三个补丁（不同 service）
	t.Log("Step 5: Uploading third patch (different service)")
	patch3 := Patch{
		ID:        "i9j0k1l2",
		Service:   "Order Service",
		Module:    "订单处理",
		Author:    "alice",
		Timestamp: time.Date(2026, 5, 17, 16, 0, 0, 0, time.Local),
		Content:   "## 场景：订单状态不一致 - 2026-05-17 排查记录\n\n- **现象**: 订单状态与支付状态不一致\n- **关键词**: status, inconsistent\n\n## 根本原因\n分布式事务未正确提交\n\n---\n*归档时间: 2026-05-17 16:00:00 | 作者: alice*",
	}

	if err := store.Upload(ctx, patch3); err != nil {
		t.Fatalf("Upload patch3 failed: %v", err)
	}
	t.Log("  Patch 3 uploaded successfully")

	// 6. 模拟用户 B：新客户端 clone 并下载全部补丁
	t.Log("Step 6: Simulating User B - fresh clone")
	localDirB := filepath.Join(tmpDir, "patchstore-b")
	storeB := NewGitPatchStore(bareRepo, localDirB, "main", "bob", "bob@test.com")

	patches, err := storeB.DownloadAll(ctx)
	if err != nil {
		t.Fatalf("DownloadAll failed: %v", err)
	}

	if len(patches) != 3 {
		t.Fatalf("Expected 3 patches, got %d", len(patches))
	}
	t.Logf("  User B downloaded %d patches", len(patches))

	// 7. 验证补丁内容
	t.Log("Step 7: Verifying patch contents")
	sort.Slice(patches, func(i, j int) bool {
		return patches[i].ID < patches[j].ID
	})

	for i, p := range patches {
		t.Logf("  Patch %d: id=%s service=%s module=%s author=%s", i+1, p.ID, p.Service, p.Module, p.Author)
		if p.Service == "" || p.Module == "" || p.Content == "" {
			t.Errorf("Patch %d has empty required fields", i+1)
		}
	}

	// 8. 并发上传测试：用户 A 和 B 同时上传
	t.Log("Step 8: Concurrent upload test")
	setupGitUser(localDirB)

	patchA := Patch{
		ID:        "aaaa1111",
		Service:   "Payment Service",
		Module:    "核心支付模块",
		Author:    "alice",
		Timestamp: time.Date(2026, 5, 18, 9, 0, 0, 0, time.Local),
		Content:   "## 场景：支付网关连接超时\n\n- **现象**: 网关连接超时\n\n---\n*归档时间: 2026-05-18 09:00:00 | 作者: alice*",
	}

	patchB := Patch{
		ID:        "bbbb2222",
		Service:   "Payment Service",
		Module:    "核心支付模块",
		Author:    "bob",
		Timestamp: time.Date(2026, 5, 18, 9, 5, 0, 0, time.Local),
		Content:   "## 场景：支付限额异常\n\n- **现象**: 正常金额被限额\n\n---\n*归档时间: 2026-05-18 09:05:00 | 作者: bob*",
	}

	// 用户 A 上传
	if err := store.Upload(ctx, patchA); err != nil {
		t.Fatalf("User A upload failed: %v", err)
	}
	t.Log("  User A uploaded patchA")

	// 用户 B 上传（需要先 pull 再 push，模拟冲突场景）
	if err := storeB.Upload(ctx, patchB); err != nil {
		t.Fatalf("User B upload failed: %v", err)
	}
	t.Log("  User B uploaded patchB")

	// 验证双方都能看到所有补丁
	patchesA, _ := store.DownloadAll(ctx)
	patchesB, _ := storeB.DownloadAll(ctx)

	if len(patchesA) != 5 {
		t.Errorf("User A: expected 5 patches, got %d", len(patchesA))
	}
	if len(patchesB) != 5 {
		t.Errorf("User B: expected 5 patches, got %d", len(patchesB))
	}
	t.Logf("  Both users have %d/%d patches after concurrent upload", len(patchesA), len(patchesB))

	// 9. 增量同步测试
	t.Log("Step 9: Incremental sync test")
	since := time.Date(2026, 5, 18, 0, 0, 0, 0, time.Local)
	incremental, err := store.Download(ctx, since)
	if err != nil {
		t.Fatalf("Incremental download failed: %v", err)
	}
	if len(incremental) != 2 {
		t.Errorf("Expected 2 incremental patches (after %s), got %d", since.Format("2006-01-02"), len(incremental))
	}
	t.Logf("  Incremental sync: %d new patches since %s", len(incremental), since.Format("2006-01-02"))

	// 10. 验证 LastSyncTime
	t.Log("Step 10: LastSyncTime test")
	lastTime, err := store.LastSyncTime(ctx)
	if err != nil {
		t.Fatalf("LastSyncTime failed: %v", err)
	}
	if lastTime.IsZero() {
		t.Error("LastSyncTime should not be zero")
	}
	t.Logf("  Last sync time: %s", lastTime.Format("2006-01-02 15:04:05"))

	t.Log("All E2E tests passed!")
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
