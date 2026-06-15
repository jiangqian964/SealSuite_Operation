package service

import (
	"errors"
	"strings"

	"sealsuite-operation/internal/repository"
	"sealsuite-operation/internal/storage"
)

type ComplexTaskService struct {
	repo repository.ComplexTaskRepository
}

func NewComplexTaskService(repo repository.ComplexTaskRepository) *ComplexTaskService {
	return &ComplexTaskService{repo: repo}
}

func (s *ComplexTaskService) Save(task storage.ComplexTask) error {
	task.ID = strings.TrimSpace(task.ID)
	task.Name = strings.TrimSpace(task.Name)
	task.ExecutionMode = strings.TrimSpace(task.ExecutionMode)
	if task.ID == "" || task.Name == "" {
		return errors.New("id/name required")
	}
	if task.ExecutionMode == "" {
		task.ExecutionMode = "workflow"
	}
	if task.Steps == nil {
		task.Steps = []storage.ComplexTaskStep{}
	}
	return s.repo.Upsert(task)
}

func (s *ComplexTaskService) Delete(id string) error {
	if strings.TrimSpace(id) == "" {
		return errors.New("id required")
	}
	return s.repo.Delete(id)
}
