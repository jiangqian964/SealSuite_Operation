package sqlite

import (
	"path/filepath"
	"testing"

	appdb "sealsuite-operation/internal/db"
	"sealsuite-operation/internal/storage"
)

func TestScheduleRepositoryCRUD(t *testing.T) {
	db, err := appdb.OpenSQLite(filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatalf("OpenSQLite err=%v", err)
	}
	defer db.Close()

	if err := appdb.Migrate(db); err != nil {
		t.Fatalf("Migrate err=%v", err)
	}

	repo := NewScheduleRepository(db)
	in := storage.JobSchedule{
		ID:              "job_1",
		DraftID:         "draft_1",
		Enabled:         true,
		WebhookConfigID: "hook_1",
		WebhookEnabled:  true,
		StartAt:         "2026-06-08T09:00:00+08:00",
		ScheduleType:    "cron",
		Cron:            "0 0 9 * * *",
		Timezone:        "Asia/Shanghai",
		PostFilter: map[string]interface{}{
			"status": "active",
		},
		FieldSelect: []string{"id", "name"},
		TruncateRules: map[string]interface{}{
			"summary": 200,
		},
	}
	if err := repo.Upsert(in); err != nil {
		t.Fatalf("Upsert err=%v", err)
	}

	got, ok, err := repo.Get("job_1")
	if err != nil {
		t.Fatalf("Get err=%v", err)
	}
	if !ok {
		t.Fatalf("expected schedule exists")
	}
	if got.TargetType != "task_draft" || got.TargetID != "draft_1" {
		t.Fatalf("expected target derived from draft_id, got=%+v", got)
	}
	if got.Cron != "0 0 9 * * *" || got.ScheduleType != "cron" {
		t.Fatalf("unexpected scheduling fields=%+v", got)
	}
	if len(got.FieldSelect) != 2 || got.FieldSelect[0] != "id" || got.FieldSelect[1] != "name" {
		t.Fatalf("unexpected field_select=%#v", got.FieldSelect)
	}
	if got.PostFilter["status"] != "active" {
		t.Fatalf("unexpected post_filter=%#v", got.PostFilter)
	}

	items, err := repo.List()
	if err != nil {
		t.Fatalf("List err=%v", err)
	}
	if len(items) != 1 || items[0].ID != "job_1" {
		t.Fatalf("unexpected items=%+v", items)
	}

	if err := repo.Delete("job_1"); err != nil {
		t.Fatalf("Delete err=%v", err)
	}
	_, ok, err = repo.Get("job_1")
	if err != nil {
		t.Fatalf("Get after delete err=%v", err)
	}
	if ok {
		t.Fatalf("expected schedule deleted")
	}
}
