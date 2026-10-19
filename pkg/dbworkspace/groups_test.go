package dbworkspace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGroupsPersistAndPreserveConnections(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "database.json")
	secrets := &memorySecrets{values: map[string]string{}}
	s := newNativeTestService(path, secrets)
	if err := s.CreateGroup("  空分组  "); err != nil {
		t.Fatal(err)
	}
	password := "group-test-credential"
	p, err := s.Save(ProfileInput{Profile: Profile{Engine: "mysql", Name: "订单库", Host: "localhost", User: "ops", Group: "业务系统"}, Password: &password})
	if err != nil {
		t.Fatal(err)
	}
	q, err := s.Save(ProfileInput{Profile: Profile{Engine: "mysql", Name: "库存库", Host: "localhost", User: "ops", Group: "业务系统"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.RenameGroup("业务系统", "生产环境"); err != nil {
		t.Fatal(err)
	}
	// Reopen from disk rather than relying on an in-memory catalog.
	s = newNativeTestService(path, secrets)
	groups, err := s.Groups()
	if err != nil || !hasGroup(groups, "空分组") || !hasGroup(groups, "生产环境") || hasGroup(groups, "业务系统") {
		t.Fatalf("catalog did not persist: %v, %v", groups, err)
	}
	profiles, _ := s.List()
	for _, profile := range profiles {
		if profile.Group != "生产环境" || profile.Revision != 2 {
			t.Fatalf("rename did not update every profile: %+v", profile)
		}
	}
	if _, err := s.Save(ProfileInput{Profile: p}); err == nil {
		t.Fatal("stale edit overwrote the renamed group")
	}
	if err := s.DeleteGroup("生产环境"); err != nil {
		t.Fatal(err)
	}
	profiles, _ = s.List()
	if len(profiles) != 2 {
		t.Fatal("deleting group removed connection profiles")
	}
	for _, profile := range profiles {
		if profile.Group != "" || profile.Revision != 3 {
			t.Fatalf("connection was not moved to root: %+v", profile)
		}
	}
	if _, actual, err := s.profile(p.ID); err != nil || actual != password || len(secrets.values) != 1 {
		t.Fatal("group management changed credentials")
	}
	if err := s.Delete(q.ID, 3); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteGroup("空分组"); err != nil {
		t.Fatal(err)
	}
	groups, _ = s.Groups()
	if len(groups) != 0 {
		t.Fatalf("deleted groups returned: %v", groups)
	}
}

func TestGroupsValidationAndLegacyCatalog(t *testing.T) {
	path := filepath.Join(t.TempDir(), "database.json")
	if err := os.WriteFile(path, []byte(`{"version":1,"profiles":[{"id":"old","revision":1,"name":"legacy","group":"旧分组"}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	s := newNativeTestService(path, &memorySecrets{values: map[string]string{}})
	for _, name := range []string{"", "   ", "a\nb", "a\x00b", strings.Repeat("字", 101), "旧分组"} {
		if err := s.CreateGroup(name); err == nil {
			t.Fatalf("accepted invalid/duplicate name %q", name)
		}
	}
	if err := s.CreateGroup("保留"); err != nil {
		t.Fatal(err)
	}
	if err := s.RenameGroup("旧分组", "保留"); err == nil {
		t.Fatal("rename unexpectedly merged groups")
	}
	if err := s.DeleteGroup("不存在"); err == nil {
		t.Fatal("deleting missing group silently succeeded")
	}
	// Moving the last legacy connection must leave its old folder available.
	p := Profile{Engine: "mysql", ID: "old", Revision: 1, Host: "localhost", User: "ops", Group: "新分组"}
	if _, err := s.Save(ProfileInput{Profile: p}); err != nil {
		t.Fatal(err)
	}
	groups, err := newNativeTestService(path, s.secrets).Groups()
	if err != nil || !hasGroup(groups, "旧分组") || !hasGroup(groups, "新分组") || !hasGroup(groups, "保留") {
		t.Fatalf("empty legacy group disappeared: %v, %v", groups, err)
	}
}
