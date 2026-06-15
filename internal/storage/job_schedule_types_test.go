package storage

import "testing"

func TestNormalizeJobScheduleDefaultsDerivesTaskDraftTargetAndTimezone(t *testing.T) {
	in := JobSchedule{
		ID:      "sched_1",
		DraftID: "draft_1",
		Enabled: true,
	}

	got := NormalizeJobSchedule(in)

	if got.TargetType != "task_draft" {
		t.Fatalf("expected target_type=task_draft, got=%q", got.TargetType)
	}
	if got.TargetID != "draft_1" {
		t.Fatalf("expected target_id derived from draft_id, got=%q", got.TargetID)
	}
	if got.ScheduleType != "interval" {
		t.Fatalf("expected default schedule_type=interval, got=%q", got.ScheduleType)
	}
	if got.Timezone != "Asia/Shanghai" {
		t.Fatalf("expected default timezone Asia/Shanghai, got=%q", got.Timezone)
	}
}

func TestEnsureJobSchedulesFileDefaultsInitializesItems(t *testing.T) {
	jf := &JobSchedulesFile{}

	EnsureJobSchedulesFileDefaults(jf)

	if jf.Version != 1 {
		t.Fatalf("expected version=1, got=%d", jf.Version)
	}
	if jf.Items == nil {
		t.Fatalf("expected items initialized")
	}
}
