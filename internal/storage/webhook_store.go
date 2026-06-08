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

type WebhookItem struct {
	ID         string            `yaml:"id" json:"id"`
	Name       string            `yaml:"name" json:"name"`
	Provider   string            `yaml:"provider" json:"provider"`
	URL        string            `yaml:"url" json:"url"`
	Method     string            `yaml:"method" json:"method"`
	Headers    map[string]string `yaml:"headers" json:"headers"`
	AuthType   string            `yaml:"auth_type" json:"auth_type"`
	BodyTmpl   string            `yaml:"body_template" json:"body_template"`
	TimeoutSec int               `yaml:"timeout_sec" json:"timeout_sec"`
	RetryCount int               `yaml:"retry_count" json:"retry_count"`
	Enabled    bool              `yaml:"enabled" json:"enabled"`
	CreatedAt  string            `yaml:"created_at" json:"created_at"`
}

type WebhookFile struct {
	Items []WebhookItem `yaml:"items" json:"items"`
}

type WebhookStore struct {
	path string
	mu   sync.Mutex
}

func NewWebhookStore(path string) *WebhookStore {
	return &WebhookStore{path: path}
}

func (s *WebhookStore) Load() (*WebhookFile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadLocked()
}

func (s *WebhookStore) Save(f *WebhookFile) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveLocked(f)
}

func (s *WebhookStore) Get(id string) (*WebhookItem, bool, error) {
	file, err := s.Load()
	if err != nil {
		return nil, false, err
	}
	id = strings.TrimSpace(id)
	for i := range file.Items {
		if file.Items[i].ID != id {
			continue
		}
		item := file.Items[i]
		return &item, true, nil
	}
	return nil, false, nil
}

func (s *WebhookStore) Upsert(item WebhookItem) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	file, err := s.loadLocked()
	if err != nil {
		return err
	}

	item = normalizeWebhookItem(item)
	replaced := false
	for i := range file.Items {
		if file.Items[i].ID != item.ID {
			continue
		}
		old := file.Items[i]
		if item.CreatedAt == "" {
			item.CreatedAt = old.CreatedAt
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
	return s.saveLocked(file)
}

func (s *WebhookStore) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	file, err := s.loadLocked()
	if err != nil {
		return err
	}

	id = strings.TrimSpace(id)
	out := make([]WebhookItem, 0, len(file.Items))
	found := false
	for _, item := range file.Items {
		if item.ID == id {
			found = true
			continue
		}
		out = append(out, item)
	}
	if !found {
		return fmt.Errorf("webhook not found: %s", id)
	}
	file.Items = out
	return s.saveLocked(file)
}

func (s *WebhookStore) loadLocked() (*WebhookFile, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return &WebhookFile{Items: []WebhookItem{}}, nil
		}
		return nil, fmt.Errorf("read webhook file: %w", err)
	}

	var out WebhookFile
	if err := yaml.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("unmarshal webhook yaml: %w", err)
	}
	if out.Items == nil {
		out.Items = []WebhookItem{}
	}
	for i := range out.Items {
		out.Items[i] = normalizeWebhookItem(out.Items[i])
	}
	return &out, nil
}

func (s *WebhookStore) saveLocked(f *WebhookFile) error {
	if f == nil {
		return fmt.Errorf("webhook file is nil")
	}
	for i := range f.Items {
		f.Items[i] = normalizeWebhookItem(f.Items[i])
	}
	data, err := yaml.Marshal(f)
	if err != nil {
		return fmt.Errorf("marshal webhook yaml: %w", err)
	}
	return AtomicWriteFile(s.path, data, 0o644)
}

func normalizeWebhookItem(item WebhookItem) WebhookItem {
	item.ID = strings.TrimSpace(item.ID)
	item.Name = strings.TrimSpace(item.Name)
	item.Provider = normalizeWebhookProvider(item.Provider)
	item.URL = strings.TrimSpace(item.URL)
	item.Method = normalizeWebhookMethod(item.Method)
	item.AuthType = strings.TrimSpace(item.AuthType)
	item.BodyTmpl = strings.TrimSpace(item.BodyTmpl)
	item.Headers = normalizeWebhookHeaders(item.Headers)
	return item
}

func normalizeWebhookProvider(provider string) string {
	provider = strings.ToLower(strings.TrimSpace(provider))
	switch provider {
	case "", "generic":
		return "generic"
	case "feishu_bot":
		return "feishu_bot"
	default:
		return "generic"
	}
}

func normalizeWebhookMethod(method string) string {
	method = strings.ToUpper(strings.TrimSpace(method))
	if method == "" {
		return "POST"
	}
	return method
}

func normalizeWebhookHeaders(headers map[string]string) map[string]string {
	if len(headers) == 0 {
		return map[string]string{}
	}
	keys := make([]string, 0, len(headers))
	clean := make(map[string]string, len(headers))
	for k, v := range headers {
		key := strings.TrimSpace(k)
		if key == "" {
			continue
		}
		if _, ok := clean[key]; ok {
			continue
		}
		keys = append(keys, key)
		clean[key] = strings.TrimSpace(v)
	}
	sort.Strings(keys)
	out := make(map[string]string, len(keys))
	for _, key := range keys {
		out[key] = clean[key]
	}
	return out
}
