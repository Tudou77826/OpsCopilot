// Package garden 实现养成系统的状态层：固定的养成属性、行为事件结算、
// 等级与形态阶段计算、闪光保底和原子持久化。
//
// 架构约束（docs/garden-presentation-architecture.md）：本包是唯一事实来源，
// 只认识"属性 + 数值 + 事件"，不知道任何呈现语义（植物、像素、建筑……）。
// 前端呈现包消费本包输出的只读快照；换呈现包不得影响这里的数据。
//
// 防刷规则（docs/garden-design.md §7.3）：同一幂等键只结算一次、每类行为有日上限、
// 事件只在后端可验证的业务结果处产生。
package garden

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"opscopilot/pkg/filetxn"
	"sort"
	"time"
)

// SchemaVersion 是花园数据结构的版本；升级必须走显式迁移。
const SchemaVersion = 1

// RuleVersion 是成长规则的版本；调整数值时递增，已结算事件不重复结算。
const RuleVersion = 1

// MaxLevel 每株的等级上限；StagePerLevel 每 stagePerLevel 级一次明显形态变化。
const (
	MaxLevel        = 30
	StagePerLevel   = 5
	NumStages       = 6 // 发芽/幼苗/分枝/繁茂/开花/成熟
	DailyCapPerKind = 8 // 每类行为每日结算上限
	// PityAt 是保底的合格事件计数：连续 PityAt 次合格事件未出现闪光发现时，
	// 下一次合格事件强制发现一株闪光（跳过发现节奏与个体上限——收藏性质让位）。
	// 以事件而非发现计数，是因为个体上限会让"发现"天然稀疏，40 次发现不可达。
	PityAt      = 40
	ShinyChance = 0.02
	// MaxPerSpecies 是单物种个体数上限：达到后该物种不再产生新植株，成长只流向
	// 最年轻的可成长个体；全部常青后，该物种的合格事件只溢出为花园进度。
	MaxPerSpecies = 6
	// DiscoverEvery 是"每 N 次同物种合格事件产生一次新植株发现"的节奏。
	// 发现独立于成长流向：它决定闪光保底的推进速度（garden-design.md §9.2 的
	// "合格的新植株发现"由此定义），也是常青后仍可抽闪光的途径。
	DiscoverEvery = 1
)

// SpeciesID 是稳定的语义位标识：可以改展示名，不能改 id（呈现包靠它对应形象）。
type SpeciesID string

const (
	SpeciesSession   SpeciesID = "session-tree"   // 连接与会话
	SpeciesCmd       SpeciesID = "cmd-mint"       // 快捷命令
	SpeciesScript    SpeciesID = "script-vine"    // 脚本自动化
	SpeciesTransfer  SpeciesID = "transfer-fern"  // 文件传输
	SpeciesGuard     SpeciesID = "guard-orchid"   // 安全治理
	SpeciesKnowledge SpeciesID = "knowledge-tree" // 诊断与知识
)

// EventKind 是合格行为的类别。一枚举值对应一个物种与一个日上限桶。
type EventKind string

const (
	EventSessionEstablished EventKind = "session-established"
	EventSessionGraceful    EventKind = "session-graceful"
	EventTransferCompleted  EventKind = "transfer-completed"
	EventScriptReplayDone   EventKind = "script-replay-done"
	EventDiagnosisSaved     EventKind = "diagnosis-saved"
)

// speciesOfEvent 把行为类别映射到受益物种。新增类别时同步扩充。
func speciesOfEvent(kind EventKind) SpeciesID {
	switch kind {
	case EventSessionEstablished, EventSessionGraceful:
		return SpeciesSession
	case EventTransferCompleted:
		return SpeciesTransfer
	case EventScriptReplayDone:
		return SpeciesScript
	case EventDiagnosisSaved:
		return SpeciesKnowledge
	default:
		return ""
	}
}

// xpFor 是单次合格事件的成长值。数值集中在此，调整时递增 RuleVersion。
func xpFor(kind EventKind) int {
	switch kind {
	case EventSessionEstablished:
		return 10
	case EventSessionGraceful:
		return 4
	case EventTransferCompleted:
		return 8
	case EventScriptReplayDone:
		return 12
	case EventDiagnosisSaved:
		return 15
	default:
		return 0
	}
}

// xpToNext 是升到下一级所需成长值：线性 + 轻微曲线，30 级合计约 4200。
func xpToNext(level int) int { return 80 + level*20 }

// StageOf 由等级推出形态阶段（0–5）；等级 1 处于发芽。
func StageOf(level int) int {
	if level < 1 {
		return 0
	}
	s := (level - 1) / StagePerLevel
	if s > NumStages-1 {
		s = NumStages - 1
	}
	return s
}

// Specimen 是一个养成实例。字段即架构文档 2.1 节的固定属性集合，
// 呈现包不得要求增删字段。
type Specimen struct {
	SpeciesID  SpeciesID `json:"speciesId"`
	InstanceID string    `json:"instanceId"`
	Level      int       `json:"level"`
	XP         int       `json:"xp"`
	Shiny      bool      `json:"shiny"`
	AcquiredAt time.Time `json:"acquiredAt"`
	LastGrewAt time.Time `json:"lastGrewAt"`
	Milestones []string  `json:"milestones"`
	SlotID     string    `json:"slotId"`
}

// Stage 返回当前形态阶段。
func (s *Specimen) Stage() int { return StageOf(s.Level) }

// State 是持久化的花园整体状态。
type State struct {
	SchemaVersion int         `json:"schemaVersion"`
	RuleVersion   int         `json:"ruleVersion"`
	GardenLevel   int         `json:"gardenLevel"`
	Specimens     []*Specimen `json:"specimens"`
	// Seen 记录已结算的幂等键，防止同一业务结果重复结算。
	Seen map[string]int64 `json:"seen"`
	// Daily 记录"UTC 日期 → 行为类别 → 当日已结算次数"，实现日上限。
	Daily map[string]map[EventKind]int `json:"daily"`
	// PitySinceShiny 距上次闪光的新植株发现数；达到 PityAt 时下一次保底。
	PitySinceShiny int       `json:"pitySinceShiny"`
	StartedAt      time.Time `json:"startedAt"`
	// pending 是未领取的反馈队列，随快照返回、由 Dismiss 消费；不需要持久化——
	// 反馈只是表现层的事，丢了不影响收藏与数值（garden-design.md §10 的"先持久化"指的是发现结果本身）。
	pending []Pending
	// eventCount 记每个物种的合格事件累计数，驱动发现节奏（DiscoverEvery）。
	eventCount map[SpeciesID]int
}

// Store 是挂接在数据目录上的花园存储。零值不可用；用 Open 打开。
type Store struct {
	path string
	now  func() time.Time
	// randf 返回 [0,1) 随机数；测试注入固定序列。
	randf func() float64
	state *State
}

// Open 打开（或初始化）path 处的花园存储。
//
// 关键约定：**文件是唯一真相，内存只是缓存**。桌面壳与 Teams 插件可以指向同一数据目录
// （shared storage 语义，见 docs/garden-design.md §11），两边都会因各自的行为写入，
// 所以每次读取都要回读、每次写入都要在文件锁内"回读 → 改 → 写回"（pkg/filetxn 的既有语义），
// 否则两边的成长会互相覆盖。
func Open(path string) (*Store, error) {
	s := &Store{path: path, now: time.Now, randf: defaultRand}
	if err := s.refresh(); err != nil {
		return nil, err
	}
	return s, nil
}

// refresh 从磁盘装载状态；文件不存在时初始化为空花园。
func (s *Store) refresh() error {
	data, err := filetxn.Read(s.path)
	if err != nil {
		return fmt.Errorf("读取花园数据失败: %w", err)
	}
	if data == nil {
		s.state = &State{
			SchemaVersion: SchemaVersion, RuleVersion: RuleVersion,
			Seen: map[string]int64{}, Daily: map[string]map[EventKind]int{},
			eventCount: map[SpeciesID]int{}, StartedAt: s.now(),
		}
		return nil
	}
	next := &State{}
	if err := json.Unmarshal(data, next); err != nil {
		return fmt.Errorf("花园数据损坏（%s），请从备份恢复或删除该文件重新开始: %w", s.path, err)
	}
	if next.SchemaVersion > SchemaVersion {
		return fmt.Errorf("花园数据版本 %d 高于当前支持的 %d，请升级 OpsCopilot", next.SchemaVersion, SchemaVersion)
	}
	if next.Seen == nil {
		next.Seen = map[string]int64{}
	}
	if next.Daily == nil {
		next.Daily = map[string]map[EventKind]int{}
	}
	if next.eventCount == nil {
		next.eventCount = map[SpeciesID]int{}
	}
	next.SchemaVersion = SchemaVersion
	next.RuleVersion = RuleVersion
	// 反馈队列不持久化（它只服务表现层），但正在运行的实例可能刚产出反馈，
	// 不能在回读时丢掉内存里尚未展示的那几条。首次装载时没有内存状态。
	if s.state != nil {
		next.pending = s.state.pending
	}
	s.state = next
	return nil
}

// beginMutation 取得文件锁并回读最新状态，返回释放函数。
// 调用方随后必须调用 Save 把结果写回，再释放锁。
func (s *Store) beginMutation() (func(), error) {
	unlock, err := filetxn.Lock(s.path)
	if err != nil {
		return nil, fmt.Errorf("锁定花园数据失败: %w", err)
	}
	if err := s.refresh(); err != nil {
		unlock()
		return nil, err
	}
	return unlock, nil
}

// Snapshot 是暴露给前端的只读快照（无呈现语义）。
type Snapshot struct {
	SchemaVersion  int         `json:"schemaVersion"`
	RuleVersion    int         `json:"ruleVersion"`
	GardenLevel    int         `json:"gardenLevel"`
	Specimens      []*Specimen `json:"specimens"`
	PitySinceShiny int         `json:"pitySinceShiny"`
	Pending        []Pending   `json:"pending"`
}

// Pending 是未领取的发现或成长反馈（garden-design.md §10：先持久化再表现）。
type Pending struct {
	Kind       string    `json:"kind"` // discovered | shiny-discovered | levelup | stage
	InstanceID string    `json:"instanceId,omitempty"`
	SpeciesID  SpeciesID `json:"speciesId,omitempty"`
	Level      int       `json:"level,omitempty"`
	At         time.Time `json:"at"`
}

// Snapshot 返回当前状态的只读视图。Specimens 按获得时间排序，保证渲染稳定。
//
// 每次调用都回读磁盘：桌面壳与插件可能共用数据目录，不回读就会显示另一进程写入前的旧状态。
// 回读失败时退回内存缓存（快照是只读展示，不该因为一次读失败让整个面板不可用）。
func (s *Store) Snapshot() *Snapshot {
	if err := s.refresh(); err != nil {
		// 保留上次成功装载的状态；错误留给调用方的日志，不在这里打断渲染。
	}
	specs := make([]*Specimen, len(s.state.Specimens))
	copy(specs, s.state.Specimens)
	sort.Slice(specs, func(i, j int) bool { return specs[i].AcquiredAt.Before(specs[j].AcquiredAt) })
	return &Snapshot{
		SchemaVersion: s.state.SchemaVersion, RuleVersion: s.state.RuleVersion,
		GardenLevel: s.state.GardenLevel, Specimens: specs,
		PitySinceShiny: s.state.PitySinceShiny,
		// 集合永不为 nil：JSON 里的 null 会让前端把"空"当成"缺失"（本项目曾因此黑屏）。
		Pending: append([]Pending{}, s.state.pending...),
	}
}

// settle 是事件结算的核心：幂等检查 → 日上限 → 找目标实例（无则新发现）→ 加经验 → 升级。
// shiny 发现只发生在"新植株"时；保底进度先持久化再返回结果。
func (s *Store) settle(kind EventKind, dedupeKey string) (Outcome, error) {
	if speciesOfEvent(kind) == "" {
		return Outcome{}, fmt.Errorf("未知的行为类别 %q", kind)
	}
	if dedupeKey == "" {
		return Outcome{}, fmt.Errorf("缺少幂等键")
	}
	if _, done := s.state.Seen[dedupeKey]; done {
		return Outcome{Duplicate: true}, nil
	}
	day := s.now().UTC().Format("2006-01-02")
	if s.state.Daily[day] == nil {
		s.state.Daily[day] = map[EventKind]int{}
	}
	if s.state.Daily[day][kind] >= DailyCapPerKind {
		return Outcome{Capped: true}, nil
	}

	species := speciesOfEvent(kind)
	gain := xpFor(kind)
	now := s.now()
	out := Outcome{Kind: kind, SpeciesID: species, XPGained: gain}

	spec := s.findYoungest(species)
	// 保底以"被受理的合格事件"计数，在受理处统一推进——无论该事件之后走
	// 发现、成长还是溢出分支。否则个体满员、事件只产生零星升级时，
	// 保底会永远冻结（实测抓到过）。计数到 PityAt 时强制发现一株闪光，
	// 无视发现节奏与个体上限（上限是防量产普通株的，不拦收藏性质的保底）。
	pityBefore := s.state.PitySinceShiny
	s.state.PitySinceShiny++
	s.state.eventCount[species]++
	pityDue := pityBefore+1 >= PityAt
	// 发现节奏独立于成长流向：同物种每 DiscoverEvery 次合格事件发现一株新个体
	// （物种个体数未达上限时）。
	isNew := pityDue ||
		(s.countSpecies(species) == 0 && s.state.eventCount[species] == 1) ||
		(s.state.eventCount[species]%DiscoverEvery == 0 && s.countSpecies(species) < MaxPerSpecies)
	// 溢出分支：物种个体已满且全部常青、保底也未到期——没有可成长的个体，
	// 事件照常登记（幂等、日上限照算），成长转为花园进度（+1 级），不产生也不修改个体。
	if spec == nil && !isNew {
		s.state.GardenLevel++
		out.Overflowed = true
		s.state.Seen[dedupeKey] = now.Unix()
		s.state.Daily[day][kind]++
		s.state.pending = append(s.state.pending, Pending{Kind: "overflow", SpeciesID: species, At: now})
		return out, nil
	}
	if isNew {
		shiny := false
		if s.state.PitySinceShiny+1 >= PityAt {
			shiny = true
		} else if s.randf() < ShinyChance {
			shiny = true
		}
		if shiny {
			s.state.PitySinceShiny = 0
		}
		spec = &Specimen{
			SpeciesID: species, InstanceID: fmt.Sprintf("i_%d", now.UnixNano()),
			Level: 1, Shiny: shiny, AcquiredAt: now, LastGrewAt: now,
		}
		s.state.Specimens = append(s.state.Specimens, spec)
		out.Discovered = true
		out.Shiny = shiny
	}

	// 经验与升级：从 1 级起步的实例第一次结算也会立即到达可成长的量级。
	spec.XP += gain
	spec.LastGrewAt = now
	for spec.Level < MaxLevel && spec.XP >= xpToNext(spec.Level) {
		spec.XP -= xpToNext(spec.Level)
		spec.Level++
		out.LevelUps++
		out.Level = spec.Level
	}
	if isNew {
		out.Level = spec.Level
	}

	s.state.Seen[dedupeKey] = now.Unix()
	s.state.Daily[day][kind]++
	s.recomputeGardenLevel()

	s.state.pending = append(s.state.pending, Pending{
		Kind: pendingKind(out), InstanceID: spec.InstanceID, SpeciesID: species, Level: spec.Level, At: now,
	})
	return out, nil
}

// pendingKind 把结算结果映射为反馈类别。
func pendingKind(o Outcome) string {
	switch {
	case o.Discovered && o.Shiny:
		return "shiny-discovered"
	case o.Discovered:
		return "discovered"
	case o.LevelUps > 0:
		return "levelup"
	default:
		return "growth"
	}
}

// findYoungest 返回该物种最新获得的实例；新成长优先落在最年轻的个体上，
// 老个体保留为收藏。没有实例时返回 nil。
func (s *Store) findYoungest(species SpeciesID) *Specimen {
	var youngest *Specimen
	for _, spec := range s.state.Specimens {
		if spec.SpeciesID != species {
			continue
		}
		if spec.Level >= MaxLevel {
			continue // 常青个体不再吸收成长，成长流向新植株（garden-design.md §7.1）
		}
		if youngest == nil || spec.AcquiredAt.After(youngest.AcquiredAt) {
			youngest = spec
		}
	}
	return youngest
}

// countSpecies 统计某物种的个体数。
func (s *Store) countSpecies(species SpeciesID) int {
	n := 0
	for _, spec := range s.state.Specimens {
		if spec.SpeciesID == species {
			n++
		}
	}
	return n
}

// recomputeGardenLevel 由"物种广度 + 总等级"推导花园等级：
// 覆盖的物种数决定大档，总等级决定档内进度。简单、可解释、不可刷。
func (s *Store) recomputeGardenLevel() {
	species := map[SpeciesID]bool{}
	totalLevel := 0
	for _, spec := range s.state.Specimens {
		species[spec.SpeciesID] = true
		totalLevel += spec.Level
	}
	breadth := len(species)
	level := breadth*2 + totalLevel/10
	if level < 1 {
		level = 1
	}
	s.state.GardenLevel = level
}

// Outcome 是一次结算的用户可见结果。
type Outcome struct {
	Kind       EventKind `json:"kind"`
	SpeciesID  SpeciesID `json:"speciesId"`
	XPGained   int       `json:"xpGained"`
	Discovered bool      `json:"discovered"`
	Shiny      bool      `json:"shiny"`
	LevelUps   int       `json:"levelUps"`
	Overflowed bool      `json:"overflowed"`
	Level      int       `json:"level"`
	Duplicate  bool      `json:"duplicate"`
	Capped     bool      `json:"capped"`
}

// Record 把一次合格业务事件交给花园结算。dedupeKey 必须来自业务结果本身
// （如传输任务 id、回放批次 id），同一 key 只结算一次。
func (s *Store) Record(kind EventKind, dedupeKey string) (Outcome, error) {
	unlock, err := s.beginMutation()
	if err != nil {
		return Outcome{}, err
	}
	defer unlock()
	out, err := s.settle(kind, dedupeKey)
	if err != nil {
		return out, err
	}
	if !out.Duplicate && !out.Capped {
		if err := s.Save(); err != nil {
			return out, err
		}
	}
	return out, nil
}

// Save 原子落盘。结算成功后必须落盘，保证"结果先持久化，再播放表现"。
func (s *Store) Save() error {
	data, err := json.MarshalIndent(s.state, "", "  ")
	if err != nil {
		return err
	}
	return filetxn.Write(s.path, data)
}

// Dismiss 把一条反馈标记为已读。反馈在快照里随 Pending 返回，前端展示后调用。
func defaultRand() float64 { return rand.Float64() }

// DismissAt 按发生时间（UnixNano）移除一条反馈。
func (s *Store) DismissAt(nano int64) {
	unlock, err := s.beginMutation()
	if err != nil {
		return
	}
	defer unlock()
	kept := s.state.pending[:0]
	for _, p := range s.state.pending {
		if p.At.UnixNano() != nano {
			kept = append(kept, p)
		}
	}
	s.state.pending = kept
	_ = s.Save()
}
