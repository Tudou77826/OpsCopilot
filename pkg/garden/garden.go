// Package garden 实现个人场景的状态层：工作事件结算货币、购买收藏、
// 自由布局和原子持久化。
//
// 架构约束（docs/garden-presentation-architecture.md）：本包是唯一事实来源，
// 只认识"属性 + 数值 + 事件"，不知道任何呈现语义（植物、像素、建筑……）。
// 前端呈现包消费本包输出的只读快照；换呈现包不得影响这里的数据。
//
// 防刷规则（docs/garden-design.md §7.3）：同一幂等键只结算一次、每类行为有日上限、
// 事件只在后端可验证的业务结果处产生。
package garden

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math/big"
	"opscopilot/pkg/filetxn"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// SchemaVersion 是养成状态结构的版本。功能尚未发布，v3 直接替换开发期结构，
// 不读取或迁移旧数据；遇到旧文件时要求显式清理。
const SchemaVersion = 3

// RuleVersion 是成长规则的版本；调整数值时递增，已结算事件不重复结算。
const RuleVersion = 3

// MaxLevel 每株的等级上限；StagePerLevel 每 stagePerLevel 级一次明显形态变化。
const (
	MaxLevel        = 100
	StagePerLevel   = 10
	NumStages       = 6 // 发芽/幼苗/分枝/繁茂/开花/成熟
	DailyCapPerKind = 8
	StartingBalance = 120
	MaxInventory    = 200
	MaxItemPrice    = 100000
	ShinyPityAt     = 40
	ShinyChance     = 0.02
)

// GrowthChannel 描述一次有效工作贡献到了哪个成长方向。它只表达行为领域，
// 不包含植物、动物或建筑等呈现语义。
type GrowthChannel string

const (
	ChannelSession   GrowthChannel = "session"
	ChannelCommand   GrowthChannel = "command"
	ChannelScript    GrowthChannel = "script"
	ChannelTransfer  GrowthChannel = "transfer"
	ChannelGuard     GrowthChannel = "guard"
	ChannelKnowledge GrowthChannel = "knowledge"
)

// ItemID 是内容包内收藏元素的稳定标识；状态层不解释它是植物、动物还是建筑。
type ItemID string

// EventKind 是合格行为的类别。一枚举值对应一个成长通道与一个日上限桶。
type EventKind string

const (
	EventSessionEstablished EventKind = "session-established"
	EventSessionGraceful    EventKind = "session-graceful"
	EventTransferCompleted  EventKind = "transfer-completed"
	EventScriptReplayDone   EventKind = "script-replay-done"
	EventDiagnosisSaved     EventKind = "diagnosis-saved"
)

// channelOfEvent 把行为类别映射到成长通道。新增类别时同步扩充。
func channelOfEvent(kind EventKind) GrowthChannel {
	switch kind {
	case EventSessionEstablished, EventSessionGraceful:
		return ChannelSession
	case EventTransferCompleted:
		return ChannelTransfer
	case EventScriptReplayDone:
		return ChannelScript
	case EventDiagnosisSaved:
		return ChannelKnowledge
	default:
		return ""
	}
}

// currencyFor 是单次合格事件奖励的货币。数值集中在此，调整时递增 RuleVersion。
func currencyFor(kind EventKind) int {
	switch kind {
	case EventSessionEstablished:
		return 8
	case EventSessionGraceful:
		return 3
	case EventTransferCompleted:
		return 10
	case EventScriptReplayDone:
		return 12
	case EventDiagnosisSaved:
		return 16
	default:
		return 0
	}
}

// xpToNext 是升到下一级所需成长值。等级从 0 开始。
func xpToNext(level int) int { return 80 + level*20 }

// StageOf 由等级推出形态阶段（0–5）：0–9 为第 0 档，50–100 固定第 5 档。
func StageOf(level int) int {
	if level < 0 {
		return 0
	}
	s := level / StagePerLevel
	if s > NumStages-1 {
		s = NumStages - 1
	}
	return s
}

// Specimen 是一个养成实例。字段即架构文档 2.1 节的固定属性集合，
// 呈现包不得要求增删字段。
type Specimen struct {
	ItemID     ItemID     `json:"itemId"`
	InstanceID string     `json:"instanceId"`
	Level      int        `json:"level"`
	XP         int        `json:"xp"`
	Shiny      bool       `json:"shiny"`
	AcquiredAt time.Time  `json:"acquiredAt"`
	LastGrewAt time.Time  `json:"lastGrewAt"`
	Milestones []string   `json:"milestones"`
	Placement  *Placement `json:"placement,omitempty"`
}

// Placement 是题材无关的归一化布局。元素自身的落点、碰撞和可活动区域由内容包解释。
type Placement struct {
	Placed bool    `json:"placed"`
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Scale  float64 `json:"scale"`
	FlipX  bool    `json:"flipX"`
}

// Stage 返回当前形态阶段。
func (s *Specimen) Stage() int { return StageOf(s.Level) }

// State 是持久化的花园整体状态。
type State struct {
	SchemaVersion int `json:"schemaVersion"`
	RuleVersion   int `json:"ruleVersion"`
	GardenLevel   int `json:"gardenLevel"`
	Balance       int `json:"balance"`
	Earned        int `json:"earned"`
	Spent         int `json:"spent"`
	// PurchasesSinceShiny 只用于下一次购买的隐藏品质保底，不形成待办或进度压力。
	PurchasesSinceShiny int         `json:"purchasesSinceShiny"`
	Specimens           []*Specimen `json:"specimens"`
	// Seen 记录已结算的幂等键，防止同一业务结果重复结算。
	Seen map[string]int64 `json:"seen"`
	// Daily 记录"UTC 日期 → 行为类别 → 当日已结算次数"，实现日上限。
	Daily     map[string]map[EventKind]int `json:"daily"`
	StartedAt time.Time                    `json:"startedAt"`
	// ChangeSignal 只记录最近一次值得轻提示的可见变化。Revision 单调递增，
	// 多次变化天然合并成一次入口闪动，不形成待领取队列或红点压力。
	ChangeSignal *ChangeSignal `json:"changeSignal"`
}

// Store 是挂接在数据目录上的花园存储。零值不可用；用 Open 打开。
type Store struct {
	enabled atomic.Bool
	// mu 串行化本进程内的状态访问；文件仍是桌面壳与插件共享的唯一真相。
	mu    sync.Mutex
	path  string
	now   func() time.Time
	randf func() (float64, error)
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
	s.enabled.Store(true)
	if err := s.refresh(); err != nil {
		return nil, err
	}
	return s, nil
}

// NewDormant creates a disabled store without touching garden data.
func NewDormant(path string) *Store {
	return &Store{path: path, now: time.Now, randf: defaultRand}
}

func (s *Store) Enabled() bool { return s.enabled.Load() }

// SetEnabled waits for any in-flight operation before disabling settlement.
func (s *Store) SetEnabled(enabled bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.enabled.Store(enabled)
}

func (s *Store) ReadSnapshot() (*Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.Enabled() {
		return nil, fmt.Errorf("养成功能未启用")
	}
	if err := s.refreshLocked(); err != nil {
		return nil, err
	}
	return s.snapshotLocked(), nil
}

func (s *Store) ReadSignal() (*ChangeSignal, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.Enabled() {
		return nil, fmt.Errorf("养成功能未启用")
	}
	if err := s.refreshLocked(); err != nil {
		return nil, err
	}
	return cloneSignal(s.state.ChangeSignal), nil
}

// refresh 从磁盘装载状态；文件不存在时初始化为空花园。
func (s *Store) refresh() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.refreshLocked()
}

func (s *Store) refreshLocked() error {
	data, err := filetxn.Read(s.path)
	if err != nil {
		return fmt.Errorf("读取花园数据失败: %w", err)
	}
	if data == nil {
		s.state = &State{
			SchemaVersion: SchemaVersion, RuleVersion: RuleVersion,
			GardenLevel: 0, Balance: StartingBalance,
			Seen: map[string]int64{}, Daily: map[string]map[EventKind]int{},
			StartedAt: s.now(), Specimens: []*Specimen{},
		}
		return nil
	}
	next := &State{}
	if err := json.Unmarshal(data, next); err != nil {
		return fmt.Errorf("花园数据损坏（%s），请从备份恢复或删除该文件重新开始: %w", s.path, err)
	}
	if next.SchemaVersion != SchemaVersion {
		return fmt.Errorf("养成数据版本 %d 与当前开发版本 %d 不兼容；功能尚未发布，请删除 %s 后重新开始", next.SchemaVersion, SchemaVersion, s.path)
	}
	if next.Seen == nil {
		next.Seen = map[string]int64{}
	}
	if next.Daily == nil {
		next.Daily = map[string]map[EventKind]int{}
	}
	if next.Specimens == nil {
		next.Specimens = []*Specimen{}
	}
	s.state = next
	return nil
}

// beginMutation 取得文件锁并回读最新状态，返回释放函数（只释放文件锁）。
// 调用方必须已持有 s.mu，随后必须调用 Save 把结果写回，再释放两把锁。
func (s *Store) beginMutation() (func(), error) {
	if !s.Enabled() {
		return nil, fmt.Errorf("养成功能未启用")
	}
	unlock, err := filetxn.Lock(s.path)
	if err != nil {
		return nil, fmt.Errorf("锁定花园数据失败: %w", err)
	}
	if err := s.refreshLocked(); err != nil {
		unlock()
		return nil, err
	}
	return unlock, nil
}

// Snapshot 是暴露给前端的只读快照（无呈现语义）。
type Snapshot struct {
	SchemaVersion int           `json:"schemaVersion"`
	RuleVersion   int           `json:"ruleVersion"`
	GardenLevel   int           `json:"gardenLevel"`
	Balance       int           `json:"balance"`
	Earned        int           `json:"earned"`
	Spent         int           `json:"spent"`
	Specimens     []*Specimen   `json:"specimens"`
	ChangeSignal  *ChangeSignal `json:"changeSignal"`
}

// ChangeSignal 是低打扰入口提示所需的最小信息。它不是待办、未读数或领取队列；
// 客户端只比较 Revision，并在一次短暂闪动后恢复普通状态。
type ChangeSignal struct {
	Revision   uint64    `json:"revision"`
	Kind       string    `json:"kind"` // currency-earned
	Amount     int       `json:"amount,omitempty"`
	InstanceID string    `json:"instanceId,omitempty"`
	ItemID     ItemID    `json:"itemId,omitempty"`
	At         time.Time `json:"at"`
}

// PurchaseResult 是一次购买后的权威余额与新实例。
type PurchaseResult struct {
	Balance  int       `json:"balance"`
	Specimen *Specimen `json:"specimen"`
}

func validItemID(id ItemID) bool {
	if len(id) == 0 || len(id) > 128 {
		return false
	}
	for _, ch := range id {
		if (ch < 'a' || ch > 'z') && (ch < '0' || ch > '9') && ch != '-' && ch != '.' {
			return false
		}
	}
	return true
}

// Purchase 购买内容包声明的元素。价格和初始等级随内容包请求传入；这是本地、
// 非竞争性功能，状态层只维护余额、范围和库存上限，不解释商品题材。
func (s *Store) Purchase(itemID ItemID, price, initialLevel int) (*PurchaseResult, error) {
	if !validItemID(itemID) {
		return nil, fmt.Errorf("收藏元素 id 无效")
	}
	if price <= 0 || price > MaxItemPrice {
		return nil, fmt.Errorf("收藏元素价格无效")
	}
	if initialLevel < 0 || initialLevel > MaxLevel {
		return nil, fmt.Errorf("收藏元素初始等级无效")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.beginMutation()
	if err != nil {
		return nil, err
	}
	defer unlock()
	if len(s.state.Specimens) >= MaxInventory {
		return nil, fmt.Errorf("收藏库存已满（%d）", MaxInventory)
	}
	if s.state.Balance < price {
		return nil, fmt.Errorf("灵感币不足：需要 %d，当前 %d", price, s.state.Balance)
	}
	now := s.now()
	roll, err := s.randf()
	if err != nil {
		return nil, fmt.Errorf("生成收藏品质失败: %w", err)
	}
	shiny := s.state.PurchasesSinceShiny >= ShinyPityAt-1 || roll < ShinyChance
	if shiny {
		s.state.PurchasesSinceShiny = 0
	} else {
		s.state.PurchasesSinceShiny++
	}
	spec := &Specimen{ItemID: itemID, InstanceID: fmt.Sprintf("i_%d", now.UnixNano()), Level: initialLevel,
		Shiny: shiny, AcquiredAt: now, LastGrewAt: now, Milestones: []string{}, Placement: &Placement{Scale: 1}}
	s.state.Specimens = append(s.state.Specimens, spec)
	s.state.Balance -= price
	s.state.Spent += price
	s.recomputeGardenLevel()
	if err := s.Save(); err != nil {
		return nil, err
	}
	copy := *spec
	placement := *spec.Placement
	copy.Placement = &placement
	return &PurchaseResult{Balance: s.state.Balance, Specimen: &copy}, nil
}

func (s *Store) findSpecimen(instanceID string) *Specimen {
	for _, spec := range s.state.Specimens {
		if spec.InstanceID == instanceID {
			return spec
		}
	}
	return nil
}

// Place 保存归一化自由布局；坐标与缩放范围由状态层守住，视觉碰撞由内容包处理。
func (s *Store) Place(instanceID string, x, y, scale float64, flipX bool) (*Snapshot, error) {
	if instanceID == "" || x < 0 || x > 1 || y < 0 || y > 1 || scale < 0.5 || scale > 1.5 {
		return nil, fmt.Errorf("摆放参数无效")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.beginMutation()
	if err != nil {
		return nil, err
	}
	defer unlock()
	spec := s.findSpecimen(instanceID)
	if spec == nil {
		return nil, fmt.Errorf("收藏实例不存在")
	}
	spec.Placement = &Placement{Placed: true, X: x, Y: y, Scale: scale, FlipX: flipX}
	if err := s.Save(); err != nil {
		return nil, err
	}
	return s.snapshotLocked(), nil
}

// Stow 把元素收回库存但不删除收藏。
func (s *Store) Stow(instanceID string) (*Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.beginMutation()
	if err != nil {
		return nil, err
	}
	defer unlock()
	spec := s.findSpecimen(instanceID)
	if spec == nil {
		return nil, fmt.Errorf("收藏实例不存在")
	}
	spec.Placement = &Placement{Scale: 1}
	if err := s.Save(); err != nil {
		return nil, err
	}
	return s.snapshotLocked(), nil
}

// Snapshot 返回当前状态的只读视图。Specimens 按获得时间排序，保证渲染稳定。
//
// 每次调用都回读磁盘：桌面壳与插件可能共用数据目录，不回读就会显示另一进程写入前的旧状态。
// 回读失败时退回内存缓存（快照是只读展示，不该因为一次读失败让整个面板不可用）。
func (s *Store) Snapshot() *Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.Enabled() {
		return nil
	}
	if err := s.refreshLocked(); err != nil {
		// 保留上次成功装载的状态；错误留给调用方的日志，不在这里打断渲染。
	}
	return s.snapshotLocked()
}

func (s *Store) snapshotLocked() *Snapshot {
	specs := make([]*Specimen, len(s.state.Specimens))
	for i, source := range s.state.Specimens {
		copy := *source
		if source.Placement != nil {
			placement := *source.Placement
			copy.Placement = &placement
		}
		specs[i] = &copy
	}
	sort.Slice(specs, func(i, j int) bool { return specs[i].AcquiredAt.Before(specs[j].AcquiredAt) })
	return &Snapshot{
		SchemaVersion: s.state.SchemaVersion, RuleVersion: s.state.RuleVersion,
		GardenLevel: s.state.GardenLevel, Specimens: specs,
		Balance: s.state.Balance, Earned: s.state.Earned, Spent: s.state.Spent,
		ChangeSignal: cloneSignal(s.state.ChangeSignal),
	}
}

// Signal 返回轻量变化游标，不包含收藏清单或呈现资源。
func (s *Store) Signal() *ChangeSignal {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.Enabled() {
		return nil
	}
	if err := s.refreshLocked(); err != nil {
		return cloneSignal(s.state.ChangeSignal)
	}
	return cloneSignal(s.state.ChangeSignal)
}

func cloneSignal(signal *ChangeSignal) *ChangeSignal {
	if signal == nil {
		return nil
	}
	copy := *signal
	return &copy
}

// settle 是事件结算的核心：幂等检查 → 日上限 → 增加货币。
// 工作行为不再直接创造或修改收藏元素，用户通过商店决定获得什么。
func (s *Store) settle(kind EventKind, dedupeKey string) (Outcome, error) {
	channel := channelOfEvent(kind)
	if channel == "" {
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

	gain := currencyFor(kind)
	now := s.now()
	s.state.Balance += gain
	s.state.Earned += gain
	s.state.Seen[dedupeKey] = now.Unix()
	s.state.Daily[day][kind]++
	s.bumpChange("currency-earned", nil, gain, now)
	s.recomputeGardenLevel()
	out := Outcome{Kind: kind, GrowthChannel: channel, CurrencyGained: gain, Balance: s.state.Balance}
	return out, nil
}

func (s *Store) bumpChange(kind string, spec *Specimen, amount int, at time.Time) {
	revision := uint64(1)
	if s.state.ChangeSignal != nil {
		revision = s.state.ChangeSignal.Revision + 1
	}
	signal := &ChangeSignal{Revision: revision, Kind: kind, Amount: amount, At: at}
	if spec != nil {
		signal.InstanceID, signal.ItemID = spec.InstanceID, spec.ItemID
	}
	s.state.ChangeSignal = signal
}

// recomputeGardenLevel 由收藏数量与累计工作收入推导整体等级。
func (s *Store) recomputeGardenLevel() {
	s.state.GardenLevel = len(s.state.Specimens) + s.state.Earned/100
}

func defaultRand() (float64, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		return 0, err
	}
	return float64(n.Int64()) / 1_000_000, nil
}

// Outcome 是一次结算的用户可见结果。
type Outcome struct {
	Kind           EventKind     `json:"kind"`
	GrowthChannel  GrowthChannel `json:"growthChannel"`
	CurrencyGained int           `json:"currencyGained"`
	Balance        int           `json:"balance"`
	Duplicate      bool          `json:"duplicate"`
	Capped         bool          `json:"capped"`
}

// Record 把一次合格业务事件交给花园结算。dedupeKey 必须来自业务结果本身
// （如传输任务 id、回放批次 id），同一 key 只结算一次。
func (s *Store) Record(kind EventKind, dedupeKey string) (Outcome, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.Enabled() {
		return Outcome{}, nil
	}
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
