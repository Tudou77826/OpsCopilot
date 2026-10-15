package servicecenter

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"opscopilot/pkg/achievements"
	social "opscopilot/pkg/companionsocial"
	"opscopilot/pkg/secretstore"
)

func TestSocialLostAckAndOfflineExitRemainRetryable(t *testing.T) {
	s := testServer(t)
	member, token := provisionSocial(t, s, "org", "Member")
	var dropAck, offline atomic.Bool
	var uploads atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if offline.Load() && r.URL.Path == "/api/v1/social/participation" {
			w.WriteHeader(503)
			return
		}
		if r.URL.Path == "/api/v1/social/receipts" {
			uploads.Add(1)
			if dropAck.Load() {
				accepted := httptest.NewRecorder()
				s.Handler().ServeHTTP(accepted, r)
				if accepted.Code != 200 {
					t.Error(accepted.Body.String())
				}
				w.WriteHeader(503)
				return
			}
		}
		s.Handler().ServeHTTP(w, r)
	}))
	defer server.Close()
	local := achievements.NewDormant(filepath.Join(t.TempDir(), "achievements.json"))
	local.SetEnabled(true)
	client := achievements.NewClient(local, socialTestSecrets{})
	ctx := context.Background()
	if _, err := client.Connect(ctx, server.URL, token, false); err != nil {
		t.Fatal(err)
	}
	if err := client.Participation(ctx, true); err != nil {
		t.Fatal(err)
	}
	local.Record(social.MedalQuickCommandUsed, "cmd", true)
	dropAck.Store(true)
	if _, err := client.Sync(ctx); err == nil {
		t.Fatal("lost ack reported success")
	}
	status, _ := local.Status()
	if status.Pending == 0 {
		t.Fatal("lost ack removed outbox")
	}
	dropAck.Store(false)
	if _, err := client.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	profile, err := client.Profile(ctx, string(member.UserID))
	if err != nil || profile.Achievements.Progress[6].Count != 1 {
		t.Fatal(profile, err)
	}
	local.Record(social.MedalQuickCommandUsed, "another", true)
	offline.Store(true)
	if err = client.Participation(ctx, false); err == nil {
		t.Fatal("offline exit falsely confirmed")
	}
	if _, err = client.Connect(ctx, server.URL, token, true); err != nil {
		t.Fatal(err)
	}
	status, _ = local.Status()
	if status.Binding.Member.Participating || status.ParticipationPending == nil || *status.ParticipationPending {
		t.Fatal("reconnect lost queued exit", status)
	}
	before := uploads.Load()
	offline.Store(false)
	if _, err = client.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	status, _ = local.Status()
	if uploads.Load() != before || status.Pending == 0 || status.ParticipationPending != nil {
		t.Fatal("exit uploaded pending work", status)
	}
	var me struct {
		Member social.Member `json:"member"`
	}
	json.Unmarshal(memberRequest(s, token, "GET", "/api/v1/social/me", nil).Body.Bytes(), &me)
	if me.Member.Participating {
		t.Fatal("exit not confirmed")
	}
}

func TestSocialPersistenceAndProfileRevisionSnapshotPrivacy(t *testing.T) {
	cfg := Config{DataDir: t.TempDir(), PublicURL: "https://ops.internal", AdminToken: strings.Repeat("admin", 8)}
	s, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	member, token := provisionSocial(t, s, "org", "Member")
	memberRequest(s, token, "PUT", "/api/v1/social/participation", map[string]bool{"enabled": true})
	receipt := social.AchievementReceipt{Kind: social.MedalKnowledgeArchive, ObjectID: "archive", Contributor: member.UserID, Revision: 1, Active: true}
	if w := memberRequest(s, token, "POST", "/api/v1/social/receipts", social.SyncBatch{SourceID: "source", Receipts: []social.AchievementReceipt{receipt}}); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	update := social.ProfileUpdate{SourceID: "source", Revision: 1, Selected: []social.MedalKind{social.MedalKnowledgeArchive}}
	for i := 0; i < 2; i++ {
		if w := memberRequest(s, token, "PUT", "/api/v1/social/profile", update); w.Code != 200 {
			t.Fatal(w.Body.String())
		}
	}
	var page social.RankingPage
	w := memberRequest(s, token, "GET", "/api/v1/social/rankings?kind=knowledge-archive&offset=0&limit=1", nil)
	json.Unmarshal(w.Body.Bytes(), &page)
	update.Revision = 2
	update.Selected = []social.MedalKind{}
	if w = memberRequest(s, token, "PUT", "/api/v1/social/profile", update); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	if w = memberRequest(s, token, "GET", "/api/v1/social/rankings?kind=knowledge-archive&offset=0&limit=1&snapshotId="+page.SnapshotID, nil); w.Code != 409 {
		t.Fatal("removed medal remained in snapshot")
	}
	stale := update
	stale.Revision = 1
	if w = memberRequest(s, token, "PUT", "/api/v1/social/profile", stale); w.Code != 409 {
		t.Fatal("stale profile overwritten")
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	bytes, err := os.ReadFile(filepath.Join(cfg.DataDir, "social.db"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(bytes), token) {
		t.Fatal("raw member token stored")
	}
	s, err = New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var me struct {
		Profile social.PublicProfile `json:"profile"`
	}
	w = memberRequest(s, token, "GET", "/api/v1/social/me", nil)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	json.Unmarshal(w.Body.Bytes(), &me)
	if me.Profile.Achievements.Progress[0].Count != 1 || len(me.Profile.Medals) != 0 {
		t.Fatal("restart lost latest projection", me)
	}
}

type socialTestSecrets map[string]string

func (s socialTestSecrets) Set(service, user, token string) error {
	s[service+user] = token
	return nil
}
func (s socialTestSecrets) Get(service, user string) (string, error) {
	token, ok := s[service+user]
	if !ok {
		return "", secretstore.ErrNotFound
	}
	return token, nil
}
func (s socialTestSecrets) Delete(service, user string) error { delete(s, service+user); return nil }
func provisionSocial(t *testing.T, s *Server, org, name string) (social.Member, string) {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"organizationId": org, "displayName": name})
	w := request(s, "POST", "/api/admin/social/members", body, true)
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	var result struct {
		Member social.Member `json:"member"`
		Token  string        `json:"token"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	return result.Member, result.Token
}
func memberRequest(s *Server, token, method, path string, input any) *httptest.ResponseRecorder {
	body, _ := json.Marshal(input)
	r := httptest.NewRequest(method, path, bytes.NewReader(body))
	r.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	return w
}
func TestSocialEndToEndBatchRetryWithdrawalAndIsolation(t *testing.T) {
	s := testServer(t)
	server := httptest.NewServer(s.Handler())
	defer server.Close()
	alice, token := provisionSocial(t, s, "team", "Alice")
	bob, bobToken := provisionSocial(t, s, "team", "Bob")
	_, outsiderToken := provisionSocial(t, s, "other", "Other")
	local := achievements.NewDormant(filepath.Join(t.TempDir(), "achievements.json"))
	local.SetEnabled(true)
	secrets := socialTestSecrets{}
	client := achievements.NewClient(local, secrets)
	ctx := context.Background()
	if _, err := client.Connect(ctx, server.URL, token, false); err != nil {
		t.Fatal(err)
	}
	if err := client.Participation(ctx, true); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 105; i++ {
		if err := local.Record(social.MedalScriptCreated, string(rune('a'+i)), true); err != nil {
			t.Fatal(err)
		}
	}
	if status, err := client.Sync(ctx); err != nil || status.Pending != 0 {
		t.Fatalf("batching: %+v / %v", status, err)
	}
	profile, err := client.Profile(ctx, string(alice.UserID))
	if err != nil || profile.Achievements.Progress[1].Count != 105 {
		t.Fatal(profile, err)
	}
	st, _ := local.Read()
	rows := []social.AchievementReceipt{}
	for _, r := range st.Receipts {
		rows = append(rows, r)
		if len(rows) == 100 {
			break
		}
	}
	batch := social.SyncBatch{SourceID: st.SourceID, Receipts: rows}
	for i := 0; i < 2; i++ {
		w := memberRequest(s, token, "POST", "/api/v1/social/receipts", batch)
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	local.Record(social.MedalScriptCreated, "a", false)
	if _, err = client.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	profile, err = client.Profile(ctx, string(alice.UserID))
	if err != nil || profile.Achievements.Progress[1].Count != 104 {
		t.Fatal(profile, err)
	}
	// A delayed publication must not resurrect the withdrawn object.
	old := social.AchievementReceipt{Kind: social.MedalScriptCreated, ObjectID: achievements.ObjectID("a"), Contributor: alice.UserID, Revision: 1, Active: true}
	w := memberRequest(s, token, "POST", "/api/v1/social/receipts", social.SyncBatch{SourceID: st.SourceID, Receipts: []social.AchievementReceipt{old}})
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	profile, _ = client.Profile(ctx, string(alice.UserID))
	if profile.Achievements.Progress[1].Count != 104 {
		t.Fatal("withdrawal resurrected")
	}
	if w = memberRequest(s, outsiderToken, "GET", "/api/v1/social/profiles/"+string(alice.UserID), nil); w.Code != 404 {
		t.Fatal("cross organization profile allowed", w.Code)
	}
	if w = memberRequest(s, bobToken, "GET", "/api/v1/social/profiles/"+string(bob.UserID), nil); w.Code != 200 {
		t.Fatal("own private profile blocked")
	}
	if w = memberRequest(s, token, "GET", "/api/v1/social/profiles/"+string(bob.UserID), nil); w.Code != 404 {
		t.Fatal("nonparticipant exposed")
	}
	page, err := client.Rankings(ctx, social.MedalScriptCreated, 0, 1, "")
	if err != nil || page.Total != 1 || page.Entries[0].Count != 104 {
		t.Fatal(page, err)
	}
	if err = client.Participation(ctx, false); err != nil {
		t.Fatal(err)
	}
	if _, err = client.Rankings(ctx, social.MedalScriptCreated, 1, 1, page.SnapshotID); err == nil {
		t.Fatal("withdrawal left snapshot accessible")
	}
	if w = memberRequest(s, bobToken, "GET", "/api/v1/social/profiles/"+string(alice.UserID), nil); w.Code != 404 {
		t.Fatal("withdrawal left profile accessible")
	}
	data := memberRequest(s, token, "GET", "/api/v1/social/me", nil).Body.String()
	if strings.Contains(data, "objectId") || strings.Contains(data, "sourceId") || strings.Contains(data, token) {
		t.Fatal("private state exposed", data)
	}
}
func TestSocialAtomicRejectionRevocationAndUnknownFields(t *testing.T) {
	s := testServer(t)
	member, token := provisionSocial(t, s, "team", "Alice")
	memberRequest(s, token, "PUT", "/api/v1/social/participation", map[string]bool{"enabled": true})
	good := social.AchievementReceipt{Kind: social.MedalScriptCreated, ObjectID: "script", Contributor: member.UserID, Revision: 1, Active: true}
	bad := good
	bad.Contributor = "forged"
	w := memberRequest(s, token, "POST", "/api/v1/social/receipts", social.SyncBatch{SourceID: "source", Receipts: []social.AchievementReceipt{good, bad}})
	if w.Code == 200 {
		t.Fatal("forged actor accepted")
	}
	var me struct {
		Profile social.PublicProfile `json:"profile"`
	}
	json.Unmarshal(memberRequest(s, token, "GET", "/api/v1/social/me", nil).Body.Bytes(), &me)
	if me.Profile.Achievements.Progress[1].Count != 0 {
		t.Fatal("batch partially committed")
	}
	w = memberRequest(s, token, "PUT", "/api/v1/social/profile", map[string]any{"revision": 1, "sourceId": "source", "selected": []string{}, "xp": 999})
	if w.Code == 200 {
		t.Fatal("unknown score field accepted")
	}
	w = request(s, "DELETE", "/api/admin/social/members/"+string(member.UserID), nil, true)
	if w.Code != 200 {
		t.Fatal(w.Code)
	}
	if w = memberRequest(s, token, "GET", "/api/v1/social/me", nil); w.Code != http.StatusUnauthorized {
		t.Fatal("revoked token accepted")
	}
}
