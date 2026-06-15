package sqlite

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"sealsuite-operation/internal/storage"
)

type LLMAPIRepository struct {
	baseRepo
}

type llmAPISettings struct {
	Timeout         int                    `json:"timeout"`
	Temperature     float64                `json:"temperature"`
	MaxTokens       int                    `json:"max_tokens"`
	Thinking        bool                   `json:"thinking"`
	ReasoningEffort string                 `json:"reasoning_effort"`
	ResponseFormat  map[string]interface{} `json:"response_format"`
	SystemPrompt    string                 `json:"system_prompt"`
}

func NewLLMAPIRepository(db *sql.DB) *LLMAPIRepository {
	return &LLMAPIRepository{baseRepo: baseRepo{db: db}}
}

func (r *LLMAPIRepository) Load() (*storage.LLMAPIFile, error) {
	rows, err := r.db.Query(`
		SELECT
			id, name, provider, base_url, api_key, model, enabled, is_active,
			tags_json, settings_json, created_at
		FROM llm_apis
		ORDER BY created_at ASC, id ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := &storage.LLMAPIFile{Items: []storage.LLMAPIItem{}}
	for rows.Next() {
		item, isActive, err := scanLLMAPI(rows)
		if err != nil {
			return nil, err
		}
		if isActive {
			out.ActiveID = item.ID
		}
		out.Items = append(out.Items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *LLMAPIRepository) Get(id string) (*storage.LLMAPIItem, bool, error) {
	row := r.db.QueryRow(`
		SELECT
			id, name, provider, base_url, api_key, model, enabled, is_active,
			tags_json, settings_json, created_at
		FROM llm_apis
		WHERE id = ?
	`, strings.TrimSpace(id))
	item, _, err := scanLLMAPI(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, false, nil
		}
		return nil, false, err
	}
	return &item, true, nil
}

func (r *LLMAPIRepository) UpsertAndMaybeActivate(item storage.LLMAPIItem, activate bool) error {
	item.ID = strings.TrimSpace(item.ID)
	item.Name = strings.TrimSpace(item.Name)
	item.Provider = strings.TrimSpace(item.Provider)
	item.BaseURL = strings.TrimSpace(item.BaseURL)
	item.Model = strings.TrimSpace(item.Model)
	item.ReasoningEffort = strings.TrimSpace(item.ReasoningEffort)
	item.SystemPrompt = strings.TrimSpace(item.SystemPrompt)

	existing, ok, err := r.Get(item.ID)
	if err != nil {
		return err
	}
	if ok {
		if item.CreatedAt == "" {
			item.CreatedAt = existing.CreatedAt
		}
		if item.APIKey == "" {
			item.APIKey = existing.APIKey
		}
		if item.ResponseFormat == nil {
			item.ResponseFormat = existing.ResponseFormat
		}
	}
	if item.CreatedAt == "" {
		item.CreatedAt = time.Now().Format(time.RFC3339)
	}
	now := time.Now().Format(time.RFC3339)

	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if activate {
		if _, err := tx.Exec(`UPDATE llm_apis SET is_active = 0, updated_at = ?`, now); err != nil {
			return err
		}
	}

	settings := llmAPISettings{
		Timeout:         item.Timeout,
		Temperature:     item.Temperature,
		MaxTokens:       item.MaxTokens,
		Thinking:        item.Thinking,
		ReasoningEffort: item.ReasoningEffort,
		ResponseFormat:  item.ResponseFormat,
		SystemPrompt:    item.SystemPrompt,
	}

	if _, err := tx.Exec(`
		INSERT INTO llm_apis (
			id, name, provider, base_url, api_key, model, enabled, is_active,
			tags_json, settings_json, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name = excluded.name,
			provider = excluded.provider,
			base_url = excluded.base_url,
			api_key = excluded.api_key,
			model = excluded.model,
			enabled = excluded.enabled,
			is_active = excluded.is_active,
			tags_json = excluded.tags_json,
			settings_json = excluded.settings_json,
			updated_at = excluded.updated_at
	`, item.ID, item.Name, item.Provider, item.BaseURL, item.APIKey, item.Model,
		boolToInt(item.Enabled), boolToInt(activate),
		mustJSONArray(item.Tags), mustJSON(settings), item.CreatedAt, now); err != nil {
		return err
	}

	return tx.Commit()
}

func (r *LLMAPIRepository) Activate(id string) (*storage.LLMAPIItem, error) {
	id = strings.TrimSpace(id)
	item, ok, err := r.Get(id)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("llm api not found: %s", id)
	}

	now := time.Now().Format(time.RFC3339)
	tx, err := r.db.Begin()
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec(`UPDATE llm_apis SET is_active = 0, updated_at = ?`, now); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(`UPDATE llm_apis SET is_active = 1, updated_at = ? WHERE id = ?`, now, id); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return item, nil
}

func (r *LLMAPIRepository) Delete(id string) error {
	id = strings.TrimSpace(id)
	var isActive int
	err := r.db.QueryRow(`SELECT is_active FROM llm_apis WHERE id = ?`, id).Scan(&isActive)
	if err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("llm api not found: %s", id)
		}
		return err
	}
	if intToBool(isActive) {
		return fmt.Errorf("cannot delete active llm api")
	}
	_, err = r.db.Exec(`DELETE FROM llm_apis WHERE id = ?`, id)
	return err
}

func scanLLMAPI(s scanner) (storage.LLMAPIItem, bool, error) {
	var (
		item        storage.LLMAPIItem
		enabled     int
		isActive    int
		tagsJSON    string
		settingsRaw string
		settings    llmAPISettings
	)
	err := s.Scan(
		&item.ID,
		&item.Name,
		&item.Provider,
		&item.BaseURL,
		&item.APIKey,
		&item.Model,
		&enabled,
		&isActive,
		&tagsJSON,
		&settingsRaw,
		&item.CreatedAt,
	)
	if err != nil {
		return storage.LLMAPIItem{}, false, err
	}
	item.Enabled = intToBool(enabled)
	if err := scanJSON(tagsJSON, "[]", &item.Tags); err != nil {
		return storage.LLMAPIItem{}, false, err
	}
	if err := scanJSON(settingsRaw, "{}", &settings); err != nil {
		return storage.LLMAPIItem{}, false, err
	}
	item.Timeout = settings.Timeout
	item.Temperature = settings.Temperature
	item.MaxTokens = settings.MaxTokens
	item.Thinking = settings.Thinking
	item.ReasoningEffort = settings.ReasoningEffort
	item.ResponseFormat = settings.ResponseFormat
	item.SystemPrompt = settings.SystemPrompt
	if item.ResponseFormat == nil {
		item.ResponseFormat = map[string]interface{}{}
	}
	return item, intToBool(isActive), nil
}
