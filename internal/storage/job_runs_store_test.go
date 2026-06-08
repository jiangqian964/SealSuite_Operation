package storage

import (
	"path/filepath"
	"testing"
)

func TestJobRunsStoreSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "job-runs.json")
	s := JobRunsStore{Path: p}

	in := &JobRunsFile{
		Version: 1,
		Items: map[string]JobRunRecord{
			"users-sync": {
				LastRun:    "2026-06-04T19:30:00+08:00",
				OK:         true,
				DurationMs: 321,
				Error:      "",
			},
		},
	}

	if err := s.Save(in); err != nil {
		t.Fatalf("Save err=%v", err)
	}
	got, err := s.Load()
	if err != nil {
		t.Fatalf("Load err=%v", err)
	}
	if got.Items["users-sync"].DurationMs != 321 {
		t.Fatalf("unexpected record: %#v", got.Items["users-sync"])
	}
}

func TestJobRunsStorePutOverwritesLatestRecord(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "job-runs.json")
	s := JobRunsStore{Path: p}

	if err := s.Put("users-sync", JobRunRecord{LastRun: "t1", OK: false, DurationMs: 1, Error: "e1"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Put("users-sync", JobRunRecord{LastRun: "t2", OK: true, DurationMs: 2, Error: ""}); err != nil {
		t.Fatal(err)
	}
	got, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.Items["users-sync"].LastRun != "t2" || !got.Items["users-sync"].OK {
		t.Fatalf("expected latest record overwritten, got=%#v", got.Items["users-sync"])
	}
}

