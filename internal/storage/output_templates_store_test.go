package storage

import (
	"path/filepath"
	"testing"
)

func TestOutputTemplatesStoreCRUD(t *testing.T) {
	p := filepath.Join(t.TempDir(), "output-templates.yaml")
	s := OutputTemplatesStore{Path: p}

	if err := s.Upsert(OutputTemplate{
		ID:     "out_users_summary",
		Name:   "用户汇总输出",
		Format: "markdown",
		Title:  "用户汇总",
		Content: "# 用户汇总",
	}); err != nil {
		t.Fatalf("upsert err=%v", err)
	}

	got, ok, err := s.Get("out_users_summary")
	if err != nil {
		t.Fatalf("get err=%v", err)
	}
	if !ok || got.Name != "用户汇总输出" {
		t.Fatalf("unexpected get result: ok=%v got=%+v", ok, got)
	}

	if err := s.Delete("out_users_summary"); err != nil {
		t.Fatalf("delete err=%v", err)
	}
	_, ok, err = s.Get("out_users_summary")
	if err != nil {
		t.Fatalf("get after delete err=%v", err)
	}
	if ok {
		t.Fatalf("expected deleted item not found")
	}
}
