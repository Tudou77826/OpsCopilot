package assistantreview

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"opscopilot/pkg/assistantcontrol"
)

func TestDirectFastRequestUsesOneBoundedChoiceCall(t *testing.T) {
	for _, tc := range []struct {
		name, output      string
		status            int
		fallback, allowed bool
	}{
		{"allow", "A", 200, false, true}, {"human", "B", 200, false, false}, {"invalid", "probably safe", 200, false, false}, {"service error", "", 503, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requests, fallbacks := 0, 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				if r.URL.Path != "/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer fixture-key" {
					t.Error("wrong configured endpoint or authentication")
				}
				var request struct {
					Model     string                           `json:"model"`
					MaxTokens int                              `json:"max_tokens"`
					Stream    bool                             `json:"stream"`
					Tools     []any                            `json:"tools"`
					Messages  []struct{ Role, Content string } `json:"messages"`
					Template  map[string]any                   `json:"chat_template_kwargs"`
				}
				if json.NewDecoder(r.Body).Decode(&request) != nil {
					t.Error("invalid request")
				}
				if request.Model != "configured-fast" || request.MaxTokens != 16 || request.Stream || len(request.Tools) != 0 || len(request.Messages) != 2 || len(request.Template) != 0 {
					t.Error("not a bounded fast classifier")
				}
				if !strings.Contains(request.Messages[0].Content, "A =") || !strings.Contains(request.Messages[0].Content, "E =") || !strings.Contains(request.Messages[1].Content, "exact command") {
					t.Error("missing fixed choices or original action")
				}
				w.Header().Set("Content-Type", "application/json")
				if tc.status != 200 {
					w.WriteHeader(tc.status)
					_, _ = w.Write([]byte(`{"error":{"message":"private response must not be shown"}}`))
					return
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"role": "assistant", "content": tc.output}, "finish_reason": "stop"}}})
			}))
			defer server.Close()
			s := Service{Fast: NewFastClassifier(server.URL+"/v1", "fixture-key", "configured-fast"), Fallback: func(context.Context, assistantcontrol.ReviewInput) assistantcontrol.ReviewVerdict {
				fallbacks++
				return assistantcontrol.ReviewVerdict{Reason: "fallback fixture"}
			}}
			v := s.Review(context.Background(), assistantcontrol.ReviewInput{UserRequest: "inspect logs", Action: "exact command", Location: "exact node"})
			if requests != 1 || (fallbacks == 1) != tc.fallback || v.Allowed() != tc.allowed || strings.Contains(v.Notice, "private response") {
				t.Fatal(v, requests, fallbacks)
			}
		})
	}
}

func TestFastConfigurationSupportsUnauthenticatedEndpoints(t *testing.T) {
	if NewFastClassifier("", "key", "model") != nil || NewFastClassifier("http://localhost/v1", "key", " ") != nil {
		t.Fatal("missing configuration accepted")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"A"},"finish_reason":"stop"}]}`))
	}))
	defer server.Close()
	value, err := NewFastClassifier(server.URL+"/v1", "", "fast")(context.Background(), assistantcontrol.ReviewInput{})
	if err != nil || value != "A" {
		t.Fatal(value, err)
	}
}

func TestDeepSeekFastReviewDisablesThinkingAtTopLevel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Thinking  map[string]string `json:"thinking"`
			MaxTokens int               `json:"max_tokens"`
		}
		_ = json.NewDecoder(r.Body).Decode(&request)
		if request.Thinking["type"] != "disabled" || request.MaxTokens != 16 {
			t.Error("DeepSeek thinking was not disabled")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"A"},"finish_reason":"stop"}]}`))
	}))
	defer server.Close()
	value, err := NewFastClassifier(server.URL+"/v1", "fixture", "deepseek-v4-flash")(context.Background(), assistantcontrol.ReviewInput{})
	if err != nil || value != "A" {
		t.Fatal(value, err)
	}
}
