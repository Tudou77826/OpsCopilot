package terminalhandoff

import (
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestDesktopFixture explicitly holds the local real-PTY endpoint for a desktop
// UI trial. It is absent from the product binary and never uses saved servers.
func TestDesktopFixture(t *testing.T) {
	output := os.Getenv("OPSCOPILOT_HANDOFF_DESKTOP")
	if output == "" {
		t.Skip("opt-in desktop fixture disabled")
	}
	ownerAlive, closeOwner, err := fixtureOwner(os.Getenv("OPSCOPILOT_HANDOFF_OWNER_PID"))
	if err != nil {
		t.Fatal(err)
	}
	defer closeOwner()
	_, _, server := startLab(t, "/bin/sh")
	data, err := json.Marshal(map[string]interface{}{"host": "127.0.0.1", "port": server.listener.Addr().(*net.TCPAddr).Port, "user": "lab", "password": "isolated-fixture"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(output), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(output, data, 0600); err != nil {
		t.Fatal(err)
	}
	ticker := time.NewTicker(300 * time.Millisecond)
	defer ticker.Stop()
	var deadline <-chan time.Time
	if os.Getenv("OPSCOPILOT_HANDOFF_KEEP") != "1" {
		deadline = time.After(20 * time.Minute)
	}
	for {
		select {
		case <-ticker.C:
			if !ownerAlive() {
				t.Log("owning preview process exited")
				return
			}
			if _, err := os.Stat(output + ".stop"); err == nil {
				return
			}
		case <-deadline:
			return
		}
	}
}
