package service

import (
	"testing"

	"sealsuite-operation/internal/storage"
)

type scheduleRepoStub struct {
	upserted storage.JobSchedule
}

func (s *scheduleRepoStub) Get(id string) (storage.JobSchedule, bool, error) {
	return storage.JobSchedule{}, false, nil
}
func (s *scheduleRepoStub) List() ([]storage.JobSchedule, error) { return nil, nil }
func (s *scheduleRepoStub) Delete(id string) error               { return nil }
func (s *scheduleRepoStub) Upsert(in storage.JobSchedule) error {
	s.upserted = in
	return nil
}

func TestScheduleServiceSaveRequiresID(t *testing.T) {
	svc := NewScheduleService(&scheduleRepoStub{})
	err := svc.Save(storage.JobSchedule{TargetType: "task_draft", TargetID: "draft_1"})
	if err == nil {
		t.Fatalf("expected id validation error")
	}
}

func TestScheduleServiceSaveRequiresTarget(t *testing.T) {
	svc := NewScheduleService(&scheduleRepoStub{})
	err := svc.Save(storage.JobSchedule{ID: "job_1"})
	if err == nil {
		t.Fatalf("expected target validation error")
	}
}

func TestScheduleServiceSavePersistsValidSchedule(t *testing.T) {
	repo := &scheduleRepoStub{}
	svc := NewScheduleService(repo)

	in := storage.JobSchedule{
		ID:         "job_1",
		TargetType: "task_draft",
		TargetID:   "draft_1",
		Enabled:    true,
	}

	if err := svc.Save(in); err != nil {
		t.Fatalf("Save err=%v", err)
	}
	if repo.upserted.ID != in.ID || repo.upserted.TargetType != in.TargetType || repo.upserted.TargetID != in.TargetID {
		t.Fatalf("unexpected upserted schedule=%+v", repo.upserted)
	}
}
