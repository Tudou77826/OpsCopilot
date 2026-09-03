package filetxn

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
)

func TestMergePreservesUnrelatedAndRejectsConflict(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.json")
	base := []byte(`{"theme":"dark","llm":{"key":"private","model":"a"},"unknown":7}`)
	if _, e := Merge(p, nil, base); e != nil {
		t.Fatal(e)
	}
	if _, e := Merge(p, base, []byte(`{"theme":"light","llm":{"key":"private","model":"a"},"unknown":7}`)); e != nil {
		t.Fatal(e)
	}
	merged, e := Merge(p, base, []byte(`{"theme":"dark","llm":{"key":"private","model":"b"}}`))
	if e != nil {
		t.Fatal(e)
	}
	var v map[string]any
	json.Unmarshal(merged, &v)
	if v["theme"] != "light" || v["unknown"] != float64(7) {
		t.Fatalf("lost unrelated fields: %s", merged)
	}
	if _, e = Merge(p, base, []byte(`{"theme":"blue"}`)); !errors.Is(e, ErrConflict) {
		t.Fatalf("expected conflict: %v", e)
	}
	before, _ := os.ReadFile(p)
	if string(before) != string(merged) {
		t.Fatal("conflict wrote data")
	}
	if e = os.WriteFile(p, []byte(`broken`), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e = Merge(p, base, base); e == nil {
		t.Fatal("must reject corrupted file")
	}
	after, _ := os.ReadFile(p)
	if string(after) != "broken" {
		t.Fatal("corrupt file overwritten")
	}
}

func TestProcessLock(t *testing.T) {
	if p := os.Getenv("OPS_TXN_HELPER"); p != "" {
		for i := 0; i < 20; i++ {
			release, e := Lock(p)
			if e != nil {
				t.Fatal(e)
			}
			b, e := Read(p)
			if e != nil {
				t.Fatal(e)
			}
			n := 0
			if len(b) > 0 {
				if e = json.Unmarshal(b, &n); e != nil {
					t.Fatal(e)
				}
			}
			b, _ = json.Marshal(n + 1)
			e = Write(p, b)
			release()
			if e != nil {
				t.Fatal(e)
			}
		}
		return
	}
	p := filepath.Join(t.TempDir(), "counter.json")
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cmd := exec.Command(os.Args[0], "-test.run=^TestProcessLock$")
			cmd.Env = append(os.Environ(), "OPS_TXN_HELPER="+p)
			if b, e := cmd.CombinedOutput(); e != nil {
				t.Errorf("helper: %v %s", e, b)
			}
		}()
	}
	wg.Wait()
	b, e := os.ReadFile(p)
	if e != nil || string(b) != "80" {
		t.Fatalf("lost updates: %s %v", b, e)
	}
}
