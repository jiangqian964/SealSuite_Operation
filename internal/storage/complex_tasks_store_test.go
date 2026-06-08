package storage

import (
	"path/filepath"
	"testing"
)

func TestComplexTasksStoreUpsertGetDelete(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "complex-tasks.yaml")
	s := ComplexTasksStore{Path: p}

	in := ComplexTask{
		ID:            "weekly-security-summary",
		Name:          "每周安全汇总",
		Goal:          "汇总安全异常并生成中文总结",
		ExecutionMode: "manual",
		Steps: []ComplexTaskStep{
			{ID: "s1", Type: "api_call", Name: "查询设备"},
			{ID: "s2", Type: "llm_inference", Name: "生成总结"},
		},
	}
	if err := s.Upsert(in); err != nil {
		t.Fatalf("Upsert err=%v", err)
	}

	got, ok, err := s.Get("weekly-security-summary")
	if err != nil || !ok || len(got.Steps) != 2 {
		t.Fatalf("unexpected get: got=%#v ok=%v err=%v", got, ok, err)
	}

	if err := s.Delete("weekly-security-summary"); err != nil {
		t.Fatalf("Delete err=%v", err)
	}
	_, ok, err = s.Get("weekly-security-summary")
	if err != nil || ok {
		t.Fatalf("expected deleted, ok=%v err=%v", ok, err)
	}
}

