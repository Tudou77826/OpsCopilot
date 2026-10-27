package acp

import (
	"bytes"
	"encoding/json"
	"net/http"
)

// Simulate iCode's own judge for opt-in tests using a real iCode process.
func serveReviewFixture(w http.ResponseWriter, request any) bool {
	data, _ := json.Marshal(request)
	if !bytes.Contains(bytes.ToLower(data), []byte("auto-approval assistant")) {
		return false
	}
	approved := !bytes.Contains(data, []byte("run_skill_script"))
	content, _ := json.Marshal(map[string]any{"approved": approved, "reason": "fixture"})
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"id": "judge-fixture", "object": "chat.completion", "choices": []any{map[string]any{"index": 0, "message": map[string]string{"role": "assistant", "content": string(content)}, "finish_reason": "stop"}}})
	return true
}
