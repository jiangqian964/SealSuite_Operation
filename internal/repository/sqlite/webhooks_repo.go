package sqlite

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"sealsuite-operation/internal/storage"
	"sealsuite-operation/internal/webhook"
)

type WebhookRepository struct {
	baseRepo
}

func NewWebhookRepository(db *sql.DB) *WebhookRepository {
	return &WebhookRepository{baseRepo: baseRepo{db: db}}
}

func (r *WebhookRepository) Load() (*storage.WebhookFile, error) {
	rows, err := r.db.Query(`
		SELECT
			id, name, provider, url, method, headers_json, auth_type, body_template,
			timeout_sec, retry_count, enabled, created_at
		FROM webhooks
		ORDER BY created_at ASC, id ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := &storage.WebhookFile{Items: []storage.WebhookItem{}}
	for rows.Next() {
		item, err := scanWebhook(rows)
		if err != nil {
			return nil, err
		}
		out.Items = append(out.Items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *WebhookRepository) Get(id string) (*storage.WebhookItem, bool, error) {
	row := r.db.QueryRow(`
		SELECT
			id, name, provider, url, method, headers_json, auth_type, body_template,
			timeout_sec, retry_count, enabled, created_at
		FROM webhooks
		WHERE id = ?
	`, strings.TrimSpace(id))
	item, err := scanWebhook(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, false, nil
		}
		return nil, false, err
	}
	return &item, true, nil
}

func (r *WebhookRepository) Upsert(item storage.WebhookItem) error {
	item.ID = strings.TrimSpace(item.ID)
	item.Name = strings.TrimSpace(item.Name)
	item.Provider = webhook.NormalizeProvider(item.Provider)
	item.URL = strings.TrimSpace(item.URL)
	item.Method = strings.TrimSpace(item.Method)
	item.AuthType = strings.TrimSpace(item.AuthType)
	item.BodyTmpl = strings.TrimSpace(item.BodyTmpl)

	existing, ok, err := r.Get(item.ID)
	if err != nil {
		return err
	}
	if ok && item.CreatedAt == "" {
		item.CreatedAt = existing.CreatedAt
	}
	if item.CreatedAt == "" {
		item.CreatedAt = time.Now().Format(time.RFC3339)
	}
	now := time.Now().Format(time.RFC3339)

	_, err = r.db.Exec(`
		INSERT INTO webhooks (
			id, name, provider, url, method, headers_json, auth_type, body_template,
			timeout_sec, retry_count, enabled, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name = excluded.name,
			provider = excluded.provider,
			url = excluded.url,
			method = excluded.method,
			headers_json = excluded.headers_json,
			auth_type = excluded.auth_type,
			body_template = excluded.body_template,
			timeout_sec = excluded.timeout_sec,
			retry_count = excluded.retry_count,
			enabled = excluded.enabled,
			updated_at = excluded.updated_at
	`, item.ID, item.Name, item.Provider, item.URL, item.Method,
		mustJSON(item.Headers), item.AuthType, item.BodyTmpl,
		item.TimeoutSec, item.RetryCount, boolToInt(item.Enabled), item.CreatedAt, now)
	return err
}

func (r *WebhookRepository) Delete(id string) error {
	id = strings.TrimSpace(id)
	res, err := r.db.Exec(`DELETE FROM webhooks WHERE id = ?`, id)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return fmt.Errorf("webhook not found: %s", id)
	}
	return nil
}

func scanWebhook(s scanner) (storage.WebhookItem, error) {
	var (
		item    storage.WebhookItem
		headers string
		enabled int
	)
	err := s.Scan(
		&item.ID,
		&item.Name,
		&item.Provider,
		&item.URL,
		&item.Method,
		&headers,
		&item.AuthType,
		&item.BodyTmpl,
		&item.TimeoutSec,
		&item.RetryCount,
		&enabled,
		&item.CreatedAt,
	)
	if err != nil {
		return storage.WebhookItem{}, err
	}
	item.Enabled = intToBool(enabled)
	item.Provider = webhook.NormalizeProvider(item.Provider)
	if err := scanJSON(headers, "{}", &item.Headers); err != nil {
		return storage.WebhookItem{}, err
	}
	if item.Headers == nil {
		item.Headers = map[string]string{}
	}
	return item, nil
}
