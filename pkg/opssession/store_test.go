package opssession

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"opscopilot/pkg/remote"
)

var testKey = []byte("01234567890123456789012345678901")

func newTestStore(t *testing.T) (*Store, string) {
	t.Helper()
	root := t.TempDir()
	s, err := New(root, testKey)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s, root
}

func sshNode(id, host string, port int, password string) Node {
	return Node{
		ID:       id,
		Name:     id,
		Kind:     "ssh",
		Password: nil,
		SSH: &remote.ConnectConfig{
			Host:     host,
			Port:     port,
			User:     "operator",
			Password: password,
		},
	}
}

func mustInit(t *testing.T, s *Store, id string) State {
	t.Helper()
	state, err := s.Init(id, "production")
	if err != nil {
		t.Fatalf("Init(%q): %v", id, err)
	}
	return state
}

func mustPut(t *testing.T, s *Store, id string, node Node) Node {
	t.Helper()
	got, err := s.Put(id, node)
	if err != nil {
		t.Fatalf("Put(%q, %q): %v", id, node.ID, err)
	}
	return got
}

func TestStoreRestoresNodeAndPasswordAcrossNewInstances(t *testing.T) {
	first, root := newTestStore(t)
	mustInit(t, first, "session-a")
	want := sshNode("node-a", "10.0.0.8", 2222, "ssh-secret")
	mustPut(t, first, "session-a", want)

	second, err := New(root, testKey)
	if err != nil {
		t.Fatalf("New second store: %v", err)
	}
	got, err := second.Node("session-a", "node-a")
	if err != nil {
		t.Fatalf("Node after reopening store: %v", err)
	}
	if got.SSH == nil || got.SSH.Password != "ssh-secret" || got.SSH.Port != 2222 {
		t.Fatalf("restored node = %#v, want password and port restored", got)
	}
}

func TestStoreStateFileDoesNotContainPlaintextPassword(t *testing.T) {
	s, root := newTestStore(t)
	mustInit(t, s, "session-a")
	mustPut(t, s, "session-a", sshNode("node-a", "host-a", 22, "password-must-stay-encrypted"))

	b, err := os.ReadFile(filepath.Join(root, "sessions", "session-a", "state.enc"))
	if err != nil {
		t.Fatalf("read state.enc: %v", err)
	}
	if string(b) == "" || string(b) == "password-must-stay-encrypted" ||
		containsBytes(b, []byte("password-must-stay-encrypted")) {
		t.Fatalf("state.enc contains plaintext password")
	}
}

func TestStoreRejectsWrongKeyAndCrossSessionCiphertext(t *testing.T) {
	s, root := newTestStore(t)
	mustInit(t, s, "session-a")
	mustInit(t, s, "session-b")
	mustPut(t, s, "session-a", sshNode("node-a", "host-a", 22, "secret-a"))
	mustPut(t, s, "session-b", sshNode("node-b", "host-b", 22, "secret-b"))

	wrongKey := append([]byte(nil), testKey...)
	wrongKey[0]++
	wrong, err := New(root, wrongKey)
	if err != nil {
		t.Fatalf("New wrong-key store: %v", err)
	}
	if _, err = wrong.Inspect("session-a"); err == nil {
		t.Fatal("Inspect with wrong key succeeded")
	}

	aPath := filepath.Join(root, "sessions", "session-a", "state.enc")
	bPath := filepath.Join(root, "sessions", "session-b", "state.enc")
	aCiphertext, err := os.ReadFile(aPath)
	if err != nil {
		t.Fatalf("read session-a ciphertext: %v", err)
	}
	if err = os.WriteFile(bPath, aCiphertext, 0600); err != nil {
		t.Fatalf("replace session-b ciphertext: %v", err)
	}
	if _, err = s.Inspect("session-b"); err == nil {
		t.Fatal("Inspect accepted ciphertext from another Session")
	}
}

func TestStoreKeepsSameIPNodesIndependentAndPutUpdatesOne(t *testing.T) {
	s, _ := newTestStore(t)
	mustInit(t, s, "session-a")
	mustPut(t, s, "session-a", sshNode("node-one", "10.0.0.9", 22, "one-password"))
	mustPut(t, s, "session-a", sshNode("node-two", "10.0.0.9", 2200, "two-password"))

	updated := sshNode("node-one", "10.0.0.9", 22, "")
	got := mustPut(t, s, "session-a", updated)
	if got.SSH.Password != "one-password" {
		t.Fatalf("Put without password cleared existing password: %q", got.SSH.Password)
	}
	one, err := s.Node("session-a", "node-one")
	if err != nil {
		t.Fatalf("Node one: %v", err)
	}
	two, err := s.Node("session-a", "node-two")
	if err != nil {
		t.Fatalf("Node two: %v", err)
	}
	if one.SSH.Port != 22 || one.SSH.Password != "one-password" || two.SSH.Port != 2200 || two.SSH.Password != "two-password" {
		t.Fatalf("nodes changed unexpectedly: one=%#v two=%#v", one, two)
	}
}

func TestStoreCloseClearsCredentialsAndWorkAndIsIdempotent(t *testing.T) {
	s, root := newTestStore(t)
	mustInit(t, s, "session-a")
	mustPut(t, s, "session-a", sshNode("node-a", "host-a", 22, "close-secret"))
	work, err := s.WorkDir("session-a")
	if err != nil {
		t.Fatalf("WorkDir: %v", err)
	}
	if err = os.WriteFile(filepath.Join(work, "artifact"), []byte("temporary"), 0600); err != nil {
		t.Fatalf("write work artifact: %v", err)
	}

	if err = s.Close(context.Background(), "session-a"); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err = s.Close(context.Background(), "session-a"); err != nil {
		t.Fatalf("second Close: %v", err)
	}
	state, err := s.Inspect("session-a")
	if err != nil {
		t.Fatalf("Inspect closed Session: %v", err)
	}
	if !state.Closed || len(state.Nodes) != 0 || state.Secrets != nil {
		t.Fatalf("closed state retained credentials or nodes: %#v", state)
	}
	if _, err = os.Stat(filepath.Join(root, "sessions", "session-a", "work")); !os.IsNotExist(err) {
		t.Fatalf("work directory still exists, err=%v", err)
	}
	if _, err = s.Init("session-a", "production"); !errors.Is(err, ErrClosed) {
		t.Fatalf("Init after Close error = %v, want ErrClosed", err)
	}
}

func TestStoreCloseCancelsBeginAndWaitsForFinish(t *testing.T) {
	s, _ := newTestStore(t)
	mustInit(t, s, "session-a")
	ctx, finish, err := s.Begin(context.Background(), "session-a")
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	closeDone := make(chan error, 1)
	go func() { closeDone <- s.Close(context.Background(), "session-a") }()

	select {
	case <-ctx.Done():
	case <-time.After(3 * time.Second):
		t.Fatal("Begin context was not canceled by Close")
	}
	if !errors.Is(context.Cause(ctx), ErrClosed) {
		t.Fatalf("Begin context cause = %v, want ErrClosed", context.Cause(ctx))
	}

	finish()
	select {
	case err = <-closeDone:
		if err != nil {
			t.Fatalf("Close after Begin finish: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Close did not wait for Begin finish or did not return")
	}
}

func TestStoreSessionsDoNotAffectEachOther(t *testing.T) {
	s, _ := newTestStore(t)
	mustInit(t, s, "session-a")
	mustInit(t, s, "session-b")
	mustPut(t, s, "session-a", sshNode("node-a", "host-a", 22, "secret-a"))
	mustPut(t, s, "session-b", sshNode("node-b", "host-b", 22, "secret-b"))

	if err := s.Close(context.Background(), "session-a"); err != nil {
		t.Fatalf("Close session-a: %v", err)
	}
	state, err := s.Inspect("session-b")
	if err != nil {
		t.Fatalf("Inspect session-b: %v", err)
	}
	if state.Closed || len(state.Nodes) != 1 || state.Nodes[0].SSH.Password != "secret-b" {
		t.Fatalf("session-b changed after closing session-a: %#v", state)
	}
}

func containsBytes(haystack, needle []byte) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		match := true
		for j := range needle {
			if haystack[i+j] != needle[j] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}
