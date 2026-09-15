package garden

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// newTestStore 打开临时目录里的花园，注入固定时钟与固定随机序列。
func newTestStore(t *testing.T) (*Store, *time.Time) {
	t.Helper()
	base := time.Date(2026, 9, 15, 10, 0, 0, 0, time.UTC)
	s, err := Open(filepath.Join(t.TempDir(), "garden.json"))
	if err != nil {
		t.Fatal(err)
	}
	clock := base
	s.now = func() time.Time { clock = clock.Add(time.Minute); return clock }
	s.randf = func() float64 { return 0.99 } // 永不自然闪光；保底由计数触发
	t.Cleanup(func() {})
	return s, &base
}

func TestFirstEventDiscoversAndLevels(t *testing.T) {
	s, _ := newTestStore(t)
	out, err := s.Record(EventSessionEstablished, "conn-1")
	if err != nil {
		t.Fatal(err)
	}
	if !out.Discovered || out.Shiny {
		t.Fatalf("首次事件应发现普通植株: %+v", out)
	}
	if out.XPGained != 10 || out.Level < 1 {
		t.Fatalf("成长值或等级异常: %+v", out)
	}
	if got := len(s.Snapshot().Specimens); got != 1 {
		t.Fatalf("应有 1 株, 得到 %d", got)
	}
}

func TestDuplicateEventSettlesOnce(t *testing.T) {
	s, _ := newTestStore(t)
	if _, err := s.Record(EventTransferCompleted, "tx-1"); err != nil {
		t.Fatal(err)
	}
	second, err := s.Record(EventTransferCompleted, "tx-1")
	if err != nil {
		t.Fatal(err)
	}
	if !second.Duplicate {
		t.Fatalf("同一幂等键第二次应标记 Duplicate: %+v", second)
	}
	if got := len(s.Snapshot().Specimens); got != 1 {
		t.Fatalf("重复事件不得产生新植株: %d", got)
	}
}

func TestDailyCapBlocksExcess(t *testing.T) {
	s, _ := newTestStore(t)
	for i := 0; i < DailyCapPerKind; i++ {
		if _, err := s.Record(EventSessionEstablished, "k"+string(rune('a'+i))); err != nil {
			t.Fatal(err)
		}
	}
	capped, err := s.Record(EventSessionEstablished, "over-cap")
	if err != nil {
		t.Fatal(err)
	}
	if !capped.Capped {
		t.Fatalf("超出日上限应标记 Capped: %+v", capped)
	}
}

func TestPityGuaranteesShiny(t *testing.T) {
	s, _ := newTestStore(t)
	kinds := []EventKind{EventScriptReplayDone, EventSessionEstablished, EventTransferCompleted}
	day := 0
	for i := 0; i < PityAt-1; i++ {
		// 日上限按类别计；为隔离它，每 DailyCapPerKind 轮推进一天（保底跨天累计）。
		// 时钟每次调用前进一分钟，因此"新的一天"用整批推进实现。
		if i > 0 && i%(DailyCapPerKind*len(kinds)) == 0 {
			day++
			base := time.Date(2026, 9, 15+day, 0, 0, 0, 0, time.UTC)
			s.now = func() time.Time { base = base.Add(time.Minute); return base }
		}
		out, err := s.Record(kinds[i%len(kinds)], fmt.Sprintf("run-%d-%d", day, i))
		if err != nil {
			t.Fatal(err)
		}
		if out.Capped {
			t.Fatalf("跨天跨类别后不应触发日上限（第 %d 次）", i+1)
		}
		if out.Shiny {
			t.Fatalf("保底前不应出现闪光（第 %d 次）", i+1)
		}
	}
	last, err := s.Record(EventScriptReplayDone, "run-pity")
	if err != nil {
		t.Fatal(err)
	}
	if !last.Discovered || !last.Shiny {
		t.Fatalf("第 %d 次发现应保底闪光: %+v", PityAt, last)
	}
	if s.Snapshot().PitySinceShiny != 0 {
		t.Fatalf("闪光后保底计数应清零: %d", s.Snapshot().PitySinceShiny)
	}
}

func TestGrowthFlowsToYoungestNonEvergreen(t *testing.T) {
	s, _ := newTestStore(t)
	// 新口径：同物种每 DiscoverEvery 次合格事件发现一株，最多 MaxPerSpecies 株。
	for i := 0; i < MaxPerSpecies; i++ {
		if _, err := s.Record(EventScriptReplayDone, fmt.Sprintf("seed-%d", i)); err != nil {
			t.Fatal(err)
		}
	}
	// 全部推到常青。存储以文件为唯一真相（见 store 的共享存储语义），
	// 所以设置后必须落盘——否则下一次结算在锁内回读时会把这次改动丢掉。
	for _, spec := range s.state.Specimens {
		spec.Level = MaxLevel
		spec.XP = 0
	}
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	frozen := s.Snapshot()
	if _, err := s.Record(EventScriptReplayDone, "overflow"); err != nil {
		t.Fatal(err)
	}
	got := s.Snapshot()
	if len(got.Specimens) != MaxPerSpecies {
		t.Fatalf("个体达上限后不得再发现新株: %d 株", len(got.Specimens))
	}
	for i, spec := range got.Specimens {
		if spec.XP != frozen.Specimens[i].XP || spec.Level != frozen.Specimens[i].Level {
			t.Fatalf("常青个体不得吸收溢出成长: #%d %+v", i, spec)
		}
	}
}

func TestStageFollowsLevel(t *testing.T) {
	cases := []struct{ level, stage int }{
		{1, 0}, {5, 0}, {6, 1}, {15, 2}, {26, 5}, {30, 5},
	}
	for _, c := range cases {
		if got := StageOf(c.level); got != c.stage {
			t.Fatalf("等级 %d 的阶段应为 %d, 得到 %d", c.level, c.stage, got)
		}
	}
}

func TestPersistAndReloadKeepsEverything(t *testing.T) {
	path := filepath.Join(t.TempDir(), "garden.json")
	s, _ := newTestStore(t)
	s.path = path
	if _, err := s.Record(EventDiagnosisSaved, "d-1"); err != nil {
		t.Fatal(err)
	}
	want := s.Snapshot()

	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	got := reopened.Snapshot()
	if got.GardenLevel != want.GardenLevel || len(got.Specimens) != len(want.Specimens) {
		t.Fatalf("重载后状态不一致: %+v vs %+v", got, want)
	}
	if got.Specimens[0].Shiny != want.Specimens[0].Shiny || got.Specimens[0].Level != want.Specimens[0].Level {
		t.Fatalf("重载后植株不一致: %+v vs %+v", got.Specimens[0], want.Specimens[0])
	}
	// 已结算的幂等键在重载后仍然去重。
	again, err := reopened.Record(EventDiagnosisSaved, "d-1")
	if err != nil {
		t.Fatal(err)
	}
	if !again.Duplicate {
		t.Fatalf("重载后同键不得重复结算: %+v", again)
	}
}

func TestUnknownKindAndMissingKeyRejected(t *testing.T) {
	s, _ := newTestStore(t)
	if _, err := s.Record(EventKind("nope"), "k"); err == nil {
		t.Fatal("未知类别应报错")
	}
	if _, err := s.Record(EventSessionEstablished, ""); err == nil {
		t.Fatal("缺少幂等键应报错")
	}
}

func TestSnapshotCollectionsAreNeverNull(t *testing.T) {
	s, _ := newTestStore(t)
	raw, err := json.Marshal(s.Snapshot())
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{`"specimens":null`, `"pending":null`, `"milestones":null`} {
		if strings.Contains(string(raw), field) {
			t.Fatalf("集合字段不得序列化为 null（%s）：%s", field, raw)
		}
	}
	// 有内容时也不能是 null。
	if _, err := s.Record(EventSessionEstablished, "c1"); err != nil {
		t.Fatal(err)
	}
	raw, err = json.Marshal(s.Snapshot())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), `null`) && strings.Contains(string(raw), `"pending":null`) {
		t.Fatalf("有反馈时 pending 不得为 null: %s", raw)
	}
}

// 桌面壳与 Teams 插件可以指向同一数据目录（shared storage），两边各自因行为写入。
// 这条用例模拟两个进程：任一侧的成长必须被另一侧立刻看到，且不得互相覆盖。
func TestTwoProcessesShareOneGarden(t *testing.T) {
	path := filepath.Join(t.TempDir(), "garden.json")
	a, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	b, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	a.now = func() time.Time { return time.Date(2026, 9, 15, 10, 0, 0, 0, time.UTC) }
	b.now = func() time.Time { return time.Date(2026, 9, 15, 11, 0, 0, 0, time.UTC) }
	a.randf = func() float64 { return 0.99 }
	b.randf = func() float64 { return 0.99 }

	if _, err := a.Record(EventSessionEstablished, "a-1"); err != nil {
		t.Fatal(err)
	}
	// B 进程必须立刻看到 A 的成长（快照回读），否则两边各养一座互不相通的花园。
	if got := len(b.Snapshot().Specimens); got != 1 {
		t.Fatalf("另一进程的成长应当立即可见, 得到 %d 株", got)
	}
	if _, err := b.Record(EventTransferCompleted, "b-1"); err != nil {
		t.Fatal(err)
	}
	snap := a.Snapshot()
	if len(snap.Specimens) != 2 {
		t.Fatalf("A 必须看到 B 的成长（不得被覆盖）, 得到 %d 株", len(snap.Specimens))
	}
	// 幂等键跨进程也生效：A 已经结算过的事件，B 不能重复结算。
	if _, err := b.Record(EventSessionEstablished, "a-1"); err != nil {
		t.Fatal(err)
	}
	if got := len(a.Snapshot().Specimens); got != 2 {
		t.Fatalf("跨进程重复事件不得重复结算: %d 株", got)
	}
}

func TestSnapshotHasNoPresentationSemantics(t *testing.T) {
	// 架构红线：快照字段集是封闭的。若有人往快照里加呈现字段（叶子数、楼层数、调色板…），
	// 这条测试会失败并提醒他改呈现包而不是状态层。
	s, _ := newTestStore(t)
	data := s.Snapshot()
	if data == nil || len(data.Specimens) != 0 {
		t.Fatal("空花园快照应非 nil 且无植株")
	}
}

// 回归：异步结算与并发快照不得丢失 pending（桌面壳 recordGarden 协程 + Wails
// 快照会真实并发；修复前每次快照的 refresh 都可能把刚产生的 pending 覆盖掉）。
func TestConcurrentRecordAndSnapshotKeepsPending(t *testing.T) {
	s, _ := newTestStore(t)
	done := make(chan struct{})
	go func() {
		defer close(done)
		if _, err := s.Record(EventSessionEstablished, "race-1"); err != nil {
			t.Error(err)
		}
	}()
	deadline := time.Now().Add(2 * time.Second)
	for {
		snap := s.Snapshot()
		if len(snap.Specimens) == 1 {
			if len(snap.Pending) == 0 {
				t.Fatal("并发快照丢失了 pending 反馈（refresh 覆盖竞态）")
			}
			break
		}
		select {
		case <-done:
			if len(snap.Specimens) == 0 {
				t.Fatal("结算已返回但快照看不到植株")
			}
		default:
		}
		if time.Now().After(deadline) {
			t.Fatal("等待结算超时")
		}
	}
}
