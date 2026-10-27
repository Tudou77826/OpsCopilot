package assistantcontrol

import (
	"strings"
	"testing"
)

func TestPageReferencesPersistAsLabelsWithoutRecoveringHandles(t *testing.T) {
	r := PageReference{Kind: "terminal", TabID: "unique-tab-handle", ConnectionID: "unique-connection-handle", Title: "业务节点"}
	h := &History{}
	h.Begin("turn", "检查这个页签", r)
	r.Title = "changed"
	snapshot := h.Snapshot()
	if snapshot.Messages[0].References[0].Title != "业务节点" {
		t.Fatal("reference was not captured with message")
	}
	if err := validateHistory(snapshot); err != nil {
		t.Fatal(err)
	}
	snapshot.Messages[0].References[0].Title = "mutated snapshot"
	if h.Snapshot().Messages[0].References[0].Title != "业务节点" {
		t.Fatal("mutable snapshot changed history")
	}
	snapshot = h.Snapshot()
	seed := RecoveryContext(snapshot)
	if !strings.Contains(seed, "业务节点") || strings.Contains(seed, "unique-tab-handle") || strings.Contains(seed, "unique-connection-handle") {
		t.Fatal("recovery must preserve labels, never restore resource handles", seed)
	}
	snapshot.Messages[0].References = append(snapshot.Messages[0].References, snapshot.Messages[0].References[0])
	if validateHistory(snapshot) == nil {
		t.Fatal("duplicate references accepted")
	}
}

func TestPageReferenceValidationBoundsAndKinds(t *testing.T) {
	valid := PageReference{Kind: "file", TabID: "file-tab", ConnectionID: "ssh-identity", Title: "文件"}
	if err := ValidatePageReferences([]PageReference{valid}); err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []PageReference{
		{Kind: "settings", TabID: "x", ConnectionID: "y"},
		{Kind: "terminal", TabID: "x"},
		{Kind: "database", TabID: "x", ConnectionID: "y", Title: strings.Repeat("x", 513)},
	} {
		if ValidatePageReferences([]PageReference{invalid}) == nil {
			t.Fatal("invalid reference accepted", invalid.Kind)
		}
	}
	if ValidatePageReferences(make([]PageReference, 9)) == nil {
		t.Fatal("too many references accepted")
	}
}
