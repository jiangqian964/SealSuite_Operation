package sqlite

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"sealsuite-operation/internal/storage"
)

type FeishuAPIRepository struct {
	baseRepo
}

func NewFeishuAPIRepository(db *sql.DB) *FeishuAPIRepository {
	return &FeishuAPIRepository{baseRepo: baseRepo{db: db}}
}

func (r *FeishuAPIRepository) Load() (*storage.FeishuAPIFile, error) {
	rows, err := r.db.Query(`
		SELECT id, name, app_id, app_secret, base_url, enabled, is_active, created_at
		FROM feishu_apis
		ORDER BY created_at ASC, id ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := &storage.FeishuAPIFile{Items: []storage.FeishuAPIItem{}}
	for rows.Next() {
		var (
			item      storage.FeishuAPIItem
			enabled   int
			isActive  int
			appSecret sql.NullString
		)
		err := rows.Scan(&item.ID, &item.Name, &item.AppID, &appSecret, &item.BaseURL, &enabled, &isActive, &item.CreatedAt)
		if err != nil {
			return nil, err
		}
		item.Enabled = intToBool(enabled)
		if appSecret.Valid {
			item.AppSecret = appSecret.String
		}
		if intToBool(isActive) {
			out.ActiveID = item.ID
		}
		out.Items = append(out.Items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *FeishuAPIRepository) Get(id string) (*storage.FeishuAPIItem, bool, error) {
	var (
		item      storage.FeishuAPIItem
		enabled   int
		isActive  int
		appSecret sql.NullString
	)
	err := r.db.QueryRow(`
		SELECT id, name, app_id, app_secret, base_url, enabled, is_active, created_at
		FROM feishu_apis
		WHERE id = ?
	`, strings.TrimSpace(id)).Scan(&item.ID, &item.Name, &item.AppID, &appSecret, &item.BaseURL, &enabled, &isActive, &item.CreatedAt)
	item.Enabled = intToBool(enabled)
	if appSecret.Valid {
		item.AppSecret = appSecret.String
	}
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, false, nil
		}
		return nil, false, err
	}
	return &item, true, nil
}

func (r *FeishuAPIRepository) UpsertAndMaybeActivate(item storage.FeishuAPIItem, activate bool) error {
	item.ID = strings.TrimSpace(item.ID)
	item.Name = strings.TrimSpace(item.Name)
	item.AppID = strings.TrimSpace(item.AppID)
	item.BaseURL = strings.TrimSpace(item.BaseURL)

	existing, ok, err := r.Get(item.ID)
	if err != nil {
		return err
	}
	if ok {
		if item.AppSecret == "" {
			item.AppSecret = existing.AppSecret
		}
		if item.CreatedAt == "" {
			item.CreatedAt = existing.CreatedAt
		}
	}
	if item.CreatedAt == "" {
		item.CreatedAt = time.Now().Format(time.RFC3339)
	}
	now := time.Now().Format(time.RFC3339)

	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if activate {
		if _, err := tx.Exec(`UPDATE feishu_apis SET is_active = 0, updated_at = ?`, now); err != nil {
			return err
		}
	}

	var appSecret interface{}
	if item.AppSecret != "" {
		appSecret = item.AppSecret
	} else {
		appSecret = nil
	}

	if _, err := tx.Exec(`
		INSERT INTO feishu_apis (
			id, name, app_id, app_secret, base_url, enabled, is_active,
			created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name = excluded.name,
			app_id = excluded.app_id,
			app_secret = COALESCE(excluded.app_secret, feishu_apis.app_secret),
			base_url = excluded.base_url,
			enabled = excluded.enabled,
			is_active = excluded.is_active,
			updated_at = excluded.updated_at
	`, item.ID, item.Name, item.AppID, appSecret, item.BaseURL,
		boolToInt(item.Enabled), boolToInt(activate),
		item.CreatedAt, now); err != nil {
		return err
	}

	return tx.Commit()
}

func (r *FeishuAPIRepository) Activate(id string) (*storage.FeishuAPIItem, error) {
	id = strings.TrimSpace(id)
	item, ok, err := r.Get(id)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("feishu api not found: %s", id)
	}

	now := time.Now().Format(time.RFC3339)
	tx, err := r.db.Begin()
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec(`UPDATE feishu_apis SET is_active = 0, updated_at = ?`, now); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(`UPDATE feishu_apis SET is_active = 1, updated_at = ? WHERE id = ?`, now, id); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return item, nil
}

func (r *FeishuAPIRepository) Delete(id string) error {
	id = strings.TrimSpace(id)
	var isActive int
	err := r.db.QueryRow(`SELECT is_active FROM feishu_apis WHERE id = ?`, id).Scan(&isActive)
	if err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("feishu api not found: %s", id)
		}
		return err
	}
	if intToBool(isActive) {
		return fmt.Errorf("cannot delete active feishu api")
	}
	_, err = r.db.Exec(`DELETE FROM feishu_apis WHERE id = ?`, id)
	return err
}
