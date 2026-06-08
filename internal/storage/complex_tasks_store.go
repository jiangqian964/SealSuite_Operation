package storage

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type ComplexTasksFile struct {
	Version int           `yaml:"version" json:"version"`
	Items   []ComplexTask `yaml:"items" json:"items"`
}

type ComplexTask struct {
	ID            string            `yaml:"id" json:"id"`
	Name          string            `yaml:"name" json:"name"`
	Goal          string            `yaml:"goal,omitempty" json:"goal,omitempty"`
	ExecutionMode string            `yaml:"execution_mode" json:"execution_mode"`
	WebhookConfigID string          `yaml:"webhook_config_id,omitempty" json:"webhook_config_id,omitempty"`
	WebhookEnabled  bool            `yaml:"webhook_enabled,omitempty" json:"webhook_enabled,omitempty"`
	Steps         []ComplexTaskStep `yaml:"steps" json:"steps"`
}

type ComplexTaskStep struct {
	ID     string                 `yaml:"id" json:"id"`
	Type   string                 `yaml:"type" json:"type"`
	Name   string                 `yaml:"name" json:"name"`
	Config map[string]interface{} `yaml:"config,omitempty" json:"config,omitempty"`
}

type ComplexTasksStore struct {
	Path string
}

func (s ComplexTasksStore) Load() (*ComplexTasksFile, error) {
	b, err := os.ReadFile(s.Path)
	if err != nil {
		if os.IsNotExist(err) {
			return &ComplexTasksFile{Version: 1, Items: []ComplexTask{}}, nil
		}
		return nil, fmt.Errorf("read complex tasks file: %w", err)
	}
	var cf ComplexTasksFile
	if err := yaml.Unmarshal(b, &cf); err != nil {
		return nil, fmt.Errorf("unmarshal complex tasks yaml: %w", err)
	}
	if cf.Version == 0 {
		cf.Version = 1
	}
	if cf.Items == nil {
		cf.Items = []ComplexTask{}
	}
	return &cf, nil
}

func (s ComplexTasksStore) Save(cf *ComplexTasksFile) error {
	if cf == nil {
		return fmt.Errorf("complex tasks file is nil")
	}
	if cf.Version == 0 {
		cf.Version = 1
	}
	b, err := yaml.Marshal(cf)
	if err != nil {
		return fmt.Errorf("marshal complex tasks yaml: %w", err)
	}
	return AtomicWriteFile(s.Path, b, 0o644)
}

func (s ComplexTasksStore) Get(id string) (*ComplexTask, bool, error) {
	cf, err := s.Load()
	if err != nil {
		return nil, false, err
	}
	for _, item := range cf.Items {
		if item.ID == id {
			cp := item
			return &cp, true, nil
		}
	}
	return nil, false, nil
}

func (s ComplexTasksStore) Upsert(task ComplexTask) error {
	cf, err := s.Load()
	if err != nil {
		return err
	}
	replaced := false
	for i := range cf.Items {
		if cf.Items[i].ID == task.ID {
			cf.Items[i] = task
			replaced = true
			break
		}
	}
	if !replaced {
		cf.Items = append(cf.Items, task)
	}
	return s.Save(cf)
}

func (s ComplexTasksStore) Delete(id string) error {
	cf, err := s.Load()
	if err != nil {
		return err
	}
	out := make([]ComplexTask, 0, len(cf.Items))
	for _, item := range cf.Items {
		if item.ID != id {
			out = append(out, item)
		}
	}
	cf.Items = out
	return s.Save(cf)
}
