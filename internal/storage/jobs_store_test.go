package storage

import (
	"path/filepath"
	"testing"
)

func TestJobsStoreLoadSaveRoundTrip(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "jobs.yaml")

	s := JobsStore{Path: p}
	jf := &JobsFile{
		Version: 1,
		Jobs: []Job{
			{
				Name:    "daily-users-check",
				Enabled: true,
				Cron:    "0 */1 * * * *",
				Type:    "api_poll",
				Params: map[string]interface{}{
					"template_id": "users_list",
				},
			},
		},
	}

	if err := s.Save(jf); err != nil {
		t.Fatalf("Save err=%v", err)
	}

	got, err := s.Load()
	if err != nil {
		t.Fatalf("Load err=%v", err)
	}
	if got.Version != 1 || len(got.Jobs) != 1 || got.Jobs[0].Name != "daily-users-check" {
		t.Fatalf("unexpected: %#v", got)
	}
}

func TestJobsFileValidateRejectsDuplicateName(t *testing.T) {
	jf := &JobsFile{
		Version: 1,
		Jobs: []Job{
			{Name: "a", Enabled: true, Cron: "0 */1 * * * *", Type: "api_poll"},
			{Name: "a", Enabled: true, Cron: "0 */1 * * * *", Type: "api_poll"},
		},
	}
	if err := jf.Validate(); err == nil {
		t.Fatalf("expected validation error")
	}
}

func TestJobsStoreUpsertGetDelete(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "jobs.yaml")
	s := JobsStore{Path: p}

	if err := s.Upsert(Job{
		Name:    "a",
		Enabled: true,
		Cron:    "0 */1 * * * *",
		Type:    "api_poll",
		Params: map[string]interface{}{
			"template_id": "users_list",
		},
	}); err != nil {
		t.Fatal(err)
	}

	got, ok, err := s.Get("a")
	if err != nil || !ok || got.Name != "a" {
		t.Fatalf("unexpected get: job=%#v ok=%v err=%v", got, ok, err)
	}

	if err := s.Delete("a"); err != nil {
		t.Fatal(err)
	}

	_, ok, err = s.Get("a")
	if err != nil || ok {
		t.Fatalf("expected deleted, ok=%v err=%v", ok, err)
	}
}
