package sqlite

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"sealsuite-operation/internal/storage"
)

type ConnectionsRepository struct {
	baseRepo
}

func NewConnectionsRepository(db *sql.DB) *ConnectionsRepository {
	return &ConnectionsRepository{baseRepo: baseRepo{db: db}}
}

func (r *ConnectionsRepository) Load() (*storage.ConnectionsFile, error) {
	rows, err := r.db.Query(`
		SELECT
			id, name, scheme, host, port, access_key_id, secret_ref, created_at, is_active
		FROM app_connections
		ORDER BY created_at ASC, id ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := &storage.ConnectionsFile{
		Version: 1,
		Items:   []storage.ConnectionItem{},
	}
	for rows.Next() {
		var (
			item     storage.ConnectionItem
			isActive int
		)
		if err := rows.Scan(
			&item.ID,
			&item.Name,
			&item.Scheme,
			&item.Host,
			&item.Port,
			&item.AccessKeyID,
			&item.SecretRef,
			&item.CreatedAt,
			&isActive,
		); err != nil {
			return nil, err
		}
		if intToBool(isActive) {
			out.ActiveID = item.ID
		}
		out.Items = append(out.Items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *ConnectionsRepository) Get(id string) (*storage.ConnectionItem, bool, error) {
	row := r.db.QueryRow(`
		SELECT
			id, name, scheme, host, port, access_key_id, secret_ref, created_at
		FROM app_connections
		WHERE id = ?
	`, strings.TrimSpace(id))
	var item storage.ConnectionItem
	if err := row.Scan(
		&item.ID,
		&item.Name,
		&item.Scheme,
		&item.Host,
		&item.Port,
		&item.AccessKeyID,
		&item.SecretRef,
		&item.CreatedAt,
	); err != nil {
		if err == sql.ErrNoRows {
			return nil, false, nil
		}
		return nil, false, err
	}
	return &item, true, nil
}

func (r *ConnectionsRepository) AddAndActivate(item storage.ConnectionItem) error {
	item.ID = strings.TrimSpace(item.ID)
	item.Name = strings.TrimSpace(item.Name)
	item.Scheme = strings.TrimSpace(item.Scheme)
	item.Host = strings.TrimSpace(item.Host)
	item.AccessKeyID = strings.TrimSpace(item.AccessKeyID)
	item.SecretRef = strings.TrimSpace(item.SecretRef)
	if item.CreatedAt == "" {
		item.CreatedAt = time.Now().Format(time.RFC3339)
	}
	now := time.Now().Format(time.RFC3339)

	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec(`UPDATE app_connections SET is_active = 0, updated_at = ?`, now); err != nil {
		return err
	}

	var createdAt string
	err = tx.QueryRow(`SELECT created_at FROM app_connections WHERE id = ?`, item.ID).Scan(&createdAt)
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	if strings.TrimSpace(createdAt) == "" {
		createdAt = item.CreatedAt
	}

	if _, err := tx.Exec(`
		INSERT INTO app_connections (
			id, name, scheme, host, port, access_key_id, secret_ref, secret_value, is_active, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, 1, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name = excluded.name,
			scheme = excluded.scheme,
			host = excluded.host,
			port = excluded.port,
			access_key_id = excluded.access_key_id,
			secret_ref = excluded.secret_ref,
			secret_value = excluded.secret_value,
			is_active = 1,
			updated_at = excluded.updated_at
	`, item.ID, item.Name, item.Scheme, item.Host, item.Port, item.AccessKeyID, item.SecretRef, "", createdAt, now); err != nil {
		return err
	}

	if _, err := tx.Exec(`
		DELETE FROM app_connections
		WHERE id IN (
			SELECT id
			FROM app_connections
			ORDER BY datetime(created_at) DESC, id DESC
			LIMIT -1 OFFSET 3
		)
	`); err != nil {
		return err
	}

	return tx.Commit()
}

func (r *ConnectionsRepository) Activate(id string) (*storage.ConnectionItem, error) {
	id = strings.TrimSpace(id)
	item, ok, err := r.Get(id)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("connection not found: %s", id)
	}

	now := time.Now().Format(time.RFC3339)
	tx, err := r.db.Begin()
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec(`UPDATE app_connections SET is_active = 0, updated_at = ?`, now); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(`UPDATE app_connections SET is_active = 1, updated_at = ? WHERE id = ?`, now, id); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return item, nil
}

func (r *ConnectionsRepository) Delete(id string) error {
	id = strings.TrimSpace(id)

	var isActive int
	err := r.db.QueryRow(`SELECT is_active FROM app_connections WHERE id = ?`, id).Scan(&isActive)
	if err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("connection not found: %s", id)
		}
		return err
	}
	if intToBool(isActive) {
		return fmt.Errorf("cannot delete active connection")
	}

	_, err = r.db.Exec(`DELETE FROM app_connections WHERE id = ?`, id)
	return err
}
