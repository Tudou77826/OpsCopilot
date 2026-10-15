package companion

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"opscopilot/pkg/filetxn"
)

type daily struct {
	XP    int               `json:"xp"`
	Kinds map[EventKind]int `json:"kinds"`
}
type state struct {
	SchemaVersion int               `json:"schemaVersion"`
	RuleVersion   int               `json:"ruleVersion"`
	Revision      uint64            `json:"revision"`
	Pet           *Pet              `json:"pet"`
	Seen          map[string]bool   `json:"seen"`
	Daily         map[string]*daily `json:"daily"`
}

// Store always reloads under a file lock before mutation. Desktop and sidecar
// share the same file without losing each other's edits. No cache is committed
// before the corresponding atomic file write succeeds.
type Store struct {
	mu      sync.Mutex
	enabled atomic.Bool
	path    string
	now     func() time.Time
	draw    func(int) (int, error)
}

func NewDormant(path string) *Store {
	return &Store{path: path, now: time.Now, draw: func(n int) (int, error) {
		value, err := rand.Int(rand.Reader, big.NewInt(int64(n)))
		if err != nil {
			return 0, err
		}
		return int(value.Int64()), nil
	}}
}
func (s *Store) Enabled() bool         { return s.enabled.Load() }
func (s *Store) SetEnabled(value bool) { s.mu.Lock(); defer s.mu.Unlock(); s.enabled.Store(value) }

func (s *Store) load() (*state, error) {
	data, err := filetxn.Read(s.path)
	if err != nil {
		return nil, fmt.Errorf("读取伙伴数据失败: %w", err)
	}
	if data == nil {
		return &state{SchemaVersion: SchemaVersion, RuleVersion: RuleVersion, Seen: map[string]bool{}, Daily: map[string]*daily{}}, nil
	}
	var st state
	if err := json.Unmarshal(data, &st); err != nil {
		return nil, fmt.Errorf("解析伙伴数据失败: %w", err)
	}
	if st.SchemaVersion != SchemaVersion || st.RuleVersion != RuleVersion {
		return nil, fmt.Errorf("伙伴数据版本不兼容，请保留存档并使用匹配版本")
	}
	if st.Pet != nil && (st.Pet.ID == "" || !validName(st.Pet.Name) || !validAppearance(st.Pet.SpeciesID, st.Pet.PaletteID) || st.Pet.XP < 0 || st.Pet.AdoptedAt.IsZero()) {
		return nil, fmt.Errorf("伙伴数据无效")
	}
	if st.Seen == nil {
		st.Seen = map[string]bool{}
	}
	if st.Daily == nil {
		st.Daily = map[string]*daily{}
	}
	for day, bucket := range st.Daily {
		if _, err := time.Parse("2006-01-02", day); err != nil {
			return nil, fmt.Errorf("伙伴成长日期无效")
		}
		if bucket == nil || bucket.XP < 0 || bucket.XP > DailyXPMax || bucket.Kinds == nil {
			return nil, fmt.Errorf("伙伴成长记录无效")
		}
		for kind, count := range bucket.Kinds {
			if reward(kind) == 0 || count < 0 || count > DailyKindMax {
				return nil, fmt.Errorf("伙伴成长次数无效")
			}
		}
	}
	return &st, nil
}
func snapshot(st *state, now time.Time) *Snapshot {
	out := &Snapshot{SchemaVersion: st.SchemaVersion, RuleVersion: st.RuleVersion, Revision: st.Revision, DailyXPMax: DailyXPMax, CanHatch: true}
	if day := st.Daily[now.UTC().Format("2006-01-02")]; day != nil {
		out.TodayXP = day.XP
	}
	if st.Pet != nil {
		pet := *st.Pet
		out.Pet = &pet
		out.HatchCost = RehatchCost
		out.CanHatch = pet.XP >= RehatchCost
		out.Level = LevelOf(pet.XP)
		if out.Level < MaxLevel {
			out.LevelXP = pet.XP % XPPerLevel
			out.LevelXPRequired = XPPerLevel
		}
	}
	return out
}
func (s *Store) ReadSnapshot() (*Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.Enabled() {
		return nil, ErrDisabled
	}
	st, err := s.load()
	if err != nil {
		return nil, err
	}
	return snapshot(st, s.now()), nil
}
func (s *Store) write(st *state) error {
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return fmt.Errorf("编码伙伴数据失败: %w", err)
	}
	if err := filetxn.Write(s.path, data); err != nil {
		return fmt.Errorf("保存伙伴数据失败: %w", err)
	}
	return nil
}

// Hatch commits the draw and its fee in one transaction. A revision can be spent
// only once, including across desktop and sidecar processes. Existing saves are
// read without migration; only an explicit paid hatch changes their appearance.
func (s *Store) Hatch(revision uint64) (*Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.Enabled() {
		return nil, ErrDisabled
	}
	release, err := filetxn.Lock(s.path)
	if err != nil {
		return nil, err
	}
	defer release()
	st, err := s.load()
	if err != nil {
		return nil, err
	}
	if st.Revision != revision {
		return nil, filetxn.ErrConflict
	}
	if st.Revision == ^uint64(0) {
		return nil, fmt.Errorf("伙伴版本已超限")
	}
	cost, current := 0, ""
	if st.Pet != nil {
		cost, current = RehatchCost, st.Pet.SpeciesID
		if st.Pet.XP < cost {
			return nil, ErrInsufficientXP
		}
	}
	candidates := make([]Species, 0, len(speciesCatalog))
	for _, species := range speciesCatalog {
		if species.ID != current {
			candidates = append(candidates, species)
		}
	}
	if len(candidates) == 0 {
		return nil, fmt.Errorf("没有可开出的伙伴")
	}
	index, err := s.draw(len(candidates))
	if err != nil {
		return nil, fmt.Errorf("开蛋抽取失败: %w", err)
	}
	if index < 0 || index >= len(candidates) {
		return nil, fmt.Errorf("开蛋抽取结果无效")
	}
	species := candidates[index]
	if st.Pet == nil {
		st.Pet = &Pet{ID: uuid.NewString(), Name: species.Name, AdoptedAt: s.now().UTC()}
	}
	st.Pet.SpeciesID, st.Pet.PaletteID = species.ID, "mint"
	st.Pet.XP -= cost
	st.Revision++
	if err := s.write(st); err != nil {
		return nil, err
	}
	return snapshot(st, s.now()), nil
}

// Rename cannot change the draw, palette, XP or settlement history.
func (s *Store) Rename(name string, revision uint64) (*Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.Enabled() {
		return nil, ErrDisabled
	}
	name = strings.TrimSpace(name)
	if !validName(name) {
		return nil, fmt.Errorf("伙伴名称需为 1 至 24 字，且不能包含控制字符")
	}
	release, err := filetxn.Lock(s.path)
	if err != nil {
		return nil, err
	}
	defer release()
	st, err := s.load()
	if err != nil {
		return nil, err
	}
	if st.Pet == nil {
		return nil, fmt.Errorf("尚未开蛋获取伙伴")
	}
	if st.Revision != revision {
		return nil, filetxn.ErrConflict
	}
	if st.Pet.Name == name {
		return snapshot(st, s.now()), nil
	}
	if st.Revision == ^uint64(0) {
		return nil, fmt.Errorf("伙伴版本已超限")
	}
	st.Pet.Name = name
	st.Revision++
	if err := s.write(st); err != nil {
		return nil, err
	}
	return snapshot(st, s.now()), nil
}

// Record only accepts backend business-result keys. Raw keys are hashed before
// storage; no commands, hosts or terminal output are accepted. A capped result
// is also consumed, so replaying it tomorrow cannot earn delayed XP.
func (s *Store) Record(kind EventKind, key string) (Outcome, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.Enabled() {
		return Outcome{}, nil
	}
	gain := reward(kind)
	if gain == 0 || strings.TrimSpace(key) == "" || len(key) > 512 {
		return Outcome{}, fmt.Errorf("伙伴成长事件无效")
	}
	release, err := filetxn.Lock(s.path)
	if err != nil {
		return Outcome{}, err
	}
	defer release()
	st, err := s.load()
	if err != nil {
		return Outcome{}, err
	}
	if st.Pet == nil {
		return Outcome{}, nil
	}
	hash := sha256.Sum256([]byte(string(kind) + "\x00" + key))
	dedupe := hex.EncodeToString(hash[:])
	if st.Seen[dedupe] {
		return Outcome{Duplicate: true}, nil
	}
	day := s.now().UTC().Format("2006-01-02")
	bucket := st.Daily[day]
	if bucket == nil {
		bucket = &daily{Kinds: map[EventKind]int{}}
		st.Daily[day] = bucket
	}
	st.Seen[dedupe] = true
	out := Outcome{}
	if bucket.Kinds[kind] >= DailyKindMax || bucket.XP >= DailyXPMax {
		out.Capped = true
	} else {
		if gain > DailyXPMax-bucket.XP {
			gain = DailyXPMax - bucket.XP
		}
		previous := LevelOf(st.Pet.XP)
		if st.Revision == ^uint64(0) || st.Pet.XP > int(^uint(0)>>1)-gain {
			return Outcome{}, fmt.Errorf("伙伴成长数据已超限")
		}
		st.Pet.XP += gain
		bucket.XP += gain
		bucket.Kinds[kind]++
		st.Revision++
		out.XPGained = gain
		out.LevelChanged = previous != LevelOf(st.Pet.XP)
	}
	if err := s.write(st); err != nil {
		return Outcome{}, err
	}
	return out, nil
}
