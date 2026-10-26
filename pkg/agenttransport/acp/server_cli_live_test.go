package acp

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"opscopilot/pkg/assistantcontrol"
	"opscopilot/pkg/assistantskills"
)

func TestLiveICodeNativeServerCLIReusesSession(t *testing.T) {
	executable, cli := os.Getenv("OPSCOPILOT_CHRYS_TEST_EXE"), os.Getenv("OPSCOPILOT_TEST_CLI_BINARY")
	if executable == "" || cli == "" {
		t.Skip("opt-in iCode and prebuilt server CLI")
	}
	if !filepath.IsAbs(executable) || !filepath.IsAbs(cli) {
		t.Fatal("iCode and CLI paths must be absolute")
	}
	if _, err := os.Stat(executable); err != nil {
		t.Fatal("configured iCode executable unavailable")
	}
	if _, err := os.Stat(cli); err != nil {
		t.Fatal("configured server CLI unavailable")
	}

	root := t.TempDir()
	source := filepath.Join(root, "source")
	if err := os.MkdirAll(filepath.Join(source, "scripts"), 0700); err != nil {
		t.Fatal(err)
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	keyPath := filepath.Join(root, "server.key")
	if err := os.WriteFile(keyPath, key, 0600); err != nil {
		t.Fatal(err)
	}
	secret := "acp-cli-secret-" + hex.EncodeToString(key[:12])
	write := func(path, body string) {
		if err := os.WriteFile(path, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(source, "SKILL.md"), "---\nname: server-cli-check\ndescription: Run the server CLI lifecycle check.\n---\nSERVER_CLI_SKILL_BODY\n")
	write(filepath.Join(source, "scripts", "bootstrap.py"), serverCLIBootstrapScript)
	write(filepath.Join(source, "scripts", "reuse.py"), serverCLIReuseScript)
	library := assistantskills.Library{Directory: filepath.Join(root, "skills")}
	if err := library.Import(source, false); err != nil {
		t.Fatal(err)
	}

	var calls atomic.Int32
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Messages []struct {
				Role    string          `json:"role"`
				Content json.RawMessage `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		if serveReviewFixture(w, req) {
			return
		}
		all, _ := json.Marshal(req.Messages)
		if strings.Contains(string(all), secret) {
			t.Error("credential reached model context")
		}
		step := calls.Add(1)
		var tool string
		var args any
		switch step {
		case 1:
			tool, args = "load_skill", map[string]string{"skill_name": "server-cli-check"}
		case 2:
			tool, args = "run_skill_script", map[string]string{"skill_name": "server-cli-check", "script_name": "scripts/bootstrap.py"}
		case 3:
			if !strings.Contains(string(all), "BOOTSTRAP_OK") {
				t.Errorf("bootstrap result missing: %s", serverCLIToolDiagnostics(all, secret))
			}
			tool, args = "run_skill_script", map[string]string{"skill_name": "server-cli-check", "script_name": "scripts/reuse.py"}
		case 4:
			if !strings.Contains(string(all), "REUSE_OK") || !strings.Contains(string(all), "closed=false") {
				t.Errorf("Session was not reused: %s", serverCLIToolDiagnostics(all, secret))
			}
		default:
			t.Errorf("unexpected model request %d", step)
		}
		message := map[string]any{"role": "assistant", "content": "ACP_SERVER_CLI_OK"}
		reason := "stop"
		if tool != "" {
			b, _ := json.Marshal(args)
			message = map[string]any{"role": "assistant", "content": nil, "tool_calls": []any{map[string]any{"id": tool, "type": "function", "function": map[string]string{"name": tool, "arguments": string(b)}}}}
			reason = "tool_calls"
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "fixture", "object": "chat.completion", "model": "fixture", "choices": []any{map[string]any{"index": 0, "message": message, "finish_reason": reason}}})
	}))
	defer model.Close()

	serverRoot, keyPath := filepath.Join(root, "server"), filepath.Join(root, "server.key")
	env := map[string]string{"OPSCOPILOT_SERVER_ROOT": serverRoot, "OPSCOPILOT_SERVER_KEY_FILE": keyPath, "OPSCOPILOT_TEST_CLI_BINARY": cli, "OPSCOPILOT_ACP_CLI_SECRET": secret, "OPSCOPILOT_MODEL_API_KEY": "fixture"}
	envJSON, _ := json.Marshal(env)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	coordinator := &assistantcontrol.Coordinator{}
	go func() {
		for ctx.Err() == nil {
			s := coordinator.Snapshot()
			if s.Prompt != nil {
				_ = coordinator.Respond(s.TurnID, s.Prompt.ID, assistantcontrol.Answer{Selected: []string{"allow"}})
			}
			time.Sleep(10 * time.Millisecond)
		}
	}()
	turn, _ := coordinator.Begin(ctx, assistantcontrol.Target{}, nil)
	defer turn.Finish("completed", "")
	conversation := &Conversation{}
	defer conversation.Close()
	config := ManagedConfig{Executable: executable, BaseURL: model.URL + "/v1", APIKey: "fixture", Model: "fixture", LocalModel: LocalModel{Environment: string(envJSON)}, ParentDirectory: root, SkillsDirectory: library.Directory, AuditDirectory: filepath.Join(root, "audit")}
	if err := conversation.Run(ctx, config, turn, &managedFixtureTools{}, "Run the server CLI lifecycle Skill."); err != nil {
		// Test-only diagnostic: do not emit raw protocol data or subprocess stderr.
		if failure, ok := err.(*managedFailure); ok {
			t.Fatalf("%v (cause type=%T deadline=%t)", err, failure.cause, errors.Is(err, context.DeadlineExceeded))
		}
		t.Fatal(err)
	}
	if turn.Snapshot().Output != "ACP_SERVER_CLI_OK" || calls.Load() != 4 {
		t.Fatal("native CLI workflow incomplete", calls.Load(), turn.Snapshot())
	}
	state, err := os.ReadFile(filepath.Join(serverRoot, "sessions", "acp-cli-session", "state.enc"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(state), secret) {
		t.Fatal("Session state contains plaintext credential")
	}
}

// Only this synthetic fixture is logged; remove its generated credential before
// bounding the diagnostic so test failures remain useful on deployed hosts.
func serverCLIToolDiagnostics(messages []byte, secret string) string {
	var decoded []struct {
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal(messages, &decoded); err != nil {
		return "invalid fixture messages"
	}
	var results []string
	for _, message := range decoded {
		if message.Role == "tool" {
			results = append(results, string(message.Content))
		}
	}
	text := strings.ReplaceAll(strings.Join(results, "\n"), secret, "[redacted]")
	if len(text) > 12000 {
		text = text[len(text)-12000:]
	}
	return text
}

const serverCLIBootstrapScript = `import json, os, subprocess
binary = os.environ["OPSCOPILOT_TEST_CLI_BINARY"]
secret = os.environ["OPSCOPILOT_ACP_CLI_SECRET"]
def call(args, stdin=""):
    p = subprocess.run([binary] + args, input=stdin, universal_newlines=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
    text = (p.stdout or "") + (p.stderr or "")
    if secret in text or p.returncode != 0: raise RuntimeError("server CLI bootstrap failed")
    return json.loads(p.stdout)
call(["session","init","--session","acp-cli-session","--environment","native-test"])
node = {"id":"dummy-node","kind":"ssh","ssh":{"host":"127.0.0.1","port":22,"user":"fixture","password":secret,"protocol":"ssh"}}
call(["connection","put","--session","acp-cli-session","--input","-"], json.dumps(node))
listed = call(["connection","list","--session","acp-cli-session"])
status = call(["session","status","--session","acp-cli-session"])
if "dummy-node" not in json.dumps(listed["data"]) or status["data"]["closed"]: raise RuntimeError("bootstrap state missing")
print("BOOTSTRAP_OK closed=false passwordSet=true")
`

const serverCLIReuseScript = `import json, os, subprocess
binary = os.environ["OPSCOPILOT_TEST_CLI_BINARY"]
secret = os.environ["OPSCOPILOT_ACP_CLI_SECRET"]
def call(args):
    p = subprocess.run([binary] + args, universal_newlines=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
    text = (p.stdout or "") + (p.stderr or "")
    if secret in text or p.returncode != 0: raise RuntimeError("server CLI reuse failed")
    return json.loads(p.stdout)
status = call(["session","status","--session","acp-cli-session"])
listed = call(["connection","list","--session","acp-cli-session"])
if status["data"]["closed"] or "dummy-node" not in json.dumps(listed["data"]): raise RuntimeError("persisted Session missing")
print("REUSE_OK closed=false dummy-node=true")
`
