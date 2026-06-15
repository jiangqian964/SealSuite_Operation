package sqlite

import (
	"path/filepath"
	"testing"

	appdb "sealsuite-operation/internal/db"
	"sealsuite-operation/internal/storage"
)

func TestComplexTaskRepositoryUpsertAndGet(t *testing.T) {
	db, err := appdb.OpenSQLite(filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatalf("OpenSQLite err=%v", err)
	}
	defer db.Close()
	if err := appdb.Migrate(db); err != nil {
		t.Fatalf("Migrate err=%v", err)
	}

	repo := NewComplexTaskRepository(db)
	in := storage.ComplexTask{
		ID:             "complex_1",
		Name:           "complex one",
		ExecutionMode:  "workflow",
		WebhookEnabled: true,
		Steps: []storage.ComplexTaskStep{
			{ID: "step_1", Type: "api_call", Name: "load"},
			{ID: "step_2", Type: "output", Name: "render", Config: map[string]interface{}{"format": "markdown"}},
		},
	}
	if err := repo.Upsert(in); err != nil {
		t.Fatalf("Upsert err=%v", err)
	}

	got, ok, err := repo.Get("complex_1")
	if err != nil {
		t.Fatalf("Get err=%v", err)
	}
	if !ok {
		t.Fatalf("expected complex task exists")
	}
	if got.ID != "complex_1" || got.Name != "complex one" {
		t.Fatalf("unexpected task=%+v", got)
	}
	if len(got.Steps) != 2 || got.Steps[1].Type != "output" {
		t.Fatalf("unexpected steps=%+v", got.Steps)
	}
}
