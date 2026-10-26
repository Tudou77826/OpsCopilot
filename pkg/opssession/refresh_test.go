package opssession

import (
	"opscopilot/pkg/dbworkspace"
	"opscopilot/pkg/remote"
	"testing"
)

func TestCredentialRefreshKeepsRouteWithoutCopyingAnotherIdentity(t *testing.T) {
	s, _ := newTestStore(t)
	mustInit(t, s, "refresh")
	n := sshNode("app", "192.0.2.2", 22, "old")
	n.SSH.Bastion = &remote.ConnectConfig{Host: "192.0.2.1", Port: 22, User: "jump", Password: "jump-secret"}
	if _, e := s.Put("refresh", n); e != nil {
		t.Fatal(e)
	}
	n = sshNode("app", "192.0.2.2", 22, "new")
	got, e := s.Put("refresh", n)
	if e != nil {
		t.Fatal(e)
	}
	if got.SSH.Bastion == nil || got.SSH.Bastion.Password != "jump-secret" || got.SSH.Password != "new" {
		t.Fatal("refresh lost route or credentials")
	}
	n = sshNode("app", "192.0.2.3", 22, "")
	got, e = s.Put("refresh", n)
	if e != nil {
		t.Fatal(e)
	}
	if got.SSH.Password != "" || got.SSH.Bastion != nil {
		t.Fatal("different target inherited old credentials or route")
	}
	password := "db-secret"
	db := Node{ID: "db", Kind: "database", Password: &password, Database: &dbworkspace.Profile{Host: "192.0.2.4", Port: 3306, User: "read", DriverID: "driver", GatewayID: "app"}}
	got, e = s.Put("refresh", db)
	if e != nil {
		t.Fatal(e)
	}
	if got.GatewayID != "app" {
		t.Fatal("profile gateway lost")
	}
	db.Password = nil
	got, e = s.Put("refresh", db)
	if e != nil {
		t.Fatal(e)
	}
	if got.Password == nil || *got.Password != password {
		t.Fatal("same database lost credential")
	}
	db.Database.User = "another"
	got, e = s.Put("refresh", db)
	if e != nil {
		t.Fatal(e)
	}
	if got.Password != nil {
		t.Fatal("another database identity inherited credential")
	}
}
