package achievements

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	social "opscopilot/pkg/companionsocial"
)

func enabledStore(t *testing.T) *Store {
	t.Helper()
	s := NewDormant(filepath.Join(t.TempDir(), "achievements.json"))
	s.SetEnabled(true)
	return s
}

func TestRecoveryUsesOriginalDayAndUnreadableSourceKeepsIntent(t *testing.T) {
	s := enabledStore(t)
	s.now = func() time.Time { return time.Date(2026, 1, 1, 23, 59, 0, 0, time.UTC) }
	id, err := s.Prepare(social.MedalScriptCreated, "script", true)
	if err != nil {
		t.Fatal(err)
	}
	s.now = func() time.Time { return time.Date(2026, 1, 2, 0, 1, 0, 0, time.UTC) }
	if err = s.Recover(func(Intent) (bool, error) { return false, fmt.Errorf("unreadable") }); err == nil {
		t.Fatal("unreadable source discarded")
	}
	st, _ := s.Read()
	if _, ok := st.Intents[id]; !ok {
		t.Fatal("intent lost")
	}
	if err = s.Recover(func(Intent) (bool, error) { return true, nil }); err != nil {
		t.Fatal(err)
	}
	st, _ = s.Read()
	if !st.Receipts[key(social.MedalActiveDay, "2026-01-01")].Active || st.Receipts[key(social.MedalActiveDay, "2026-01-02")].Active {
		t.Fatal(st.Receipts)
	}
}

func TestReconcileOwnObjectsAndPreserveUnreadableRecords(t *testing.T) {
	s := enabledStore(t)
	if err := s.TrackExisting(social.MedalScriptCreated, "imported", true, func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	if known, _ := s.Has(social.MedalScriptCreated, "imported"); known {
		t.Fatal("import attributed")
	}
	s.Record(social.MedalScriptCreated, "owned", true)
	if err := s.Reconcile(func(social.MedalKind, string) (bool, error) { return false, fmt.Errorf("corrupt") }); err == nil {
		t.Fatal("corruption suppressed")
	}
	status, _ := s.Status()
	if status.Summary.Progress[1].Count != 1 {
		t.Fatal(status)
	}
	if err := s.Reconcile(func(kind social.MedalKind, ref string) (bool, error) {
		if ref != "owned" {
			t.Fatal("legacy scanned")
		}
		return false, nil
	}); err != nil {
		t.Fatal(err)
	}
	status, _ = s.Status()
	if status.Summary.Progress[1].Count != 0 {
		t.Fatal(status)
	}
	st, _ := s.Read()
	if st.Pending[key(social.MedalScriptCreated, ObjectID("owned"))].Revision != 2 {
		t.Fatal(st.Pending)
	}
}

func TestDispatchDeduplicatesTargetsPartialSuccessAndFailedUse(t *testing.T) {
	s := enabledStore(t)
	calls := 0
	result, err := Dispatch(s, "cmd", []string{"a", "a", "b"}, func(id string) error {
		calls++
		if id == "b" {
			return fmt.Errorf("offline")
		}
		return nil
	})
	if err != nil || result.Sent != 1 || result.Failed != 1 || calls != 2 {
		t.Fatal(result, err, calls)
	}
	if _, err = Dispatch(s, "failed", []string{"b"}, func(string) error { return fmt.Errorf("failed") }); err == nil {
		t.Fatal("failed dispatch accepted")
	}
	if _, err = Dispatch(s, "cmd", []string{"a"}, func(string) error { return nil }); err != nil {
		t.Fatal(err)
	}
	status, _ := s.Status()
	if status.Summary.Progress[6].Count != 1 {
		t.Fatal(status)
	}
}
func TestDormantAndCorruptionPreserveFiles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "achievements.json")
	s := NewDormant(path)
	if err := s.Record(social.MedalScriptUsed, "id", true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("disabled store touched disk")
	}
	if _, err := s.Status(); err == nil {
		t.Fatal("disabled read allowed")
	}
	os.WriteFile(path, []byte("broken"), 0600)
	s.SetEnabled(true)
	if err := s.Record(social.MedalScriptUsed, "id", true); err == nil {
		t.Fatal("corrupt state replaced")
	}
	data, _ := os.ReadFile(path)
	if string(data) != "broken" {
		t.Fatal("corrupt state changed")
	}
}
func TestSharedWritersDeduplicateAndAckPreservesNewerEdit(t *testing.T) {
	s := enabledStore(t)
	other := NewDormant(s.path)
	other.SetEnabled(true)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			target := s
			if i%2 == 0 {
				target = other
			}
			if err := target.Record(social.MedalScriptCreated, "script", true); err != nil {
				t.Error(err)
			}
		}(i)
	}
	wg.Wait()
	status, _ := s.Status()
	if status.Summary.Progress[1].Count != 1 {
		t.Fatal(status)
	}
	binding := Binding{BaseURL: "http://internal", Member: social.Member{UserID: "alice", OrganizationID: "org"}}
	if err := s.Bind(binding, true); err != nil {
		t.Fatal(err)
	}
	st, _ := s.Read()
	old := st.Receipts[key(social.MedalScriptCreated, ObjectID("script"))]
	if err := other.Record(social.MedalScriptCreated, "script", false); err != nil {
		t.Fatal(err)
	}
	if err := s.Ack(binding, st.SourceID, []social.AchievementReceipt{old}); err != nil {
		t.Fatal(err)
	}
	st, _ = s.Read()
	if st.Pending[key(old.Kind, old.ObjectID)].Active {
		t.Fatal("late ack lost withdrawal")
	}
	if st.Pending[key(old.Kind, old.ObjectID)].Revision != 2 {
		t.Fatal(st.Pending)
	}
}
func TestJournalRecoveryAndFailedBusiness(t *testing.T) {
	s := enabledStore(t)
	business := filepath.Join(t.TempDir(), "saved")
	id, err := s.Prepare(social.MedalQuickCommandCreated, business, true)
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(business, []byte("saved"), 0600)
	fresh := NewDormant(s.path)
	fresh.SetEnabled(true)
	if err = fresh.Recover(func(i Intent) (bool, error) { _, err := os.Stat(i.Reference); return err == nil, nil }); err != nil {
		t.Fatal(err)
	}
	st, _ := fresh.Read()
	if _, ok := st.Intents[id]; ok {
		t.Fatal("repair not cleared")
	}
	status, _ := fresh.Status()
	if status.Summary.Progress[2].Count != 1 {
		t.Fatal(status)
	}
	if err = s.Do(social.MedalScriptCreated, "failed", true, func() error { return fmt.Errorf("business failed") }); err == nil {
		t.Fatal("failure swallowed")
	}
	status, _ = s.Status()
	if status.Summary.Progress[1].Count != 0 || status.Repairs != 0 {
		t.Fatal(status)
	}
}
func TestBindingRequiresClaimAndRejectsOtherAccount(t *testing.T) {
	s := enabledStore(t)
	s.Record(social.MedalKnowledgeRead, "doc", true)
	binding := Binding{BaseURL: "http://internal", Member: social.Member{UserID: "alice", OrganizationID: "org"}}
	if err := s.Bind(binding, false); err == nil {
		t.Fatal("implicit history claim")
	}
	if err := s.Bind(binding, true); err != nil {
		t.Fatal(err)
	}
	binding.Member.UserID = "bob"
	if err := s.Bind(binding, true); err == nil {
		t.Fatal("account switch overwrote data")
	}
	data, _ := os.ReadFile(s.path)
	var st State
	if json.Unmarshal(data, &st) != nil || st.Owner != "alice" {
		t.Fatal("original binding lost")
	}
}
