package storage

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type OutputTemplatesFile struct {
	Version int              `yaml:"version" json:"version"`
	Items   []OutputTemplate `yaml:"items" json:"items"`
}

type OutputTemplate struct {
	ID                 string                 `yaml:"id" json:"id"`
	Name               string                 `yaml:"name" json:"name"`
	Format             string                 `yaml:"format" json:"format"`
	Title              string                 `yaml:"title,omitempty" json:"title,omitempty"`
	Content            string                 `yaml:"content,omitempty" json:"content,omitempty"`
	SourceComplexTaskID string                `yaml:"source_complex_task_id,omitempty" json:"source_complex_task_id,omitempty"`
	OutputConfig       map[string]interface{} `yaml:"output_config,omitempty" json:"output_config,omitempty"`
	Meta               map[string]interface{} `yaml:"meta,omitempty" json:"meta,omitempty"`
}

type OutputTemplatesStore struct {
	Path string
}

func (s OutputTemplatesStore) Load() (*OutputTemplatesFile, error) {
	b, err := os.ReadFile(s.Path)
	if err != nil {
		if os.IsNotExist(err) {
			return &OutputTemplatesFile{Version: 1, Items: []OutputTemplate{}}, nil
		}
		return nil, fmt.Errorf("read output templates file: %w", err)
	}
	var tf OutputTemplatesFile
	if err := yaml.Unmarshal(b, &tf); err != nil {
		return nil, fmt.Errorf("unmarshal output templates yaml: %w", err)
	}
	if tf.Version == 0 {
		tf.Version = 1
	}
	if tf.Items == nil {
		tf.Items = []OutputTemplate{}
	}
	return &tf, nil
}

func (s OutputTemplatesStore) Save(tf *OutputTemplatesFile) error {
	if tf == nil {
		return fmt.Errorf("output templates file is nil")
	}
	if tf.Version == 0 {
		tf.Version = 1
	}
	b, err := yaml.Marshal(tf)
	if err != nil {
		return fmt.Errorf("marshal output templates yaml: %w", err)
	}
	return AtomicWriteFile(s.Path, b, 0o644)
}

func (s OutputTemplatesStore) Get(id string) (*OutputTemplate, bool, error) {
	tf, err := s.Load()
	if err != nil {
		return nil, false, err
	}
	for _, item := range tf.Items {
		if item.ID == id {
			cp := item
			return &cp, true, nil
		}
	}
	return nil, false, nil
}

func (s OutputTemplatesStore) Upsert(item OutputTemplate) error {
	tf, err := s.Load()
	if err != nil {
		return err
	}
	replaced := false
	for i := range tf.Items {
		if tf.Items[i].ID == item.ID {
			tf.Items[i] = item
			replaced = true
			break
		}
	}
	if !replaced {
		tf.Items = append(tf.Items, item)
	}
	return s.Save(tf)
}

func (s OutputTemplatesStore) Delete(id string) error {
	tf, err := s.Load()
	if err != nil {
		return err
	}
	out := make([]OutputTemplate, 0, len(tf.Items))
	for _, item := range tf.Items {
		if item.ID != id {
			out = append(out, item)
		}
	}
	tf.Items = out
	return s.Save(tf)
}
