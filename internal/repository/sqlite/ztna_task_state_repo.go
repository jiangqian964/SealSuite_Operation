package sqlite

import (
	"database/sql"

	"sealsuite-operation/internal/storage"
)

type ZTNATaskStateRepository struct {
	baseRepo
}

func NewZTNATaskStateRepository(db *sql.DB) *ZTNATaskStateRepository {
	return &ZTNATaskStateRepository{baseRepo: baseRepo{db: db}}
}

func (r *ZTNATaskStateRepository) Get() (storage.ZTNATaskState, error) {
	var state storage.ZTNATaskState
	row := r.db.QueryRow(`
		SELECT
			id, last_sync_at, last_sync_count, last_sync_error,
			last_stats_at, last_stats_user_count, last_stats_resource_count, last_stats_error,
			last_analysis_at, last_analysis_count, last_analysis_error,
			sync_schedule_enabled, stats_schedule_enabled, analysis_schedule_enabled
		FROM ztna_task_state WHERE id = 'default'
	`)

	var syncEnabled, statsEnabled, analysisEnabled int
	err := row.Scan(
		&state.ID, &state.LastSyncAt, &state.LastSyncCount, &state.LastSyncError,
		&state.LastStatsAt, &state.LastStatsUserCount, &state.LastStatsResourceCount, &state.LastStatsError,
		&state.LastAnalysisAt, &state.LastAnalysisCount, &state.LastAnalysisError,
		&syncEnabled, &statsEnabled, &analysisEnabled,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			_, initErr := r.db.Exec(`
				INSERT INTO ztna_task_state (id, sync_schedule_enabled, stats_schedule_enabled, analysis_schedule_enabled)
				VALUES ('default', 1, 1, 1)
			`)
			if initErr != nil {
				return state, initErr
			}
			return storage.ZTNATaskState{
				ID:                      "default",
				SyncScheduleEnabled:     true,
				StatsScheduleEnabled:    true,
				AnalysisScheduleEnabled: true,
			}, nil
		}
		return state, err
	}

	state.SyncScheduleEnabled = intToBool(syncEnabled)
	state.StatsScheduleEnabled = intToBool(statsEnabled)
	state.AnalysisScheduleEnabled = intToBool(analysisEnabled)
	return state, nil
}

func (r *ZTNATaskStateRepository) UpdateLastSync(syncAt string, count int, errMsg string) error {
	_, err := r.db.Exec(`
		UPDATE ztna_task_state SET last_sync_at = ?, last_sync_count = ?, last_sync_error = ? WHERE id = 'default'
	`, syncAt, count, errMsg)
	if err == nil {
		return nil
	}
	_, err = r.db.Exec(`
		INSERT INTO ztna_task_state (id, last_sync_at, last_sync_count, last_sync_error) VALUES ('default', ?, ?, ?)
	`, syncAt, count, errMsg)
	return err
}

func (r *ZTNATaskStateRepository) UpdateLastStats(statsAt string, userCount int, resourceCount int, errMsg string) error {
	_, err := r.db.Exec(`
		UPDATE ztna_task_state SET last_stats_at = ?, last_stats_user_count = ?, last_stats_resource_count = ?, last_stats_error = ? WHERE id = 'default'
	`, statsAt, userCount, resourceCount, errMsg)
	if err == nil {
		return nil
	}
	_, err = r.db.Exec(`
		INSERT INTO ztna_task_state (id, last_stats_at, last_stats_user_count, last_stats_resource_count, last_stats_error)
		VALUES ('default', ?, ?, ?, ?)
	`, statsAt, userCount, resourceCount, errMsg)
	return err
}

func (r *ZTNATaskStateRepository) UpdateLastAnalysis(analysisAt string, count int, errMsg string) error {
	_, err := r.db.Exec(`
		UPDATE ztna_task_state SET last_analysis_at = ?, last_analysis_count = ?, last_analysis_error = ? WHERE id = 'default'
	`, analysisAt, count, errMsg)
	if err == nil {
		return nil
	}
	_, err = r.db.Exec(`
		INSERT INTO ztna_task_state (id, last_analysis_at, last_analysis_count, last_analysis_error)
		VALUES ('default', ?, ?, ?)
	`, analysisAt, count, errMsg)
	return err
}

func (r *ZTNATaskStateRepository) UpdateSyncScheduleEnabled(enabled bool) error {
	_, err := r.db.Exec(`
		UPDATE ztna_task_state SET sync_schedule_enabled = ? WHERE id = 'default'
	`, boolToInt(enabled))
	if err == nil {
		return nil
	}
	_, err = r.db.Exec(`
		INSERT INTO ztna_task_state (id, sync_schedule_enabled) VALUES ('default', ?)
	`, boolToInt(enabled))
	return err
}

func (r *ZTNATaskStateRepository) UpdateStatsScheduleEnabled(enabled bool) error {
	_, err := r.db.Exec(`
		UPDATE ztna_task_state SET stats_schedule_enabled = ? WHERE id = 'default'
	`, boolToInt(enabled))
	if err == nil {
		return nil
	}
	_, err = r.db.Exec(`
		INSERT INTO ztna_task_state (id, stats_schedule_enabled) VALUES ('default', ?)
	`, boolToInt(enabled))
	return err
}

func (r *ZTNATaskStateRepository) UpdateAnalysisScheduleEnabled(enabled bool) error {
	_, err := r.db.Exec(`
		UPDATE ztna_task_state SET analysis_schedule_enabled = ? WHERE id = 'default'
	`, boolToInt(enabled))
	if err == nil {
		return nil
	}
	_, err = r.db.Exec(`
		INSERT INTO ztna_task_state (id, analysis_schedule_enabled) VALUES ('default', ?)
	`, boolToInt(enabled))
	return err
}
