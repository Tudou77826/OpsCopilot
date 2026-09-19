package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// 桌面养成绑定：开箱即用（快照集合永不为 nil）、轻量变化信号、未启用时显式报错、
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
	if snap.Specimens == nil {
		t.Fatal("收藏集合不得为 null（曾因此黑屏）")
	}
	if snap.GardenLevel != 0 {
		t.Fatalf("空收藏等级应为 0，得到 %d", snap.GardenLevel)
	}

	// 一条事件奖励货币，不直接创造收藏（首次写入后 garden.json 才落盘）。
	// recordGarden 是异步的，轮询等结算完成。
	app.recordGarden("session-established", "conn:test-1")
	deadline := time.Now().Add(2 * time.Second)
	for {
		snap, err = app.GardenSnapshot()
		if err != nil {
			t.Fatalf("GardenSnapshot after record: %v", err)
		}
		if snap.Balance > 120 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("结算一次后余额应增加，得到 %d", snap.Balance)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, err := os.Stat(filepath.Join(dir, "garden.json")); err != nil {
		t.Fatalf("garden.json 未落在工作目录: %v", err)
	}
	if len(snap.Specimens) != 0 {
		t.Fatalf("工作行为不应自动创造收藏，得到 %d", len(snap.Specimens))
	}

	bought, err := app.GardenPurchase("cmd-mint", 35, 50)
	if err != nil || bought.Specimen == nil {
		t.Fatalf("GardenPurchase: result=%+v err=%v", bought, err)
	}
	placed, err := app.GardenPlace(bought.Specimen.InstanceID, 0.4, 0.8, 1, false)
	if err != nil || len(placed.Specimens) != 1 || !placed.Specimens[0].Placement.Placed {
		t.Fatalf("GardenPlace: result=%+v err=%v", placed, err)
	}
	stowed, err := app.GardenStow(bought.Specimen.InstanceID)
	if err != nil || stowed.Specimens[0].Placement.Placed {
		t.Fatalf("GardenStow: result=%+v err=%v", stowed, err)
	}

	signal, err := app.GardenSignal()
	if err != nil || signal == nil || signal.Revision == 0 {
		t.Fatalf("GardenSignal 应返回可见变化 revision: signal=%+v err=%v", signal, err)
	}

	// 未启用：nil 守卫必须显式报错而不是 panic。
	disabled := &App{}
	if _, err := disabled.GardenSnapshot(); err == nil {
		t.Fatal("未启用时 GardenSnapshot 应报错")
	}
	if _, err := disabled.GardenSignal(); err == nil {
		t.Fatal("未启用时 GardenSignal 应报错")
	}
	disabled.recordGarden("session-established", "conn:nope") // 不应 panic
}
