package storage

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

type LLMAPIItem struct {
	ID              string                 `yaml:"id" json:"id"`
	Name            string                 `yaml:"name" json:"name"`
	Tags            []string               `yaml:"tags" json:"tags"`
	Enabled         bool                   `yaml:"enabled" json:"enabled"`
	Provider        string                 `yaml:"provider" json:"provider"`
	BaseURL         string                 `yaml:"base_url" json:"base_url"`
	APIKey          string                 `yaml:"api_key" json:"api_key"`
	Model           string                 `yaml:"model" json:"model"`
	Timeout         int                    `yaml:"timeout" json:"timeout"`
	Temperature     float64                `yaml:"temperature" json:"temperature"`
	MaxTokens       int                    `yaml:"max_tokens" json:"max_tokens"`
	Thinking        bool                   `yaml:"thinking" json:"thinking"`
	ReasoningEffort string                 `yaml:"reasoning_effort" json:"reasoning_effort"`
	ResponseFormat  map[string]interface{} `yaml:"response_format" json:"response_format"`
	SystemPrompt    string                 `yaml:"system_prompt" json:"system_prompt"`
	CreatedAt       string                 `yaml:"created_at" json:"created_at"`
}

type LLMAPIFile struct {
	ActiveID string       `yaml:"active_id" json:"active_id"`
	Items    []LLMAPIItem `yaml:"items" json:"items"`
}

type LLMAPIStore struct {
	path string
	mu   sync.Mutex
}

func NewLLMAPIStore(path string) *LLMAPIStore {
	return &LLMAPIStore{path: path}
}

func (s *LLMAPIStore) Load() (*LLMAPIFile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadLocked()
}

func (s *LLMAPIStore) Save(f *LLMAPIFile) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveLocked(f)
}

func (s *LLMAPIStore) UpsertAndMaybeActivate(item LLMAPIItem, activate bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	file, err := s.loadLocked()
	if err != nil {
		return err
	}

	item = normalizeLLMAPIItem(item)
	replaced := false
	for i := range file.Items {
		if file.Items[i].ID != item.ID {
			continue
		}
		old := file.Items[i]
		if item.CreatedAt == "" {
			item.CreatedAt = old.CreatedAt
		}
		if item.APIKey == "" {
			item.APIKey = old.APIKey
		}
		if item.ResponseFormat == nil {
			item.ResponseFormat = cloneMap(old.ResponseFormat)
		}
		file.Items[i] = item
		replaced = true
		break
	}

	if !replaced {
		if item.CreatedAt == "" {
			item.CreatedAt = time.Now().Format(time.RFC3339)
		}
		file.Items = append(file.Items, item)
	}

	if activate {
		file.ActiveID = item.ID
	}
	return s.saveLocked(file)
}

func (s *LLMAPIStore) Activate(id string) (*LLMAPIItem, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	file, err := s.loadLocked()
	if err != nil {
		return nil, err
	}
	id = strings.TrimSpace(id)
	for _, item := range file.Items {
		if item.ID == id {
			file.ActiveID = id
			if err := s.saveLocked(file); err != nil {
				return nil, err
			}
			cp := item
			cp.Tags = append([]string(nil), item.Tags...)
			cp.ResponseFormat = cloneMap(item.ResponseFormat)
			return &cp, nil
		}
	}
	return nil, fmt.Errorf("llm api not found: %s", id)
}

func (s *LLMAPIStore) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	file, err := s.loadLocked()
	if err != nil {
		return err
	}
	id = strings.TrimSpace(id)
	if file.ActiveID == id {
		return fmt.Errorf("cannot delete active llm api")
	}
	out := make([]LLMAPIItem, 0, len(file.Items))
	for _, item := range file.Items {
		if item.ID != id {
			out = append(out, item)
		}
	}
	file.Items = out
	return s.saveLocked(file)
}

func (s *LLMAPIStore) loadLocked() (*LLMAPIFile, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return &LLMAPIFile{Items: []LLMAPIItem{}}, nil
		}
		return nil, fmt.Errorf("read llm api file: %w", err)
	}
	var out LLMAPIFile
	if err := yaml.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("unmarshal llm api yaml: %w", err)
	}
	if out.Items == nil {
		out.Items = []LLMAPIItem{}
	}
	for i := range out.Items {
		out.Items[i] = normalizeLLMAPIItem(out.Items[i])
	}
	return &out, nil
}

func (s *LLMAPIStore) saveLocked(f *LLMAPIFile) error {
	if f == nil {
		return fmt.Errorf("llm api file is nil")
	}
	for i := range f.Items {
		f.Items[i] = normalizeLLMAPIItem(f.Items[i])
	}
	data, err := yaml.Marshal(f)
	if err != nil {
		return fmt.Errorf("marshal llm api yaml: %w", err)
	}
	return AtomicWriteFile(s.path, data, 0o644)
}

func normalizeLLMAPIItem(item LLMAPIItem) LLMAPIItem {
	item.ID = strings.TrimSpace(item.ID)
	item.Name = strings.TrimSpace(item.Name)
	item.Tags = normalizeLLMAPITags(item.Tags)
	item.Provider = strings.TrimSpace(item.Provider)
	item.BaseURL = strings.TrimSpace(item.BaseURL)
	item.Model = strings.TrimSpace(item.Model)
	item.ReasoningEffort = strings.TrimSpace(item.ReasoningEffort)
	item.SystemPrompt = strings.TrimSpace(item.SystemPrompt)
	if item.ResponseFormat != nil {
		item.ResponseFormat = cloneMap(item.ResponseFormat)
	}
	return item
}

func normalizeLLMAPITags(tags []string) []string {
	if len(tags) == 0 {
		return []string{}
	}
	seen := make(map[string]struct{}, len(tags))
	out := make([]string, 0, len(tags))
	for _, raw := range tags {
		tag := strings.TrimSpace(raw)
		if tag == "" {
			continue
		}
		if _, ok := seen[tag]; ok {
			continue
		}
		seen[tag] = struct{}{}
		out = append(out, tag)
	}
	sort.Strings(out)
	return out
}
