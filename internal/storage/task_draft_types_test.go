package storage

import "testing"

func TestNormalizeTaskDraftDefaultsFallsBackToOnce(t *testing.T) {
	in := TaskDraft{
		ID:        "draft_1",
		Name:      "draft one",
		CycleMode: "invalid",
		RunCount:  0,
		RunUntil:  " 2026-06-08T09:00:00+08:00 ",
	}

	got := NormalizeTaskDraft(in)

	if got.CycleMode != "once" {
		t.Fatalf("expected cycle_mode=once, got=%q", got.CycleMode)
	}
	if got.RunCount != 1 {
		t.Fatalf("expected run_count=1, got=%d", got.RunCount)
	}
	if got.RunUntil != "" {
		t.Fatalf("expected run_until cleared for once mode, got=%q", got.RunUntil)
	}
}

func TestEnsureTaskDraftsFileDefaultsInitializesItems(t *testing.T) {
	tf := &TaskDraftsFile{}

	EnsureTaskDraftsFileDefaults(tf)

	if tf.Version != 1 {
		t.Fatalf("expected version=1, got=%d", tf.Version)
	}
	if tf.Items == nil {
		t.Fatalf("expected items initialized")
	}
}
