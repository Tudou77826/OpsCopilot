package assistanttools

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"opscopilot/pkg/assistantcontrol"
	"opscopilot/pkg/assistantreference"
	"opscopilot/pkg/knowledge"
	"opscopilot/pkg/terminalcontrol"
)

func referenceFixture(t *testing.T) (*ReferenceTools, string) {
	t.Helper()
	dir := t.TempDir()
	body := "---\nservice: payments\nmodule: api\ntype: sop\n---\n\n## 场景：支付超时\n- **关键词**: E215, timeout\n\n历史处理：检查连接池；必须验证当前日志。\npassword=hidden-secret\n\n## 场景：回调失败\n- **关键词**: callback\n\n检查回调日志。\n"
	if err := os.WriteFile(filepath.Join(dir, "payments.md"), []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	c := &assistantcontrol.Coordinator{}
	turn, err := c.Begin(context.Background(), assistantcontrol.Target{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { turn.Finish("completed", "") })
	return &ReferenceTools{Turn: turn, KnowledgeDirectory: func() string { return dir }, Commands: func() ([]ReferenceCommand, error) {
		return []ReferenceCommand{
			{Kind: "terminal", ID: "same", Name: "看日志", Group: "排查", Content: "tail -n 20 /var/log/app.log; password=hidden-secret", Enabled: true},
			{Kind: "assistant", ID: "same", Name: "same", Description: "发现数据库", Content: "查找 {{business}}", Skill: "discover", Enabled: false},
		}, nil
	}}, dir
}

func TestReferencesUseExistingKnowledgeSearchAndReadWithoutSSH(t *testing.T) {
	r, dir := referenceFixture(t)
	before := r.Turn.Snapshot()
	listed, err := r.Call(context.Background(), "knowledge_list", `{"service":"payments","limit":1}`)
	if err != nil {
		t.Fatal(err)
	}
	page := listed.(map[string]any)
	if page["total"] != 2 || page["has_more"] != true || len(page["items"].([]referenceScenario)) != 1 {
		t.Fatal(page)
	}
	result, err := r.Call(context.Background(), "knowledge_search", `{"query":"E215"}`)
	if err != nil {
		t.Fatal(err)
	}
	hits := result.(*knowledge.SearchResult).Hits
	if len(hits) != 1 {
		t.Fatal(result)
	}
	data, _ := json.Marshal(map[string]string{"id": hits[0].ID})
	result, err = r.Call(context.Background(), "knowledge_read", string(data))
	if err != nil {
		t.Fatal(err)
	}
	read := result.(map[string]any)
	if read["source"] != "payments.md" || !strings.Contains(read["content"].(string), "检查连接池") || strings.Contains(read["content"].(string), "hidden-secret") || read["redacted"] != true || read["truncated"] != false {
		t.Fatal("incorrect content/source or redaction flags")
	}
	after := r.Turn.Snapshot()
	if after.Target != before.Target || after.Prompt != nil || after.Phase != before.Phase {
		t.Fatal("reference read changed authority")
	}
	if _, err = os.Stat(filepath.Join(dir, ".catalog.json")); !os.IsNotExist(err) {
		t.Fatal("read wrote catalog", err)
	}
	// Re-read the shared source, not a stale assistant copy.
	if err = os.Remove(filepath.Join(dir, "payments.md")); err != nil {
		t.Fatal(err)
	}
	if _, err = r.Call(context.Background(), "knowledge_read", string(data)); err == nil {
		t.Fatal("deleted source remained visible")
	}
}

func TestReferenceCommandsAreDiscoverableDefinitionsNotDispatches(t *testing.T) {
	r, _ := referenceFixture(t)
	result, err := r.Call(context.Background(), "quick_command_list", `{"query":"数据库"}`)
	if err != nil {
		t.Fatal(err)
	}
	page := result.(map[string]any)
	items := page["items"].([]ReferenceCommand)
	if len(items) != 1 || items[0].Kind != "assistant" || items[0].Enabled || items[0].Content != "" {
		t.Fatal(items)
	}
	result, err = r.Call(context.Background(), "quick_command_read", `{"kind":"terminal","id":"same"}`)
	if err != nil {
		t.Fatal(err)
	}
	read := result.(map[string]any)
	command := read["command"].(ReferenceCommand)
	if command.Kind != "terminal" || !strings.Contains(command.Content, "tail -n 20") || strings.Contains(command.Content, "hidden-secret") || read["redacted"] != true {
		t.Fatal("command identity or credential filtering failed")
	}
	result, err = r.Call(context.Background(), "quick_command_read", `{"kind":"assistant","id":"same"}`)
	if err != nil || result.(map[string]any)["command"].(ReferenceCommand).Content != "查找 {{business}}" {
		t.Fatal(result, err)
	}
	if r.Turn.Snapshot().Prompt != nil || r.Turn.Snapshot().Target != (assistantcontrol.Target{}) {
		t.Fatal("definition read requested execution authority")
	}
	r.Commands = func() ([]ReferenceCommand, error) { return nil, errors.New("source failed") }
	if _, err = r.Call(context.Background(), "quick_command_list", `{}`); err == nil {
		t.Fatal("load failure hidden as empty commands")
	}
}

func TestReferenceArgumentsRejectPathsExecutionAndInvalidPagination(t *testing.T) {
	for _, test := range []struct{ name, args string }{
		{"knowledge_read", `{"id":"x","path":"../secret"}`},
		{"knowledge_list", `{"approved":true}`},
		{"knowledge_search", `{"query":null}`},
		{"knowledge_search", `{"query":" "}`},
		{"knowledge_search", `{"query":"x","limit":51}`},
		{"knowledge_list", `{"offset":-1}`},
		{"knowledge_list", `{"limit":0}`},
		{"knowledge_list", `{"offset":1.5}`},
		{"quick_command_read", `{"id":"x"}`},
		{"quick_command_read", `{"kind":"terminal","id":"x","execute":true}`},
		{"quick_command_list", `{"kind":"local"}`},
		{"knowledge_read", `{"id":"a\ncommand"}`},
		{"knowledge_list", `null`},
	} {
		if _, err := assistantreference.ParseArguments(test.name, test.args); err == nil {
			t.Errorf("accepted %s %s", test.name, test.args)
		}
	}
}

func TestReferencesMissingEmptyAndInvalidKnowledgeSources(t *testing.T) {
	r, dir := referenceFixture(t)
	r.KnowledgeDirectory = func() string { return filepath.Join(dir, "missing") }
	result, err := r.Call(context.Background(), "knowledge_list", `{}`)
	if err != nil || result.(map[string]any)["total"] != 0 {
		t.Fatal(result, err)
	}
	result, err = r.Call(context.Background(), "knowledge_search", `{"query":"missing"}`)
	if err != nil || len(result.(*knowledge.SearchResult).Hits) != 0 || strings.Contains(result.(*knowledge.SearchResult).Hint, "exec ") {
		t.Fatal(result, err)
	}
	r.KnowledgeDirectory = func() string { return filepath.Join(dir, "payments.md") }
	if _, err = r.Call(context.Background(), "knowledge_list", `{}`); err == nil {
		t.Fatal("file accepted as directory")
	}
}

func TestReferencesRespectStopAndLeaveTerminalOwnershipUnchanged(t *testing.T) {
	r, _ := referenceFixture(t)
	w := NewWorkspaceTools(r.Turn, terminalcontrol.Target{}, nil, nil, nil, func(target terminalcontrol.Target) *TerminalTools {
		return &TerminalTools{Turn: r.Turn, Target: target}
	})
	w.References = r
	defer w.Close()
	original := w.current
	original.readAllowed = true
	if _, err := w.Call(context.Background(), "quick_command_list", `{}`); err != nil {
		t.Fatal(err)
	}
	if w.current != original || !original.readAllowed || w.generation.Load() != 0 {
		t.Fatal("read changed resource ownership")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := w.Call(ctx, "knowledge_list", `{}`); !errors.Is(err, assistantcontrol.ErrStale) {
		t.Fatal(err)
	}
	r.Commands = func() ([]ReferenceCommand, error) { r.Turn.Finish("stopped", ""); return []ReferenceCommand{}, nil }
	if _, err := w.Call(context.Background(), "quick_command_list", `{}`); !errors.Is(err, assistantcontrol.ErrStale) {
		t.Fatal("completed read escaped stopped turn", err)
	}
}
