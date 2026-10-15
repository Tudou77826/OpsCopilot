// Package achievements persists local confirmations and a coalesced outbox.
// It accepts backend-confirmed facts, never UI-supplied totals or medal tiers.
package achievements

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"opscopilot/pkg/companion"
	social "opscopilot/pkg/companionsocial"
	"opscopilot/pkg/filetxn"
)

var ErrDisabled = fmt.Errorf("功能奖章尚未启用")

type Binding struct {
	BaseURL string        `json:"baseUrl"`
	Member  social.Member `json:"member"`
}
type Intent struct {
	ID        string           `json:"id"`
	Kind      social.MedalKind `json:"kind"`
	Reference string           `json:"reference"`
	Active    bool             `json:"active"`
	Day       string           `json:"day"`
}
type State struct {
	SchemaVersion        int                                  `json:"schemaVersion"`
	SourceID             string                               `json:"sourceId"`
	Owner                social.UserID                        `json:"owner"`
	Binding              *Binding                             `json:"binding,omitempty"`
	Receipts             map[string]social.AchievementReceipt `json:"receipts"`
	Pending              map[string]social.AchievementReceipt `json:"pending"`
	Intents              map[string]Intent                    `json:"intents"`
	References           map[string]string                    `json:"references,omitempty"`
	ParticipationPending *bool                                `json:"participationPending,omitempty"`
	Profile              *social.ProfileUpdate                `json:"profile,omitempty"`
	ProfilePending       bool                                 `json:"profilePending,omitempty"`
	CompanionRevision    uint64                               `json:"companionRevision,omitempty"`
}
type Status struct {
	Summary              social.AchievementSummary `json:"summary"`
	Binding              *Binding                  `json:"binding,omitempty"`
	Pending              int                       `json:"pending"`
	Repairs              int                       `json:"repairs"`
	ParticipationPending *bool                     `json:"participationPending,omitempty"`
	Selected             []social.MedalKind        `json:"selected"`
	ProfilePending       bool                      `json:"profilePending"`
}
type Store struct {
	mu      sync.Mutex
	enabled atomic.Bool
	path    string
	now     func() time.Time
}

func NewDormant(path string) *Store      { return &Store{path: path, now: time.Now} }
func (s *Store) SetEnabled(enabled bool) { s.mu.Lock(); defer s.mu.Unlock(); s.enabled.Store(enabled) }
func (s *Store) Enabled() bool           { return s.enabled.Load() }
func ObjectID(reference string) string {
	sum := sha256.Sum256([]byte(reference))
	return hex.EncodeToString(sum[:])
}
func key(kind social.MedalKind, id string) string { return string(kind) + ":" + id }
func (s *Store) load() (*State, error) {
	data, err := filetxn.Read(s.path)
	if err != nil {
		return nil, err
	}
	st := &State{SchemaVersion: 1, SourceID: uuid.NewString(), Owner: social.UserID(uuid.NewString()), Receipts: map[string]social.AchievementReceipt{}, Pending: map[string]social.AchievementReceipt{}, Intents: map[string]Intent{}, References: map[string]string{}}
	if data == nil {
		return st, nil
	}
	if err = json.Unmarshal(data, st); err != nil {
		return nil, fmt.Errorf("读取奖章存档失败: %w", err)
	}
	if st.SchemaVersion != 1 || !social.ValidID(st.SourceID) || !social.ValidID(string(st.Owner)) || st.Receipts == nil || st.Pending == nil || st.Intents == nil {
		return nil, fmt.Errorf("奖章存档版本或结构无效，请保留原文件")
	}
	if st.Binding != nil && st.Binding.Member.UserID != st.Owner {
		return nil, fmt.Errorf("奖章账号绑定无效")
	}
	for k, r := range st.Receipts {
		if k != key(r.Kind, r.ObjectID) {
			return nil, fmt.Errorf("奖章对象键无效")
		}
	}
	if _, err = summary(st); err != nil {
		return nil, err
	}
	if st.References == nil {
		return nil, fmt.Errorf("本地成果索引无效")
	}
	if st.ProfilePending && st.Profile == nil {
		return nil, fmt.Errorf("展示同步队列无效")
	}
	if st.Profile != nil {
		sum, _ := summary(st)
		if _, err = sum.Showcase(st.Profile.Selected); err != nil {
			return nil, err
		}
		if st.Profile.SourceID != st.SourceID || st.Profile.Revision == 0 || (st.Profile.Companion != nil && !st.Profile.Companion.Valid()) {
			return nil, fmt.Errorf("展示存档无效")
		}
	}
	for k, r := range st.Pending {
		if saved, ok := st.Receipts[k]; !ok || saved != r {
			return nil, fmt.Errorf("奖章同步队列损坏")
		}
	}
	for k, i := range st.Intents {
		if k != i.ID || !social.ValidID(i.ID) || !i.Kind.Valid() || i.Reference == "" || len(i.Reference) > 1024 {
			return nil, fmt.Errorf("奖章补偿记录无效")
		}
		if _, err := time.Parse("2006-01-02", i.Day); err != nil {
			return nil, fmt.Errorf("奖章补偿日期无效")
		}
	}
	for k, ref := range st.References {
		r, ok := st.Receipts[k]
		if !ok || !r.Kind.Creation() || ref == "" || len(ref) > 1024 || ObjectID(ref) != r.ObjectID {
			return nil, fmt.Errorf("本地成果索引无效")
		}
	}
	return st, nil
}
func summary(st *State) (social.AchievementSummary, error) {
	rows := make([]social.AchievementReceipt, 0, len(st.Receipts))
	for _, r := range st.Receipts {
		rows = append(rows, r)
	}
	return social.ProjectAchievements(st.Owner, rows)
}
func (s *Store) mutate(fn func(*State) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.Enabled() {
		return ErrDisabled
	}
	release, err := filetxn.Lock(s.path)
	if err != nil {
		return err
	}
	defer release()
	st, err := s.load()
	if err != nil {
		return err
	}
	if err = fn(st); err != nil {
		return err
	}
	data, err := json.Marshal(st)
	if err != nil {
		return err
	}
	current, err := filetxn.Read(s.path)
	if err != nil {
		return err
	}
	if bytes.Equal(current, data) {
		return nil
	}
	return filetxn.Write(s.path, data)
}
func (s *Store) Read() (*State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.Enabled() {
		return nil, ErrDisabled
	}
	return s.load()
}
func (s *Store) Status() (Status, error) {
	st, err := s.Read()
	if err != nil {
		return Status{}, err
	}
	sum, err := summary(st)
	selected := []social.MedalKind{}
	if st.Profile != nil {
		selected = st.Profile.Selected
	}
	return Status{Summary: sum, Binding: st.Binding, Pending: len(st.Pending), Repairs: len(st.Intents), ParticipationPending: st.ParticipationPending, Selected: selected, ProfilePending: st.ProfilePending}, err
}
func apply(st *State, kind social.MedalKind, reference string, active bool) error {
	if !active && !kind.Creation() {
		return fmt.Errorf("使用记录不能撤回")
	}
	id := ObjectID(reference)
	if kind == social.MedalActiveDay {
		id = reference
	}
	probe := social.AchievementReceipt{Kind: kind, ObjectID: id, Contributor: st.Owner, Revision: 1, Active: active}
	if _, err := social.ProjectAchievements(st.Owner, []social.AchievementReceipt{probe}); err != nil {
		return err
	}
	k := key(kind, id)
	if kind.Creation() {
		st.References[k] = reference
	}
	old, exists := st.Receipts[k]
	if exists && old.Active == active {
		return nil
	}
	if exists {
		if old.Revision == ^uint64(0) {
			return fmt.Errorf("奖章版本已超限")
		}
		probe.Revision = old.Revision + 1
	}
	st.Receipts[k] = probe
	st.Pending[k] = probe
	return nil
}
func (s *Store) Has(kind social.MedalKind, reference string) (bool, error) {
	if !s.Enabled() {
		return false, nil
	}
	st, err := s.Read()
	if err != nil {
		return false, err
	}
	_, exists := st.Receipts[key(kind, ObjectID(reference))]
	return exists, nil
}
func (s *Store) Record(kind social.MedalKind, reference string, active bool) error {
	if !s.Enabled() {
		return nil
	}
	return s.mutate(func(st *State) error {
		if reference == "" || len(reference) > 1024 {
			return fmt.Errorf("奖章对象无效")
		}
		if err := apply(st, kind, reference, active); err != nil {
			return err
		}
		if active && kind != social.MedalActiveDay {
			return apply(st, social.MedalActiveDay, s.now().UTC().Format("2006-01-02"), true)
		}
		return nil
	})
}

// Prepare is durable before business persistence. Resolve commits the confirmed
// result and outbox together; crashed operations stay available for recovery.
func (s *Store) Prepare(kind social.MedalKind, reference string, active bool) (string, error) {
	if !s.Enabled() {
		return "", nil
	}
	id := uuid.NewString()
	err := s.mutate(func(st *State) error {
		if (!kind.Creation() && kind != social.MedalCaseCompleted) || (!active && !kind.Creation()) || reference == "" || len(reference) > 1024 {
			return fmt.Errorf("奖章意图无效")
		}
		for _, i := range st.Intents {
			if i.Kind == kind && i.Reference == reference {
				return fmt.Errorf("奖章操作仍待恢复")
			}
		}
		st.Intents[id] = Intent{ID: id, Kind: kind, Reference: reference, Active: active, Day: s.now().UTC().Format("2006-01-02")}
		return nil
	})
	return id, err
}
func (s *Store) Resolve(id string, committed bool) error {
	if id == "" {
		return nil
	}
	return s.mutate(func(st *State) error {
		i, ok := st.Intents[id]
		if !ok {
			return nil
		}
		if committed {
			if err := apply(st, i.Kind, i.Reference, i.Active); err != nil {
				return err
			}
			if i.Active {
				if err := apply(st, social.MedalActiveDay, i.Day, true); err != nil {
					return err
				}
			}
		}
		delete(st.Intents, id)
		return nil
	})
}
func (s *Store) Recover(check func(Intent) (bool, error)) error {
	if !s.Enabled() {
		return nil
	}
	return s.mutate(func(st *State) error {
		for id, i := range st.Intents {
			committed, err := check(i)
			if err != nil {
				return err
			}
			old, exists := st.Receipts[key(i.Kind, ObjectID(i.Reference))]
			if committed {
				if err = apply(st, i.Kind, i.Reference, i.Active); err != nil {
					return err
				}
				if i.Active && (!exists || !old.Active) {
					if err = apply(st, social.MedalActiveDay, i.Day, true); err != nil {
						return err
					}
				}
			}
			delete(st.Intents, id)
		}
		return nil
	})
}

// Do holds the shared ledger lock across journal -> business write -> confirm.
// Recovery therefore cannot inspect an operation still running in another
// cooperating host. Business locks must always be acquired inside this call.
func (s *Store) Do(kind social.MedalKind, reference string, active bool, writeBusiness func() error) error {
	if !s.Enabled() {
		return writeBusiness()
	}
	var businessErr error
	err := s.mutate(func(st *State) error {
		if (!kind.Creation() && kind != social.MedalCaseCompleted) || (!active && !kind.Creation()) || reference == "" || len(reference) > 1024 {
			return fmt.Errorf("奖章意图无效")
		}
		if old, ok := st.Receipts[key(kind, ObjectID(reference))]; ok && old.Active != active && old.Revision == ^uint64(0) {
			return fmt.Errorf("奖章版本已超限")
		}
		if len(st.Intents) > 0 {
			return fmt.Errorf("业务确认待恢复，请先恢复奖章记录")
		}
		id := uuid.NewString()
		st.Intents[id] = Intent{ID: id, Kind: kind, Reference: reference, Active: active, Day: s.now().UTC().Format("2006-01-02")}
		data, e := json.Marshal(st)
		if e != nil {
			return e
		}
		if e = filetxn.Write(s.path, data); e != nil {
			return e
		}
		businessErr = writeBusiness()
		if businessErr == nil {
			if e = apply(st, kind, reference, active); e != nil {
				return e
			}
			if active {
				if e = apply(st, social.MedalActiveDay, st.Intents[id].Day, true); e != nil {
					return e
				}
			}
		}
		delete(st.Intents, id)
		return nil
	})
	if err != nil {
		return fmt.Errorf("业务奖章确认未完成，待恢复: %w", err)
	}
	return businessErr
}
func (s *Store) Bind(binding Binding, claimLocal bool) error {
	return s.mutate(func(st *State) error {
		if !social.ValidID(string(binding.Member.UserID)) || !social.ValidID(binding.Member.OrganizationID) {
			return fmt.Errorf("成员身份无效")
		}
		if st.Binding != nil && (st.Binding.BaseURL != binding.BaseURL || st.Binding.Member.UserID != binding.Member.UserID) {
			return fmt.Errorf("该存档已绑定其他账号，请使用独立数据目录")
		}
		if st.Binding == nil && len(st.Receipts) > 0 && !claimLocal {
			return fmt.Errorf("本地已有使用记录，需要明确确认归属后绑定")
		}
		if len(st.Intents) > 0 {
			return fmt.Errorf("请先恢复待确认业务结果")
		}
		first := st.Binding == nil
		if st.ParticipationPending != nil {
			binding.Member.Participating = *st.ParticipationPending
		}
		st.Owner = binding.Member.UserID
		st.Binding = &binding
		if !first {
			return nil
		}
		for k, r := range st.Receipts {
			r.Contributor = st.Owner
			st.Receipts[k] = r
			st.Pending[k] = r
		}
		return nil
	})
}

// Ack clears only exactly acknowledged versions, preserving edits made while
// the HTTP request was in flight. Delayed acknowledgements cannot lose work.
func (s *Store) Ack(binding Binding, sourceID string, rows []social.AchievementReceipt) error {
	return s.mutate(func(st *State) error {
		if st.Binding == nil || st.Binding.BaseURL != binding.BaseURL || st.Owner != binding.Member.UserID || st.SourceID != sourceID {
			return fmt.Errorf("同步账号已变化")
		}
		for _, r := range rows {
			k := key(r.Kind, r.ObjectID)
			if pending, ok := st.Pending[k]; ok && pending == r {
				delete(st.Pending, k)
			}
		}
		return nil
	})
}
func (s *Store) SetParticipation(enabled bool) error {
	return s.mutate(func(st *State) error {
		if st.Binding == nil {
			return fmt.Errorf("请先连接成员账号")
		}
		st.Binding.Member.Participating = enabled
		st.ParticipationPending = &enabled
		return nil
	})
}

func (s *Store) AckParticipation(enabled bool) error {
	return s.mutate(func(st *State) error {
		if st.ParticipationPending != nil && *st.ParticipationPending == enabled {
			st.ParticipationPending = nil
		}
		return nil
	})
}

// Reconcile only objects authored while collection was enabled. Local private
// references never leave the machine. An unreadable source aborts the entire
// repair; it must not be interpreted as a deletion.
func (s *Store) Reconcile(check func(social.MedalKind, string) (bool, error)) error {
	if !s.Enabled() {
		return nil
	}
	return s.mutate(func(st *State) error {
		for k, ref := range st.References {
			r := st.Receipts[k]
			active, err := check(r.Kind, ref)
			if err != nil {
				return err
			}
			// Restoring a deleted owned object is allowed, but a maintenance scan
			// itself never creates a day of product use.
			if err = apply(st, r.Kind, ref, active); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Store) QueueProfile(update social.ProfileUpdate) error {
	return s.mutate(func(st *State) error {
		return queueProfile(st, update)
	})
}

// QueueCompanion preserves the user's medal selection and only queues a changed
// public card. No account connection or participation is implied by adoption.
func (s *Store) QueueCompanion(card *companion.Card, revision uint64) error {
	return s.mutate(func(st *State) error {
		if revision < st.CompanionRevision {
			return nil
		}
		if revision == st.CompanionRevision {
			// Upgrade an old stage-only projection once, without changing the pet's XP/revision.
			if card == nil || st.Profile == nil || st.Profile.Companion == nil {
				return nil
			}
			previous := st.Profile.Companion
			if previous.Level != 0 || previous.Stage == "" || card.Stage != "" || !card.Valid() || previous.SpeciesID != card.SpeciesID || previous.Name != card.Name || previous.PaletteID != card.PaletteID {
				return nil
			}
		}
		st.CompanionRevision = revision
		update := social.ProfileUpdate{Selected: []social.MedalKind{}, Companion: card}
		if st.Profile != nil {
			previous := st.Profile.Companion
			if (previous == nil && card == nil) || (previous != nil && card != nil && *previous == *card) {
				return nil
			}
			update.Selected = append([]social.MedalKind{}, st.Profile.Selected...)
		} else if card == nil {
			return nil
		}
		return queueProfile(st, update)
	})
}

func (s *Store) QueueSelection(selected []social.MedalKind) error {
	return s.mutate(func(st *State) error {
		update := social.ProfileUpdate{Selected: append([]social.MedalKind{}, selected...)}
		if st.Profile != nil {
			update.Companion = st.Profile.Companion
		}
		return queueProfile(st, update)
	})
}

func queueProfile(st *State, update social.ProfileUpdate) error {
	sum, err := summary(st)
	if err != nil {
		return err
	}
	if _, err = sum.Showcase(update.Selected); err != nil {
		return err
	}
	if update.Companion != nil && !update.Companion.Valid() {
		return fmt.Errorf("伙伴外观无效")
	}
	update.SourceID = st.SourceID
	update.Revision = 1
	if st.Profile != nil {
		if st.Profile.Revision == ^uint64(0) {
			return fmt.Errorf("展示版本已超限")
		}
		update.Revision = st.Profile.Revision + 1
	}
	st.Profile = &update
	st.ProfilePending = true
	return nil
}
func (s *Store) AckProfile(update social.ProfileUpdate) error {
	return s.mutate(func(st *State) error {
		if st.Profile != nil && st.Profile.Revision == update.Revision && st.Profile.SourceID == update.SourceID {
			st.ProfilePending = false
		}
		return nil
	})
}
