package garden

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func openTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "garden.json"))
	if err != nil {
		t.Fatal(err)
	}
	s.randf = func() (float64, error) { return 0.99, nil }
	return s
}

func TestWorkEventEarnsCurrencyWithoutCreatingContent(t *testing.T) {
	s := openTestStore(t)
	out, err := s.Record(EventSessionEstablished, "conn-1")
	if err != nil {
		t.Fatal(err)
	}
	if out.CurrencyGained != 8 || out.Balance != StartingBalance+8 {
		t.Fatalf("货币结算不正确: %+v", out)
	}
	snap := s.Snapshot()
	if len(snap.Specimens) != 0 {
		t.Fatalf("工作行为不应直接创造收藏: %d", len(snap.Specimens))
	}
	if snap.Earned != 8 || snap.GardenLevel != 0 {
		t.Fatalf("累计收入或等级不正确: %+v", snap)
	}
	if snap.ChangeSignal == nil || snap.ChangeSignal.Kind != "currency-earned" || snap.ChangeSignal.Amount != 8 {
		t.Fatalf("轻提示信号不正确: %+v", snap.ChangeSignal)
	}
}

func TestRecordIsIdempotentAndCapped(t *testing.T) {
	s := openTestStore(t)
	first, err := s.Record(EventScriptReplayDone, "run-1")
	if err != nil {
		t.Fatal(err)
	}
	duplicate, err := s.Record(EventScriptReplayDone, "run-1")
	if err != nil || !duplicate.Duplicate {
		t.Fatalf("重复事件应被识别: %+v %v", duplicate, err)
	}
	for i := 2; i <= DailyCapPerKind; i++ {
		if _, err := s.Record(EventScriptReplayDone, "run-"+time.Unix(int64(i), 0).Format("150405")); err != nil {
			t.Fatal(err)
		}
	}
	capped, err := s.Record(EventScriptReplayDone, "run-over-cap")
	if err != nil || !capped.Capped {
		t.Fatalf("超过日上限应停止结算: %+v %v", capped, err)
	}
	want := StartingBalance + first.CurrencyGained*DailyCapPerKind
	if got := s.Snapshot().Balance; got != want {
		t.Fatalf("重复或超限事件改变了余额: got=%d want=%d", got, want)
	}
}

func TestPurchasePlaceAndStow(t *testing.T) {
	s := openTestStore(t)
	bought, err := s.Purchase("cmd-mint", 35, 50)
	if err != nil {
		t.Fatal(err)
	}
	if bought.Balance != StartingBalance-35 || bought.Specimen.Placement == nil || bought.Specimen.Placement.Placed {
		t.Fatalf("新购收藏应先进入库存: %+v", bought)
	}
	id := bought.Specimen.InstanceID
	placed, err := s.Place(id, 0.24, 0.78, 1.2, true)
	if err != nil {
		t.Fatal(err)
	}
	p := placed.Specimens[0].Placement
	if p == nil || !p.Placed || p.X != 0.24 || p.Y != 0.78 || p.Scale != 1.2 || !p.FlipX {
		t.Fatalf("摆放未持久化: %+v", p)
	}
	stowed, err := s.Stow(id)
	if err != nil {
		t.Fatal(err)
	}
	if stowed.Specimens[0].Placement == nil || stowed.Specimens[0].Placement.Placed {
		t.Fatalf("收回后应在库存: %+v", stowed.Specimens[0].Placement)
	}
}

func TestCommerceAndPlacementValidation(t *testing.T) {
	s := openTestStore(t)
	if _, err := s.Purchase("Bad_ID", 10, 1); err == nil {
		t.Fatal("非法商品 id 应被拒绝")
	}
	if _, err := s.Purchase("cmd-mint", StartingBalance+1, 1); err == nil {
		t.Fatal("余额不足应被拒绝")
	}
	bought, err := s.Purchase("cmd-mint", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Place(bought.Specimen.InstanceID, -0.1, 0.5, 1, false); err == nil {
		t.Fatal("越界坐标应被拒绝")
	}
	if _, err := s.Place(bought.Specimen.InstanceID, 0.5, 0.5, 2, false); err == nil {
		t.Fatal("越界缩放应被拒绝")
	}
	if _, err := s.Stow("missing"); err == nil {
		t.Fatal("不存在实例应被拒绝")
	}
}

func TestStatePersistsAndIsSharedAcrossStores(t *testing.T) {
	path := filepath.Join(t.TempDir(), "garden.json")
	a, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	b, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.Record(EventTransferCompleted, "transfer-1"); err != nil {
		t.Fatal(err)
	}
	b.randf = func() (float64, error) { return 0.99, nil }
	bought, err := b.Purchase("transfer-fern", 45, 50)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.Place(bought.Specimen.InstanceID, 0.7, 0.8, 1, false); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	snap := reopened.Snapshot()
	if snap.Balance != StartingBalance+10-45 || len(snap.Specimens) != 1 || !snap.Specimens[0].Placement.Placed {
		t.Fatalf("跨实例状态未合并: %+v", snap)
	}
}

func TestSnapshotCollectionsAndPlacementAreCopied(t *testing.T) {
	s := openTestStore(t)
	bought, err := s.Purchase("cmd-mint", 10, 1)
	if err != nil {
		t.Fatal(err)
	}
	first := s.Snapshot()
	if first.Specimens == nil {
		t.Fatal("集合不得为 nil")
	}
	first.Specimens[0].Placement.Placed = true
	if s.Snapshot().Specimens[0].Placement.Placed {
		t.Fatal("快照不得泄漏内部 placement 指针")
	}
	bought.Specimen.Placement.Placed = true
	if s.Snapshot().Specimens[0].Placement.Placed {
		t.Fatal("购买结果不得泄漏内部 placement 指针")
	}
}

func TestRecordRejectsUnknownAndMissingKey(t *testing.T) {
	s := openTestStore(t)
	if _, err := s.Record("unknown", "x"); err == nil {
		t.Fatal("未知事件应报错")
	}
	if _, err := s.Record(EventSessionEstablished, ""); err == nil {
		t.Fatal("空幂等键应报错")
	}
}

func TestPurchaseCanProduceShinyAndHasHiddenPity(t *testing.T) {
	s := openTestStore(t)
	s.randf = func() (float64, error) { return 0, nil }
	bought, err := s.Purchase("cmd-mint", 1, 1)
	if err != nil || !bought.Specimen.Shiny {
		t.Fatalf("命中概率时应获得闪光品质: %+v %v", bought, err)
	}

	s.state.PurchasesSinceShiny = ShinyPityAt - 1
	s.randf = func() (float64, error) { return 0.99, nil }
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	bought, err = s.Purchase("transfer-fern", 1, 1)
	if err != nil || !bought.Specimen.Shiny || s.state.PurchasesSinceShiny != 0 {
		t.Fatalf("第 %d 次购买应保底闪光并清零计数: %+v %v", ShinyPityAt, bought, err)
	}
}

func TestDevelopmentSchemaV2IsRejected(t *testing.T) {
	path := filepath.Join(t.TempDir(), "garden.json")
	old := State{SchemaVersion: 2, RuleVersion: 2, Specimens: []*Specimen{}, Seen: map[string]int64{}, Daily: map[string]map[EventKind]int{}}
	raw, err := json.Marshal(old)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(path); err == nil {
		t.Fatal("开发期旧结构不得被静默迁移")
	}
}
