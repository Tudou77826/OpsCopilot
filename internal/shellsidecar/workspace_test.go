package shellsidecar

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorkspacePathsAndStreaming(t *testing.T) {
	data := t.TempDir()
	if err := os.WriteFile(filepath.Join(data, "secret.json"), []byte("private"), 0600); err != nil {
		t.Fatal(err)
	}
	files, err := NewWorkspaceFT(nil, filepath.Join(data, "files"))
	if err != nil {
		t.Fatal(err)
	}
	defer files.Close()
	for _, path := range []string{"../secret.json", "/../secret.json", "C:/secret.json", "a/../../secret.json", "a\\..\\secret.json"} {
		if files.LocalStat(path).OK {
			t.Fatalf("escaped via %s", path)
		}
	}
	if files.LocalRemove("/").OK || files.LocalRename("/", "moved").OK {
		t.Fatal("root mutation allowed")
	}
	server := httptest.NewServer(files.WorkspaceHTTP("test-secret"))
	defer server.Close()
	call := func(method, path string, body io.Reader, token string) *http.Response {
		req, err := http.NewRequest(method, server.URL+"/workspace?path="+path, body)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer "+token)
		response, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return response
	}
	denied := call("PUT", "sample.txt", strings.NewReader("untrusted"), "bad")
	denied.Body.Close()
	if denied.StatusCode != 403 {
		t.Fatal(denied.StatusCode)
	}
	content := bytes.Repeat([]byte("large-stream-content\n"), 70000)
	put := call("PUT", "sample.txt", bytes.NewReader(content), "test-secret")
	put.Body.Close()
	if put.StatusCode != 201 {
		t.Fatal(put.StatusCode)
	}
	conflict := call("PUT", "sample.txt", strings.NewReader("overwrite"), "test-secret")
	conflict.Body.Close()
	if conflict.StatusCode != 409 {
		t.Fatal("silently overwrote file")
	}
	get := call("GET", "sample.txt", nil, "test-secret")
	actual, err := io.ReadAll(get.Body)
	get.Body.Close()
	if err != nil || !bytes.Equal(actual, content) {
		t.Fatal("stream content mismatch")
	}
	if err := os.Symlink(data, filepath.Join(data, "files", "escape")); err == nil {
		if files.LocalStat("escape/secret.json").OK {
			t.Fatal("symlink escaped root")
		}
	} else {
		t.Log("OS symlink creation unavailable; junction coverage is in Node integration")
	}
}

func TestAISessionConfigDoesNotPersistSecret(t *testing.T) {
	data := t.TempDir()
	svc, err := NewAIConfigService(data)
	if err != nil {
		t.Fatal(err)
	}
	status := svc.SaveSession(AIConfigUpdate{ApiKey: "session-private-secret", BaseURL: "https://model.example/v1", FastModel: "fixture"})
	if !status.Configured || status.Source != "session" || strings.Contains(status.KeyHint, "session-private") {
		t.Fatal(status)
	}
	if _, err := os.Stat(filepath.Join(data, "ai-config.json")); !os.IsNotExist(err) {
		t.Fatal("session config persisted")
	}
	fresh, err := NewAIConfigService(data)
	if err != nil {
		t.Fatal(err)
	}
	if fresh.load().APIKey != "" {
		t.Fatal("secret survived restart")
	}
}
