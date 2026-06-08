package storage

import (
	"path/filepath"
	"testing"
)

func TestTaskDraftsStoreUpsertGetDelete(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "task-drafts.yaml")
	s := TaskDraftsStore{Path: p}

	in := TaskDraft{
		ID:               "draft_users_sync",
		Name:             "用户同步草稿",
		Mode:             "api_only",
		SourceTemplateID: "users_list",
		InputConfig: map[string]interface{}{
			"query": map[string]interface{}{"page": "1"},
		},
	}
	if err := s.Upsert(in); err != nil {
		t.Fatalf("Upsert err=%v", err)
	}

	got, ok, err := s.Get("draft_users_sync")
	if err != nil || !ok || got.ID != "draft_users_sync" {
		t.Fatalf("unexpected get: got=%#v ok=%v err=%v", got, ok, err)
	}

	if err := s.Delete("draft_users_sync"); err != nil {
		t.Fatalf("Delete err=%v", err)
	}
	_, ok, err = s.Get("draft_users_sync")
	if err != nil || ok {
		t.Fatalf("expected deleted, ok=%v err=%v", ok, err)
	}
}

