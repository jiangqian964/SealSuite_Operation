package sqlite

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"sealsuite-operation/internal/storage"
)

type TemplateRepository struct {
	baseRepo
}

func NewTemplateRepository(db *sql.DB) *TemplateRepository {
	return &TemplateRepository{baseRepo: baseRepo{db: db}}
}

func (r *TemplateRepository) Load() (*storage.TemplatesFile, error) {
	rows, err := r.db.Query(`
		SELECT
			id, name, category, method, path, query_schema_json, path_params_schema_json,
			body_schema_json, description
		FROM api_templates
		ORDER BY created_at ASC, id ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := &storage.TemplatesFile{
		Version:   1,
		Templates: []storage.Template{},
	}
	for rows.Next() {
		item, err := scanTemplate(rows)
		if err != nil {
			return nil, err
		}
		out.Templates = append(out.Templates, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *TemplateRepository) Save(tf *storage.TemplatesFile) error {
	if tf == nil {
		return fmt.Errorf("templates file is nil")
	}
	if err := tf.Validate(); err != nil {
		return err
	}

	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec(`DELETE FROM api_templates`); err != nil {
		return err
	}

	now := time.Now().Format(time.RFC3339)
	for _, tpl := range tf.Templates {
		if _, err := tx.Exec(`
			INSERT INTO api_templates (
				id, name, method, path, category, description,
				query_schema_json, path_params_schema_json, body_schema_json,
				created_at, updated_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, tpl.ID, tpl.Name, tpl.Method, tpl.Path, tpl.Category, strings.TrimSpace(tpl.DryRunQueryParam),
			mustJSON(tpl.QuerySchema), mustJSON(tpl.PathParamsSchema), mustJSON(tpl.BodySchema),
			now, now); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (r *TemplateRepository) Get(id string) (*storage.Template, bool, error) {
	row := r.db.QueryRow(`
		SELECT
			id, name, category, method, path, query_schema_json, path_params_schema_json,
			body_schema_json, description
		FROM api_templates
		WHERE id = ?
	`, strings.TrimSpace(id))
	item, err := scanTemplate(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, false, nil
		}
		return nil, false, err
	}
	return &item, true, nil
}

func (r *TemplateRepository) Upsert(tpl storage.Template) error {
	existing, ok, err := r.Get(tpl.ID)
	if err != nil {
		return err
	}
	createdAt := time.Now().Format(time.RFC3339)
	if ok {
		createdAt = firstExistingTemplateCreatedAt(r.db, existing.ID, createdAt)
	}
	now := time.Now().Format(time.RFC3339)
	_, err = r.db.Exec(`
		INSERT INTO api_templates (
			id, name, method, path, category, description,
			query_schema_json, path_params_schema_json, body_schema_json,
			created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name = excluded.name,
			method = excluded.method,
			path = excluded.path,
			category = excluded.category,
			description = excluded.description,
			query_schema_json = excluded.query_schema_json,
			path_params_schema_json = excluded.path_params_schema_json,
			body_schema_json = excluded.body_schema_json,
			updated_at = excluded.updated_at
	`, tpl.ID, tpl.Name, tpl.Method, tpl.Path, tpl.Category, strings.TrimSpace(tpl.DryRunQueryParam),
		mustJSON(tpl.QuerySchema), mustJSON(tpl.PathParamsSchema), mustJSON(tpl.BodySchema),
		createdAt, now)
	return err
}

func (r *TemplateRepository) Delete(id string) error {
	_, err := r.db.Exec(`DELETE FROM api_templates WHERE id = ?`, strings.TrimSpace(id))
	return err
}

func scanTemplate(s scanner) (storage.Template, error) {
	var (
		item           storage.Template
		querySchemaRaw string
		pathSchemaRaw  string
		bodySchemaRaw  string
		dryRunParam    string
	)
	err := s.Scan(
		&item.ID,
		&item.Name,
		&item.Category,
		&item.Method,
		&item.Path,
		&querySchemaRaw,
		&pathSchemaRaw,
		&bodySchemaRaw,
		&dryRunParam,
	)
	if err != nil {
		return storage.Template{}, err
	}
	if err := scanJSON(querySchemaRaw, "{}", &item.QuerySchema); err != nil {
		return storage.Template{}, err
	}
	if err := scanJSON(pathSchemaRaw, "{}", &item.PathParamsSchema); err != nil {
		return storage.Template{}, err
	}
	if err := scanJSON(bodySchemaRaw, "{}", &item.BodySchema); err != nil {
		return storage.Template{}, err
	}
	item.DryRunQueryParam = strings.TrimSpace(dryRunParam)
	return item, nil
}

func firstExistingTemplateCreatedAt(db *sql.DB, id, fallback string) string {
	var createdAt string
	if err := db.QueryRow(`SELECT created_at FROM api_templates WHERE id = ?`, id).Scan(&createdAt); err != nil {
		return fallback
	}
	if strings.TrimSpace(createdAt) == "" {
		return fallback
	}
	return createdAt
}
