package service

import (
	"sealsuite-operation/internal/repository"
	"sealsuite-operation/internal/storage"
)

type ExternalIPSyncService struct {
	repo repository.ExternalIPSyncTaskRepository
}

func NewExternalIPSyncService(repo repository.ExternalIPSyncTaskRepository) *ExternalIPSyncService {
	return &ExternalIPSyncService{repo: repo}
}

func (s *ExternalIPSyncService) Save(task storage.ExternalIPSyncTask) error {
	task = storage.NormalizeExternalIPSyncTask(task)
	if err := task.Validate(); err != nil {
		return err
	}
	return s.repo.Upsert(task)
}
