package sqlite

import (
	"path/filepath"
	"testing"

	appdb "sealsuite-operation/internal/db"
	"sealsuite-operation/internal/storage"
)

func TestApprovalConfigRepository_SaveAndGet(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "app.db")
	conn, err := appdb.OpenSQLite(dbPath)
	if err != nil {
		t.Fatalf("OpenSQLite err=%v", err)
	}
	defer conn.Close()
	if err := appdb.Migrate(conn); err != nil {
		t.Fatalf("Migrate err=%v", err)
	}

	repo := NewApprovalConfigRepository(conn)

	// 默认配置：未启用、空分组
	cfg, err := repo.Get()
	if err != nil {
		t.Fatalf("Get default err=%v", err)
	}
	if cfg.Enabled {
		t.Fatalf("expected default enabled=false")
	}
	if len(cfg.GroupIDs) != 0 {
		t.Fatalf("expected empty group ids")
	}

	// 保存配置
	want := storage.ApprovalConfig{
		GroupIDs:  []string{"g1", "g2", "g3"},
		WebhookID: "wh_001",
		Enabled:   true,
	}
	if err := repo.Save(want); err != nil {
		t.Fatalf("Save err=%v", err)
	}

	// 读取并校验
	got, err := repo.Get()
	if err != nil {
		t.Fatalf("Get after save err=%v", err)
	}
	if got.Enabled != want.Enabled {
		t.Errorf("enabled mismatch: got=%v want=%v", got.Enabled, want.Enabled)
	}
	if got.WebhookID != want.WebhookID {
		t.Errorf("webhook_id mismatch: got=%q want=%q", got.WebhookID, want.WebhookID)
	}
	if len(got.GroupIDs) != len(want.GroupIDs) {
		t.Fatalf("group_ids length mismatch: got=%d want=%d", len(got.GroupIDs), len(want.GroupIDs))
	}
	for i := range want.GroupIDs {
		if got.GroupIDs[i] != want.GroupIDs[i] {
			t.Errorf("group_ids[%d] mismatch: got=%q want=%q", i, got.GroupIDs[i], want.GroupIDs[i])
		}
	}
}

func TestApprovalTaskRepository_CRUD(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "app.db")
	conn, err := appdb.OpenSQLite(dbPath)
	if err != nil {
		t.Fatalf("OpenSQLite err=%v", err)
	}
	defer conn.Close()
	if err := appdb.Migrate(conn); err != nil {
		t.Fatalf("Migrate err=%v", err)
	}

	repo := NewApprovalTaskRepository(conn)

	task := storage.ApprovalTask{
		ID:               "task_001",
		EventID:          "evt_001",
		DeviceIdentifier: "SN-ABC123",
		RawPayload:       `{"event":"apply"}`,
		Status:           storage.ApprovalStatusPending,
	}
	if err := repo.Create(task); err != nil {
		t.Fatalf("Create err=%v", err)
	}

	// 按 ID 查询
	got, ok, err := repo.Get("task_001")
	if err != nil {
		t.Fatalf("Get err=%v", err)
	}
	if !ok {
		t.Fatalf("task not found")
	}
	if got.DeviceIdentifier != "SN-ABC123" {
		t.Errorf("device_identifier mismatch: got=%q", got.DeviceIdentifier)
	}

	// 按 EventID 查询（幂等）
	byEvent, ok, err := repo.GetByEventID("evt_001")
	if err != nil {
		t.Fatalf("GetByEventID err=%v", err)
	}
	if !ok {
		t.Fatalf("task not found by event_id")
	}
	if byEvent.ID != "task_001" {
		t.Errorf("GetByEventID returned wrong task: %q", byEvent.ID)
	}

	// 更新状态
	if err := repo.UpdateStatus("task_001", storage.ApprovalStatusApproved); err != nil {
		t.Fatalf("UpdateStatus err=%v", err)
	}
	got2, _, _ := repo.Get("task_001")
	if got2.Status != storage.ApprovalStatusApproved {
		t.Errorf("status not updated: got=%q", got2.Status)
	}

	// 列表
	list, err := repo.List("")
	if err != nil {
		t.Fatalf("List err=%v", err)
	}
	if len(list) != 1 {
		t.Errorf("list count mismatch: got=%d want=1", len(list))
	}
}
