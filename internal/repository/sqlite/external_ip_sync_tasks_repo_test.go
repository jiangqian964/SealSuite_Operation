package sqlite

import (
	"path/filepath"
	"testing"

	appdb "sealsuite-operation/internal/db"
	"sealsuite-operation/internal/storage"
)

func TestExternalIPSyncTaskRepositoryUpsertAndGet(t *testing.T) {
	db, err := appdb.OpenSQLite(filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatalf("OpenSQLite err=%v", err)
	}
	defer db.Close()

	if err := appdb.Migrate(db); err != nil {
		t.Fatalf("Migrate err=%v", err)
	}

	repo := NewExternalIPSyncTaskRepository(db)
	task := storage.ExternalIPSyncTask{
		ID:               "google_ipv4_sync",
		Name:             "Google IPv4 同步",
		SourceType:       "google_ip_ranges",
		SourceURL:        "https://www.gstatic.com/ipranges/goog.json",
		IPVersion:        "ipv4",
		ResourceID:       "res_google",
		ResourceTagNames: "办公网,研发部",
		WriteAction:      "append_if_missing",
		FeilianAPIPath:   "/api/open/v1/addr/management/add",
		SkipWhenEmpty:    true,
		Enabled:          true,
	}
	if err := repo.Upsert(task); err != nil {
		t.Fatalf("Upsert err=%v", err)
	}

	got, found, err := repo.Get("google_ipv4_sync")
	if err != nil || !found {
		t.Fatalf("Get found=%v err=%v", found, err)
	}
	if got.IPVersion != "ipv4" || got.ResourceID != "res_google" {
		t.Fatalf("unexpected task: %+v", got)
	}
	if got.ResourceTagNames != "办公网,研发部" || !got.SkipWhenEmpty || !got.Enabled {
		t.Fatalf("unexpected flags or tag names: %+v", got)
	}

	items, err := repo.List()
	if err != nil {
		t.Fatalf("List err=%v", err)
	}
	if len(items) != 1 || items[0].ID != "google_ipv4_sync" {
		t.Fatalf("unexpected items=%+v", items)
	}

	task.Name = "Google IPv4 同步已更新"
	task.DryRun = true
	if err := repo.Upsert(task); err != nil {
		t.Fatalf("Upsert update err=%v", err)
	}

	got, found, err = repo.Get("google_ipv4_sync")
	if err != nil || !found {
		t.Fatalf("Get after update found=%v err=%v", found, err)
	}
	if got.Name != "Google IPv4 同步已更新" || !got.DryRun {
		t.Fatalf("unexpected updated task: %+v", got)
	}

	if err := repo.Delete("google_ipv4_sync"); err != nil {
		t.Fatalf("Delete err=%v", err)
	}
	_, found, err = repo.Get("google_ipv4_sync")
	if err != nil {
		t.Fatalf("Get after delete err=%v", err)
	}
	if found {
		t.Fatalf("expected task deleted")
	}
}
