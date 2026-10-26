package security

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDeploymentPoliciesNeverReplaceMissingOrInvalidFiles(t *testing.T) {
	for _, loader := range []func(string) error{
		func(p string) error { _, e := LoadWhitelistManager(p); return e },
		func(p string) error { _, e := LoadFileAccessChecker(p); return e },
	} {
		p := filepath.Join(t.TempDir(), "policy.json")
		if loader(p) == nil {
			t.Fatal("missing explicit deployment policy accepted")
		}
		if _, e := os.Stat(p); !os.IsNotExist(e) {
			t.Fatal("missing policy was created")
		}
		invalid := []byte("{invalid")
		if e := os.WriteFile(p, invalid, 0600); e != nil {
			t.Fatal(e)
		}
		if loader(p) == nil {
			t.Fatal("invalid policy accepted")
		}
		b, _ := os.ReadFile(p)
		if string(b) != string(invalid) {
			t.Fatal("invalid deployment policy overwritten")
		}
		if e := loader(""); e != nil {
			t.Fatal(e)
		}
	}
}
