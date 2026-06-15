package db

import (
	"path/filepath"
	"testing"
)

func TestMigrateCreatesCoreTables(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "app.db")
	conn, err := OpenSQLite(dbPath)
	if err != nil {
		t.Fatalf("OpenSQLite err=%v", err)
	}
	defer conn.Close()

	if err := Migrate(conn); err != nil {
		t.Fatalf("Migrate err=%v", err)
	}

	required := []string{
		"app_connections",
		"llm_apis",
		"webhooks",
		"api_templates",
		"output_templates",
		"task_drafts",
		"complex_tasks",
		"complex_task_steps",
		"job_schedules",
		"legacy_jobs",
		"app_meta",
		"job_runs",
		"audit_logs",
	}

	for _, table := range required {
		var name string
		err := conn.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&name)
		if err != nil {
			t.Fatalf("table %s missing err=%v", table, err)
		}
	}
}

func TestOpenSQLiteWithMigrationReady(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "app.db")
	conn, err := OpenSQLite(dbPath)
	if err != nil {
		t.Fatalf("OpenSQLite err=%v", err)
	}
	defer conn.Close()

	if err := Migrate(conn); err != nil {
		t.Fatalf("Migrate err=%v", err)
	}
}
