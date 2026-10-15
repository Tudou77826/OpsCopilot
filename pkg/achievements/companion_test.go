package achievements

import (
	"opscopilot/pkg/companion"
	social "opscopilot/pkg/companionsocial"
	"testing"
)

func TestCompanionProjectionOrderingSelectionAndAcknowledgement(t *testing.T) {
	s := enabledStore(t)
	first := companion.Card{SpeciesID: companion.SpeciesID, Name: "微光", PaletteID: "mint", Level: 1}
	if err := s.QueueCompanion(&first, 1); err != nil {
		t.Fatal(err)
	}
	if err := s.QueueSelection([]social.MedalKind{social.MedalScriptCreated}); err != nil {
		t.Fatal(err)
	}
	before, _ := s.Read()
	latest := first
	latest.Name = "新名字"
	latest.PaletteID = "lavender"
	latest.Level = 3
	if err := s.QueueCompanion(&latest, 3); err != nil {
		t.Fatal(err)
	}
	// Delayed growth notification must not overwrite a later appearance save.
	if err := s.QueueCompanion(&first, 2); err != nil {
		t.Fatal(err)
	}
	current, _ := s.Read()
	if *current.Profile.Companion != latest || current.CompanionRevision != 3 || len(current.Profile.Selected) != 1 || current.Profile.Selected[0] != social.MedalScriptCreated {
		t.Fatal(current.Profile)
	}
	if err := s.AckProfile(*before.Profile); err != nil {
		t.Fatal(err)
	}
	current, _ = s.Read()
	if !current.ProfilePending {
		t.Fatal("old ack cleared newer projection")
	}
	version := current.Profile.Revision
	if err := s.QueueCompanion(&latest, 4); err != nil {
		t.Fatal(err)
	}
	current, _ = s.Read()
	if current.Profile.Revision != version || current.CompanionRevision != 4 {
		t.Fatal("same public card queued another revision")
	}
	if err := s.QueueSelection([]social.MedalKind{social.MedalKnowledgeRead}); err != nil {
		t.Fatal(err)
	}
	current, _ = s.Read()
	if *current.Profile.Companion != latest {
		t.Fatal("selection replaced companion")
	}
}

func TestLegacyCardUpgradesAtSamePetRevisionOnceWithoutLosingSelection(t *testing.T) {
	s := enabledStore(t)
	legacy := companion.Card{SpeciesID: companion.SpeciesID, Name: "微光", PaletteID: "mint", Stage: "growing"}
	if err := s.QueueCompanion(&legacy, 5); err != nil {
		t.Fatal(err)
	}
	if err := s.QueueSelection([]social.MedalKind{social.MedalScriptCreated}); err != nil {
		t.Fatal(err)
	}
	before, _ := s.Read()
	current := legacy
	current.Stage = ""
	current.Level = 5
	if err := s.QueueCompanion(&current, 5); err != nil {
		t.Fatal(err)
	}
	after, _ := s.Read()
	if after.CompanionRevision != 5 || after.Profile.Revision != before.Profile.Revision+1 || *after.Profile.Companion != current || len(after.Profile.Selected) != 1 || !after.ProfilePending {
		t.Fatal(after.Profile)
	}
	version := after.Profile.Revision
	changed := current
	changed.Level = 6
	if err := s.QueueCompanion(&changed, 5); err != nil {
		t.Fatal(err)
	}
	if err := s.QueueCompanion(&legacy, 4); err != nil {
		t.Fatal(err)
	}
	after, _ = s.Read()
	if after.Profile.Revision != version || *after.Profile.Companion != current {
		t.Fatal("same-version or stale card overwrote upgraded projection")
	}
}
