package sqlite

import (
	"database/sql"
	"strings"
	"time"

	"sealsuite-operation/internal/storage"
)

type OutputTemplateRepository struct {
	baseRepo
}

type outputTemplateExtras struct {
	Title               string                 `json:"title,omitempty"`
	SourceComplexTaskID string                 `json:"source_complex_task_id,omitempty"`
	OutputConfig        map[string]interface{} `json:"output_config,omitempty"`
	Meta                map[string]interface{} `json:"meta,omitempty"`
}

func NewOutputTemplateRepository(db *sql.DB) *OutputTemplateRepository {
	return &OutputTemplateRepository{baseRepo: baseRepo{db: db}}
}

func (r *OutputTemplateRepository) Load() (*storage.OutputTemplatesFile, error) {
	rows, err := r.db.Query(`
		SELECT
			id, name, format, content_template, description
		FROM output_templates
		ORDER BY created_at ASC, id ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := &storage.OutputTemplatesFile{
		Version: 1,
		Items:   []storage.OutputTemplate{},
	}
	for rows.Next() {
		item, err := scanOutputTemplate(rows)
		if err != nil {
			return nil, err
		}
		out.Items = append(out.Items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *OutputTemplateRepository) Get(id string) (*storage.OutputTemplate, bool, error) {
	row := r.db.QueryRow(`
		SELECT
			id, name, format, content_template, description
		FROM output_templates
		WHERE id = ?
	`, strings.TrimSpace(id))
	item, err := scanOutputTemplate(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, false, nil
		}
		return nil, false, err
	}
	return &item, true, nil
}

func (r *OutputTemplateRepository) Upsert(item storage.OutputTemplate) error {
	createdAt := time.Now().Format(time.RFC3339)
	if existing, ok, err := r.Get(item.ID); err != nil {
		return err
	} else if ok {
		createdAt = firstExistingOutputTemplateCreatedAt(r.db, existing.ID, createdAt)
	}
	now := time.Now().Format(time.RFC3339)

	extras := outputTemplateExtras{
		Title:               item.Title,
		SourceComplexTaskID: item.SourceComplexTaskID,
		OutputConfig:        item.OutputConfig,
		Meta:                item.Meta,
	}

	_, err := r.db.Exec(`
		INSERT INTO output_templates (
			id, name, format, content_template, description, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name = excluded.name,
			format = excluded.format,
			content_template = excluded.content_template,
			description = excluded.description,
			updated_at = excluded.updated_at
	`, item.ID, item.Name, item.Format, item.Content, mustJSON(extras), createdAt, now)
	return err
}

func (r *OutputTemplateRepository) Delete(id string) error {
	_, err := r.db.Exec(`DELETE FROM output_templates WHERE id = ?`, strings.TrimSpace(id))
	return err
}

func scanOutputTemplate(s scanner) (storage.OutputTemplate, error) {
	var (
		item        storage.OutputTemplate
		content     string
		description string
		extras      outputTemplateExtras
	)
	err := s.Scan(&item.ID, &item.Name, &item.Format, &content, &description)
	if err != nil {
		return storage.OutputTemplate{}, err
	}
	item.Content = content
	if err := scanJSON(description, "{}", &extras); err == nil {
		item.Title = extras.Title
		item.SourceComplexTaskID = extras.SourceComplexTaskID
		item.OutputConfig = extras.OutputConfig
		item.Meta = extras.Meta
	} else {
		item.Title = strings.TrimSpace(description)
	}
	if item.OutputConfig == nil {
		item.OutputConfig = map[string]interface{}{}
	}
	if item.Meta == nil {
		item.Meta = map[string]interface{}{}
	}
	return item, nil
}

func firstExistingOutputTemplateCreatedAt(db *sql.DB, id, fallback string) string {
	var createdAt string
	if err := db.QueryRow(`SELECT created_at FROM output_templates WHERE id = ?`, id).Scan(&createdAt); err != nil {
		return fallback
	}
	if strings.TrimSpace(createdAt) == "" {
		return fallback
	}
	return createdAt
}
