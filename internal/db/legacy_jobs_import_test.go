package db

import (
	"os"
	"path/filepath"
	"testing"

	sqliteRepo "sealsuite-operation/internal/repository/sqlite"
	"sealsuite-operation/internal/storage"
)

func TestBootstrapLegacyJobsFromYAMLImportsOnce(t *testing.T) {
	dir := t.TempDir()
	jobsPath := filepath.Join(dir, "jobs.yaml")
	if err := os.WriteFile(jobsPath, []byte(`version: 1
jobs:
  - name: yaml-job
    enabled: true
    cron: "0 */5 * * * *"
    type: api_poll
    params:
      template_id: users_list
`), 0o644); err != nil {
		t.Fatalf("write jobs yaml err=%v", err)
	}

	dbPath := filepath.Join(dir, "app.db")
	conn, err := OpenSQLite(dbPath)
	if err != nil {
		t.Fatalf("OpenSQLite err=%v", err)
	}
	defer conn.Close()
	if err := Migrate(conn); err != nil {
		t.Fatalf("Migrate err=%v", err)
	}

	if err := BootstrapLegacyJobsFromYAML(conn, jobsPath); err != nil {
		t.Fatalf("BootstrapLegacyJobsFromYAML err=%v", err)
	}

	repo := sqliteRepo.NewLegacyJobRepository(conn)
	file, err := repo.Load()
	if err != nil {
		t.Fatalf("Load err=%v", err)
	}
	if len(file.Jobs) != 1 || file.Jobs[0].Name != "yaml-job" {
		t.Fatalf("expected imported yaml job, got=%+v", file.Jobs)
	}

	if err := os.WriteFile(jobsPath, []byte(`version: 1
jobs:
  - name: yaml-job-rewritten
    enabled: true
    cron: "0 */1 * * * *"
    type: api_poll
    params:
      template_id: users_list
`), 0o644); err != nil {
		t.Fatalf("rewrite jobs yaml err=%v", err)
	}

	if err := BootstrapLegacyJobsFromYAML(conn, jobsPath); err != nil {
		t.Fatalf("BootstrapLegacyJobsFromYAML second call err=%v", err)
	}

	file, err = repo.Load()
	if err != nil {
		t.Fatalf("Load after second bootstrap err=%v", err)
	}
	if len(file.Jobs) != 1 || file.Jobs[0].Name != "yaml-job" {
		t.Fatalf("expected one-time import to preserve sqlite data, got=%+v", file.Jobs)
	}
}

func TestBootstrapLegacyJobsFromYAMLSkipsWhenSQLiteAlreadyHasJobs(t *testing.T) {
	dir := t.TempDir()
	jobsPath := filepath.Join(dir, "jobs.yaml")
	if err := os.WriteFile(jobsPath, []byte(`version: 1
jobs:
  - name: yaml-job
    enabled: true
    cron: "0 */5 * * * *"
    type: api_poll
    params:
      template_id: users_list
`), 0o644); err != nil {
		t.Fatalf("write jobs yaml err=%v", err)
	}

	dbPath := filepath.Join(dir, "app.db")
	conn, err := OpenSQLite(dbPath)
	if err != nil {
		t.Fatalf("OpenSQLite err=%v", err)
	}
	defer conn.Close()
	if err := Migrate(conn); err != nil {
		t.Fatalf("Migrate err=%v", err)
	}

	repo := sqliteRepo.NewLegacyJobRepository(conn)
	if err := repo.Upsert(storage.Job{
		Name:    "sqlite-job",
		Enabled: true,
		Cron:    "0 */10 * * * *",
		Type:    "api_poll",
		Params: map[string]interface{}{
			"template_id": "devices_list",
		},
	}); err != nil {
		t.Fatalf("Upsert err=%v", err)
	}

	if err := BootstrapLegacyJobsFromYAML(conn, jobsPath); err != nil {
		t.Fatalf("BootstrapLegacyJobsFromYAML err=%v", err)
	}

	file, err := repo.Load()
	if err != nil {
		t.Fatalf("Load err=%v", err)
	}
	if len(file.Jobs) != 1 || file.Jobs[0].Name != "sqlite-job" {
		t.Fatalf("expected existing sqlite jobs preserved, got=%+v", file.Jobs)
	}
}
