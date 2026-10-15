package shellsidecar

import (
	"opscopilot/pkg/assistantsettings"
	"opscopilot/pkg/companion"
)

func (a *ControlAPI) assistantPreferences() (assistantsettings.Snapshot, error) {
	settings, err := a.AssistantSettings.Read()
	if err == nil && a.Companion != nil && a.Companion.inner.Enabled() {
		pet, readErr := a.Companion.inner.ReadSnapshot()
		if readErr != nil {
			return settings, readErr
		}
		if pet.Pet != nil {
			settings.DefaultPrompt = companion.PersonaFor(pet.Pet.SpeciesID)
		}
	}
	return settings, err
}
