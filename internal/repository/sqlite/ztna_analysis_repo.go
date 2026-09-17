package sqlite

import (
	"database/sql"
	"encoding/json"
	"strings"
	"time"

	"sealsuite-operation/internal/storage"
)

type ZTNAAnalysisRepository struct {
	baseRepo
}

func NewZTNAAnalysisRepository(db *sql.DB) *ZTNAAnalysisRepository {
	return &ZTNAAnalysisRepository{baseRepo: baseRepo{db: db}}
}

func (r *ZTNAAnalysisRepository) List(limit, offset int, filter storage.ZTNAStatsFilter) ([]storage.ZTNAPolicyAnalysis, int, error) {
	whereClauses := []string{}
	whereArgs := []interface{}{}

	if filter.UserID != "" {
		whereClauses = append(whereClauses, "user_id = ?")
		whereArgs = append(whereArgs, filter.UserID)
	}
	if filter.Category != "" {
		whereClauses = append(whereClauses, "category = ?")
		whereArgs = append(whereArgs, filter.Category)
	}
	if filter.ShouldRevoke != nil {
		whereClauses = append(whereClauses, "should_revoke = ?")
		whereArgs = append(whereArgs, boolToInt(*filter.ShouldRevoke))
	}

	whereSQL := ""
	if len(whereClauses) > 0 {
		whereSQL = " WHERE " + strings.Join(whereClauses, " AND ")
	}

	countQuery := `SELECT COUNT(*) FROM ztna_policy_analysis` + whereSQL
	countRow := r.db.QueryRow(countQuery, whereArgs...)
	var total int
	if err := countRow.Scan(&total); err != nil {
		return nil, 0, err
	}

	query := `
		SELECT
			id, stats_id, user_id, user_name, user_role, user_department,
			dest_ip, dest_port, protocol, resource_tag,
			category, should_revoke, confidence, reasoning,
			suggested_policy, method, status,
			reviewed_by, reviewed_at, review_comment,
			analyzed_at, revoked, revoked_at, revoke_error
		FROM ztna_policy_analysis
	` + whereSQL + `
		ORDER BY analyzed_at DESC
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

	items := make([]storage.ZTNAPolicyAnalysis, 0)
	for rows.Next() {
		item, err := scanZTNAPolicyAnalysis(rows)
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

func (r *ZTNAAnalysisRepository) Count() (int, error) {
	row := r.db.QueryRow(`SELECT COUNT(*) FROM ztna_policy_analysis`)
	var count int
	err := row.Scan(&count)
	return count, err
}

func (r *ZTNAAnalysisRepository) GetByStatsID(statsID string) (storage.ZTNAPolicyAnalysis, bool, error) {
	row := r.db.QueryRow(`
		SELECT
			id, stats_id, user_id, user_name, user_role, user_department,
			dest_ip, dest_port, protocol, resource_tag,
			category, should_revoke, confidence, reasoning,
			suggested_policy, method, status,
			reviewed_by, reviewed_at, review_comment,
			analyzed_at, revoked, revoked_at, revoke_error
		FROM ztna_policy_analysis WHERE stats_id = ?
	`, strings.TrimSpace(statsID))

	item, err := scanZTNAPolicyAnalysis(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return storage.ZTNAPolicyAnalysis{}, false, nil
		}
		return storage.ZTNAPolicyAnalysis{}, false, err
	}
	return item, true, nil
}

func (r *ZTNAAnalysisRepository) Get(id string) (storage.ZTNAPolicyAnalysis, bool, error) {
	row := r.db.QueryRow(`
		SELECT
			id, stats_id, user_id, user_name, user_role, user_department,
			dest_ip, dest_port, protocol, resource_tag,
			category, should_revoke, confidence, reasoning,
			suggested_policy, method, status,
			reviewed_by, reviewed_at, review_comment,
			analyzed_at, revoked, revoked_at, revoke_error
		FROM ztna_policy_analysis WHERE id = ?
	`, strings.TrimSpace(id))

	item, err := scanZTNAPolicyAnalysis(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return storage.ZTNAPolicyAnalysis{}, false, nil
		}
		return storage.ZTNAPolicyAnalysis{}, false, err
	}
	return item, true, nil
}

func (r *ZTNAAnalysisRepository) Upsert(result storage.ZTNAPolicyAnalysis) error {
	now := time.Now().Format(time.RFC3339)
	if result.AnalyzedAt == "" {
		result.AnalyzedAt = now
	}

	reasoningJSON := result.Reasoning
	if len(result.Reasons) > 0 {
		b, _ := json.Marshal(result.Reasons)
		reasoningJSON = string(b)
	}

	_, err := r.db.Exec(`
		INSERT INTO ztna_policy_analysis (
			id, stats_id, user_id, user_name, user_role, user_department,
			dest_ip, dest_port, protocol, resource_tag,
			category, should_revoke, confidence, reasoning,
			suggested_policy, method, status,
			reviewed_by, reviewed_at, review_comment,
			analyzed_at, revoked, revoked_at, revoke_error
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			stats_id = excluded.stats_id,
			user_id = excluded.user_id,
			user_name = excluded.user_name,
			user_role = excluded.user_role,
			user_department = excluded.user_department,
			dest_ip = excluded.dest_ip,
			dest_port = excluded.dest_port,
			protocol = excluded.protocol,
			resource_tag = excluded.resource_tag,
			category = excluded.category,
			should_revoke = excluded.should_revoke,
			confidence = excluded.confidence,
			reasoning = excluded.reasoning,
			suggested_policy = excluded.suggested_policy,
			method = excluded.method,
			status = excluded.status,
			reviewed_by = excluded.reviewed_by,
			reviewed_at = excluded.reviewed_at,
			review_comment = excluded.review_comment,
			analyzed_at = excluded.analyzed_at,
			revoked = excluded.revoked,
			revoked_at = excluded.revoked_at,
			revoke_error = excluded.revoke_error
	`, result.ID, result.StatsID, result.UserID, result.UserName, result.UserRole, result.UserDepartment,
		result.DestIP, result.DestPort, result.Protocol, result.ResourceTag,
		result.Category, boolToInt(result.ShouldRevoke), result.Confidence, reasoningJSON,
		result.SuggestedPolicy, result.Method, result.Status,
		result.ReviewedBy, result.ReviewedAt, result.ReviewComment,
		result.AnalyzedAt, boolToInt(result.Revoked), result.RevokedAt, result.RevokeError)
	return err
}

func (r *ZTNAAnalysisRepository) MarkReviewed(id string, reviewedBy string, comment string) error {
	now := time.Now().Format(time.RFC3339)
	_, err := r.db.Exec(`
		UPDATE ztna_policy_analysis SET status = 'reviewed', reviewed_by = ?, reviewed_at = ?, review_comment = ?
		WHERE id = ?
	`, reviewedBy, now, comment, strings.TrimSpace(id))
	return err
}

func (r *ZTNAAnalysisRepository) Summary() (storage.ZTNAAnalysisSummary, error) {
	var summary storage.ZTNAAnalysisSummary

	row := r.db.QueryRow(`SELECT COUNT(*) FROM ztna_policy_analysis`)
	if err := row.Scan(&summary.Total); err != nil {
		return summary, err
	}

	rows, err := r.db.Query(`
		SELECT category, COUNT(*) FROM ztna_policy_analysis GROUP BY category
	`)
	if err != nil {
		return summary, err
	}
	defer rows.Close()

	for rows.Next() {
		var cat string
		var cnt int
		if err := rows.Scan(&cat, &cnt); err != nil {
			return summary, err
		}
		switch cat {
		case storage.ZTNAnalysisCategoryShouldHave:
			summary.ShouldHaveCount = cnt
		case storage.ZTNAnalysisCategoryShouldRemove:
			summary.ShouldRemoveCount = cnt
		case storage.ZTNAnalysisCategoryNeedsReview:
			summary.NeedsReviewCount = cnt
		case storage.ZTNAnalysisCategoryUnknown:
			summary.UnknownCount = cnt
		}
	}
	if err := rows.Err(); err != nil {
		return summary, err
	}

	row = r.db.QueryRow(`SELECT COUNT(DISTINCT user_id) FROM ztna_policy_analysis`)
	_ = row.Scan(&summary.UserCount)

	row = r.db.QueryRow(`SELECT COUNT(DISTINCT dest_ip || ':' || CAST(dest_port AS TEXT)) FROM ztna_policy_analysis`)
	_ = row.Scan(&summary.ResourceCount)

	return summary, nil
}

func scanZTNAPolicyAnalysis(s scanner) (storage.ZTNAPolicyAnalysis, error) {
	var (
		item           storage.ZTNAPolicyAnalysis
		shouldRevoke   int
		revoked        int
	)
	err := s.Scan(
		&item.ID, &item.StatsID, &item.UserID, &item.UserName, &item.UserRole, &item.UserDepartment,
		&item.DestIP, &item.DestPort, &item.Protocol, &item.ResourceTag,
		&item.Category, &shouldRevoke, &item.Confidence, &item.Reasoning,
		&item.SuggestedPolicy, &item.Method, &item.Status,
		&item.ReviewedBy, &item.ReviewedAt, &item.ReviewComment,
		&item.AnalyzedAt, &revoked, &item.RevokedAt, &item.RevokeError,
	)
	if err != nil {
		return storage.ZTNAPolicyAnalysis{}, err
	}
	item.ShouldRevoke = intToBool(shouldRevoke)
	item.Revoked = intToBool(revoked)

	if item.Reasoning != "" {
		var reasons []string
		if json.Unmarshal([]byte(item.Reasoning), &reasons) == nil {
			item.Reasons = reasons
		}
	}

	return item, nil
}
