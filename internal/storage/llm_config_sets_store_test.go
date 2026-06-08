package storage

import (
	"path/filepath"
	"testing"
)

func TestLLMConfigSetsStoreUpsertAndMaybeActivateKeepsAPIKeys(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "llm-config-sets.yaml")
	s := NewLLMConfigSetsStore(p)

	err := s.UpsertAndMaybeActivate(LLMConfigSetItem{
		ID:   "set-a",
		Name: "Set A",
		Planner: map[string]interface{}{
			"provider": "deepseek",
			"model":    "deepseek-v3",
			"api_key":  "PLANNER-OLD",
		},
		Formatter: map[string]interface{}{
			"provider": "volcengine_ark",
			"model":    "doubao-seed",
			"api_key":  "FORMATTER-OLD",
		},
	}, false)
	if err != nil {
		t.Fatalf("initial upsert err=%v", err)
	}

	err = s.UpsertAndMaybeActivate(LLMConfigSetItem{
		ID:   "set-a",
		Name: "Set A Updated",
		Planner: map[string]interface{}{
			"model":   "deepseek-v4-pro",
			"api_key": "",
		},
		Formatter: map[string]interface{}{
			"model":   "doubao-seed-1-6",
			"api_key": "",
		},
	}, true)
	if err != nil {
		t.Fatalf("update upsert err=%v", err)
	}

	got, ok, err := s.Get("set-a")
	if err != nil {
		t.Fatalf("get err=%v", err)
	}
	if !ok {
		t.Fatalf("expected config set to exist")
	}

	if got.CreatedAt == "" {
		t.Fatalf("expected created_at to be set")
	}
	if got.Name != "Set A Updated" {
		t.Fatalf("expected updated name, got=%q", got.Name)
	}
	if got.Planner["api_key"] != "PLANNER-OLD" {
		t.Fatalf("expected planner api_key kept, got=%v", got.Planner["api_key"])
	}
	if got.Formatter["api_key"] != "FORMATTER-OLD" {
		t.Fatalf("expected formatter api_key kept, got=%v", got.Formatter["api_key"])
	}
	if got.Planner["model"] != "deepseek-v4-pro" {
		t.Fatalf("expected planner model updated, got=%v", got.Planner["model"])
	}
	if got.Formatter["model"] != "doubao-seed-1-6" {
		t.Fatalf("expected formatter model updated, got=%v", got.Formatter["model"])
	}

	file, err := s.Load()
	if err != nil {
		t.Fatalf("load err=%v", err)
	}
	if file.ActiveID != "set-a" {
		t.Fatalf("expected active_id set, got=%q", file.ActiveID)
	}
}

func TestLLMConfigSetsStoreDeleteRejectsActive(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "llm-config-sets.yaml")
	s := NewLLMConfigSetsStore(p)

	for _, item := range []LLMConfigSetItem{
		{ID: "set-a", Name: "Set A", Planner: map[string]interface{}{}, Formatter: map[string]interface{}{}},
		{ID: "set-b", Name: "Set B", Planner: map[string]interface{}{}, Formatter: map[string]interface{}{}},
	} {
		if err := s.UpsertAndMaybeActivate(item, false); err != nil {
			t.Fatalf("upsert err=%v", err)
		}
	}
	if _, err := s.Activate("set-a"); err != nil {
		t.Fatalf("activate err=%v", err)
	}

	if err := s.Delete("set-a"); err == nil {
		t.Fatalf("expected deleting active item to fail")
	}
	if err := s.Delete("set-b"); err != nil {
		t.Fatalf("delete inactive err=%v", err)
	}

	file, err := s.Load()
	if err != nil {
		t.Fatalf("load err=%v", err)
	}
	if len(file.Items) != 1 || file.Items[0].ID != "set-a" {
		t.Fatalf("expected only active item remains, got=%v", file.Items)
	}
}
