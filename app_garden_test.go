package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// 桌面花园绑定：开箱即用（快照集合永不为 nil）、时间戳校验、未启用时显式报错、
// recordGarden 在未启用时是安全的空操作。
func TestGardenBindings(t *testing.T) {
	dir := t.TempDir()
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWd) })

	app := &App{}
	app.garden = openGarden()
	if app.garden == nil {
		t.Fatal("openGarden 在空目录下应成功初始化")
	}

	snap, err := app.GardenSnapshot()
	if err != nil {
		t.Fatalf("GardenSnapshot: %v", err)
	}
	if snap.Specimens == nil || snap.Pending == nil {
		t.Fatal("集合字段不得为 null（曾因此黑屏）")
	}
	if snap.GardenLevel < 1 {
		t.Fatalf("花园等级应从 1 起，得到 %d", snap.GardenLevel)
	}

	// 一条事件让第一株长出来，快照应能看到它（首次写入后 garden.json 才落盘）。
	// recordGarden 是异步的，轮询等结算完成。
	app.recordGarden("session-established", "conn:test-1")
	deadline := time.Now().Add(2 * time.Second)
	for {
		snap, err = app.GardenSnapshot()
		if err != nil {
			t.Fatalf("GardenSnapshot after record: %v", err)
		}
		if len(snap.Specimens) == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("结算一次后应有 1 株，得到 %d", len(snap.Specimens))
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, err := os.Stat(filepath.Join(dir, "garden.json")); err != nil {
		t.Fatalf("garden.json 未落在工作目录: %v", err)
	}

	if err := app.GardenDismiss(snap.Pending[0].At.Format("2006-01-02T15:04:05.999999999Z07:00")); err != nil {
		t.Fatalf("GardenDismiss: %v", err)
	}
	if err := app.GardenDismiss("not-a-time"); err == nil || !strings.Contains(err.Error(), "时间戳格式无效") {
		t.Fatalf("非法时间戳应报格式错误，得到 %v", err)
	}

	// 未启用：nil 守卫必须显式报错而不是 panic。
	disabled := &App{}
	if _, err := disabled.GardenSnapshot(); err == nil {
		t.Fatal("未启用时 GardenSnapshot 应报错")
	}
	disabled.recordGarden("session-established", "conn:nope") // 不应 panic
}
