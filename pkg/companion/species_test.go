package companion

import "testing"

func TestSpeciesCatalogHasDistinctPersonas(t *testing.T) {
	if len(speciesCatalog) < 7 {
		t.Fatal("at least seven companions required")
	}
	ids, personas := map[string]bool{}, map[string]bool{}
	for _, species := range speciesCatalog {
		if ids[species.ID] || personas[species.Persona] || species.Persona == "" || species.Character == "" {
			t.Fatal("invalid species", species)
		}
		ids[species.ID], personas[species.Persona] = true, true
	}
}
func firstMustCard(s *Snapshot, t *testing.T) Card {
	t.Helper()
	c, err := s.PublicCard()
	if err != nil {
		t.Fatal(err)
	}
	return c
}
