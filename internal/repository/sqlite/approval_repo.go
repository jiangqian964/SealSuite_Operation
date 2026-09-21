package sqlite

import (
	"database/sql"
	"strings"
	"time"

	"sealsuite-operation/internal/storage"
)

// ApprovalConfigRepository 自动化审批配置仓储
// 配置以单行形式存储（id 固定为 1）。
type ApprovalConfigRepository struct {
	baseRepo
}

func NewApprovalConfigRepository(db *sql.DB) *ApprovalConfigRepository {
	return &ApprovalConfigRepository{baseRepo: baseRepo{db: db}}
}

// Get 获取审批配置；若不存在则返回默认配置（未启用、空分组）。
func (r *ApprovalConfigRepository) Get() (*storage.ApprovalConfig, error) {
	var (
		groupIDsJSON string
		webhookID    string
		enabled      int
	)
	err := r.db.QueryRow(`
		SELECT group_ids_json, webhook_id, enabled
		FROM approval_config
		WHERE id = 1
	`).Scan(&groupIDsJSON, &webhookID, &enabled)
	if err != nil {
		if err == sql.ErrNoRows {
			return &storage.ApprovalConfig{GroupIDs: []string{}}, nil
		}
		return nil, err
	}

	cfg := &storage.ApprovalConfig{
		GroupIDs:  []string{},
		WebhookID: strings.TrimSpace(webhookID),
		Enabled:   intToBool(enabled),
	}
	if err := scanJSON(groupIDsJSON, "[]", &cfg.GroupIDs); err != nil {
		return nil, err
	}
	if cfg.GroupIDs == nil {
		cfg.GroupIDs = []string{}
	}
	return cfg, nil
}

// Save 保存审批配置（UPSERT，id 固定为 1）。
func (r *ApprovalConfigRepository) Save(cfg storage.ApprovalConfig) error {
	now := time.Now().Format(time.RFC3339)
	if cfg.GroupIDs == nil {
		cfg.GroupIDs = []string{}
	}
	_, err := r.db.Exec(`
		INSERT INTO approval_config (id, group_ids_json, webhook_id, enabled, updated_at)
		VALUES (1, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			group_ids_json = excluded.group_ids_json,
			webhook_id = excluded.webhook_id,
			enabled = excluded.enabled,
			updated_at = excluded.updated_at
	`, mustJSONArray(cfg.GroupIDs), strings.TrimSpace(cfg.WebhookID), boolToInt(cfg.Enabled), now)
	return err
}

// ApprovalTaskRepository 审批任务流水仓储
type ApprovalTaskRepository struct {
	baseRepo
}

func NewApprovalTaskRepository(db *sql.DB) *ApprovalTaskRepository {
	return &ApprovalTaskRepository{baseRepo: baseRepo{db: db}}
}

// List 列出审批任务；status 为空则返回全部，按创建时间倒序。
func (r *ApprovalTaskRepository) List(status string) ([]storage.ApprovalTask, error) {
	query := `
		SELECT id, event_id, device_identifier, raw_payload_json, status,
		       feishu_device_record_id, webhook_delivery_json, error_message,
		       created_at, updated_at
		FROM approval_tasks
	`
	var args []interface{}
	if status = strings.TrimSpace(status); status != "" {
		query += " WHERE status = ?"
		args = append(args, status)
	}
	query += " ORDER BY created_at DESC, id ASC"

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]storage.ApprovalTask, 0)
	for rows.Next() {
		item, err := scanApprovalTask(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// Get 按任务 ID 查询。
func (r *ApprovalTaskRepository) Get(id string) (*storage.ApprovalTask, bool, error) {
	row := r.db.QueryRow(`
		SELECT id, event_id, device_identifier, raw_payload_json, status,
		       feishu_device_record_id, webhook_delivery_json, error_message,
		       created_at, updated_at
		FROM approval_tasks
		WHERE id = ?
	`, strings.TrimSpace(id))
	item, err := scanApprovalTask(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, false, nil
		}
		return nil, false, err
	}
	return &item, true, nil
}

// GetByEventID 按飞书事件 ID 查询（用于幂等去重）。
func (r *ApprovalTaskRepository) GetByEventID(eventID string) (*storage.ApprovalTask, bool, error) {
	row := r.db.QueryRow(`
		SELECT id, event_id, device_identifier, raw_payload_json, status,
		       feishu_device_record_id, webhook_delivery_json, error_message,
		       created_at, updated_at
		FROM approval_tasks
		WHERE event_id = ?
	`, strings.TrimSpace(eventID))
	item, err := scanApprovalTask(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, false, nil
		}
		return nil, false, err
	}
	return &item, true, nil
}

// Create 新建审批任务。
func (r *ApprovalTaskRepository) Create(task storage.ApprovalTask) error {
	now := time.Now().Format(time.RFC3339)
	if task.CreatedAt == "" {
		task.CreatedAt = now
	}
	if task.UpdatedAt == "" {
		task.UpdatedAt = now
	}
	if task.Status == "" {
		task.Status = storage.ApprovalStatusPending
	}
	_, err := r.db.Exec(`
		INSERT INTO approval_tasks (
			id, event_id, device_identifier, raw_payload_json, status,
			feishu_device_record_id, webhook_delivery_json, error_message,
			created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		task.ID, strings.TrimSpace(task.EventID), strings.TrimSpace(task.DeviceIdentifier),
		task.RawPayload, string(task.Status),
		task.FeishuDeviceRecordID, task.WebhookDelivery, task.ErrorMessage,
		task.CreatedAt, task.UpdatedAt,
	)
	return err
}

// UpdateStatus 更新任务状态。
func (r *ApprovalTaskRepository) UpdateStatus(id string, status storage.ApprovalTaskStatus) error {
	_, err := r.db.Exec(`
		UPDATE approval_tasks SET status = ?, updated_at = ? WHERE id = ?
	`, string(status), time.Now().Format(time.RFC3339), strings.TrimSpace(id))
	return err
}

// UpdateFeishuRecordID 更新飞书 device_record_id。
func (r *ApprovalTaskRepository) UpdateFeishuRecordID(id, recordID string) error {
	_, err := r.db.Exec(`
		UPDATE approval_tasks SET feishu_device_record_id = ?, updated_at = ? WHERE id = ?
	`, strings.TrimSpace(recordID), time.Now().Format(time.RFC3339), strings.TrimSpace(id))
	return err
}

// UpdateWebhookDelivery 更新 Webhook 投递结果 JSON。
func (r *ApprovalTaskRepository) UpdateWebhookDelivery(id, delivery string) error {
	_, err := r.db.Exec(`
		UPDATE approval_tasks SET webhook_delivery_json = ?, updated_at = ? WHERE id = ?
	`, delivery, time.Now().Format(time.RFC3339), strings.TrimSpace(id))
	return err
}

// UpdateError 更新错误信息。
func (r *ApprovalTaskRepository) UpdateError(id, errMsg string) error {
	_, err := r.db.Exec(`
		UPDATE approval_tasks SET error_message = ?, updated_at = ? WHERE id = ?
	`, errMsg, time.Now().Format(time.RFC3339), strings.TrimSpace(id))
	return err
}

func scanApprovalTask(s scanner) (storage.ApprovalTask, error) {
	var (
		item        storage.ApprovalTask
		rawPayload  string
		delivery    string
		errMsg      string
		feishuRecID string
	)
	err := s.Scan(
		&item.ID, &item.EventID, &item.DeviceIdentifier, &rawPayload, &item.Status,
		&feishuRecID, &delivery, &errMsg,
		&item.CreatedAt, &item.UpdatedAt,
	)
	if err != nil {
		return storage.ApprovalTask{}, err
	}
	item.RawPayload = rawPayload
	item.WebhookDelivery = delivery
	item.ErrorMessage = errMsg
	item.FeishuDeviceRecordID = feishuRecID
	return item, nil
}
