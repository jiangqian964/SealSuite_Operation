package sqlite

import (
	"database/sql"
	"strings"
	"time"

	"sealsuite-operation/internal/repository"
	"sealsuite-operation/internal/storage"
)

var _ repository.ExternalIPSyncTaskRepository = (*ExternalIPSyncTaskRepository)(nil)

type ExternalIPSyncTaskRepository struct {
	baseRepo
}

func NewExternalIPSyncTaskRepository(db *sql.DB) *ExternalIPSyncTaskRepository {
	return &ExternalIPSyncTaskRepository{baseRepo: baseRepo{db: db}}
}

func (r *ExternalIPSyncTaskRepository) List() ([]storage.ExternalIPSyncTask, error) {
	rows, err := r.db.Query(`
		SELECT
			id, name, source_type, source_url, ip_version,
			resource_id, resource_name_snapshot, write_action, feilian_api_path,
			dry_run, skip_when_empty, enabled, created_at, updated_at
		FROM external_ip_sync_tasks
		ORDER BY created_at ASC, id ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]storage.ExternalIPSyncTask, 0)
	for rows.Next() {
		item, err := scanExternalIPSyncTask(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func (r *ExternalIPSyncTaskRepository) Get(id string) (storage.ExternalIPSyncTask, bool, error) {
	row := r.db.QueryRow(`
		SELECT
			id, name, source_type, source_url, ip_version,
			resource_id, resource_name_snapshot, write_action, feilian_api_path,
			dry_run, skip_when_empty, enabled, created_at, updated_at
		FROM external_ip_sync_tasks
		WHERE id = ?
	`, strings.TrimSpace(id))

	item, err := scanExternalIPSyncTask(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return storage.ExternalIPSyncTask{}, false, nil
		}
		return storage.ExternalIPSyncTask{}, false, err
	}
	return item, true, nil
}

func (r *ExternalIPSyncTaskRepository) Upsert(task storage.ExternalIPSyncTask) error {
	task = storage.NormalizeExternalIPSyncTask(task)
	if err := task.Validate(); err != nil {
		return err
	}
	now := time.Now().Format(time.RFC3339)

	_, err := r.db.Exec(`
		INSERT INTO external_ip_sync_tasks (
			id, name, source_type, source_url, ip_version,
			resource_id, resource_name_snapshot, write_action, feilian_api_path,
			dry_run, skip_when_empty, enabled, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name = excluded.name,
			source_type = excluded.source_type,
			source_url = excluded.source_url,
			ip_version = excluded.ip_version,
			resource_id = excluded.resource_id,
			resource_name_snapshot = excluded.resource_name_snapshot,
			write_action = excluded.write_action,
			feilian_api_path = excluded.feilian_api_path,
			dry_run = excluded.dry_run,
			skip_when_empty = excluded.skip_when_empty,
			enabled = excluded.enabled,
			updated_at = excluded.updated_at
	`, task.ID, task.Name, task.SourceType, task.SourceURL, task.IPVersion,
		task.ResourceID, task.ResourceNameSnapshot, task.WriteAction, task.FeilianAPIPath,
		boolToInt(task.DryRun), boolToInt(task.SkipWhenEmpty), boolToInt(task.Enabled), now, now)
	return err
}

func (r *ExternalIPSyncTaskRepository) Delete(id string) error {
	_, err := r.db.Exec(`DELETE FROM external_ip_sync_tasks WHERE id = ?`, strings.TrimSpace(id))
	return err
}

func scanExternalIPSyncTask(s scanner) (storage.ExternalIPSyncTask, error) {
	var (
		item          storage.ExternalIPSyncTask
		dryRun        int
		skipWhenEmpty int
		enabled       int
	)

	err := s.Scan(
		&item.ID,
		&item.Name,
		&item.SourceType,
		&item.SourceURL,
		&item.IPVersion,
		&item.ResourceID,
		&item.ResourceNameSnapshot,
		&item.WriteAction,
		&item.FeilianAPIPath,
		&dryRun,
		&skipWhenEmpty,
		&enabled,
		&item.CreatedAt,
		&item.UpdatedAt,
	)
	if err != nil {
		return storage.ExternalIPSyncTask{}, err
	}

	item.DryRun = intToBool(dryRun)
	item.SkipWhenEmpty = intToBool(skipWhenEmpty)
	item.Enabled = intToBool(enabled)
	item = storage.NormalizeExternalIPSyncTask(item)
	return item, nil
}
