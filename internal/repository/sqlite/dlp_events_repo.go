package sqlite

import (
	"database/sql"
	"strings"
	"time"

	"sealsuite-operation/internal/storage"
)

type DLPEventRepository struct {
	baseRepo
}

func NewDLPEventRepository(db *sql.DB) *DLPEventRepository {
	return &DLPEventRepository{baseRepo: baseRepo{db: db}}
}

func (r *DLPEventRepository) List(limit, offset int) ([]storage.DLPEvent, int, error) {
	countRow := r.db.QueryRow(`SELECT COUNT(*) FROM dlp_events`)
	var total int
	if err := countRow.Scan(&total); err != nil {
		return nil, 0, err
	}

	query := `
		SELECT
			id, file_info_name, file_info_path, file_info_type,
			leak_way_app_name, evidence_url, event_type,
			user_id, user_name, device_id, event_time,
			imported_at, analyzed, raw_json
		FROM dlp_events
		ORDER BY event_time DESC
	`
	if limit > 0 {
		query += " LIMIT ? OFFSET ?"
	}

	var rows *sql.Rows
	var err error
	if limit > 0 {
		rows, err = r.db.Query(query, limit, offset)
	} else {
		rows, err = r.db.Query(query)
	}
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	items := make([]storage.DLPEvent, 0)
	for rows.Next() {
		item, err := scanDLPEvent(rows)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

type DLPEventWithAnalysis struct {
	Event    storage.DLPEvent        `json:"event"`
	Analysis *storage.DLPAnalysisResult `json:"analysis"`
}

type DLPEventFilter struct {
	Status   string
	Category string
	Exclude  string
}

func (r *DLPEventRepository) ListWithAnalysis(limit, offset int, filter DLPEventFilter) ([]DLPEventWithAnalysis, int, error) {
	whereClauses := []string{}
	whereArgs := []interface{}{}

	if filter.Status == "analyzed" {
		whereClauses = append(whereClauses, "ar.id IS NOT NULL")
	} else if filter.Status == "pending" {
		whereClauses = append(whereClauses, "ar.id IS NULL")
	}

	if filter.Category != "" {
		whereClauses = append(whereClauses, "ar.category = ?")
		whereArgs = append(whereArgs, filter.Category)
	}

	if filter.Exclude == "true" {
		whereClauses = append(whereClauses, "ar.should_exclude = 1 AND ar.whitelisted = 0")
	} else if filter.Exclude == "false" {
		whereClauses = append(whereClauses, "ar.should_exclude = 0 AND ar.whitelisted = 0")
	}

	whereSQL := ""
	if len(whereClauses) > 0 {
		whereSQL = " WHERE " + strings.Join(whereClauses, " AND ")
	}

	countQuery := `SELECT COUNT(*) FROM dlp_events e LEFT JOIN dlp_analysis_results ar ON e.id = ar.event_id` + whereSQL
	countRow := r.db.QueryRow(countQuery, whereArgs...)
	var total int
	if err := countRow.Scan(&total); err != nil {
		return nil, 0, err
	}

	query := `
		SELECT
			e.id, e.file_info_name, e.file_info_path, e.file_info_type,
			e.leak_way_app_name, e.evidence_url, e.event_type,
			e.user_id, e.user_name, e.device_id, e.event_time,
			e.imported_at, e.analyzed, e.raw_json,
			ar.id as ar_id, ar.event_id as ar_event_id, ar.category, ar.should_exclude,
			ar.confidence, ar.reasoning, ar.analyzed_at, ar.whitelisted, ar.match_type, ar.match_value
		FROM dlp_events e
		LEFT JOIN dlp_analysis_results ar ON e.id = ar.event_id
	` + whereSQL + `
		ORDER BY e.event_time DESC
	`
	if limit > 0 {
		query += " LIMIT ? OFFSET ?"
		whereArgs = append(whereArgs, limit, offset)
	}

	var rows *sql.Rows
	var err error
	if limit > 0 {
		rows, err = r.db.Query(query, whereArgs...)
	} else {
		rows, err = r.db.Query(query, whereArgs...)
	}
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	items := make([]DLPEventWithAnalysis, 0)
	for rows.Next() {
		var (
			event     storage.DLPEvent
			analysis  storage.DLPAnalysisResult
			analyzed  int
			arID      sql.NullString
			arEventID sql.NullString
			category  sql.NullString
			shouldExclude sql.NullInt64
			confidence sql.NullFloat64
			reasoning sql.NullString
			analyzedAt sql.NullString
			whitelisted sql.NullInt64
			matchType sql.NullString
			matchValue sql.NullString
		)
		err := rows.Scan(
			&event.ID, &event.FileInfoName, &event.FileInfoPath, &event.FileInfoType,
			&event.LeakWayAppName, &event.EvidenceURL, &event.EventType,
			&event.UserID, &event.UserName, &event.DeviceID, &event.EventTime,
			&event.ImportedAt, &analyzed, &event.RawJSON,
			&arID, &arEventID, &category, &shouldExclude,
			&confidence, &reasoning, &analyzedAt,
			&whitelisted, &matchType, &matchValue,
		)
		if err != nil {
			return nil, 0, err
		}
		event.Analyzed = intToBool(analyzed)
		analysis.ID = arID.String
		analysis.EventID = arEventID.String
		analysis.Category = category.String
		analysis.ShouldExclude = shouldExclude.Valid && shouldExclude.Int64 != 0
		if confidence.Valid {
			analysis.Confidence = confidence.Float64
		}
		analysis.Reasoning = reasoning.String
		analysis.AnalyzedAt = analyzedAt.String
		analysis.Whitelisted = whitelisted.Valid && whitelisted.Int64 != 0
		analysis.MatchType = matchType.String
		analysis.MatchValue = matchValue.String

		var analysisPtr *storage.DLPAnalysisResult
		if arID.Valid && arID.String != "" {
			analysisPtr = &analysis
		}

		items = append(items, DLPEventWithAnalysis{
			Event:    event,
			Analysis: analysisPtr,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (r *DLPEventRepository) Get(id string) (storage.DLPEvent, bool, error) {
	row := r.db.QueryRow(`
		SELECT
			id, file_info_name, file_info_path, file_info_type,
			leak_way_app_name, evidence_url, event_type,
			user_id, user_name, device_id, event_time,
			imported_at, analyzed, raw_json
		FROM dlp_events WHERE id = ?
	`, strings.TrimSpace(id))

	item, err := scanDLPEvent(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return storage.DLPEvent{}, false, nil
		}
		return storage.DLPEvent{}, false, err
	}
	return item, true, nil
}

func (r *DLPEventRepository) ListUnafelyzed(limit int) ([]storage.DLPEvent, error) {
	rows, err := r.db.Query(`
		SELECT
			id, file_info_name, file_info_path, file_info_type,
			leak_way_app_name, evidence_url, event_type,
			user_id, user_name, device_id, event_time,
			imported_at, analyzed, raw_json
		FROM dlp_events
		WHERE analyzed = 0
		ORDER BY event_time ASC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]storage.DLPEvent, 0)
	for rows.Next() {
		item, err := scanDLPEvent(rows)
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

func (r *DLPEventRepository) ListRetainedAlerts(limit int) ([]storage.DLPEvent, error) {
	rows, err := r.db.Query(`
		SELECT
			e.id, e.file_info_name, e.file_info_path, e.file_info_type,
			e.leak_way_app_name, e.evidence_url, e.event_type,
			e.user_id, e.user_name, e.device_id, e.event_time,
			e.imported_at, e.analyzed, e.raw_json
		FROM dlp_events e
		INNER JOIN dlp_analysis_results ar ON e.id = ar.event_id
		WHERE ar.should_exclude = 0 AND ar.whitelisted = 0
		ORDER BY ar.analyzed_at DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]storage.DLPEvent, 0)
	for rows.Next() {
		item, err := scanDLPEvent(rows)
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

func (r *DLPEventRepository) Upsert(event storage.DLPEvent) error {
	now := time.Now().Format(time.RFC3339)
	if event.ImportedAt == "" {
		event.ImportedAt = now
	}

	_, err := r.db.Exec(`
		INSERT INTO dlp_events (
			id, file_info_name, file_info_path, file_info_type,
			leak_way_app_name, evidence_url, event_type,
			user_id, user_name, device_id, event_time,
			imported_at, analyzed, raw_json
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			file_info_name = excluded.file_info_name,
			file_info_path = excluded.file_info_path,
			file_info_type = excluded.file_info_type,
			leak_way_app_name = excluded.leak_way_app_name,
			evidence_url = excluded.evidence_url,
			event_type = excluded.event_type,
			user_id = excluded.user_id,
			user_name = excluded.user_name,
			device_id = excluded.device_id,
			event_time = excluded.event_time,
			imported_at = excluded.imported_at,
			analyzed = excluded.analyzed,
			raw_json = excluded.raw_json
	`, event.ID, event.FileInfoName, event.FileInfoPath, event.FileInfoType,
		event.LeakWayAppName, event.EvidenceURL, event.EventType,
		event.UserID, event.UserName, event.DeviceID, event.EventTime,
		event.ImportedAt, boolToInt(event.Analyzed), event.RawJSON)
	return err
}

func (r *DLPEventRepository) MarkAnalyzed(id string) error {
	_, err := r.db.Exec(`
		UPDATE dlp_events SET analyzed = 1 WHERE id = ?
	`, strings.TrimSpace(id))
	return err
}

func (r *DLPEventRepository) GetState() (storage.DLPAnalysisState, error) {
	var state storage.DLPAnalysisState
	row := r.db.QueryRow(`
		SELECT last_sync_at, last_sync_count, sync_schedule_enabled,
		       last_analysis_at, last_analysis_count, analysis_schedule_enabled
		FROM dlp_analysis_state WHERE id = 1
	`)

	var syncEnabled, analysisEnabled int
	err := row.Scan(
		&state.LastSyncAt, &state.LastSyncCount, &syncEnabled,
		&state.LastAnalysisAt, &state.LastAnalysisCount, &analysisEnabled,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			_, initErr := r.db.Exec(`
				INSERT INTO dlp_analysis_state (id, sync_schedule_enabled, analysis_schedule_enabled)
				VALUES (1, 1, 1)
			`)
			if initErr != nil {
				return state, initErr
			}
			return storage.DLPAnalysisState{
				SyncScheduleEnabled:     true,
				AnalysisScheduleEnabled: true,
			}, nil
		}
		return state, err
	}

	state.SyncScheduleEnabled = intToBool(syncEnabled)
	state.AnalysisScheduleEnabled = intToBool(analysisEnabled)
	return state, nil
}

func (r *DLPEventRepository) UpdateSyncState(syncAt string, count int) error {
	_, err := r.db.Exec(`
		UPDATE dlp_analysis_state SET last_sync_at = ?, last_sync_count = ? WHERE id = 1
	`, syncAt, count)
	if err == nil {
		return nil
	}
	_, err = r.db.Exec(`
		INSERT INTO dlp_analysis_state (id, last_sync_at, last_sync_count) VALUES (1, ?, ?)
	`, syncAt, count)
	return err
}

func (r *DLPEventRepository) UpdateAnalysisState(analysisAt string, count int) error {
	_, err := r.db.Exec(`
		UPDATE dlp_analysis_state SET last_analysis_at = ?, last_analysis_count = ? WHERE id = 1
	`, analysisAt, count)
	if err == nil {
		return nil
	}
	_, err = r.db.Exec(`
		INSERT INTO dlp_analysis_state (id, last_analysis_at, last_analysis_count) VALUES (1, ?, ?)
	`, analysisAt, count)
	return err
}

func (r *DLPEventRepository) UpdateSyncScheduleEnabled(enabled bool) error {
	_, err := r.db.Exec(`
		UPDATE dlp_analysis_state SET sync_schedule_enabled = ? WHERE id = 1
	`, boolToInt(enabled))
	if err == nil {
		return nil
	}
	_, err = r.db.Exec(`
		INSERT INTO dlp_analysis_state (id, sync_schedule_enabled) VALUES (1, ?)
	`, boolToInt(enabled))
	return err
}

func (r *DLPEventRepository) UpdateAnalysisScheduleEnabled(enabled bool) error {
	_, err := r.db.Exec(`
		UPDATE dlp_analysis_state SET analysis_schedule_enabled = ? WHERE id = 1
	`, boolToInt(enabled))
	if err == nil {
		return nil
	}
	_, err = r.db.Exec(`
		INSERT INTO dlp_analysis_state (id, analysis_schedule_enabled) VALUES (1, ?)
	`, boolToInt(enabled))
	return err
}

func scanDLPEvent(s scanner) (storage.DLPEvent, error) {
	var (
		item     storage.DLPEvent
		analyzed int
	)
	err := s.Scan(
		&item.ID, &item.FileInfoName, &item.FileInfoPath, &item.FileInfoType,
		&item.LeakWayAppName, &item.EvidenceURL, &item.EventType,
		&item.UserID, &item.UserName, &item.DeviceID, &item.EventTime,
		&item.ImportedAt, &analyzed, &item.RawJSON,
	)
	if err != nil {
		return storage.DLPEvent{}, err
	}
	item.Analyzed = intToBool(analyzed)
	return item, nil
}
