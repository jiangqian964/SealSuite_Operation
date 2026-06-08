package storage

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

type TemplatesFile struct {
	Version   int        `yaml:"version" json:"version"`
	Templates []Template `yaml:"templates" json:"templates"`
}

type Template struct {
	ID       string `yaml:"id" json:"id"`
	Name     string `yaml:"name" json:"name"`
	Category string `yaml:"category" json:"category"`
	Method   string `yaml:"method" json:"method"`
	Path     string `yaml:"path" json:"path"`

	QuerySchema      map[string]interface{} `yaml:"query_schema,omitempty" json:"query_schema,omitempty"`
	PathParamsSchema map[string]interface{} `yaml:"path_params_schema,omitempty" json:"path_params_schema,omitempty"`
	BodySchema       map[string]interface{} `yaml:"body_schema,omitempty" json:"body_schema,omitempty"`

	// 若飞连 API 支持 dry-run，可在模板中声明 dry_run 的 query 参数名（例如 "dry_run"）
	DryRunQueryParam string `yaml:"dry_run_query_param,omitempty" json:"dry_run_query_param,omitempty"`
}

func (tf *TemplatesFile) Validate() error {
	if tf.Version == 0 {
		return fmt.Errorf("api-templates.yaml: version is required")
	}
	seen := map[string]struct{}{}
	for i, t := range tf.Templates {
		if t.ID == "" {
			return fmt.Errorf("api-templates.yaml: templates[%d].id is required", i)
		}
		if _, ok := seen[t.ID]; ok {
			return fmt.Errorf("api-templates.yaml: duplicated template id: %s", t.ID)
		}
		seen[t.ID] = struct{}{}

		if t.Method == "" {
			return fmt.Errorf("api-templates.yaml: templates[%d].method is required", i)
		}
		switch strings.ToUpper(t.Method) {
		case "GET", "POST", "PUT", "PATCH", "DELETE":
		default:
			return fmt.Errorf("api-templates.yaml: templates[%d].method invalid: %s", i, t.Method)
		}

		if t.Path == "" || !strings.HasPrefix(t.Path, "/") {
			return fmt.Errorf("api-templates.yaml: templates[%d].path must start with /", i)
		}
	}
	return nil
}

type TemplatesStore struct {
	Path string
}

func (s TemplatesStore) Load() (*TemplatesFile, error) {
	b, err := os.ReadFile(s.Path)
	if err != nil {
		if os.IsNotExist(err) {
			return &TemplatesFile{Version: 1, Templates: []Template{}}, nil
		}
		return nil, fmt.Errorf("read templates file: %w", err)
	}
	var tf TemplatesFile
	if err := yaml.Unmarshal(b, &tf); err != nil {
		return nil, fmt.Errorf("unmarshal templates yaml: %w", err)
	}
	if err := tf.Validate(); err != nil {
		return nil, err
	}
	return &tf, nil
}

func (s TemplatesStore) Save(tf *TemplatesFile) error {
	if tf == nil {
		return fmt.Errorf("templates file is nil")
	}
	if err := tf.Validate(); err != nil {
		return err
	}
	b, err := yaml.Marshal(tf)
	if err != nil {
		return fmt.Errorf("marshal templates yaml: %w", err)
	}
	return AtomicWriteFile(s.Path, b, 0o644)
}

func (s TemplatesStore) Get(id string) (*Template, bool, error) {
	tf, err := s.Load()
	if err != nil {
		return nil, false, err
	}
	for _, t := range tf.Templates {
		if t.ID == id {
			cp := t
			return &cp, true, nil
		}
	}
	return nil, false, nil
}

func (s TemplatesStore) Upsert(tpl Template) error {
	tf, err := s.Load()
	if err != nil {
		return err
	}
	replaced := false
	for i := range tf.Templates {
		if tf.Templates[i].ID == tpl.ID {
			tf.Templates[i] = tpl
			replaced = true
			break
		}
	}
	if !replaced {
		tf.Templates = append(tf.Templates, tpl)
	}
	return s.Save(tf)
}

func (s TemplatesStore) Delete(id string) error {
	tf, err := s.Load()
	if err != nil {
		return err
	}
	out := make([]Template, 0, len(tf.Templates))
	for _, t := range tf.Templates {
		if t.ID != id {
			out = append(out, t)
		}
	}
	tf.Templates = out
	return s.Save(tf)
}
