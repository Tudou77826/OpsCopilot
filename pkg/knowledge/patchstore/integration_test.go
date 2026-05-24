package patchstore

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
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

// TestGitFeedbackStoreE2E 端到端测试：反馈存储的完整 Git 同步流程
func TestGitFeedbackStoreE2E(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available, skipping E2E test")
	}

	tmpDir := t.TempDir()
	bareRepo := filepath.Join(tmpDir, "knowledge-bare.git")

	// 初始化 bare 仓库（与 patch store 共享）
	runGit(t, tmpDir, "init", "--bare", bareRepo)
	cloneDir := filepath.Join(tmpDir, "init-clone")
	runGit(t, tmpDir, "clone", bareRepo, cloneDir)
	runGit(t, cloneDir, "config", "user.name", "Test User")
	runGit(t, cloneDir, "config", "user.email", "test@example.com")
	os.WriteFile(filepath.Join(cloneDir, ".gitkeep"), []byte(""), 0644)
	runGit(t, cloneDir, "add", ".gitkeep")
	runGit(t, cloneDir, "commit", "-m", "init")
	runGit(t, cloneDir, "branch", "-M", "main")
	runGit(t, cloneDir, "push", "-u", "origin", "main")
	os.RemoveAll(cloneDir)

	ctx := context.Background()
	patchID := "archive/Order_Service_订单处理.md#L10"

	// Step 1: 用户 A 创建反馈存储并提交评分
	t.Log("Step 1: User A saves a rating")
	localDirA := filepath.Join(tmpDir, "patchstore-a")
	storeA := NewGitFeedbackStore(localDirA, bareRepo, "main")

	ratingA := UserFeedback{
		PatchID: patchID,
		Service: "Order Service",
		Module:  "订单处理",
		User:    "alice",
		Rating:  &Rating{Score: 5, Comment: "排查路径非常清晰", Timestamp: time.Now()},
	}
	if err := storeA.SaveFeedback(ctx, ratingA); err != nil {
		t.Fatalf("User A save rating failed: %v", err)
	}
	t.Log("  User A rating saved and pushed")

	// Step 2: 用户 B 从 remote clone，验证能看到用户 A 的评分
	t.Log("Step 2: User B clones and sees User A's feedback")
	localDirB := filepath.Join(tmpDir, "patchstore-b")
	storeB := NewGitFeedbackStore(localDirB, bareRepo, "main")

	// Pull triggers clone + fetch, simulating what syncPatches does in production
	if err := storeB.Pull(ctx); err != nil {
		t.Fatalf("User B pull (initial clone) failed: %v", err)
	}
	feedbackB, err := storeB.GetFeedback(ctx, patchID)
	if err != nil {
		t.Fatalf("User B get feedback failed: %v", err)
	}
	if len(feedbackB) != 1 {
		t.Fatalf("User B expected 1 feedback, got %d", len(feedbackB))
	}
	if feedbackB[0].Rating == nil || feedbackB[0].Rating.Score != 5 {
		t.Errorf("User B expected rating score 5, got %+v", feedbackB[0].Rating)
	}
	t.Logf("  User B sees: %s gave %d stars", feedbackB[0].User, feedbackB[0].Rating.Score)

	// Step 3: 用户 B 提交自己的评分（同一 patch）
	t.Log("Step 3: User B saves their own rating on same patch")
	ratingB := UserFeedback{
		PatchID: patchID,
		Service: "Order Service",
		Module:  "订单处理",
		User:    "bob",
		Rating:  &Rating{Score: 3, Comment: "根因分析不够深入", Timestamp: time.Now()},
	}
	if err := storeB.SaveFeedback(ctx, ratingB); err != nil {
		t.Fatalf("User B save rating failed: %v", err)
	}
	t.Log("  User B rating saved and pushed")

	// Step 4: 用户 A pull 后能看到两人的评分
	t.Log("Step 4: User A pulls and sees both ratings")
	if err := storeA.Pull(ctx); err != nil {
		t.Fatalf("User A pull failed: %v", err)
	}
	feedbackA, err := storeA.GetFeedback(ctx, patchID)
	if err != nil {
		t.Fatalf("User A get feedback failed: %v", err)
	}
	if len(feedbackA) != 2 {
		t.Fatalf("User A expected 2 feedbacks, got %d", len(feedbackA))
	}
	t.Logf("  User A sees %d ratings", len(feedbackA))
	for _, fb := range feedbackA {
		t.Logf("    - %s: %d stars", fb.User, fb.Rating.Score)
	}

	// Step 5: 用户 A 提交一个 Issue
	t.Log("Step 5: User A reports an issue")
	issueA := UserFeedback{
		PatchID: patchID,
		Service: "Order Service",
		Module:  "订单处理",
		User:    "alice",
		Rating:  ratingA.Rating, // Preserve existing rating
		Issues: []Issue{
			{
				ID:          "a1b2c3d4",
				Type:        "bug",
				Priority:    "high",
				Title:       "curl 命令中的占位符未替换",
				Description: "排查路径中 <PLACEHOLDER> 没有给出实际值",
				Reporter:    "alice",
				Status:      "open",
				Timestamp:   time.Now(),
			},
		},
	}
	if err := storeA.SaveFeedback(ctx, issueA); err != nil {
		t.Fatalf("User A save issue failed: %v", err)
	}
	t.Log("  User A issue saved and pushed")

	// Step 6: 用户 B pull 后能看到 Issue
	t.Log("Step 6: User B pulls and sees the issue")
	if err := storeB.Pull(ctx); err != nil {
		t.Fatalf("User B pull failed: %v", err)
	}
	feedbackB2, err := storeB.GetFeedback(ctx, patchID)
	if err != nil {
		t.Fatalf("User B get feedback (after issue) failed: %v", err)
	}

	aliceFB := findFeedback(feedbackB2, "alice")
	if aliceFB == nil || len(aliceFB.Issues) != 1 {
		t.Fatalf("User B expected alice to have 1 issue, got %+v", aliceFB)
	}
	if aliceFB.Issues[0].Status != "open" {
		t.Errorf("Expected issue status 'open', got '%s'", aliceFB.Issues[0].Status)
	}
	t.Logf("  User B sees issue: [%s] %s (status: %s)",
		aliceFB.Issues[0].Type, aliceFB.Issues[0].Title, aliceFB.Issues[0].Status)

	// Step 7: 用户 B 更新 Issue 状态
	t.Log("Step 7: User B marks the issue as resolved")
	if err := storeB.UpdateIssueStatus(ctx, patchID, "a1b2c3d4", "resolved"); err != nil {
		t.Fatalf("User B update issue status failed: %v", err)
	}
	t.Log("  Issue status updated and pushed")

	// Step 8: 用户 A pull 后看到 Issue 已解决
	t.Log("Step 8: User A pulls and sees resolved issue")
	if err := storeA.Pull(ctx); err != nil {
		t.Fatalf("User A pull failed: %v", err)
	}
	feedbackA2, err := storeA.GetFeedback(ctx, patchID)
	if err != nil {
		t.Fatalf("User A get feedback (after resolve) failed: %v", err)
	}

	aliceFB2 := findFeedback(feedbackA2, "alice")
	if aliceFB2 == nil || len(aliceFB2.Issues) != 1 {
		t.Fatalf("User A expected alice to still have 1 issue")
	}
	if aliceFB2.Issues[0].Status != "resolved" {
		t.Errorf("User A expected issue status 'resolved', got '%s'", aliceFB2.Issues[0].Status)
	}
	t.Logf("  User A sees issue status: %s", aliceFB2.Issues[0].Status)

	// Step 9: 第三个用户提交评分，验证三方数据聚合
	t.Log("Step 9: Third user Charlie saves rating")
	localDirC := filepath.Join(tmpDir, "patchstore-c")
	storeC := NewGitFeedbackStore(localDirC, bareRepo, "main")

	ratingC := UserFeedback{
		PatchID: patchID,
		Service: "Order Service",
		Module:  "订单处理",
		User:    "charlie",
		Rating:  &Rating{Score: 4, Comment: "整体不错", Timestamp: time.Now()},
	}
	if err := storeC.SaveFeedback(ctx, ratingC); err != nil {
		t.Fatalf("Charlie save rating failed: %v", err)
	}

	// 用户 A 验证能看到所有三人的数据
	if err := storeA.Pull(ctx); err != nil {
		t.Fatalf("User A final pull failed: %v", err)
	}
	feedbackA3, err := storeA.GetFeedback(ctx, patchID)
	if err != nil {
		t.Fatalf("User A final get feedback failed: %v", err)
	}
	if len(feedbackA3) != 3 {
		t.Fatalf("User A expected 3 feedbacks, got %d", len(feedbackA3))
	}

	// 验证平均评分
	var sum int
	for _, fb := range feedbackA3 {
		if fb.Rating != nil {
			sum += fb.Rating.Score
		}
	}
	avg := float64(sum) / float64(len(feedbackA3))
	if avg != 4.0 { // (5+3+4)/3 = 4.0
		t.Errorf("Expected avg rating 4.0, got %.1f", avg)
	}
	t.Logf("  All 3 users visible, avg rating: %.1f", avg)

	// Step 10: 验证反馈文件在 Git 仓库中的持久化
	t.Log("Step 10: Verify feedback files persist in Git repo")
	var fbFileCount int
	filepath.Walk(filepath.Join(localDirA, "feedback"), func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if strings.HasSuffix(info.Name(), ".json") {
			fbFileCount++
		}
		return nil
	})
	if fbFileCount == 0 {
		t.Error("No feedback files found in local repo")
	}
	t.Logf("  %d feedback JSON files in local repo", fbFileCount)

	t.Log("All feedback E2E tests passed!")
}

func findFeedback(list []UserFeedback, user string) *UserFeedback {
	for i := range list {
		if list[i].User == user {
			return &list[i]
		}
	}
	return nil
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
