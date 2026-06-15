package service

import (
	"errors"
	"strings"

	"sealsuite-operation/internal/repository"
	"sealsuite-operation/internal/storage"
)

type ScheduleService struct {
	repo repository.ScheduleRepository
}

func NewScheduleService(repo repository.ScheduleRepository) *ScheduleService {
	return &ScheduleService{repo: repo}
}

func (s *ScheduleService) Save(in storage.JobSchedule) error {
	in.ID = strings.TrimSpace(in.ID)
	in.TargetType = strings.TrimSpace(in.TargetType)
	in.TargetID = strings.TrimSpace(in.TargetID)
	in.DraftID = strings.TrimSpace(in.DraftID)
	in = storage.NormalizeJobSchedule(in)

	if in.ID == "" {
		return errors.New("id required")
	}
	if in.TargetType == "" || in.TargetID == "" {
		return errors.New("target_type/target_id required")
	}
	return s.repo.Upsert(in)
}
