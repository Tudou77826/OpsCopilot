package config

import (
	"os"
	"testing"
)

func TestCompanionDefaultAndExplicitChoice(t *testing.T) {
	for _, test := range []struct {
		name, data string
		enabled    bool
	}{
		{"new", "", true},
		{"missing", `{}`, true},
		{"legacyGarden", `{"experimental":{"garden_enabled":false}}`, true},
		{"enabled", `{"experimental":{"companion_enabled":true}}`, true},
		{"disabled", `{"experimental":{"companion_enabled":false}}`, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			manager := NewManagerWithDir(dir)
			if test.data != "" {
				if err := os.WriteFile(manager.configPath, []byte(test.data), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if err := manager.Load(); err != nil {
				t.Fatal(err)
			}
			if manager.Config.Experimental.CompanionEnabled != test.enabled {
				t.Fatal("unexpected initial companion state")
			}
			if err := manager.Save(); err != nil {
				t.Fatal(err)
			}
			reread := NewManagerWithDir(dir)
			if err := reread.Load(); err != nil {
				t.Fatal(err)
			}
			if reread.Config.Experimental.CompanionEnabled != test.enabled {
				t.Fatal("companion choice not persisted")
			}
		})
	}
}
