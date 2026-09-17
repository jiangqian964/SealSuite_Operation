package sqlite

import (
	"database/sql"
	"strings"
	"time"

	"sealsuite-operation/internal/storage"
)

type ZTNAAccessStatsRepository struct {
	baseRepo
}

func NewZTNAAccessStatsRepository(db *sql.DB) *ZTNAAccessStatsRepository {
	return &ZTNAAccessStatsRepository{baseRepo: baseRepo{db: db}}
}

func (r *ZTNAAccessStatsRepository) List(limit, offset int, filter storage.ZTNAStatsFilter) ([]storage.ZTNAAccessStats, int, error) {
	whereClauses := []string{}
	whereArgs := []interface{}{}

	if filter.UserID != "" {
		whereClauses = append(whereClauses, "user_id = ?")
		whereArgs = append(whereArgs, filter.UserID)
	}
	if filter.UserKeyword != "" {
		whereClauses = append(whereClauses, "(user_name LIKE ? OR user_department LIKE ?)")
		kw := "%" + filter.UserKeyword + "%"
		whereArgs = append(whereArgs, kw, kw)
	}
	if filter.DestIP != "" {
		whereClauses = append(whereClauses, "dest_ip = ?")
		whereArgs = append(whereArgs, filter.DestIP)
	}

	whereSQL := ""
	if len(whereClauses) > 0 {
		whereSQL = " WHERE " + strings.Join(whereClauses, " AND ")
	}

	countQuery := `SELECT COUNT(*) FROM ztna_access_stats` + whereSQL
	countRow := r.db.QueryRow(countQuery, whereArgs...)
	var total int
	if err := countRow.Scan(&total); err != nil {
		return nil, 0, err
	}

	query := `
		SELECT
			id, user_id, user_name, user_role, user_department, department_id,
			dest_ip, dest_port, protocol, resource_tag,
			first_access_time, last_access_time,
			access_days_30, access_days_90, total_access_count,
			success_count, failed_count, avg_duration_sec, total_bytes,
			work_hour_count, off_hour_count, night_count, updated_at
		FROM ztna_access_stats
	` + whereSQL + `
		ORDER BY last_access_time DESC
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

	items := make([]storage.ZTNAAccessStats, 0)
	for rows.Next() {
		item, err := scanZTNAAccessStats(rows)
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

func (r *ZTNAAccessStatsRepository) Count() (int, error) {
	row := r.db.QueryRow(`SELECT COUNT(*) FROM ztna_access_stats`)
	var count int
	err := row.Scan(&count)
	return count, err
}

func (r *ZTNAAccessStatsRepository) UserCount() (int, error) {
	row := r.db.QueryRow(`SELECT COUNT(DISTINCT user_id) FROM ztna_access_stats`)
	var count int
	err := row.Scan(&count)
	return count, err
}

func (r *ZTNAAccessStatsRepository) ResourceCount() (int, error) {
	row := r.db.QueryRow(`SELECT COUNT(DISTINCT dest_ip || ':' || CAST(dest_port AS TEXT)) FROM ztna_access_stats`)
	var count int
	err := row.Scan(&count)
	return count, err
}

func (r *ZTNAAccessStatsRepository) Upsert(stats storage.ZTNAAccessStats) error {
	now := time.Now().Format(time.RFC3339)
	if stats.UpdatedAt == "" {
		stats.UpdatedAt = now
	}

	_, err := r.db.Exec(`
		INSERT INTO ztna_access_stats (
			id, user_id, user_name, user_role, user_department, department_id,
			dest_ip, dest_port, protocol, resource_tag,
			first_access_time, last_access_time,
			access_days_30, access_days_90, total_access_count,
			success_count, failed_count, avg_duration_sec, total_bytes,
			work_hour_count, off_hour_count, night_count, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(user_id, dest_ip, dest_port, protocol) DO UPDATE SET
			user_name = excluded.user_name,
			user_role = excluded.user_role,
			user_department = excluded.user_department,
			department_id = excluded.department_id,
			resource_tag = excluded.resource_tag,
			first_access_time = excluded.first_access_time,
			last_access_time = excluded.last_access_time,
			access_days_30 = excluded.access_days_30,
			access_days_90 = excluded.access_days_90,
			total_access_count = excluded.total_access_count,
			success_count = excluded.success_count,
			failed_count = excluded.failed_count,
			avg_duration_sec = excluded.avg_duration_sec,
			total_bytes = excluded.total_bytes,
			work_hour_count = excluded.work_hour_count,
			off_hour_count = excluded.off_hour_count,
			night_count = excluded.night_count,
			updated_at = excluded.updated_at
	`, stats.ID, stats.UserID, stats.UserName, stats.UserRole, stats.UserDepartment, stats.DepartmentID,
		stats.DestIP, stats.DestPort, stats.Protocol, stats.ResourceTag,
		stats.FirstAccessTime, stats.LastAccessTime,
		stats.AccessDays30, stats.AccessDays90, stats.TotalAccessCount,
		stats.SuccessCount, stats.FailedCount, stats.AvgDurationSec, stats.TotalBytes,
		stats.WorkHourCount, stats.OffHourCount, stats.NightCount, stats.UpdatedAt)
	return err
}

func (r *ZTNAAccessStatsRepository) GetByUserDest(userID, destIP string, destPort int, protocol string) (storage.ZTNAAccessStats, bool, error) {
	row := r.db.QueryRow(`
		SELECT
			id, user_id, user_name, user_role, user_department, department_id,
			dest_ip, dest_port, protocol, resource_tag,
			first_access_time, last_access_time,
			access_days_30, access_days_90, total_access_count,
			success_count, failed_count, avg_duration_sec, total_bytes,
			work_hour_count, off_hour_count, night_count, updated_at
		FROM ztna_access_stats
		WHERE user_id = ? AND dest_ip = ? AND dest_port = ? AND protocol = ?
	`, userID, destIP, destPort, protocol)

	item, err := scanZTNAAccessStats(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return storage.ZTNAAccessStats{}, false, nil
		}
		return storage.ZTNAAccessStats{}, false, err
	}
	return item, true, nil
}

func scanZTNAAccessStats(s scanner) (storage.ZTNAAccessStats, error) {
	var item storage.ZTNAAccessStats
	err := s.Scan(
		&item.ID, &item.UserID, &item.UserName, &item.UserRole, &item.UserDepartment, &item.DepartmentID,
		&item.DestIP, &item.DestPort, &item.Protocol, &item.ResourceTag,
		&item.FirstAccessTime, &item.LastAccessTime,
		&item.AccessDays30, &item.AccessDays90, &item.TotalAccessCount,
		&item.SuccessCount, &item.FailedCount, &item.AvgDurationSec, &item.TotalBytes,
		&item.WorkHourCount, &item.OffHourCount, &item.NightCount, &item.UpdatedAt,
	)
	return item, err
}
