package sqlite

import (
	"path/filepath"
	"testing"

	appdb "sealsuite-operation/internal/db"
	"sealsuite-operation/internal/storage"
)

func TestTaskDraftRepositoryCRUD(t *testing.T) {
	db, err := appdb.OpenSQLite(filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatalf("OpenSQLite err=%v", err)
	}
	defer db.Close()

	if err := appdb.Migrate(db); err != nil {
		t.Fatalf("Migrate err=%v", err)
	}

	repo := NewTaskDraftRepository(db)
	in := storage.TaskDraft{
		ID:               "draft_1",
		Name:             "draft one",
		Mode:             "workflow",
		SourceTemplateID: "tpl_users",
		InputConfig: map[string]interface{}{
			"query": map[string]interface{}{"page": "1"},
		},
		TransformConfig: map[string]interface{}{
			"steps": []interface{}{"flatten"},
		},
		LLMConfig: map[string]interface{}{
			"planner": map[string]interface{}{"model": "gpt-4.1"},
		},
		OutputConfig: map[string]interface{}{
			"format": "markdown",
		},
	}
	if err := repo.Upsert(in); err != nil {
		t.Fatalf("Upsert err=%v", err)
	}

	got, ok, err := repo.Get("draft_1")
	if err != nil {
		t.Fatalf("Get err=%v", err)
	}
	if !ok {
		t.Fatalf("expected task draft exists")
	}
	if got.ID != "draft_1" || got.Name != "draft one" || got.Mode != "workflow" {
		t.Fatalf("unexpected draft=%+v", got)
	}
	if got.CycleMode != "once" {
		t.Fatalf("expected default cycle_mode=once, got=%q", got.CycleMode)
	}
	if got.RunCount != 1 {
		t.Fatalf("expected default run_count=1, got=%d", got.RunCount)
	}
	query, ok := got.InputConfig["query"].(map[string]interface{})
	if !ok || query["page"] != "1" {
		t.Fatalf("unexpected input_config=%#v", got.InputConfig)
	}
	planner, ok := got.LLMConfig["planner"].(map[string]interface{})
	if !ok || planner["model"] != "gpt-4.1" {
		t.Fatalf("unexpected llm_config=%#v", got.LLMConfig)
	}

	items, err := repo.List()
	if err != nil {
		t.Fatalf("List err=%v", err)
	}
	if len(items) != 1 || items[0].ID != "draft_1" {
		t.Fatalf("unexpected items=%+v", items)
	}

	if err := repo.Delete("draft_1"); err != nil {
		t.Fatalf("Delete err=%v", err)
	}
	_, ok, err = repo.Get("draft_1")
	if err != nil {
		t.Fatalf("Get after delete err=%v", err)
	}
	if ok {
		t.Fatalf("expected task draft deleted")
	}
}
