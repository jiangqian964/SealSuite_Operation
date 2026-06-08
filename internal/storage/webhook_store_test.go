package storage

import (
	"path/filepath"
	"testing"
)

func TestWebhookStoreUpsertNormalizesMethodAndPreservesCreatedAt(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "webhooks.yaml")
	s := NewWebhookStore(p)

	err := s.Upsert(WebhookItem{
		ID:      "hook-a",
		Name:    "Hook A",
		URL:     " https://example.com/hook ",
		Method:  " post ",
		Headers: map[string]string{"Content-Type": "application/json"},
		Enabled: true,
	})
	if err != nil {
		t.Fatalf("initial upsert err=%v", err)
	}

	first, err := s.Load()
	if err != nil {
		t.Fatalf("load err=%v", err)
	}
	if len(first.Items) != 1 {
		t.Fatalf("expected one item, got=%d", len(first.Items))
	}
	if first.Items[0].CreatedAt == "" {
		t.Fatalf("expected created_at to be set")
	}
	createdAt := first.Items[0].CreatedAt

	err = s.Upsert(WebhookItem{
		ID:      "hook-a",
		Name:    "Hook A Updated",
		URL:     "https://example.com/hook-2",
		Method:  "put",
		Headers: map[string]string{"X-Test": "1"},
		Enabled: false,
	})
	if err != nil {
		t.Fatalf("update upsert err=%v", err)
	}

	got, err := s.Load()
	if err != nil {
		t.Fatalf("load err=%v", err)
	}
	if len(got.Items) != 1 {
		t.Fatalf("expected one item after update, got=%d", len(got.Items))
	}
	item := got.Items[0]
	if item.Method != "PUT" {
		t.Fatalf("expected normalized method PUT, got=%q", item.Method)
	}
	if item.CreatedAt != createdAt {
		t.Fatalf("expected created_at preserved, got=%q want=%q", item.CreatedAt, createdAt)
	}
	if item.URL != "https://example.com/hook-2" {
		t.Fatalf("expected trimmed updated url, got=%q", item.URL)
	}
}

func TestWebhookStoreDeleteMissingAndExisting(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "webhooks.yaml")
	s := NewWebhookStore(p)

	for _, item := range []WebhookItem{
		{ID: "hook-a", Name: "Hook A", URL: "https://example.com/a"},
		{ID: "hook-b", Name: "Hook B", URL: "https://example.com/b"},
	} {
		if err := s.Upsert(item); err != nil {
			t.Fatalf("upsert err=%v", err)
		}
	}

	if err := s.Delete("hook-c"); err == nil {
		t.Fatalf("expected delete missing item to fail")
	}
	if err := s.Delete("hook-a"); err != nil {
		t.Fatalf("delete existing err=%v", err)
	}

	got, err := s.Load()
	if err != nil {
		t.Fatalf("load err=%v", err)
	}
	if len(got.Items) != 1 || got.Items[0].ID != "hook-b" {
		t.Fatalf("expected only hook-b remains, got=%v", got.Items)
	}
}
