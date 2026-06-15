package db

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"sealsuite-operation/internal/storage"

	"gopkg.in/yaml.v3"
)

const legacyJobsYAMLImportMetaKey = "bootstrap.import.jobs_yaml.v1"

func BootstrapLegacyJobsFromYAML(db *sql.DB, path string) error {
	if db == nil {
		return fmt.Errorf("sqlite db is nil")
	}

	path = strings.TrimSpace(path)
	if path == "" {
		path = "jobs.yaml"
	}

	imported, err := metaValueExists(db, legacyJobsYAMLImportMetaKey)
	if err != nil {
		return err
	}
	if imported {
		return nil
	}

	existingCount, err := legacyJobCount(db)
	if err != nil {
		return err
	}
	if existingCount > 0 {
		return markLegacyJobsYAMLImported(db)
	}

	jf, err := loadLegacyJobsYAML(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	return importLegacyJobsFromYAML(db, jf)
}

func loadLegacyJobsYAML(path string) (*storage.JobsFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var out storage.JobsFile
	if err := yaml.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("unmarshal jobs yaml: %w", err)
	}
	if err := out.Validate(); err != nil {
		return nil, err
	}
	return &out, nil
}

func importLegacyJobsFromYAML(db *sql.DB, jf *storage.JobsFile) error {
	if jf == nil {
		return fmt.Errorf("legacy jobs yaml is nil")
	}
	if err := jf.Validate(); err != nil {
		return err
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec(`DELETE FROM legacy_jobs`); err != nil {
		return err
	}

	now := time.Now().Format(time.RFC3339)
	for _, job := range jf.Jobs {
		if _, err := tx.Exec(`
			INSERT INTO legacy_jobs (
				name, enabled, cron_expr, type, params_json, created_at, updated_at
			) VALUES (?, ?, ?, ?, ?, ?, ?)
		`, job.Name, boolToInt(job.Enabled), job.Cron, job.Type, mustJSON(job.Params), now, now); err != nil {
			return err
		}
	}

	if _, err := tx.Exec(`
		INSERT INTO app_meta (meta_key, meta_value, updated_at)
		VALUES (?, ?, ?)
		ON CONFLICT(meta_key) DO UPDATE SET
			meta_value = excluded.meta_value,
			updated_at = excluded.updated_at
	`, legacyJobsYAMLImportMetaKey, now, now); err != nil {
		return err
	}

	return tx.Commit()
}

func markLegacyJobsYAMLImported(db *sql.DB) error {
	now := time.Now().Format(time.RFC3339)
	_, err := db.Exec(`
		INSERT INTO app_meta (meta_key, meta_value, updated_at)
		VALUES (?, ?, ?)
		ON CONFLICT(meta_key) DO UPDATE SET
			meta_value = excluded.meta_value,
			updated_at = excluded.updated_at
	`, legacyJobsYAMLImportMetaKey, now, now)
	return err
}

func legacyJobCount(db *sql.DB) (int, error) {
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM legacy_jobs`).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func metaValueExists(db *sql.DB, key string) (bool, error) {
	var value string
	err := db.QueryRow(`SELECT meta_value FROM app_meta WHERE meta_key = ?`, strings.TrimSpace(key)).Scan(&value)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(value) != "", nil
}

func mustJSON(v interface{}) string {
	if v == nil {
		return "{}"
	}
	b, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(b)
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
