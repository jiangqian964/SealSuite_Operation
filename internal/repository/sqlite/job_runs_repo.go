package sqlite

import (
	"database/sql"

	"sealsuite-operation/internal/service"
)

var _ service.RunLogWriter = (*JobRunsRepository)(nil)

type JobRunsRepository struct {
	baseRepo
}

func NewJobRunsRepository(db *sql.DB) *JobRunsRepository {
	return &JobRunsRepository{baseRepo: baseRepo{db: db}}
}

func (r *JobRunsRepository) WriteRun(run service.RunLog) error {
	_, err := r.db.Exec(`
		INSERT INTO job_runs (
			id, source_type, source_id, target_type, target_id, status, trigger_source,
			started_at, finished_at, duration_ms, error_message, result_json
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, run.ID, run.SourceType, run.SourceID, run.TargetType, run.TargetID, run.Status, run.TriggerSource,
		run.StartedAt, run.FinishedAt, run.DurationMS, run.ErrorMessage, run.ResultJSON)
	return err
}

func (r *JobRunsRepository) List(limit int) ([]service.RunLog, error) {
	query := `
		SELECT
			id, source_type, source_id, target_type, target_id, status, trigger_source,
			started_at, finished_at, duration_ms, error_message, result_json
		FROM job_runs
		ORDER BY started_at DESC, id DESC
	`
	var (
		rows *sql.Rows
		err  error
	)
	if limit > 0 {
		rows, err = r.db.Query(query+` LIMIT ?`, limit)
	} else {
		rows, err = r.db.Query(query)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]service.RunLog, 0)
	for rows.Next() {
		var item service.RunLog
		if err := rows.Scan(
			&item.ID,
			&item.SourceType,
			&item.SourceID,
			&item.TargetType,
			&item.TargetID,
			&item.Status,
			&item.TriggerSource,
			&item.StartedAt,
			&item.FinishedAt,
			&item.DurationMS,
			&item.ErrorMessage,
			&item.ResultJSON,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}
