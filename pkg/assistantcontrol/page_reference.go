package assistantcontrol

import (
	"errors"
	"unicode/utf8"
)

// PageReference identifies a resource, never a read grant or an input lease.
// Titles are display metadata; tools must continue to verify actual identities.
type PageReference struct {
	Kind         string `json:"kind"`
	TabID        string `json:"tabId"`
	ConnectionID string `json:"connectionId"`
	Title        string `json:"title"`
}

func ValidatePageReferences(refs []PageReference) error {
	if len(refs) > 8 {
		return errors.New("每条消息最多引用 8 个页签")
	}
	seen := make(map[string]bool)
	for _, r := range refs {
		if (r.Kind != "terminal" && r.Kind != "database" && r.Kind != "file") || r.TabID == "" || r.ConnectionID == "" || len(r.TabID) > 128 || len(r.ConnectionID) > 128 || len(r.Title) > 512 || !utf8.ValidString(r.Kind+r.TabID+r.ConnectionID+r.Title) || seen[r.TabID] {
			return errors.New("页签引用无效，请移除后重新选择")
		}
		seen[r.TabID] = true
	}
	return nil
}
