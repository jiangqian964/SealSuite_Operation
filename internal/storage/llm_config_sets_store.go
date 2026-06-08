package storage

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

type LLMConfigSetItem struct {
	ID        string                 `yaml:"id" json:"id"`
	Name      string                 `yaml:"name" json:"name"`
	Planner   map[string]interface{} `yaml:"planner" json:"planner"`
	Formatter map[string]interface{} `yaml:"formatter" json:"formatter"`
	CreatedAt string                 `yaml:"created_at" json:"created_at"`
}

type LLMConfigSetsFile struct {
	Version  int                `yaml:"version" json:"version"`
	ActiveID string             `yaml:"active_id" json:"active_id"`
	Items    []LLMConfigSetItem `yaml:"items" json:"items"`
}

type LLMConfigSetsStore struct {
	path string
	mu   sync.Mutex
}

func NewLLMConfigSetsStore(path string) *LLMConfigSetsStore {
	return &LLMConfigSetsStore{path: path}
}

func (s *LLMConfigSetsStore) Load() (*LLMConfigSetsFile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadLocked()
}

func (s *LLMConfigSetsStore) Save(f *LLMConfigSetsFile) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveLocked(f)
}

func (s *LLMConfigSetsStore) Get(id string) (*LLMConfigSetItem, bool, error) {
	file, err := s.Load()
	if err != nil {
		return nil, false, err
	}
	for _, item := range file.Items {
		if item.ID == id {
			cp := item
			cp.Planner = cloneMap(item.Planner)
			cp.Formatter = cloneMap(item.Formatter)
			return &cp, true, nil
		}
	}
	return nil, false, nil
}

func (s *LLMConfigSetsStore) UpsertAndMaybeActivate(item LLMConfigSetItem, activate bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	file, err := s.loadLocked()
	if err != nil {
		return err
	}

	item.ID = strings.TrimSpace(item.ID)
	item.Name = strings.TrimSpace(item.Name)
	item.Planner = sanitizeStoredRoleConfig(item.Planner)
	item.Formatter = sanitizeStoredRoleConfig(item.Formatter)

	replaced := false
	for i := range file.Items {
		if file.Items[i].ID != item.ID {
			continue
		}
		old := file.Items[i]
		if item.CreatedAt == "" {
			item.CreatedAt = old.CreatedAt
		}
		item.Planner = mergeStoredRoleConfig(old.Planner, item.Planner)
		item.Formatter = mergeStoredRoleConfig(old.Formatter, item.Formatter)
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

func (s *LLMConfigSetsStore) Activate(id string) (*LLMConfigSetItem, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	file, err := s.loadLocked()
	if err != nil {
		return nil, err
	}
	for _, item := range file.Items {
		if item.ID == id {
			file.ActiveID = id
			if err := s.saveLocked(file); err != nil {
				return nil, err
			}
			cp := item
			cp.Planner = cloneMap(item.Planner)
			cp.Formatter = cloneMap(item.Formatter)
			return &cp, nil
		}
	}
	return nil, fmt.Errorf("llm config set not found: %s", id)
}

func (s *LLMConfigSetsStore) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	file, err := s.loadLocked()
	if err != nil {
		return err
	}
	if file.ActiveID == id {
		return fmt.Errorf("cannot delete active llm config set")
	}
	out := make([]LLMConfigSetItem, 0, len(file.Items))
	for _, item := range file.Items {
		if item.ID != id {
			out = append(out, item)
		}
	}
	file.Items = out
	return s.saveLocked(file)
}

func (s *LLMConfigSetsStore) loadLocked() (*LLMConfigSetsFile, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return &LLMConfigSetsFile{Version: 1, Items: []LLMConfigSetItem{}}, nil
		}
		return nil, fmt.Errorf("read llm config sets file: %w", err)
	}
	var out LLMConfigSetsFile
	if err := yaml.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("unmarshal llm config sets yaml: %w", err)
	}
	if out.Version == 0 {
		out.Version = 1
	}
	if out.Items == nil {
		out.Items = []LLMConfigSetItem{}
	}
	return &out, nil
}

func (s *LLMConfigSetsStore) saveLocked(f *LLMConfigSetsFile) error {
	if f == nil {
		return fmt.Errorf("llm config sets file is nil")
	}
	if f.Version == 0 {
		f.Version = 1
	}
	data, err := yaml.Marshal(f)
	if err != nil {
		return fmt.Errorf("marshal llm config sets yaml: %w", err)
	}
	return AtomicWriteFile(s.path, data, 0o644)
}

func sanitizeStoredRoleConfig(in map[string]interface{}) map[string]interface{} {
	if in == nil {
		return map[string]interface{}{}
	}
	return cloneMap(in)
}

func mergeStoredRoleConfig(current, next map[string]interface{}) map[string]interface{} {
	if current == nil {
		current = map[string]interface{}{}
	}
	if next == nil {
		return cloneMap(current)
	}
	out := cloneMap(current)
	mergeRoleConfig(out, cloneMap(next))
	return out
}
