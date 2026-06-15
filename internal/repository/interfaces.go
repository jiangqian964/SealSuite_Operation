package repository

import "sealsuite-operation/internal/storage"

type TaskDraftRepository interface {
	Get(id string) (storage.TaskDraft, bool, error)
	List() ([]storage.TaskDraft, error)
	Upsert(d storage.TaskDraft) error
	Delete(id string) error
}

type LegacyJobRepository interface {
	Load() (*storage.JobsFile, error)
	Save(jf *storage.JobsFile) error
	Get(name string) (*storage.Job, bool, error)
	Upsert(job storage.Job) error
	Delete(name string) error
	ReferencedByTemplate(templateID string) ([]string, error)
}

type ScheduleRepository interface {
	Get(id string) (storage.JobSchedule, bool, error)
	List() ([]storage.JobSchedule, error)
	Upsert(s storage.JobSchedule) error
	Delete(id string) error
}

type ComplexTaskRepository interface {
	Get(id string) (storage.ComplexTask, bool, error)
	List() ([]storage.ComplexTask, error)
	Upsert(task storage.ComplexTask) error
	Delete(id string) error
}

type ExternalIPSyncTaskRepository interface {
	List() ([]storage.ExternalIPSyncTask, error)
	Get(id string) (storage.ExternalIPSyncTask, bool, error)
	Upsert(task storage.ExternalIPSyncTask) error
	Delete(id string) error
}
