package service

import (
	"testing"

	"sealsuite-operation/internal/storage"
)

type draftRepoStub struct {
	upserted storage.TaskDraft
}

func (s *draftRepoStub) Get(id string) (storage.TaskDraft, bool, error) {
	return storage.TaskDraft{}, false, nil
}
func (s *draftRepoStub) List() ([]storage.TaskDraft, error) { return nil, nil }
func (s *draftRepoStub) Delete(id string) error             { return nil }
func (s *draftRepoStub) Upsert(d storage.TaskDraft) error {
	s.upserted = d
	return nil
}

func TestTaskDraftServiceSaveRequiresIDAndName(t *testing.T) {
	svc := NewTaskDraftService(&draftRepoStub{})
	err := svc.Save(storage.TaskDraft{})
	if err == nil {
		t.Fatalf("expected validation error")
	}
}

func TestTaskDraftServiceSavePersistsValidDraft(t *testing.T) {
	repo := &draftRepoStub{}
	svc := NewTaskDraftService(repo)

	in := storage.TaskDraft{
		ID:   "draft_1",
		Name: "draft one",
		Mode: "workflow",
	}

	if err := svc.Save(in); err != nil {
		t.Fatalf("Save err=%v", err)
	}
	if repo.upserted.ID != in.ID || repo.upserted.Name != in.Name || repo.upserted.Mode != in.Mode {
		t.Fatalf("unexpected upserted draft=%+v", repo.upserted)
	}
}
