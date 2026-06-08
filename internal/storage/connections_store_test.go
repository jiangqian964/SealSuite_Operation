package storage

import (
	"path/filepath"
	"testing"
)

func TestConnectionsStoreAddAndActivateKeepsLast3(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "connections.yaml")
	s := ConnectionsStore{Path: p}

	for i := 0; i < 4; i++ {
		item := ConnectionItem{
			ID:          "id" + string(rune('0'+i)),
			Name:        "n",
			Scheme:      "https",
			Host:        "h",
			Port:        443,
			AccessKeyID: "AK",
			SecretRef:   "config",
			CreatedAt:   "2026-06-04T00:00:00+08:00",
		}
		if err := s.AddAndActivate(item); err != nil {
			t.Fatalf("AddAndActivate err=%v", err)
		}
	}

	got, err := s.Load()
	if err != nil {
		t.Fatalf("Load err=%v", err)
	}
	if len(got.Items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(got.Items))
	}
	if got.ActiveID == "" {
		t.Fatalf("expected active_id set")
	}
	if got.Items[2].ID != got.ActiveID {
		t.Fatalf("expected active is last item, active=%s last=%s", got.ActiveID, got.Items[2].ID)
	}
}

func TestConnectionsStoreDeleteRemovesInactiveOnly(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "connections.yaml")
	s := ConnectionsStore{Path: p}

	if err := s.Save(&ConnectionsFile{
		Version:  1,
		ActiveID: "conn-active",
		Items: []ConnectionItem{
			{ID: "conn-inactive", Name: "inactive"},
			{ID: "conn-active", Name: "active"},
		},
	}); err != nil {
		t.Fatalf("Save err=%v", err)
	}

	if err := s.Delete("conn-inactive"); err != nil {
		t.Fatalf("Delete inactive err=%v", err)
	}

	got, err := s.Load()
	if err != nil {
		t.Fatalf("Load err=%v", err)
	}
	if len(got.Items) != 1 || got.Items[0].ID != "conn-active" {
		t.Fatalf("expected only active item left, got=%+v", got.Items)
	}
	if got.ActiveID != "conn-active" {
		t.Fatalf("expected active id unchanged, got=%s", got.ActiveID)
	}
}
