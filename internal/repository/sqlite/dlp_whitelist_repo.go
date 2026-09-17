package sqlite

import (
	"database/sql"
	"strings"
	"time"

	"sealsuite-operation/internal/storage"
)

type DLPWhitelistRepository struct {
	baseRepo
}

func NewDLPWhitelistRepository(db *sql.DB) *DLPWhitelistRepository {
	return &DLPWhitelistRepository{baseRepo: baseRepo{db: db}}
}

func (r *DLPWhitelistRepository) List() ([]storage.DLPWhitelistItem, error) {
	rows, err := r.db.Query(`
		SELECT id, match_type, match_value, description, created_at
		FROM dlp_whitelist
		ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]storage.DLPWhitelistItem, 0)
	for rows.Next() {
		item, err := scanDLPWhitelistItem(rows)
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

func (r *DLPWhitelistRepository) Get(id string) (storage.DLPWhitelistItem, bool, error) {
	row := r.db.QueryRow(`
		SELECT id, match_type, match_value, description, created_at
		FROM dlp_whitelist WHERE id = ?
	`, strings.TrimSpace(id))

	item, err := scanDLPWhitelistItem(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return storage.DLPWhitelistItem{}, false, nil
		}
		return storage.DLPWhitelistItem{}, false, err
	}
	return item, true, nil
}

func (r *DLPWhitelistRepository) Upsert(item storage.DLPWhitelistItem) error {
	now := time.Now().Format(time.RFC3339)
	if item.CreatedAt == "" {
		item.CreatedAt = now
	}

	_, err := r.db.Exec(`
		INSERT INTO dlp_whitelist (id, match_type, match_value, description, created_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			match_type = excluded.match_type,
			match_value = excluded.match_value,
			description = excluded.description
	`, item.ID, item.MatchType, item.MatchValue, item.Description, item.CreatedAt)
	return err
}

func (r *DLPWhitelistRepository) Delete(id string) error {
	_, err := r.db.Exec(`DELETE FROM dlp_whitelist WHERE id = ?`, strings.TrimSpace(id))
	return err
}

func (r *DLPWhitelistRepository) MatchFile(filePath, fileName string) (storage.DLPWhitelistItem, bool, error) {
	items, err := r.List()
	if err != nil {
		return storage.DLPWhitelistItem{}, false, err
	}

	for _, item := range items {
		if item.Match(filePath, fileName) {
			return item, true, nil
		}
	}
	return storage.DLPWhitelistItem{}, false, nil
}

func scanDLPWhitelistItem(s scanner) (storage.DLPWhitelistItem, error) {
	var item storage.DLPWhitelistItem
	err := s.Scan(&item.ID, &item.MatchType, &item.MatchValue, &item.Description, &item.CreatedAt)
	if err != nil {
		return storage.DLPWhitelistItem{}, err
	}
	return item, nil
}
