package terminalcontrol

import (
	"os/exec"
	"runtime"
	"strings"
	"testing"
)

func TestBashPromptProbeInstallationAndRemoval(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires a native Bash executable")
	}
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("Bash is not installed")
	}
	p := &protocol{nonce: "0123456789abcdef0123456789abcdef"}
	probe := p.promptProbe()
	for _, initial := range []string{"original> ", probe + "original> "} {
		script := "PS1=" + shellQuote(initial) + "\n" + p.bootstrap() + "\nprintf '%s\\n' \"$PS1\"\n" + p.bootstrap() + "\nprintf '%s\\n' \"$PS1\"\n" + p.uninstall() + "\nprintf '%s\\n' \"$PS1\"\n"
		output, err := exec.Command(bash, "--noprofile", "--norc", "-c", script).CombinedOutput()
		if err != nil {
			t.Fatalf("Bash prompt setup: %v: %s", err, output)
		}
		lines := strings.Split(strings.TrimSuffix(string(output), "\n"), "\n")
		want := `\[` + probe + `\]` + "original> "
		if len(lines) != 3 || lines[0] != want || lines[1] != want || lines[2] != "original> " {
			t.Fatalf("prompt installation, retry or restoration incorrect: %q", lines)
		}
	}
}
