package storage

import (
	"path/filepath"
	"testing"
)

func TestTemplatesStoreLoadSaveRoundTrip(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "api-templates.yaml")

	s := TemplatesStore{Path: p}
	tf := &TemplatesFile{
		Version: 1,
		Templates: []Template{
			{
				ID:       "users_list",
				Name:     "用户-列表",
				Category: "users",
				Method:   "GET",
				Path:     "/api/v1/users",
				QuerySchema: map[string]interface{}{
					"page": map[string]interface{}{"type": "int", "default": 1},
				},
			},
		},
	}

	if err := s.Save(tf); err != nil {
		t.Fatalf("Save err=%v", err)
	}
	got, err := s.Load()
	if err != nil {
		t.Fatalf("Load err=%v", err)
	}
	if got.Version != 1 || len(got.Templates) != 1 || got.Templates[0].ID != "users_list" {
		t.Fatalf("unexpected: %#v", got)
	}
}

func TestTemplatesFileValidateRejectsDuplicateID(t *testing.T) {
	tf := &TemplatesFile{
		Version: 1,
		Templates: []Template{
			{ID: "a", Name: "A", Category: "c", Method: "GET", Path: "/x"},
			{ID: "a", Name: "A2", Category: "c", Method: "GET", Path: "/x"},
		},
	}
	if err := tf.Validate(); err == nil {
		t.Fatalf("expected validation error")
	}
}

func TestTemplatesStoreUpsertGetDelete(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "api-templates.yaml")
	s := TemplatesStore{Path: p}

	if err := s.Upsert(Template{
		ID:       "users_list",
		Name:     "用户-列表",
		Category: "users",
		Method:   "GET",
		Path:     "/api/open/v1/users",
	}); err != nil {
		t.Fatal(err)
	}

	got, ok, err := s.Get("users_list")
	if err != nil || !ok || got.ID != "users_list" {
		t.Fatalf("unexpected get: tpl=%#v ok=%v err=%v", got, ok, err)
	}

	if err := s.Delete("users_list"); err != nil {
		t.Fatal(err)
	}
	_, ok, err = s.Get("users_list")
	if err != nil || ok {
		t.Fatalf("expected deleted, ok=%v err=%v", ok, err)
	}
}
