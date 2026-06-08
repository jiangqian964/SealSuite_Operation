package storage

import (
	"path/filepath"
	"testing"
)

func TestJobSchedulesStoreUpsertGetDelete(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "job-schedules.yaml")
	s := JobSchedulesStore{Path: p}

	in := JobSchedule{
		ID:           "sched_users_daily",
		TargetType:   "task_draft",
		TargetID:     "draft_users_sync",
		Enabled:      true,
		ScheduleType: "cron",
		Cron:         "0 0 9 * * *",
		StartAt:      "2026-06-05T09:00:00+08:00",
		EndAt:        "",
		Timezone:     "Asia/Shanghai",
	}
	if err := s.Upsert(in); err != nil {
		t.Fatalf("Upsert err=%v", err)
	}

	got, ok, err := s.Get("sched_users_daily")
	if err != nil || !ok || got.TargetID != "draft_users_sync" || got.TargetType != "task_draft" {
		t.Fatalf("unexpected get: got=%#v ok=%v err=%v", got, ok, err)
	}

	if err := s.Delete("sched_users_daily"); err != nil {
		t.Fatalf("Delete err=%v", err)
	}
	_, ok, err = s.Get("sched_users_daily")
	if err != nil || ok {
		t.Fatalf("expected deleted, ok=%v err=%v", ok, err)
	}
}
