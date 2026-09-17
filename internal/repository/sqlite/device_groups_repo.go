package sqlite

import (
	"database/sql"
	"strings"
	"time"

	"sealsuite-operation/internal/storage"
)

type DeviceGroupsRepository struct {
	baseRepo
}

func NewDeviceGroupsRepository(db *sql.DB) *DeviceGroupsRepository {
	return &DeviceGroupsRepository{baseRepo: baseRepo{db: db}}
}

func (r *DeviceGroupsRepository) List() ([]storage.DeviceGroupItem, error) {
	rows, err := r.db.Query(`SELECT id, name, raw_json, created_at, updated_at FROM device_groups ORDER BY name ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]storage.DeviceGroupItem, 0)
	for rows.Next() {
		var (
			item    storage.DeviceGroupItem
			rawJSON sql.NullString
		)
		err := rows.Scan(&item.ID, &item.Name, &rawJSON, &item.CreatedAt, &item.UpdatedAt)
		if err != nil {
			return nil, err
		}
		if rawJSON.Valid {
			item.RawJSON = rawJSON.String
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *DeviceGroupsRepository) Get(id string) (*storage.DeviceGroupItem, bool, error) {
	var (
		item    storage.DeviceGroupItem
		rawJSON sql.NullString
	)
	err := r.db.QueryRow(`SELECT id, name, raw_json, created_at, updated_at FROM device_groups WHERE id = ?`,
		strings.TrimSpace(id)).Scan(&item.ID, &item.Name, &rawJSON, &item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, false, nil
		}
		return nil, false, err
	}
	if rawJSON.Valid {
		item.RawJSON = rawJSON.String
	}
	return &item, true, nil
}

func (r *DeviceGroupsRepository) Upsert(item storage.DeviceGroupItem) error {
	now := time.Now().Format(time.RFC3339)
	_, err := r.db.Exec(`
		INSERT INTO device_groups (id, name, raw_json, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name = excluded.name,
			raw_json = excluded.raw_json,
			updated_at = excluded.updated_at
	`,
		item.ID, item.Name, item.RawJSON, item.CreatedAt, now,
	)
	return err
}

func (r *DeviceGroupsRepository) ReplaceAll(items []storage.DeviceGroupItem) (int, error) {
	tx, err := r.db.Begin()
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec(`DELETE FROM device_groups`); err != nil {
		return 0, err
	}

	now := time.Now().Format(time.RFC3339)
	count := 0
	for _, item := range items {
		item.ID = strings.TrimSpace(item.ID)
		if item.ID == "" {
			continue
		}
		if item.CreatedAt == "" {
			item.CreatedAt = now
		}
		if _, err := tx.Exec(`
			INSERT INTO device_groups (id, name, raw_json, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?)
		`,
			item.ID, item.Name, item.RawJSON, item.CreatedAt, now,
		); err != nil {
			return count, err
		}
		count++
	}

	if err := tx.Commit(); err != nil {
		return count, err
	}
	return count, nil
}

func (r *DeviceGroupsRepository) GetState() (storage.DeviceGroupState, error) {
	var state storage.DeviceGroupState
	var (
		lastRefreshAt    sql.NullString
		lastRefreshCount sql.NullInt64
	)

	err := r.db.QueryRow(`
		SELECT last_refresh_at, last_refresh_count
		FROM device_groups_state
		LIMIT 1
	`).Scan(&lastRefreshAt, &lastRefreshCount)

	if err != nil {
		if err == sql.ErrNoRows {
			_, _ = r.db.Exec(`INSERT OR IGNORE INTO device_groups_state (id) VALUES (1)`)
			return state, nil
		}
		return state, err
	}

	if lastRefreshAt.Valid {
		state.LastRefreshAt = lastRefreshAt.String
	}
	if lastRefreshCount.Valid {
		state.LastRefreshCount = int(lastRefreshCount.Int64)
	}
	return state, nil
}

func (r *DeviceGroupsRepository) UpdateState(lastRefreshAt string, lastRefreshCount int) error {
	_, err := r.db.Exec(`INSERT OR IGNORE INTO device_groups_state (id) VALUES (1)`)
	if err != nil {
		return err
	}
	_, err = r.db.Exec(`
		UPDATE device_groups_state SET
			last_refresh_at = ?, last_refresh_count = ?
		WHERE id = 1
	`,
		lastRefreshAt, lastRefreshCount,
	)
	return err
}
