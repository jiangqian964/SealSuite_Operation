package sqlite

import (
	"database/sql"
	"strings"
	"time"

	"sealsuite-operation/internal/storage"
)

type DLPAnalysisRepository struct {
	baseRepo
}

func NewDLPAnalysisRepository(db *sql.DB) *DLPAnalysisRepository {
	return &DLPAnalysisRepository{baseRepo: baseRepo{db: db}}
}

func (r *DLPAnalysisRepository) List(limit, offset int) ([]storage.DLPAnalysisResult, int, error) {
	countRow := r.db.QueryRow(`SELECT COUNT(*) FROM dlp_analysis_results`)
	var total int
	if err := countRow.Scan(&total); err != nil {
		return nil, 0, err
	}

	query := `
		SELECT
			id, event_id, category, should_exclude, confidence,
			reasoning, analyzed_at, whitelisted, match_type, match_value
		FROM dlp_analysis_results
		ORDER BY analyzed_at DESC
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

	items := make([]storage.DLPAnalysisResult, 0)
	for rows.Next() {
		item, err := scanDLPAnalysisResult(rows)
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

func (r *DLPAnalysisRepository) GetByEventID(eventID string) (storage.DLPAnalysisResult, bool, error) {
	row := r.db.QueryRow(`
		SELECT
			id, event_id, category, should_exclude, confidence,
			reasoning, analyzed_at, whitelisted, match_type, match_value
		FROM dlp_analysis_results WHERE event_id = ?
	`, strings.TrimSpace(eventID))

	item, err := scanDLPAnalysisResult(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return storage.DLPAnalysisResult{}, false, nil
		}
		return storage.DLPAnalysisResult{}, false, err
	}
	return item, true, nil
}

func (r *DLPAnalysisRepository) Upsert(result storage.DLPAnalysisResult) error {
	now := time.Now().Format(time.RFC3339)
	if result.AnalyzedAt == "" {
		result.AnalyzedAt = now
	}

	_, err := r.db.Exec(`
		INSERT INTO dlp_analysis_results (
			id, event_id, category, should_exclude, confidence,
			reasoning, analyzed_at, whitelisted, match_type, match_value
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			event_id = excluded.event_id,
			category = excluded.category,
			should_exclude = excluded.should_exclude,
			confidence = excluded.confidence,
			reasoning = excluded.reasoning,
			analyzed_at = excluded.analyzed_at,
			whitelisted = excluded.whitelisted,
			match_type = excluded.match_type,
			match_value = excluded.match_value
	`, result.ID, result.EventID, result.Category, boolToInt(result.ShouldExclude),
		result.Confidence, result.Reasoning, result.AnalyzedAt,
		boolToInt(result.Whitelisted), result.MatchType, result.MatchValue)
	return err
}

func (r *DLPAnalysisRepository) MarkWhitelisted(id string, whitelisted bool, matchType, matchValue string) error {
	_, err := r.db.Exec(`
		UPDATE dlp_analysis_results SET whitelisted = ?, match_type = ?, match_value = ?
		WHERE id = ?
	`, boolToInt(whitelisted), matchType, matchValue, strings.TrimSpace(id))
	return err
}

func (r *DLPAnalysisRepository) MarkAllMatchedAsWhitelisted(matchType, matchValue string) (int, error) {
	rows, err := r.db.Query(`
		SELECT ar.id, e.file_info_path, e.file_info_name
		FROM dlp_analysis_results ar
		JOIN dlp_events e ON ar.event_id = e.id
	`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	type resultInfo struct {
		id       string
		filePath string
		fileName string
	}
	var items []resultInfo
	for rows.Next() {
		var item resultInfo
		if err := rows.Scan(&item.id, &item.filePath, &item.fileName); err != nil {
			return 0, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}

	matchFn := func(filePath, fileName string) bool {
		switch matchType {
		case "path":
			return filePath == matchValue
		case "name":
			return fileName == matchValue
		case "extension":
			if len(fileName) < len(matchValue) {
				return false
			}
			return fileName[len(fileName)-len(matchValue):] == matchValue
		default:
			return false
		}
	}

	count := 0
	for _, item := range items {
		if matchFn(item.filePath, item.fileName) {
			_, err := r.db.Exec(`
				UPDATE dlp_analysis_results SET whitelisted = 1, match_type = ?, match_value = ?
				WHERE id = ?
			`, matchType, matchValue, item.id)
			if err != nil {
				return count, err
			}
			count++
		}
	}
	return count, nil
}

func (r *DLPAnalysisRepository) UnmarkWhitelistedByRule(matchType, matchValue string) (int, error) {
	result, err := r.db.Exec(`
		UPDATE dlp_analysis_results SET whitelisted = 0, match_type = '', match_value = ''
		WHERE match_type = ? AND match_value = ?
	`, matchType, matchValue)
	if err != nil {
		return 0, err
	}
	rows, _ := result.RowsAffected()
	return int(rows), nil
}

func (r *DLPAnalysisRepository) Summary() (map[string]int, error) {
	summary := make(map[string]int)

	rows, err := r.db.Query(`
		SELECT should_exclude, COUNT(*) FROM dlp_analysis_results GROUP BY should_exclude
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var shouldExclude, count int
		if err := rows.Scan(&shouldExclude, &count); err != nil {
			return nil, err
		}
		if shouldExclude == 1 {
			summary["excluded"] = count
		} else {
			summary["kept"] = count
		}
	}

	whitelistedRow := r.db.QueryRow(`SELECT COUNT(*) FROM dlp_analysis_results WHERE whitelisted = 1`)
	var whitelisted int
	if err := whitelistedRow.Scan(&whitelisted); err != nil {
		return nil, err
	}
	summary["whitelisted"] = whitelisted

	totalRow := r.db.QueryRow(`SELECT COUNT(*) FROM dlp_analysis_results`)
	var total int
	if err := totalRow.Scan(&total); err != nil {
		return nil, err
	}
	summary["total"] = total

	return summary, nil
}

func scanDLPAnalysisResult(s scanner) (storage.DLPAnalysisResult, error) {
	var (
		item          storage.DLPAnalysisResult
		shouldExclude int
		whitelisted   int
	)
	err := s.Scan(
		&item.ID, &item.EventID, &item.Category, &shouldExclude, &item.Confidence,
		&item.Reasoning, &item.AnalyzedAt, &whitelisted, &item.MatchType, &item.MatchValue,
	)
	if err != nil {
		return storage.DLPAnalysisResult{}, err
	}
	item.ShouldExclude = intToBool(shouldExclude)
	item.Whitelisted = intToBool(whitelisted)
	return item, nil
}
