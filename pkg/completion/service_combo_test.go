package completion

import "testing"

func TestComboSuggestions(t *testing.T) {
	db, err := NewDatabase()
	if err != nil {
		t.Fatalf("NewDatabase error: %v", err)
	}
	svc := NewService(db)

	resp, err := svc.GetCompletions(CompletionRequest{
		Input:  "zgrep ",
		Cursor: len("zgrep "),
	})
	if err != nil {
		t.Fatalf("GetCompletions error: %v", err)
	}

	found := false
	for _, s := range resp.Suggestions {
		if s.Text == " -rni" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected combo suggestion %q in zgrep suggestions", " -rni")
	}

	resp2, err := svc.GetCompletions(CompletionRequest{
		Input:  "tar -c",
		Cursor: len("tar -c"),
	})
	if err != nil {
		t.Fatalf("GetCompletions error: %v", err)
	}

	found2 := false
	for _, s := range resp2.Suggestions {
		if s.Text == "-cxvf" {
			found2 = true
			break
		}
	}
	if !found2 {
		t.Fatalf("expected combo suggestion %q in tar suggestions", "-cxvf")
	}
}
