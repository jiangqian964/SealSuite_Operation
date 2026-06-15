package service

import (
	"testing"

	"sealsuite-operation/internal/storage"
)

type externalIPSyncRepoStub struct {
	upserted storage.ExternalIPSyncTask
}

func (s *externalIPSyncRepoStub) List() ([]storage.ExternalIPSyncTask, error) { return nil, nil }

func (s *externalIPSyncRepoStub) Get(id string) (storage.ExternalIPSyncTask, bool, error) {
	return storage.ExternalIPSyncTask{}, false, nil
}

func (s *externalIPSyncRepoStub) Upsert(task storage.ExternalIPSyncTask) error {
	s.upserted = task
	return nil
}

func (s *externalIPSyncRepoStub) Delete(id string) error { return nil }

func TestExternalIPSyncServiceSaveRequiresRequiredFields(t *testing.T) {
	svc := NewExternalIPSyncService(&externalIPSyncRepoStub{})
	err := svc.Save(storage.ExternalIPSyncTask{})
	if err == nil {
		t.Fatalf("expected validation error")
	}
}

func TestExternalIPSyncServiceSaveNormalizesDefaults(t *testing.T) {
	repo := &externalIPSyncRepoStub{}
	svc := NewExternalIPSyncService(repo)

	err := svc.Save(storage.ExternalIPSyncTask{
		ID:         "google_ipv6_sync",
		Name:       "Google IPv6 同步",
		IPVersion:  "ipv6",
		ResourceID: "res_v6",
	})
	if err != nil {
		t.Fatalf("Save err=%v", err)
	}
	if repo.upserted.SourceType != storage.DefaultExternalIPSyncSourceType {
		t.Fatalf("unexpected source type: %+v", repo.upserted)
	}
	if repo.upserted.SourceURL != storage.DefaultExternalIPSyncSourceURL {
		t.Fatalf("unexpected source url: %+v", repo.upserted)
	}
	if repo.upserted.WriteAction != storage.DefaultExternalIPSyncWriteAction {
		t.Fatalf("unexpected write action: %+v", repo.upserted)
	}
	if repo.upserted.FeilianAPIPath != storage.DefaultExternalIPSyncFeilianAPIPath {
		t.Fatalf("unexpected api path: %+v", repo.upserted)
	}
}
