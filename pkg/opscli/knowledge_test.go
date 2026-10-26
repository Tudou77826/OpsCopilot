package opscli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testKnowledge = `---
service: Payments
module: API
---

## 场景：接口超时
- **现象**: 请求出现 504 timeout
- **关键词**: 504, timeout, 超时
- **涉及组件**: nginx

排查密码: password="should-not-leak"
`

func writeTestKnowledge(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "payments.md"), []byte(testKnowledge), 0600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestKnowledgeListSearchRead(t *testing.T) {
	dir := writeTestKnowledge(t)
	listed, err := knowledge([]string{"list"}, dir)
	if err != nil {
		t.Fatal(err)
	}
	var listJSON map[string]any
	encoded, _ := json.Marshal(listed)
	if err := json.Unmarshal(encoded, &listJSON); err != nil || len(listJSON["services"].([]any)) != 1 {
		t.Fatalf("list = %s", encoded)
	}

	search, err := knowledge([]string{"search", "--query", "timeout", "--top", "1"}, dir)
	if err != nil {
		t.Fatal(err)
	}
	searchJSON, _ := json.Marshal(search)
	if !strings.Contains(string(searchJSON), `"hits"`) || !strings.Contains(string(searchJSON), "接口超时") {
		t.Fatalf("search = %s", searchJSON)
	}

	catResult, err := knowledge([]string{"list", "--service", "Payments", "--module", "API"}, dir)
	if err != nil {
		t.Fatal(err)
	}
	briefJSON, _ := json.Marshal(catResult)
	var brief struct {
		Scenarios []struct {
			ID string `json:"id"`
		} `json:"scenarios"`
	}
	if err := json.Unmarshal(briefJSON, &brief); err != nil || len(brief.Scenarios) != 1 {
		t.Fatalf("brief = %s", briefJSON)
	}

	read, err := knowledge([]string{"read", "--id", brief.Scenarios[0].ID}, dir)
	if err != nil {
		t.Fatal(err)
	}
	readJSON, _ := json.Marshal(read)
	if strings.Contains(string(readJSON), "should-not-leak") || !strings.Contains(string(readJSON), "[redacted]") {
		t.Fatalf("read exposed credential: %s", readJSON)
	}
}

func TestKnowledgeEmptyAndMissPreserveSearchSemantics(t *testing.T) {
	empty := t.TempDir()
	got, err := knowledge([]string{"list"}, empty)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(got)
	if string(b) != `{"services":[]}` {
		t.Fatalf("empty list = %s", b)
	}
	got, err = knowledge([]string{"search", "--query", "unknown"}, empty)
	if err != nil {
		t.Fatal(err)
	}
	b, _ = json.Marshal(got)
	if !strings.Contains(string(b), `"miss_kind":"domain_uncovered"`) || !strings.Contains(string(b), `"hint"`) {
		t.Fatalf("empty search = %s", b)
	}

	dir := writeTestKnowledge(t)
	got, err = knowledge([]string{"search", "--query", "Payments"}, dir)
	if err != nil {
		t.Fatal(err)
	}
	b, _ = json.Marshal(got)
	if !strings.Contains(string(b), `"miss_kind":"term_mismatch"`) || !strings.Contains(string(b), `"covered_services"`) {
		t.Fatalf("miss search = %s", b)
	}
}

func TestKnowledgeRejectsInvalidArgsAndLoadErrors(t *testing.T) {
	dir := writeTestKnowledge(t)
	for _, args := range [][]string{
		{"unknown"}, {"list", "--service", "Payments"},
		{"search"}, {"search", "--query", "x", "extra"},
		{"read"}, {"read", "--id", "missing"},
	} {
		if _, err := knowledge(args, dir); err == nil {
			t.Errorf("knowledge(%v) unexpectedly succeeded", args)
		}
	}
	file := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(file, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := knowledge([]string{"list"}, file); err == nil {
		t.Fatal("regular file was accepted as knowledge directory")
	}
	if _, err := knowledge([]string{"list"}, filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("missing directory was accepted")
	}
}

func TestKnowledgeReadDoesNotWriteCatalog(t *testing.T) {
	dir := writeTestKnowledge(t)
	if _, err := knowledge([]string{"list"}, dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".catalog.json")); !os.IsNotExist(err) {
		t.Fatalf("read-only knowledge call wrote catalog: %v", err)
	}
}
