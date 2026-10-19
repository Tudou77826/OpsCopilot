package achievements

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	social "opscopilot/pkg/companionsocial"
)

type forbiddenSelectionSecrets struct{ t *testing.T }

func (s forbiddenSelectionSecrets) Set(string, string, string) error {
	s.t.Fatal("local selection accessed credentials")
	return nil
}
func (s forbiddenSelectionSecrets) Get(string, string) (string, error) {
	s.t.Fatal("local selection accessed credentials")
	return "", nil
}
func (s forbiddenSelectionSecrets) Delete(string, string) error {
	s.t.Fatal("local selection accessed credentials")
	return nil
}

type forbiddenSelectionTransport struct{ t *testing.T }

func (r forbiddenSelectionTransport) RoundTrip(*http.Request) (*http.Response, error) {
	r.t.Fatal("local selection attempted network access")
	return nil, fmt.Errorf("offline")
}

func TestShowcaseSelectionIsLocalForParticipatingMember(t *testing.T) {
	store := enabledStore(t)
	if err := store.Record(social.MedalScriptCreated, "one", true); err != nil {
		t.Fatal(err)
	}
	binding := Binding{BaseURL: "https://offline.invalid", Member: social.Member{UserID: "alice", OrganizationID: "org", Participating: true}}
	if err := store.Bind(binding, true); err != nil {
		t.Fatal(err)
	}
	client := NewClient(store, forbiddenSelectionSecrets{t})
	client.http = &http.Client{Transport: forbiddenSelectionTransport{t}}
	status, err := client.ShowcaseSelection(context.Background(), []social.MedalKind{social.MedalScriptCreated})
	if err != nil {
		t.Fatal(err)
	}
	if len(status.Selected) != 1 || status.Selected[0] != social.MedalScriptCreated || !status.ProfilePending {
		t.Fatal("local selection was not saved and queued")
	}
	state, err := store.Read()
	if err != nil || state.Binding == nil || *state.Binding != binding {
		t.Fatal("existing organization binding changed", err)
	}
}
