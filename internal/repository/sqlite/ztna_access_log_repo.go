package sqlite

import (
	"database/sql"
	"strings"
	"time"

	"sealsuite-operation/internal/storage"
)

type ZTNAAccessLogRepository struct {
	baseRepo
}

func NewZTNAAccessLogRepository(db *sql.DB) *ZTNAAccessLogRepository {
	return &ZTNAAccessLogRepository{baseRepo: baseRepo{db: db}}
}

func (r *ZTNAAccessLogRepository) List(limit, offset int, filter storage.ZTNAAccessLogFilter) ([]storage.ZTNAAccessLog, int, error) {
	whereClauses := []string{}
	whereArgs := []interface{}{}

	if filter.UserID != "" {
		whereClauses = append(whereClauses, "user_id = ?")
		whereArgs = append(whereArgs, filter.UserID)
	}
	if filter.DestIP != "" {
		whereClauses = append(whereClauses, "dest_ip = ?")
		whereArgs = append(whereArgs, filter.DestIP)
	}
	if filter.Action != "" {
		whereClauses = append(whereClauses, "action = ?")
		whereArgs = append(whereArgs, filter.Action)
	}

	whereSQL := ""
	if len(whereClauses) > 0 {
		whereSQL = " WHERE " + strings.Join(whereClauses, " AND ")
	}

	countQuery := `SELECT COUNT(*) FROM ztna_access_logs` + whereSQL
	countRow := r.db.QueryRow(countQuery, whereArgs...)
	var total int
	if err := countRow.Scan(&total); err != nil {
		return nil, 0, err
	}

	query := `
		SELECT
			id, log_id, user_id, user_full_name, department_id, department_path,
			roles_name, roles_id, dest_ip, dest_port, action, protocol,
			event_time, imported_at, raw_json
		FROM ztna_access_logs
	` + whereSQL + `
		ORDER BY event_time DESC
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

	items := make([]storage.ZTNAAccessLog, 0)
	for rows.Next() {
		item, err := scanZTNAAccessLog(rows)
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

func (r *ZTNAAccessLogRepository) Count() (int, error) {
	row := r.db.QueryRow(`SELECT COUNT(*) FROM ztna_access_logs`)
	var count int
	err := row.Scan(&count)
	return count, err
}

func (r *ZTNAAccessLogRepository) GetLatestEventTime() (string, error) {
	row := r.db.QueryRow(`SELECT event_time FROM ztna_access_logs ORDER BY event_time DESC LIMIT 1`)
	var eventTime string
	err := row.Scan(&eventTime)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return eventTime, err
}

func (r *ZTNAAccessLogRepository) Upsert(log storage.ZTNAAccessLog) error {
	now := time.Now().Format(time.RFC3339)
	if log.ImportedAt == "" {
		log.ImportedAt = now
	}

	_, err := r.db.Exec(`
		INSERT INTO ztna_access_logs (
			id, log_id, user_id, user_full_name, department_id, department_path,
			roles_name, roles_id, dest_ip, dest_port, action, protocol,
			event_time, imported_at, raw_json
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(log_id) DO UPDATE SET
			user_id = excluded.user_id,
			user_full_name = excluded.user_full_name,
			department_id = excluded.department_id,
			department_path = excluded.department_path,
			roles_name = excluded.roles_name,
			roles_id = excluded.roles_id,
			dest_ip = excluded.dest_ip,
			dest_port = excluded.dest_port,
			action = excluded.action,
			protocol = excluded.protocol,
			event_time = excluded.event_time,
			raw_json = excluded.raw_json
	`, log.ID, log.LogID, log.UserID, log.UserFullName, log.DepartmentID, log.DepartmentPath,
		log.RolesName, log.RolesID, log.DestIP, log.DestPort, log.Action, log.Protocol,
		log.EventTime, log.ImportedAt, log.RawJSON)
	return err
}

func (r *ZTNAAccessLogRepository) BatchUpsert(logs []storage.ZTNAAccessLog) (int, error) {
	if len(logs) == 0 {
		return 0, nil
	}

	tx, err := r.db.Begin()
	if err != nil {
		return 0, err
	}

	count := 0
	for _, logItem := range logs {
		now := time.Now().Format(time.RFC3339)
		if logItem.ImportedAt == "" {
			logItem.ImportedAt = now
		}

		_, err := tx.Exec(`
			INSERT OR IGNORE INTO ztna_access_logs (
				id, log_id, user_id, user_full_name, department_id, department_path,
				roles_name, roles_id, dest_ip, dest_port, action, protocol,
				event_time, imported_at, raw_json
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, logItem.ID, logItem.LogID, logItem.UserID, logItem.UserFullName, logItem.DepartmentID, logItem.DepartmentPath,
			logItem.RolesName, logItem.RolesID, logItem.DestIP, logItem.DestPort, logItem.Action, logItem.Protocol,
			logItem.EventTime, logItem.ImportedAt, logItem.RawJSON)
		if err != nil {
			_ = tx.Rollback()
			return count, err
		}
		count++
	}

	if err := tx.Commit(); err != nil {
		return count, err
	}
	return count, nil
}

func scanZTNAAccessLog(s scanner) (storage.ZTNAAccessLog, error) {
	var item storage.ZTNAAccessLog
	err := s.Scan(
		&item.ID, &item.LogID, &item.UserID, &item.UserFullName, &item.DepartmentID, &item.DepartmentPath,
		&item.RolesName, &item.RolesID, &item.DestIP, &item.DestPort, &item.Action, &item.Protocol,
		&item.EventTime, &item.ImportedAt, &item.RawJSON,
	)
	return item, err
}
