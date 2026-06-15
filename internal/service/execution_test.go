package service

import (
	"testing"
	"time"
)

type runLogWriterStub struct {
	called bool
	run    RunLog
}

func (s *runLogWriterStub) WriteRun(run RunLog) error {
	s.called = true
	s.run = run
	return nil
}

func TestExecutionServiceRecordTaskDraftRunWritesSuccessLog(t *testing.T) {
	logs := &runLogWriterStub{}
	svc := NewExecutionService(logs)

	startedAt := time.Date(2026, 6, 8, 10, 0, 0, 0, time.UTC)
	finishedAt := startedAt.Add(1500 * time.Millisecond)

	if err := svc.RecordTaskDraftRun("draft_1", "manual", startedAt, finishedAt, map[string]interface{}{"ok": true}, nil); err != nil {
		t.Fatalf("RecordTaskDraftRun err=%v", err)
	}
	if !logs.called {
		t.Fatalf("expected run log write")
	}
	if logs.run.SourceType != "task_draft" || logs.run.SourceID != "draft_1" {
		t.Fatalf("unexpected source=%+v", logs.run)
	}
	if logs.run.TargetType != "task_draft" || logs.run.TargetID != "draft_1" {
		t.Fatalf("unexpected target=%+v", logs.run)
	}
	if logs.run.Status != "success" {
		t.Fatalf("expected success status, got=%q", logs.run.Status)
	}
	if logs.run.TriggerSource != "manual" {
		t.Fatalf("expected manual trigger source, got=%q", logs.run.TriggerSource)
	}
	if logs.run.DurationMS != 1500 {
		t.Fatalf("expected duration 1500ms, got=%d", logs.run.DurationMS)
	}
	if logs.run.StartedAt == "" || logs.run.FinishedAt == "" || logs.run.ResultJSON == "" {
		t.Fatalf("expected serialized run log, got=%+v", logs.run)
	}
}
