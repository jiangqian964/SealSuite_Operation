package sqlite

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"sealsuite-operation/internal/repository"
	"sealsuite-operation/internal/storage"
)

var _ repository.ComplexTaskRepository = (*ComplexTaskRepository)(nil)

type ComplexTaskRepository struct {
	baseRepo
}

func NewComplexTaskRepository(db *sql.DB) *ComplexTaskRepository {
	return &ComplexTaskRepository{baseRepo: baseRepo{db: db}}
}

func (r *ComplexTaskRepository) Get(id string) (storage.ComplexTask, bool, error) {
	row := r.db.QueryRow(`
		SELECT
			id, name, goal, execution_mode, webhook_config_id, webhook_enabled
		FROM complex_tasks
		WHERE id = ?
	`, strings.TrimSpace(id))

	task, err := scanComplexTask(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return storage.ComplexTask{}, false, nil
		}
		return storage.ComplexTask{}, false, err
	}

	steps, err := r.listSteps(task.ID)
	if err != nil {
		return storage.ComplexTask{}, false, err
	}
	task.Steps = steps
	return task, true, nil
}

func (r *ComplexTaskRepository) List() ([]storage.ComplexTask, error) {
	rows, err := r.db.Query(`
		SELECT
			id, name, goal, execution_mode, webhook_config_id, webhook_enabled
		FROM complex_tasks
		ORDER BY created_at ASC, id ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]storage.ComplexTask, 0)
	for rows.Next() {
		task, err := scanComplexTask(rows)
		if err != nil {
			return nil, err
		}
		steps, err := r.listSteps(task.ID)
		if err != nil {
			return nil, err
		}
		task.Steps = steps
		items = append(items, task)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func (r *ComplexTaskRepository) Upsert(task storage.ComplexTask) error {
	task.ID = strings.TrimSpace(task.ID)
	task.Name = strings.TrimSpace(task.Name)
	task.Goal = strings.TrimSpace(task.Goal)
	task.ExecutionMode = strings.TrimSpace(task.ExecutionMode)
	task.WebhookConfigID = strings.TrimSpace(task.WebhookConfigID)
	if task.Steps == nil {
		task.Steps = []storage.ComplexTaskStep{}
	}
	now := time.Now().Format(time.RFC3339)

	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	_, err = tx.Exec(`
		INSERT INTO complex_tasks (
			id, name, goal, execution_mode, webhook_config_id, webhook_enabled, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name = excluded.name,
			goal = excluded.goal,
			execution_mode = excluded.execution_mode,
			webhook_config_id = excluded.webhook_config_id,
			webhook_enabled = excluded.webhook_enabled,
			updated_at = excluded.updated_at
	`, task.ID, task.Name, task.Goal, task.ExecutionMode, task.WebhookConfigID, boolToInt(task.WebhookEnabled), now, now)
	if err != nil {
		return err
	}

	if _, err := tx.Exec(`DELETE FROM complex_task_steps WHERE task_id = ?`, task.ID); err != nil {
		return err
	}

	for idx, step := range task.Steps {
		stepID := strings.TrimSpace(step.ID)
		if stepID == "" {
			stepID = fmt.Sprintf("%s_step_%d", task.ID, idx+1)
		}
		dbStepID := complexTaskStepDBID(task.ID, stepID)
		if _, err := tx.Exec(`
			INSERT INTO complex_task_steps (
				id, task_id, step_order, type, name, config_json, created_at, updated_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		`, dbStepID, task.ID, idx+1, strings.TrimSpace(step.Type), strings.TrimSpace(step.Name), mustJSON(step.Config), now, now); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (r *ComplexTaskRepository) Delete(id string) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	id = strings.TrimSpace(id)
	if _, err := tx.Exec(`DELETE FROM complex_task_steps WHERE task_id = ?`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM complex_tasks WHERE id = ?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *ComplexTaskRepository) listSteps(taskID string) ([]storage.ComplexTaskStep, error) {
	rows, err := r.db.Query(`
		SELECT id, type, name, config_json
		FROM complex_task_steps
		WHERE task_id = ?
		ORDER BY step_order ASC, id ASC
	`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	steps := make([]storage.ComplexTaskStep, 0)
	for rows.Next() {
		var (
			step      storage.ComplexTaskStep
			configRaw string
		)
		if err := rows.Scan(&step.ID, &step.Type, &step.Name, &configRaw); err != nil {
			return nil, err
		}
		step.ID = visibleComplexTaskStepID(taskID, step.ID)
		if err := scanJSON(configRaw, "{}", &step.Config); err != nil {
			return nil, err
		}
		steps = append(steps, step)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return steps, nil
}

func scanComplexTask(s scanner) (storage.ComplexTask, error) {
	var (
		task           storage.ComplexTask
		webhookEnabled int
	)
	err := s.Scan(
		&task.ID,
		&task.Name,
		&task.Goal,
		&task.ExecutionMode,
		&task.WebhookConfigID,
		&webhookEnabled,
	)
	if err != nil {
		return storage.ComplexTask{}, err
	}
	task.WebhookEnabled = intToBool(webhookEnabled)
	if task.Steps == nil {
		task.Steps = []storage.ComplexTaskStep{}
	}
	return task, nil
}

func complexTaskStepDBID(taskID, stepID string) string {
	taskID = strings.TrimSpace(taskID)
	stepID = strings.TrimSpace(stepID)
	if taskID == "" {
		return stepID
	}
	if stepID == "" {
		return taskID
	}
	return taskID + ":" + stepID
}

func visibleComplexTaskStepID(taskID, dbStepID string) string {
	taskID = strings.TrimSpace(taskID)
	dbStepID = strings.TrimSpace(dbStepID)
	prefix := taskID + ":"
	if taskID != "" && strings.HasPrefix(dbStepID, prefix) {
		return strings.TrimPrefix(dbStepID, prefix)
	}
	return dbStepID
}
