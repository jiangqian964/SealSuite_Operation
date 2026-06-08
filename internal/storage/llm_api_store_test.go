package storage

import (
	"path/filepath"
	"testing"
)

func TestLLMAPIStoreUpsertAndMaybeActivateKeepsAPIKeyAndNormalizesTags(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "llm-apis.yaml")
	s := NewLLMAPIStore(p)

	err := s.UpsertAndMaybeActivate(LLMAPIItem{
		ID:       "api-a",
		Name:     "API A",
		Tags:     []string{" planner ", "formatter", "planner"},
		Provider: "deepseek",
		Model:    "deepseek-v3",
		APIKey:   "KEY-OLD",
	}, false)
	if err != nil {
		t.Fatalf("initial upsert err=%v", err)
	}

	err = s.UpsertAndMaybeActivate(LLMAPIItem{
		ID:       "api-a",
		Name:     "API A Updated",
		Tags:     []string{" default ", "", "planner"},
		Provider: "deepseek",
		Model:    "deepseek-v4-pro",
		APIKey:   "",
	}, true)
	if err != nil {
		t.Fatalf("update upsert err=%v", err)
	}

	file, err := s.Load()
	if err != nil {
		t.Fatalf("load err=%v", err)
	}
	if file.ActiveID != "api-a" {
		t.Fatalf("expected active_id set, got=%q", file.ActiveID)
	}
	if len(file.Items) != 1 {
		t.Fatalf("expected one item, got=%d", len(file.Items))
	}
	got := file.Items[0]
	if got.CreatedAt == "" {
		t.Fatalf("expected created_at to be set")
	}
	if got.Name != "API A Updated" {
		t.Fatalf("expected updated name, got=%q", got.Name)
	}
	if got.APIKey != "KEY-OLD" {
		t.Fatalf("expected api_key kept, got=%q", got.APIKey)
	}
	if got.Model != "deepseek-v4-pro" {
		t.Fatalf("expected model updated, got=%q", got.Model)
	}
	if len(got.Tags) != 2 || got.Tags[0] != "default" || got.Tags[1] != "planner" {
		t.Fatalf("expected normalized tags, got=%v", got.Tags)
	}
}

func TestLLMAPIStoreDeleteRejectsActive(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "llm-apis.yaml")
	s := NewLLMAPIStore(p)

	for _, item := range []LLMAPIItem{
		{ID: "api-a", Name: "API A"},
		{ID: "api-b", Name: "API B"},
	} {
		if err := s.UpsertAndMaybeActivate(item, false); err != nil {
			t.Fatalf("upsert err=%v", err)
		}
	}
	if _, err := s.Activate("api-a"); err != nil {
		t.Fatalf("activate err=%v", err)
	}

	if err := s.Delete("api-a"); err == nil {
		t.Fatalf("expected deleting active item to fail")
	}
	if err := s.Delete("api-b"); err != nil {
		t.Fatalf("delete inactive err=%v", err)
	}

	file, err := s.Load()
	if err != nil {
		t.Fatalf("load err=%v", err)
	}
	if len(file.Items) != 1 || file.Items[0].ID != "api-a" {
		t.Fatalf("expected only active item remains, got=%v", file.Items)
	}
}
