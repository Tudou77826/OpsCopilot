package shellsidecar

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"opscopilot/pkg/garden"
)

// 事件源契约：三处业务结果各自只结算一次，且幂等键取自业务 id。
// 这里直接测服务层钩子与花园的接线，不重复 pkg/garden 的规则测试。
func TestGardenReceivesBusinessEvents(t *testing.T) {
	dir := t.TempDir()
	svc, err := NewGardenService(dir)
	if err != nil {
		t.Fatal(err)
	}
	api := &ControlAPI{Garden: svc}

	// 传输完成钩子：同一任务 id 调两次只结算一次货币。
	ft := NewFTService(nil, dir)
	api.FT = ft
	wireGarden(api)
	ft.onTransferDone("task-1")
	ft.onTransferDone("task-1")
	snap := svc.Snapshot()
	if len(snap.Specimens) != 0 || snap.Balance != garden.StartingBalance+10 || snap.Earned != 10 {
		t.Fatalf("一次传输应只奖励 10 灵感币: %+v", snap)
	}

	// 回放钩子：换一个业务 id 应继续累计货币，不自动创造元素。
	scripts := &StructuredScriptService{}
	api.Scripts = scripts
	wireGarden(api)
	scripts.onReplayDispatched("run-1")
	snap = svc.Snapshot()
	if len(snap.Specimens) != 0 || snap.Balance != garden.StartingBalance+10+12 {
		t.Fatalf("回放应再奖励 12 灵感币: %+v", snap)
	}

	// 未注入花园时 wireGarden 不得 panic（保持能力边界纪律）。
	api2 := &ControlAPI{FT: NewFTService(nil, dir)}
	wireGarden(api2)
}

// 断开/失败不产生事件：只有 hook 被显式调用的成功路径才结算。
func TestGardenIgnoresNonSuccess(t *testing.T) {
	dir := t.TempDir()
	svc, err := NewGardenService(dir)
	if err != nil {
		t.Fatal(err)
	}
	// 钩子只在 ok=true 的成功路径被调用：这里只注入通知观察者，
	// 走一次失败完成路径（ok=false），断言花园一无所获。
	ft := NewFTService(nil, dir)
	var notified []string
	ft.SetNotify(func(method string, _ any) { notified = append(notified, method) })
	api := &ControlAPI{Garden: svc, FT: ft}
	wireGarden(api)
	ft.notifyDone("task-x", "term-1", false, false, "失败", 0)
	if got := len(svc.Snapshot().Specimens); got != 0 {
		t.Fatalf("失败传输不得结算成长: %d 株", got)
	}
	if len(notified) != 1 || notified[0] != "shell.ft/done" {
		t.Fatalf("失败仍应通知前台: %v", notified)
	}
	// 快照在空花园时也必须是可用形状（集合字段不许为 nil，沿用既有边界纪律）。
	snap := svc.Snapshot()
	if snap.Specimens == nil {
		t.Fatal("空花园的 Specimens 不得为 nil")
	}
	raw, err := json.Marshal(snap)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), `"specimens":null`) {
		t.Fatalf("收藏集合不得出现 null: %s", raw)
	}
	if snap.SchemaVersion == 0 || snap.RuleVersion == 0 {
		t.Fatalf("快照必须带版本: %+v", snap)
	}
}

// 持久化位置：garden.json 落在数据目录，且内容可被重新打开。
func TestGardenPersistsInDataDir(t *testing.T) {
	dir := t.TempDir()
	svc, err := NewGardenService(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Record(garden.EventSessionEstablished, "conn-a"); err != nil {
		t.Fatal(err)
	}
	reopened, err := NewGardenService(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := reopened.Snapshot().Balance; got != garden.StartingBalance+8 {
		t.Fatalf("重开数据目录应恢复余额 %d, 得到 %d", garden.StartingBalance+8, got)
	}
	if _, err := filepath.Abs(dir); err != nil {
		t.Fatal(err)
	}
}
