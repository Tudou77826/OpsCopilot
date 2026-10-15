package companion

import (
	_ "embed"
	"encoding/json"
)

// SpeciesData is also consumed by the shared frontend; identities and personas have one source.
//
//go:embed species.json
var SpeciesData []byte

type Species struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Type        string `json:"type"`
	Character   string `json:"character"`
	Description string `json:"description"`
	Persona     string `json:"persona"`
}

var speciesCatalog = func() []Species {
	var catalog []Species
	if err := json.Unmarshal(SpeciesData, &catalog); err != nil {
		panic(err)
	}
	return catalog
}()

func FindSpecies(id string) (Species, bool) {
	for _, species := range speciesCatalog {
		if species.ID == id {
			return species, true
		}
	}
	return Species{}, false
}

func PersonaFor(id string) string {
	if species, ok := FindSpecies(id); ok {
		return species.Persona
	}
	return speciesCatalog[0].Persona
}
