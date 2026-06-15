package service

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type RunLog struct {
	ID            string
	SourceType    string
	SourceID      string
	TargetType    string
	TargetID      string
	Status        string
	TriggerSource string
	StartedAt     string
	FinishedAt    string
	DurationMS    int64
	ErrorMessage  string
	ResultJSON    string
}

type ExternalIPSyncExecutionSummary struct {
	TaskID       string `json:"task_id,omitempty"`
	ResourceID   string `json:"resource_id,omitempty"`
	IPVersion    string `json:"ip_version,omitempty"`
	WriteAPIPath string `json:"write_api_path,omitempty"`
	SourceTotal  int    `json:"source_total"`
	FilteredTotal int   `json:"filtered_total"`
	ExistingTotal int   `json:"existing_total"`
	ToAddTotal   int    `json:"to_add_total"`
	AddedTotal   int    `json:"added_total"`
	DryRun       bool   `json:"dry_run"`
	Status       string `json:"status,omitempty"`
	ErrorMessage string `json:"error_message,omitempty"`
}

type RunLogWriter interface {
	WriteRun(run RunLog) error
}

type ExecutionService struct {
	runLogs RunLogWriter
	now     func() time.Time
}

func NewExecutionService(runLogs RunLogWriter) *ExecutionService {
	return &ExecutionService{
		runLogs: runLogs,
		now:     time.Now,
	}
}

func (s *ExecutionService) afterRun(ok bool) {
	if s == nil || s.runLogs == nil {
		return
	}
	status := "failed"
	if ok {
		status = "success"
	}
	now := s.now()
	_ = s.runLogs.WriteRun(RunLog{
		ID:         fmt.Sprintf("run_%d", now.UnixNano()),
		Status:     status,
		StartedAt:  now.Format(time.RFC3339),
		FinishedAt: now.Format(time.RFC3339),
		ResultJSON: "null",
	})
}

func (s *ExecutionService) RecordTaskDraftRun(draftID, triggerSource string, startedAt, finishedAt time.Time, result interface{}, runErr error) error {
	return s.RecordRun(RunLog{
		SourceType:    "task_draft",
		SourceID:      strings.TrimSpace(draftID),
		TargetType:    "task_draft",
		TargetID:      strings.TrimSpace(draftID),
		Status:        statusFromError(runErr),
		TriggerSource: firstNonEmptyString(strings.TrimSpace(triggerSource), "manual"),
		StartedAt:     startedAt.Format(time.RFC3339),
		FinishedAt:    finishedAt.Format(time.RFC3339),
		DurationMS:    finishedAt.Sub(startedAt).Milliseconds(),
		ErrorMessage:  errorMessage(runErr),
		ResultJSON:    marshalRunResult(result),
	})
}

func (s *ExecutionService) RecordScheduleRun(scheduleID, targetType, targetID, triggerSource string, startedAt, finishedAt time.Time, result interface{}, runErr error) error {
	return s.RecordRun(RunLog{
		SourceType:    "job_schedule",
		SourceID:      strings.TrimSpace(scheduleID),
		TargetType:    strings.TrimSpace(targetType),
		TargetID:      strings.TrimSpace(targetID),
		Status:        statusFromError(runErr),
		TriggerSource: firstNonEmptyString(strings.TrimSpace(triggerSource), "manual"),
		StartedAt:     startedAt.Format(time.RFC3339),
		FinishedAt:    finishedAt.Format(time.RFC3339),
		DurationMS:    finishedAt.Sub(startedAt).Milliseconds(),
		ErrorMessage:  errorMessage(runErr),
		ResultJSON:    marshalRunResult(result),
	})
}

func (s *ExecutionService) RecordComplexTaskRun(taskID, triggerSource string, startedAt, finishedAt time.Time, result interface{}, runErr error) error {
	return s.RecordRun(RunLog{
		SourceType:    "complex_task",
		SourceID:      strings.TrimSpace(taskID),
		TargetType:    "complex_task",
		TargetID:      strings.TrimSpace(taskID),
		Status:        statusFromError(runErr),
		TriggerSource: firstNonEmptyString(strings.TrimSpace(triggerSource), "manual"),
		StartedAt:     startedAt.Format(time.RFC3339),
		FinishedAt:    finishedAt.Format(time.RFC3339),
		DurationMS:    finishedAt.Sub(startedAt).Milliseconds(),
		ErrorMessage:  errorMessage(runErr),
		ResultJSON:    marshalRunResult(result),
	})
}

func (s *ExecutionService) RecordRun(run RunLog) error {
	if s == nil || s.runLogs == nil {
		return nil
	}

	run.ID = strings.TrimSpace(run.ID)
	if run.ID == "" {
		run.ID = fmt.Sprintf("run_%d", s.now().UnixNano())
	}
	run.SourceType = strings.TrimSpace(run.SourceType)
	run.SourceID = strings.TrimSpace(run.SourceID)
	run.TargetType = strings.TrimSpace(run.TargetType)
	run.TargetID = strings.TrimSpace(run.TargetID)
	run.Status = strings.TrimSpace(run.Status)
	run.TriggerSource = strings.TrimSpace(run.TriggerSource)
	run.ErrorMessage = strings.TrimSpace(run.ErrorMessage)
	run.ResultJSON = strings.TrimSpace(run.ResultJSON)

	if run.Status == "" {
		run.Status = "success"
	}
	if run.StartedAt == "" {
		run.StartedAt = s.now().Format(time.RFC3339)
	}
	if run.FinishedAt == "" {
		run.FinishedAt = run.StartedAt
	}
	if run.ResultJSON == "" {
		run.ResultJSON = "null"
	}
	return s.runLogs.WriteRun(run)
}

func statusFromError(err error) string {
	if err != nil {
		return "failed"
	}
	return "success"
}

func errorMessage(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func marshalRunResult(result interface{}) string {
	if result == nil {
		return "null"
	}
	b, err := json.Marshal(result)
	if err != nil {
		return fmt.Sprintf(`{"marshal_error":%q}`, err.Error())
	}
	return string(b)
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}
