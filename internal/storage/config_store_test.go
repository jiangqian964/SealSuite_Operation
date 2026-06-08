package storage

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfigStoreUpdateSealsuiteConnectionKeepsSecretWhenEmpty(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(p, []byte(`sealsuite:
  secret_key: "OLD"
  access_key: "AK"
`), 0o644); err != nil {
		t.Fatal(err)
	}

	s := ConfigStore{Path: p}
	if err := s.UpdateSealsuiteConnection(map[string]interface{}{
		"host":       "example.com",
		"port":       443,
		"scheme":     "https",
		"access_key": "NEWAK",
		"secret_key": "", // keep old
	}); err != nil {
		t.Fatalf("Update err=%v", err)
	}

	ss, err := s.GetSealsuiteConnection()
	if err != nil {
		t.Fatal(err)
	}
	if ss["secret_key"] != "OLD" {
		t.Fatalf("expected secret_key kept, got=%v", ss["secret_key"])
	}
	if ss["access_key"] != "NEWAK" {
		t.Fatalf("expected access_key updated, got=%v", ss["access_key"])
	}
}

func TestConfigStoreUpdateLLMConfigKeepsAPIKeyWhenEmpty(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(p, []byte(`llm:
  mock_mode: false
  planner:
    api_key: "OLDKEY1"
    model: "deepseek-v3"
  formatter:
    api_key: "OLDKEY2"
    model: "gpt-4o-mini"
`), 0o644); err != nil {
		t.Fatal(err)
	}

	s := ConfigStore{Path: p}
	if err := s.UpdateLLMConfig(map[string]interface{}{
		"planner": map[string]interface{}{
			"base_url": "https://api.deepseek.com",
			"model":    "deepseek-v4-pro",
			"api_key":  "",
		},
		"formatter": map[string]interface{}{
			"base_url": "https://ark.cn-beijing.volces.com/api/v3",
			"model":    "doubao-seed-1-6",
			"api_key":  "",
		},
	}); err != nil {
		t.Fatalf("Update err=%v", err)
	}

	llm, err := s.GetLLMConfig()
	if err != nil {
		t.Fatal(err)
	}
	planner := llm["planner"].(map[string]interface{})
	formatter := llm["formatter"].(map[string]interface{})
	if planner["api_key"] != "OLDKEY1" {
		t.Fatalf("expected planner api_key kept, got=%v", planner["api_key"])
	}
	if formatter["api_key"] != "OLDKEY2" {
		t.Fatalf("expected formatter api_key kept, got=%v", formatter["api_key"])
	}
	if planner["model"] != "deepseek-v4-pro" {
		t.Fatalf("expected planner model updated, got=%v", planner["model"])
	}
	if formatter["model"] != "doubao-seed-1-6" {
		t.Fatalf("expected formatter model updated, got=%v", formatter["model"])
	}
}
