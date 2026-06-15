package sqlite

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"sealsuite-operation/internal/repository"
	"sealsuite-operation/internal/storage"
)

var _ repository.LegacyJobRepository = (*LegacyJobRepository)(nil)

type LegacyJobRepository struct {
	baseRepo
}

func NewLegacyJobRepository(db *sql.DB) *LegacyJobRepository {
	return &LegacyJobRepository{baseRepo: baseRepo{db: db}}
}

func (r *LegacyJobRepository) Load() (*storage.JobsFile, error) {
	rows, err := r.db.Query(`
		SELECT name, enabled, cron_expr, type, params_json
		FROM legacy_jobs
		ORDER BY created_at ASC, name ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := &storage.JobsFile{
		Version: 1,
		Jobs:    []storage.Job{},
	}
	for rows.Next() {
		job, err := scanLegacyJob(rows)
		if err != nil {
			return nil, err
		}
		out.Jobs = append(out.Jobs, job)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *LegacyJobRepository) Save(jf *storage.JobsFile) error {
	if jf == nil {
		return fmt.Errorf("legacy jobs file is nil")
	}
	if err := jf.Validate(); err != nil {
		return err
	}

	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec(`DELETE FROM legacy_jobs`); err != nil {
		return err
	}

	now := time.Now().Format(time.RFC3339)
	for _, job := range jf.Jobs {
		if _, err := tx.Exec(`
			INSERT INTO legacy_jobs (
				name, enabled, cron_expr, type, params_json, created_at, updated_at
			) VALUES (?, ?, ?, ?, ?, ?, ?)
		`, job.Name, boolToInt(job.Enabled), job.Cron, job.Type, mustJSON(job.Params), now, now); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *LegacyJobRepository) Get(name string) (*storage.Job, bool, error) {
	row := r.db.QueryRow(`
		SELECT name, enabled, cron_expr, type, params_json
		FROM legacy_jobs
		WHERE name = ?
	`, strings.TrimSpace(name))
	job, err := scanLegacyJob(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, false, nil
		}
		return nil, false, err
	}
	return &job, true, nil
}

func (r *LegacyJobRepository) Upsert(job storage.Job) error {
	file := &storage.JobsFile{Version: 1, Jobs: []storage.Job{job}}
	if err := file.Validate(); err != nil {
		return err
	}

	createdAt := time.Now().Format(time.RFC3339)
	if existingCreatedAt := firstExistingLegacyJobCreatedAt(r.db, job.Name); existingCreatedAt != "" {
		createdAt = existingCreatedAt
	}
	now := time.Now().Format(time.RFC3339)
	_, err := r.db.Exec(`
		INSERT INTO legacy_jobs (
			name, enabled, cron_expr, type, params_json, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(name) DO UPDATE SET
			enabled = excluded.enabled,
			cron_expr = excluded.cron_expr,
			type = excluded.type,
			params_json = excluded.params_json,
			updated_at = excluded.updated_at
	`, job.Name, boolToInt(job.Enabled), job.Cron, job.Type, mustJSON(job.Params), createdAt, now)
	return err
}

func (r *LegacyJobRepository) Delete(name string) error {
	_, err := r.db.Exec(`DELETE FROM legacy_jobs WHERE name = ?`, strings.TrimSpace(name))
	return err
}

func (r *LegacyJobRepository) ReferencedByTemplate(templateID string) ([]string, error) {
	jf, err := r.Load()
	if err != nil {
		return nil, err
	}
	templateID = strings.TrimSpace(templateID)
	refs := make([]string, 0, len(jf.Jobs))
	for _, job := range jf.Jobs {
		if job.Params == nil {
			continue
		}
		if tid, ok := job.Params["template_id"].(string); ok && strings.TrimSpace(tid) == templateID {
			refs = append(refs, job.Name)
		}
	}
	return refs, nil
}

func scanLegacyJob(s scanner) (storage.Job, error) {
	var (
		job       storage.Job
		enabled   int
		paramsRaw string
	)
	err := s.Scan(&job.Name, &enabled, &job.Cron, &job.Type, &paramsRaw)
	if err != nil {
		return storage.Job{}, err
	}
	job.Enabled = intToBool(enabled)
	if err := scanJSON(paramsRaw, "{}", &job.Params); err != nil {
		return storage.Job{}, err
	}
	return job, nil
}

func firstExistingLegacyJobCreatedAt(db *sql.DB, name string) string {
	var createdAt string
	if err := db.QueryRow(`SELECT created_at FROM legacy_jobs WHERE name = ?`, strings.TrimSpace(name)).Scan(&createdAt); err != nil {
		return ""
	}
	return strings.TrimSpace(createdAt)
}
