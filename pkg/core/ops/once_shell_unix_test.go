//go:build !windows

package ops

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestDirectoryFailureCannotRunLaterShellStatements(t *testing.T) {
	request, err := PrepareOnce(onceConfig(), "cat missing-file; echo SHOULD_NOT_RUN", OnceOptions{Directory: filepath.Join(t.TempDir(), "missing-directory")}, oncePolicy(t))
	if err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("/bin/sh", "-c", request.Command).CombinedOutput()
	if err == nil || strings.Contains(string(out), "SHOULD_NOT_RUN") {
		t.Fatalf("command continued outside its requested directory: %s err=%v", out, err)
	}
}
