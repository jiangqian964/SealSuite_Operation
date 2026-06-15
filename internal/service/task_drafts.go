package service

import (
	"errors"
	"strings"

	"sealsuite-operation/internal/repository"
	"sealsuite-operation/internal/storage"
)

type TaskDraftService struct {
	repo repository.TaskDraftRepository
}

func NewTaskDraftService(repo repository.TaskDraftRepository) *TaskDraftService {
	return &TaskDraftService{repo: repo}
}

func (s *TaskDraftService) Save(d storage.TaskDraft) error {
	d.ID = strings.TrimSpace(d.ID)
	d.Name = strings.TrimSpace(d.Name)
	if d.ID == "" || d.Name == "" {
		return errors.New("id/name required")
	}
	d = storage.NormalizeTaskDraft(d)
	return s.repo.Upsert(d)
}
