package service

import "sealsuite-operation/internal/repository"

type Services struct {
	TaskDrafts     *TaskDraftService
	Schedules      *ScheduleService
	ComplexTasks   *ComplexTaskService
	ExternalIPSync *ExternalIPSyncService
}

func NewServices(
	taskDraftRepo repository.TaskDraftRepository,
	scheduleRepo repository.ScheduleRepository,
	complexTaskRepo repository.ComplexTaskRepository,
	externalIPSyncRepo repository.ExternalIPSyncTaskRepository,
) *Services {
	return &Services{
		TaskDrafts:     NewTaskDraftService(taskDraftRepo),
		Schedules:      NewScheduleService(scheduleRepo),
		ComplexTasks:   NewComplexTaskService(complexTaskRepo),
		ExternalIPSync: NewExternalIPSyncService(externalIPSyncRepo),
	}
}
