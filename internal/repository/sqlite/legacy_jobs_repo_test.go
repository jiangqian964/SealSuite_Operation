package sqlite

import (
	"path/filepath"
	"testing"

	appdb "sealsuite-operation/internal/db"
	"sealsuite-operation/internal/storage"
)

func TestLegacyJobRepositoryCRUD(t *testing.T) {
	db, err := appdb.OpenSQLite(filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatalf("OpenSQLite err=%v", err)
	}
	defer db.Close()

	if err := appdb.Migrate(db); err != nil {
		t.Fatalf("Migrate err=%v", err)
	}

	repo := NewLegacyJobRepository(db)
	job := storage.Job{
		Name:    "legacy_job_1",
		Enabled: true,
		Cron:    "0 */5 * * * *",
		Type:    "api_poll",
		Params: map[string]interface{}{
			"template_id": "users_list",
		},
	}
	if err := repo.Upsert(job); err != nil {
		t.Fatalf("Upsert err=%v", err)
	}

	got, ok, err := repo.Get("legacy_job_1")
	if err != nil {
		t.Fatalf("Get err=%v", err)
	}
	if !ok {
		t.Fatalf("expected legacy job exists")
	}
	if got.Name != job.Name || got.Cron != job.Cron || got.Type != job.Type {
		t.Fatalf("unexpected job=%+v", got)
	}

	file, err := repo.Load()
	if err != nil {
		t.Fatalf("Load err=%v", err)
	}
	if len(file.Jobs) != 1 || file.Jobs[0].Name != "legacy_job_1" {
		t.Fatalf("unexpected jobs file=%+v", file)
	}

	refs, err := repo.ReferencedByTemplate("users_list")
	if err != nil {
		t.Fatalf("ReferencedByTemplate err=%v", err)
	}
	if len(refs) != 1 || refs[0] != "legacy_job_1" {
		t.Fatalf("unexpected refs=%v", refs)
	}

	file.Jobs[0].Enabled = false
	if err := repo.Save(file); err != nil {
		t.Fatalf("Save err=%v", err)
	}
	got, ok, err = repo.Get("legacy_job_1")
	if err != nil {
		t.Fatalf("Get after save err=%v", err)
	}
	if !ok || got.Enabled {
		t.Fatalf("expected saved disabled job, got=%+v ok=%v", got, ok)
	}

	if err := repo.Delete("legacy_job_1"); err != nil {
		t.Fatalf("Delete err=%v", err)
	}
	_, ok, err = repo.Get("legacy_job_1")
	if err != nil {
		t.Fatalf("Get after delete err=%v", err)
	}
	if ok {
		t.Fatalf("expected legacy job deleted")
	}
}
