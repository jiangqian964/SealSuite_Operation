package sqlite

import (
	"database/sql"
	"time"

	"sealsuite-operation/internal/repository"
	"sealsuite-operation/internal/storage"
)

var _ repository.TaskDraftRepository = (*TaskDraftRepository)(nil)

type TaskDraftRepository struct {
	baseRepo
}

func NewTaskDraftRepository(db *sql.DB) *TaskDraftRepository {
	return &TaskDraftRepository{baseRepo: baseRepo{db: db}}
}

func (r *TaskDraftRepository) Get(id string) (storage.TaskDraft, bool, error) {
	row := r.db.QueryRow(`
		SELECT
			id, name, mode, cycle_mode, run_count, run_until,
			source_template_id, webhook_config_id, webhook_enabled,
			input_config_json, transform_config_json, llm_config_json, output_config_json
		FROM task_drafts
		WHERE id = ?
	`, id)

	item, err := scanTaskDraft(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return storage.TaskDraft{}, false, nil
		}
		return storage.TaskDraft{}, false, err
	}
	return item, true, nil
}

func (r *TaskDraftRepository) List() ([]storage.TaskDraft, error) {
	rows, err := r.db.Query(`
		SELECT
			id, name, mode, cycle_mode, run_count, run_until,
			source_template_id, webhook_config_id, webhook_enabled,
			input_config_json, transform_config_json, llm_config_json, output_config_json
		FROM task_drafts
		ORDER BY created_at ASC, id ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]storage.TaskDraft, 0)
	for rows.Next() {
		item, err := scanTaskDraft(rows)
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

func (r *TaskDraftRepository) Upsert(d storage.TaskDraft) error {
	d = storage.NormalizeTaskDraft(d)
	now := time.Now().Format(time.RFC3339)

	_, err := r.db.Exec(`
		INSERT INTO task_drafts (
			id, name, mode, cycle_mode, run_count, run_until,
			source_template_id, webhook_config_id, webhook_enabled,
			input_config_json, transform_config_json, llm_config_json, output_config_json,
			created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name = excluded.name,
			mode = excluded.mode,
			cycle_mode = excluded.cycle_mode,
			run_count = excluded.run_count,
			run_until = excluded.run_until,
			source_template_id = excluded.source_template_id,
			webhook_config_id = excluded.webhook_config_id,
			webhook_enabled = excluded.webhook_enabled,
			input_config_json = excluded.input_config_json,
			transform_config_json = excluded.transform_config_json,
			llm_config_json = excluded.llm_config_json,
			output_config_json = excluded.output_config_json,
			updated_at = excluded.updated_at
	`, d.ID, d.Name, d.Mode, d.CycleMode, d.RunCount, d.RunUntil,
		d.SourceTemplateID, d.WebhookConfigID, boolToInt(d.WebhookEnabled),
		mustJSON(d.InputConfig), mustJSON(d.TransformConfig), mustJSON(d.LLMConfig), mustJSON(d.OutputConfig),
		now, now)
	return err
}

func (r *TaskDraftRepository) Delete(id string) error {
	_, err := r.db.Exec(`DELETE FROM task_drafts WHERE id = ?`, id)
	return err
}

type scanner interface {
	Scan(dest ...interface{}) error
}

func scanTaskDraft(s scanner) (storage.TaskDraft, error) {
	var (
		item            storage.TaskDraft
		webhookEnabled  int
		inputConfigRaw  string
		transformRaw    string
		llmConfigRaw    string
		outputConfigRaw string
	)

	err := s.Scan(
		&item.ID,
		&item.Name,
		&item.Mode,
		&item.CycleMode,
		&item.RunCount,
		&item.RunUntil,
		&item.SourceTemplateID,
		&item.WebhookConfigID,
		&webhookEnabled,
		&inputConfigRaw,
		&transformRaw,
		&llmConfigRaw,
		&outputConfigRaw,
	)
	if err != nil {
		return storage.TaskDraft{}, err
	}

	item.WebhookEnabled = intToBool(webhookEnabled)
	if err := scanJSON(inputConfigRaw, "{}", &item.InputConfig); err != nil {
		return storage.TaskDraft{}, err
	}
	if err := scanJSON(transformRaw, "{}", &item.TransformConfig); err != nil {
		return storage.TaskDraft{}, err
	}
	if err := scanJSON(llmConfigRaw, "{}", &item.LLMConfig); err != nil {
		return storage.TaskDraft{}, err
	}
	if err := scanJSON(outputConfigRaw, "{}", &item.OutputConfig); err != nil {
		return storage.TaskDraft{}, err
	}

	item = storage.NormalizeTaskDraft(item)
	return item, nil
}
