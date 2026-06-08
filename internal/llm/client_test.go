package llm

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"sealsuite-operation/internal/config"
)

func TestClientChat(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			http.NotFound(w, r)
			return
		}
		if auth := r.Header.Get("Authorization"); auth != "Bearer KEY" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"model":"gpt-test","choices":[{"message":{"content":"总结完成"}}]}`))
	}))
	defer srv.Close()

	c := NewClient(config.LLMRoleConfig{
		Enabled: true,
		BaseURL: srv.URL,
		APIKey:  "KEY",
		Model:   "gpt-test",
		Timeout: 5,
	})
	resp, err := c.Chat(ChatRequest{
		Prompt: "请总结",
		Input:  map[string]interface{}{"count": 3},
	})
	if err != nil {
		t.Fatalf("chat err=%v", err)
	}
	if !strings.Contains(resp.Content, "总结") {
		t.Fatalf("unexpected content: %+v", resp)
	}
}
