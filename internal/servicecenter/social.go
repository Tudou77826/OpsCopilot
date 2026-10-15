package servicecenter

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
	bolt "go.etcd.io/bbolt"
	social "opscopilot/pkg/companionsocial"
)

var errSocialAuth = errors.New("成员凭据无效或已停用")
var errSocialScope = errors.New("该资料不可查看")
var errSocialConflict = errors.New("数据版本冲突，请重新同步")
var errSnapshotExpired = errors.New("排名快照已失效，请重新获取")

type socialAccount struct {
	social.Member
	Enabled    bool                 `json:"enabled"`
	CreatedAt  time.Time            `json:"createdAt"`
	Selected   []social.MedalKind   `json:"selected"`
	Appearance social.ProfileUpdate `json:"appearance"`
}
type socialReceipt struct {
	social.AchievementReceipt
	SourceID string `json:"sourceId"`
}

func (s *Server) initSocial() error {
	db, err := bolt.Open(filepath.Join(s.cfg.DataDir, "social.db"), 0600, &bolt.Options{Timeout: time.Second})
	if err != nil {
		return err
	}
	err = db.Update(func(tx *bolt.Tx) error {
		for _, name := range []string{"meta", "members", "tokens", "receipts", "totals"} {
			if _, err := tx.CreateBucketIfNotExists([]byte(name)); err != nil {
				return err
			}
		}
		meta := tx.Bucket([]byte("meta"))
		version := meta.Get([]byte("schema"))
		if version != nil && !bytes.Equal(version, []byte("1")) {
			return fmt.Errorf("社交数据库版本不兼容")
		}
		return meta.Put([]byte("schema"), []byte("1"))
	})
	if err != nil {
		db.Close()
		return err
	}
	s.socialDB = db
	s.socialSnapshots = map[string]social.Snapshot{}
	return nil
}
func socialPut(b *bolt.Bucket, key string, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return b.Put([]byte(key), data)
}
func socialGet(b *bolt.Bucket, key string, v any) error {
	data := b.Get([]byte(key))
	if data == nil {
		return errSocialScope
	}
	return json.Unmarshal(data, v)
}
func socialMember(tx *bolt.Tx, credential string) (socialAccount, error) {
	var account socialAccount
	id := tx.Bucket([]byte("tokens")).Get([]byte(credential))
	if id == nil {
		return account, errSocialAuth
	}
	if err := socialGet(tx.Bucket([]byte("members")), string(id), &account); err != nil {
		return account, err
	}
	if !account.Enabled {
		return account, errSocialAuth
	}
	return account, nil
}
func socialCredential(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") {
		return ""
	}
	token := strings.TrimPrefix(auth, "Bearer ")
	if len(token) < 32 || len(token) > 256 {
		return ""
	}
	return hash(token)
}
func socialDecode(w http.ResponseWriter, r *http.Request, out any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 128<<10)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(out); err != nil {
		return fmt.Errorf("请求内容无效")
	}
	if err := dec.Decode(new(any)); err != io.EOF {
		return fmt.Errorf("请求内容无效")
	}
	return nil
}
func socialError(w http.ResponseWriter, err error) {
	code := 400
	switch {
	case errors.Is(err, errSocialAuth):
		code = 401
	case errors.Is(err, errSocialScope):
		code = 404
	case errors.Is(err, errSocialConflict), errors.Is(err, errSnapshotExpired):
		code = 409
	}
	respond(w, code, map[string]string{"error": err.Error()})
}
func socialEpoch(tx *bolt.Tx, org string) uint64 {
	v := tx.Bucket([]byte("meta")).Get([]byte("visibility:" + org))
	if len(v) == 8 {
		return binary.BigEndian.Uint64(v)
	}
	return 0
}
func bumpSocialEpoch(tx *bolt.Tx, org string) error {
	var value [8]byte
	binary.BigEndian.PutUint64(value[:], socialEpoch(tx, org)+1)
	return tx.Bucket([]byte("meta")).Put([]byte("visibility:"+org), value[:])
}
func socialSummary(tx *bolt.Tx, user social.UserID) (social.AchievementSummary, error) {
	var result social.AchievementSummary
	data := tx.Bucket([]byte("totals")).Get([]byte(user))
	if data != nil {
		err := json.Unmarshal(data, &result)
		if err != nil {
			return result, err
		}
		if result.RuleVersion == social.AchievementRuleVersion {
			return result, nil
		}
	}
	rows := []social.AchievementReceipt{}
	bucket := tx.Bucket([]byte("receipts")).Bucket([]byte(user))
	if bucket != nil {
		if err := bucket.ForEach(func(k, v []byte) error {
			var r socialReceipt
			if err := json.Unmarshal(v, &r); err != nil {
				return err
			}
			rows = append(rows, r.AchievementReceipt)
			return nil
		}); err != nil {
			return result, err
		}
	}
	return social.ProjectAchievements(user, rows)
}
func publicSocialProfile(tx *bolt.Tx, account socialAccount) (social.PublicProfile, error) {
	sum, err := socialSummary(tx, account.UserID)
	if err != nil {
		return social.PublicProfile{}, err
	}
	medals, err := sum.Showcase(account.Selected)
	return social.PublicProfile{Profile: social.Profile{UserID: account.UserID, DisplayName: account.DisplayName, Companion: account.Appearance.Companion, Medals: medals}, Achievements: sum}, err
}

func (s *Server) socialRoutes() {
	s.mux.HandleFunc("POST /api/admin/social/members", s.admin(s.createSocialMember))
	s.mux.HandleFunc("DELETE /api/admin/social/members/{id}", s.admin(s.disableSocialMember))
	s.mux.HandleFunc("GET /api/v1/social/me", s.socialMe)
	s.mux.HandleFunc("GET /api/v1/social/catalog", func(w http.ResponseWriter, r *http.Request) {
		err := s.socialDB.View(func(tx *bolt.Tx) error { _, err := socialMember(tx, socialCredential(r)); return err })
		if err != nil {
			socialError(w, err)
			return
		}
		respond(w, 200, social.MedalCatalog())
	})
	s.mux.HandleFunc("PUT /api/v1/social/participation", s.socialParticipation)
	s.mux.HandleFunc("PUT /api/v1/social/profile", s.socialProfileUpdate)
	s.mux.HandleFunc("POST /api/v1/social/receipts", s.socialIngest)
	s.mux.HandleFunc("GET /api/v1/social/profiles/{id}", s.socialProfile)
	s.mux.HandleFunc("GET /api/v1/social/rankings", s.socialRankings)
}
func (s *Server) createSocialMember(w http.ResponseWriter, r *http.Request) {
	var input struct {
		OrganizationID string `json:"organizationId"`
		DisplayName    string `json:"displayName"`
	}
	if err := socialDecode(w, r, &input); err != nil {
		socialError(w, err)
		return
	}
	if !social.ValidID(input.OrganizationID) || strings.TrimSpace(input.DisplayName) != input.DisplayName || utf8.RuneCountInString(input.DisplayName) < 1 || utf8.RuneCountInString(input.DisplayName) > 32 {
		socialError(w, fmt.Errorf("组织或显示名无效"))
		return
	}
	for _, ch := range input.DisplayName {
		if unicode.IsControl(ch) {
			socialError(w, fmt.Errorf("显示名无效"))
			return
		}
	}
	var tokenBytes [32]byte
	if _, err := rand.Read(tokenBytes[:]); err != nil {
		respond(w, 500, map[string]string{"error": "创建凭据失败"})
		return
	}
	token := base64.RawURLEncoding.EncodeToString(tokenBytes[:])
	account := socialAccount{Member: social.Member{UserID: social.UserID(uuid.NewString()), OrganizationID: input.OrganizationID, DisplayName: input.DisplayName}, Enabled: true, CreatedAt: time.Now().UTC(), Selected: []social.MedalKind{}}
	err := s.socialDB.Update(func(tx *bolt.Tx) error {
		if err := socialPut(tx.Bucket([]byte("members")), string(account.UserID), account); err != nil {
			return err
		}
		return tx.Bucket([]byte("tokens")).Put([]byte(hash(token)), []byte(account.UserID))
	})
	if err != nil {
		respond(w, 500, map[string]string{"error": "保存成员失败"})
		return
	}
	respond(w, 201, map[string]any{"member": account.Member, "token": token})
}
func (s *Server) disableSocialMember(w http.ResponseWriter, r *http.Request) {
	err := s.socialDB.Update(func(tx *bolt.Tx) error {
		var a socialAccount
		if err := socialGet(tx.Bucket([]byte("members")), r.PathValue("id"), &a); err != nil {
			return err
		}
		a.Enabled = false
		a.Participating = false
		if err := socialPut(tx.Bucket([]byte("members")), string(a.UserID), a); err != nil {
			return err
		}
		return bumpSocialEpoch(tx, a.OrganizationID)
	})
	if err != nil {
		socialError(w, err)
		return
	}
	respond(w, 200, map[string]bool{"disabled": true})
}
func (s *Server) socialMe(w http.ResponseWriter, r *http.Request) {
	var member social.Member
	var profile social.PublicProfile
	err := s.socialDB.View(func(tx *bolt.Tx) error {
		a, err := socialMember(tx, socialCredential(r))
		if err != nil {
			return err
		}
		member = a.Member
		profile, err = publicSocialProfile(tx, a)
		return err
	})
	if err != nil {
		socialError(w, err)
		return
	}
	respond(w, 200, map[string]any{"member": member, "profile": profile})
}
func (s *Server) socialParticipation(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Enabled *bool `json:"enabled"`
	}
	if err := socialDecode(w, r, &input); err != nil || input.Enabled == nil {
		socialError(w, fmt.Errorf("参与选择无效"))
		return
	}
	var member social.Member
	err := s.socialDB.Update(func(tx *bolt.Tx) error {
		a, err := socialMember(tx, socialCredential(r))
		if err != nil {
			return err
		}
		if a.Participating != *input.Enabled {
			a.Participating = *input.Enabled
			if err = socialPut(tx.Bucket([]byte("members")), string(a.UserID), a); err != nil {
				return err
			}
			if err = bumpSocialEpoch(tx, a.OrganizationID); err != nil {
				return err
			}
		}
		member = a.Member
		return nil
	})
	if err != nil {
		socialError(w, err)
		return
	}
	respond(w, 200, member)
}
func (s *Server) socialProfileUpdate(w http.ResponseWriter, r *http.Request) {
	var update social.ProfileUpdate
	if err := socialDecode(w, r, &update); err != nil {
		socialError(w, err)
		return
	}
	if !social.ValidID(update.SourceID) || update.Revision == 0 || len(update.Selected) > 3 || (update.Companion != nil && !update.Companion.Valid()) {
		socialError(w, fmt.Errorf("资料内容无效"))
		return
	}
	empty, _ := social.ProjectAchievements("validation", nil)
	if _, err := empty.Showcase(update.Selected); err != nil {
		socialError(w, err)
		return
	}
	var profile social.PublicProfile
	err := s.socialDB.Update(func(tx *bolt.Tx) error {
		a, err := socialMember(tx, socialCredential(r))
		if err != nil {
			return err
		}
		old := a.Appearance
		if old.SourceID != "" && old.SourceID != update.SourceID {
			return fmt.Errorf("伙伴资料已绑定另一份存档")
		}
		if old.Revision > update.Revision {
			return errSocialConflict
		}
		if old.Revision == update.Revision {
			previous, _ := json.Marshal(old)
			next, _ := json.Marshal(update)
			if !bytes.Equal(previous, next) {
				return errSocialConflict
			}
		}
		a.Appearance = update
		a.Selected = update.Selected
		if err = socialPut(tx.Bucket([]byte("members")), string(a.UserID), a); err != nil {
			return err
		}
		if a.Participating && old.Revision != update.Revision {
			if err = bumpSocialEpoch(tx, a.OrganizationID); err != nil {
				return err
			}
		}
		profile, err = publicSocialProfile(tx, a)
		return err
	})
	if err != nil {
		socialError(w, err)
		return
	}
	respond(w, 200, profile)
}
func (s *Server) socialIngest(w http.ResponseWriter, r *http.Request) {
	var batch social.SyncBatch
	if err := socialDecode(w, r, &batch); err != nil {
		socialError(w, err)
		return
	}
	if !social.ValidID(batch.SourceID) || len(batch.Receipts) < 1 || len(batch.Receipts) > 100 {
		socialError(w, fmt.Errorf("确认批次无效"))
		return
	}
	var ack social.SyncAck
	err := s.socialDB.Update(func(tx *bolt.Tx) error {
		a, err := socialMember(tx, socialCredential(r))
		if err != nil {
			return err
		}
		if !a.Participating {
			return fmt.Errorf("请先明确选择参与排名")
		}
		if _, err = social.ProjectAchievements(a.UserID, batch.Receipts); err != nil {
			return err
		}
		bucket, err := tx.Bucket([]byte("receipts")).CreateBucketIfNotExists([]byte(a.UserID))
		if err != nil {
			return err
		}
		changed := false
		for _, incoming := range batch.Receipts {
			if incoming.Kind == social.MedalActiveDay {
				day, _ := time.Parse("2006-01-02", incoming.ObjectID)
				if day.After(time.Now().UTC().Truncate(24 * time.Hour)) {
					return fmt.Errorf("活跃日期超出范围")
				}
			}
			if !incoming.Kind.Creation() && !incoming.Active {
				return fmt.Errorf("使用记录只能确认有效结果")
			}
			k := string(incoming.Kind) + ":" + incoming.ObjectID
			oldData := bucket.Get([]byte(k))
			if oldData != nil {
				var old socialReceipt
				if err = json.Unmarshal(oldData, &old); err != nil {
					return err
				}
				if incoming.Kind.Creation() && old.SourceID != batch.SourceID {
					return fmt.Errorf("制作成果归属另一份存档")
				}
				if old.Revision > incoming.Revision {
					continue
				}
				if old.Revision == incoming.Revision {
					if old.Active != incoming.Active {
						return errSocialConflict
					}
					continue
				}
			}
			if err = socialPut(bucket, k, socialReceipt{AchievementReceipt: incoming, SourceID: batch.SourceID}); err != nil {
				return err
			}
			changed = true
		}
		if !changed {
			sum, err := socialSummary(tx, a.UserID)
			if err != nil {
				return err
			}
			ack = social.SyncAck{Receipts: batch.Receipts, Summary: sum}
			return nil
		}
		// Rebuild once per batch, not once per object. Totals are a recoverable
		// projection stored in the same transaction as accepted confirmations.
		rows := []social.AchievementReceipt{}
		if err = bucket.ForEach(func(k, v []byte) error {
			var saved socialReceipt
			if err := json.Unmarshal(v, &saved); err != nil {
				return err
			}
			rows = append(rows, saved.AchievementReceipt)
			return nil
		}); err != nil {
			return err
		}
		sum, err := social.ProjectAchievements(a.UserID, rows)
		if err != nil {
			return err
		}
		if err = socialPut(tx.Bucket([]byte("totals")), string(a.UserID), sum); err != nil {
			return err
		}
		ack = social.SyncAck{Receipts: batch.Receipts, Summary: sum}
		return nil
	})
	if err != nil {
		socialError(w, err)
		return
	}
	respond(w, 200, ack)
}
func (s *Server) socialProfile(w http.ResponseWriter, r *http.Request) {
	var profile social.PublicProfile
	err := s.socialDB.View(func(tx *bolt.Tx) error {
		viewer, err := socialMember(tx, socialCredential(r))
		if err != nil {
			return err
		}
		var target socialAccount
		if err = socialGet(tx.Bucket([]byte("members")), r.PathValue("id"), &target); err != nil {
			return err
		}
		if !target.Enabled || target.OrganizationID != viewer.OrganizationID || (!target.Participating && target.UserID != viewer.UserID) {
			return errSocialScope
		}
		profile, err = publicSocialProfile(tx, target)
		return err
	})
	if err != nil {
		socialError(w, err)
		return
	}
	respond(w, 200, profile)
}
func (s *Server) socialRankings(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	kind := social.MedalKind(q.Get("kind"))
	offset, err := strconv.Atoi(q.Get("offset"))
	if err != nil {
		socialError(w, fmt.Errorf("分页参数无效"))
		return
	}
	limit, err := strconv.Atoi(q.Get("limit"))
	if err != nil || !kind.Valid() || offset < 0 || limit < 1 || limit > 100 {
		socialError(w, fmt.Errorf("排名查询无效"))
		return
	}
	for key := range q {
		if key != "kind" && key != "offset" && key != "limit" && key != "snapshotId" {
			socialError(w, fmt.Errorf("排名参数无效"))
			return
		}
	}
	s.socialMu.Lock()
	defer s.socialMu.Unlock()
	now := time.Now()
	for id, snapshot := range s.socialSnapshots {
		if !now.Before(snapshot.ExpiresAt) {
			delete(s.socialSnapshots, id)
		}
	}
	id := q.Get("snapshotId")
	var page social.RankingPage
	err = s.socialDB.View(func(tx *bolt.Tx) error {
		viewer, err := socialMember(tx, socialCredential(r))
		if err != nil {
			return err
		}
		var snapshot social.Snapshot
		if id != "" {
			var ok bool
			snapshot, ok = s.socialSnapshots[id]
			if !ok || snapshot.OrganizationID != viewer.OrganizationID || snapshot.Epoch != socialEpoch(tx, viewer.OrganizationID) || snapshot.Kind != kind {
				return errSnapshotExpired
			}
		} else {
			if offset != 0 {
				return errSnapshotExpired
			}
			id = uuid.NewString()
			snapshot = social.Snapshot{OrganizationID: viewer.OrganizationID, Epoch: socialEpoch(tx, viewer.OrganizationID), Kind: kind, ExpiresAt: now.Add(2 * time.Minute), Candidates: []social.RankingCandidate{}}
			err = tx.Bucket([]byte("members")).ForEach(func(k, v []byte) error {
				var a socialAccount
				if err := json.Unmarshal(v, &a); err != nil {
					return err
				}
				if !a.Enabled || !a.Participating || a.OrganizationID != viewer.OrganizationID {
					return nil
				}
				p, err := publicSocialProfile(tx, a)
				if err != nil {
					return err
				}
				snapshot.Candidates = append(snapshot.Candidates, social.RankingCandidate{Profile: p.Profile, Achievements: p.Achievements, Participating: true})
				return nil
			})
			if err != nil {
				return err
			}
			if len(s.socialSnapshots) >= 64 {
				var oldestID string
				var oldest time.Time
				for key, cached := range s.socialSnapshots {
					if oldestID == "" || cached.ExpiresAt.Before(oldest) {
						oldestID = key
						oldest = cached.ExpiresAt
					}
				}
				delete(s.socialSnapshots, oldestID)
			}
			s.socialSnapshots[id] = snapshot
		}
		page, err = social.RankAchievements(kind, id, snapshot.Candidates, offset, limit)
		return err
	})
	if err != nil {
		socialError(w, err)
		return
	}
	respond(w, 200, page)
}
