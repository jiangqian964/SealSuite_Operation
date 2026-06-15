package bootstrap

import (
	"path/filepath"
	"testing"

	"sealsuite-operation/internal/config"
)

func TestBuildInitializesSQLiteServicesAndClient(t *testing.T) {
	cfg := &config.Config{
		Database: config.DatabaseConfig{
			Path: filepath.Join(t.TempDir(), "app.db"),
		},
		SealSuite: config.SealSuiteConfig{
			BaseURL:  "http://example.com",
			Timeout:  1,
			MockMode: true,
		},
	}

	app, err := Build(cfg)
	if err != nil {
		t.Fatalf("Build err=%v", err)
	}
	defer app.DB.Close()

	if app.DB == nil {
		t.Fatalf("expected DB initialized")
	}
	if app.Services == nil || app.Services.TaskDrafts == nil || app.Services.Schedules == nil {
		t.Fatalf("expected services initialized")
	}
	if app.Client == nil {
		t.Fatalf("expected client initialized")
	}

	var name string
	if err := app.DB.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, "task_drafts").Scan(&name); err != nil {
		t.Fatalf("expected migrated task_drafts table, err=%v", err)
	}
}
