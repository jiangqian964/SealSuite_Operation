package sqlite

import (
	"database/sql"
	"strings"
	"time"

	"sealsuite-operation/internal/storage"
)

type FeishuResourcesRepository struct {
	baseRepo
}

func NewFeishuResourcesRepository(db *sql.DB) *FeishuResourcesRepository {
	return &FeishuResourcesRepository{baseRepo: baseRepo{db: db}}
}

func (r *FeishuResourcesRepository) List(typeFilter string) ([]storage.FeishuResourceItem, error) {
	var (
		rows *sql.Rows
		err  error
	)
	filter := strings.TrimSpace(typeFilter)
	if filter == "" || filter == "*" {
		rows, err = r.db.Query(`
			SELECT id, name, type, tag_ids, tag_names, raw_json, created_at, updated_at
			FROM feishu_resources
			ORDER BY type ASC, name ASC, id ASC
		`)
	} else {
		rows, err = r.db.Query(`
			SELECT id, name, type, tag_ids, tag_names, raw_json, created_at, updated_at
			FROM feishu_resources
			WHERE type = ?
			ORDER BY name ASC, id ASC
		`, filter)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]storage.FeishuResourceItem, 0)
	for rows.Next() {
		var (
			item    storage.FeishuResourceItem
			tagIDs  sql.NullString
			tagNames sql.NullString
			rawJSON sql.NullString
		)
		err := rows.Scan(&item.ID, &item.Name, &item.Type, &tagIDs, &tagNames, &rawJSON, &item.CreatedAt, &item.UpdatedAt)
		if err != nil {
			return nil, err
		}
		if tagIDs.Valid {
			item.TagIDs = tagIDs.String
		}
		if tagNames.Valid {
			item.TagNames = tagNames.String
		}
		if rawJSON.Valid {
			item.RawJSON = rawJSON.String
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func (r *FeishuResourcesRepository) Get(id string) (*storage.FeishuResourceItem, bool, error) {
	var (
		item     storage.FeishuResourceItem
		tagIDs   sql.NullString
		tagNames sql.NullString
		rawJSON  sql.NullString
	)
	err := r.db.QueryRow(`
		SELECT id, name, type, tag_ids, tag_names, raw_json, created_at, updated_at
		FROM feishu_resources
		WHERE id = ?
	`, strings.TrimSpace(id)).Scan(&item.ID, &item.Name, &item.Type, &tagIDs, &tagNames, &rawJSON, &item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, false, nil
		}
		return nil, false, err
	}
	if tagIDs.Valid {
		item.TagIDs = tagIDs.String
	}
	if tagNames.Valid {
		item.TagNames = tagNames.String
	}
	if rawJSON.Valid {
		item.RawJSON = rawJSON.String
	}
	return &item, true, nil
}

func (r *FeishuResourcesRepository) Upsert(item storage.FeishuResourceItem) error {
	item.ID = strings.TrimSpace(item.ID)
	item.Name = strings.TrimSpace(item.Name)
	item.Type = strings.TrimSpace(item.Type)
	if item.Type == "" {
		item.Type = "ip"
	}
	now := time.Now().Format(time.RFC3339)

	existing, ok, err := r.Get(item.ID)
	if err != nil {
		return err
	}
	if ok && item.CreatedAt == "" {
		item.CreatedAt = existing.CreatedAt
	}
	if item.CreatedAt == "" {
		item.CreatedAt = now
	}

	var rawJSON interface{}
	if item.RawJSON != "" {
		rawJSON = item.RawJSON
	} else {
		rawJSON = nil
	}
	var tagIDs interface{}
	if item.TagIDs != "" {
		tagIDs = item.TagIDs
	} else {
		tagIDs = nil
	}
	var tagNames interface{}
	if item.TagNames != "" {
		tagNames = item.TagNames
	} else {
		tagNames = nil
	}

	_, err = r.db.Exec(`
		INSERT INTO feishu_resources (id, name, type, tag_ids, tag_names, raw_json, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name = excluded.name,
			type = excluded.type,
			tag_ids = COALESCE(excluded.tag_ids, feishu_resources.tag_ids),
			tag_names = COALESCE(excluded.tag_names, feishu_resources.tag_names),
			raw_json = COALESCE(excluded.raw_json, feishu_resources.raw_json),
			updated_at = excluded.updated_at
	`, item.ID, item.Name, item.Type, tagIDs, tagNames, rawJSON, item.CreatedAt, now)
	return err
}

func (r *FeishuResourcesRepository) ReplaceAll(items []storage.FeishuResourceItem) (int, error) {
	now := time.Now().Format(time.RFC3339)
	tx, err := r.db.Begin()
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec(`DELETE FROM feishu_resources`); err != nil {
		return 0, err
	}

	count := 0
	for _, it := range items {
		it.ID = strings.TrimSpace(it.ID)
		it.Name = strings.TrimSpace(it.Name)
		it.Type = strings.TrimSpace(it.Type)
		if it.ID == "" {
			continue
		}
		if it.Type == "" {
			it.Type = "ip"
		}
		var rawJSON interface{}
		if it.RawJSON != "" {
			rawJSON = it.RawJSON
		} else {
			rawJSON = nil
		}
		var tagIDs interface{}
		if it.TagIDs != "" {
			tagIDs = it.TagIDs
		} else {
			tagIDs = nil
		}
		var tagNames interface{}
		if it.TagNames != "" {
			tagNames = it.TagNames
		} else {
			tagNames = nil
		}
		if _, err := tx.Exec(`
			INSERT INTO feishu_resources (id, name, type, tag_ids, tag_names, raw_json, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		`, it.ID, it.Name, it.Type, tagIDs, tagNames, rawJSON, now, now); err != nil {
			return count, err
		}
		count++
	}

	if err := tx.Commit(); err != nil {
		return count, err
	}
	return count, nil
}

func (r *FeishuResourcesRepository) GetState() (storage.FeishuResourceState, error) {
	var (
		lastRefreshAt    sql.NullString
		lastRefreshCount sql.NullInt64
		scheduleEnabled  sql.NullInt64
	)
	err := r.db.QueryRow(`
		SELECT last_refresh_at, last_refresh_count, schedule_enabled
		FROM feishu_resources_state
		LIMIT 1
	`).Scan(&lastRefreshAt, &lastRefreshCount, &scheduleEnabled)
	if err != nil {
		if err == sql.ErrNoRows {
			// First access - ensure row exists
			_, _ = r.db.Exec(`INSERT OR IGNORE INTO feishu_resources_state (id, last_refresh_at, last_refresh_count, schedule_enabled) VALUES (1, '0001-01-01T00:00:00Z', 0, 0)`)
			return storage.FeishuResourceState{LastRefreshAt: "", LastRefreshCount: 0, ScheduleEnabled: false}, nil
		}
		return storage.FeishuResourceState{}, err
	}
	state := storage.FeishuResourceState{}
	if lastRefreshAt.Valid {
		state.LastRefreshAt = lastRefreshAt.String
	}
	if lastRefreshCount.Valid {
		state.LastRefreshCount = int(lastRefreshCount.Int64)
	}
	state.ScheduleEnabled = scheduleEnabled.Int64 != 0
	return state, nil
}

func (r *FeishuResourcesRepository) UpdateState(lastRefreshAt string, count int, scheduleEnabled *bool) error {
	// Ensure row exists
	if _, err := r.db.Exec(`INSERT OR IGNORE INTO feishu_resources_state (id, last_refresh_at, last_refresh_count, schedule_enabled) VALUES (1, '0001-01-01T00:00:00Z', 0, 0)`); err != nil {
		return err
	}

	if scheduleEnabled != nil {
		if lastRefreshAt != "" {
			_, err := r.db.Exec(`UPDATE feishu_resources_state SET last_refresh_at = ?, last_refresh_count = ?, schedule_enabled = ? WHERE id = 1`,
				lastRefreshAt, count, boolToInt(*scheduleEnabled))
			return err
		}
		_, err := r.db.Exec(`UPDATE feishu_resources_state SET schedule_enabled = ? WHERE id = 1`, boolToInt(*scheduleEnabled))
		return err
	}
	if lastRefreshAt != "" {
		_, err := r.db.Exec(`UPDATE feishu_resources_state SET last_refresh_at = ?, last_refresh_count = ? WHERE id = 1`, lastRefreshAt, count)
		return err
	}
	return nil
}
