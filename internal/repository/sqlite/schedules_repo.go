package sqlite

import (
	"database/sql"
	"time"

	"sealsuite-operation/internal/repository"
	"sealsuite-operation/internal/storage"
)

var _ repository.ScheduleRepository = (*ScheduleRepository)(nil)

type ScheduleRepository struct {
	baseRepo
}

func NewScheduleRepository(db *sql.DB) *ScheduleRepository {
	return &ScheduleRepository{baseRepo: baseRepo{db: db}}
}

func (r *ScheduleRepository) Get(id string) (storage.JobSchedule, bool, error) {
	row := r.db.QueryRow(`
		SELECT
			id, draft_id, target_type, target_id, enabled,
			webhook_config_id, webhook_enabled, start_at, end_at,
			schedule_type, cron_expr, interval_expr, timezone,
			post_filter_json, field_select_json, truncate_rules_json
		FROM job_schedules
		WHERE id = ?
	`, id)

	item, err := scanSchedule(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return storage.JobSchedule{}, false, nil
		}
		return storage.JobSchedule{}, false, err
	}
	return item, true, nil
}

func (r *ScheduleRepository) List() ([]storage.JobSchedule, error) {
	rows, err := r.db.Query(`
		SELECT
			id, draft_id, target_type, target_id, enabled,
			webhook_config_id, webhook_enabled, start_at, end_at,
			schedule_type, cron_expr, interval_expr, timezone,
			post_filter_json, field_select_json, truncate_rules_json
		FROM job_schedules
		ORDER BY created_at ASC, id ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]storage.JobSchedule, 0)
	for rows.Next() {
		item, err := scanSchedule(rows)
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

func (r *ScheduleRepository) Upsert(s storage.JobSchedule) error {
	s = storage.NormalizeJobSchedule(s)
	now := time.Now().Format(time.RFC3339)

	_, err := r.db.Exec(`
		INSERT INTO job_schedules (
			id, draft_id, target_type, target_id, enabled,
			webhook_config_id, webhook_enabled, start_at, end_at,
			schedule_type, cron_expr, interval_expr, timezone,
			post_filter_json, field_select_json, truncate_rules_json,
			created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			draft_id = excluded.draft_id,
			target_type = excluded.target_type,
			target_id = excluded.target_id,
			enabled = excluded.enabled,
			webhook_config_id = excluded.webhook_config_id,
			webhook_enabled = excluded.webhook_enabled,
			start_at = excluded.start_at,
			end_at = excluded.end_at,
			schedule_type = excluded.schedule_type,
			cron_expr = excluded.cron_expr,
			interval_expr = excluded.interval_expr,
			timezone = excluded.timezone,
			post_filter_json = excluded.post_filter_json,
			field_select_json = excluded.field_select_json,
			truncate_rules_json = excluded.truncate_rules_json,
			updated_at = excluded.updated_at
	`, s.ID, s.DraftID, s.TargetType, s.TargetID, boolToInt(s.Enabled),
		s.WebhookConfigID, boolToInt(s.WebhookEnabled), s.StartAt, s.EndAt,
		s.ScheduleType, s.Cron, s.Interval, s.Timezone,
		mustJSON(s.PostFilter), mustJSONArray(s.FieldSelect), mustJSON(s.TruncateRules),
		now, now)
	return err
}

func (r *ScheduleRepository) Delete(id string) error {
	_, err := r.db.Exec(`DELETE FROM job_schedules WHERE id = ?`, id)
	return err
}

func scanSchedule(s scanner) (storage.JobSchedule, error) {
	var (
		item             storage.JobSchedule
		enabled          int
		webhookEnabled   int
		postFilterRaw    string
		fieldSelectRaw   string
		truncateRulesRaw string
	)

	err := s.Scan(
		&item.ID,
		&item.DraftID,
		&item.TargetType,
		&item.TargetID,
		&enabled,
		&item.WebhookConfigID,
		&webhookEnabled,
		&item.StartAt,
		&item.EndAt,
		&item.ScheduleType,
		&item.Cron,
		&item.Interval,
		&item.Timezone,
		&postFilterRaw,
		&fieldSelectRaw,
		&truncateRulesRaw,
	)
	if err != nil {
		return storage.JobSchedule{}, err
	}

	item.Enabled = intToBool(enabled)
	item.WebhookEnabled = intToBool(webhookEnabled)
	if err := scanJSON(postFilterRaw, "{}", &item.PostFilter); err != nil {
		return storage.JobSchedule{}, err
	}
	if err := scanJSON(fieldSelectRaw, "[]", &item.FieldSelect); err != nil {
		return storage.JobSchedule{}, err
	}
	if err := scanJSON(truncateRulesRaw, "{}", &item.TruncateRules); err != nil {
		return storage.JobSchedule{}, err
	}

	item = storage.NormalizeJobSchedule(item)
	return item, nil
}
