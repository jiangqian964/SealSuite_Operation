package storage

import (
	"fmt"
	"strings"
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

	DefaultQuery map[string]string      `yaml:"default_query,omitempty" json:"default_query,omitempty"`
	DefaultBody  map[string]interface{} `yaml:"default_body,omitempty" json:"default_body,omitempty"`

	DryRunQueryParam string `yaml:"dry_run_query_param,omitempty" json:"dry_run_query_param,omitempty"`
}

func (tf *TemplatesFile) Validate() error {
	if tf.Version == 0 {
		return fmt.Errorf("api templates: version is required")
	}
	seen := map[string]struct{}{}
	for i, t := range tf.Templates {
		if t.ID == "" {
			return fmt.Errorf("api templates: templates[%d].id is required", i)
		}
		if _, ok := seen[t.ID]; ok {
			return fmt.Errorf("api templates: duplicated template id: %s", t.ID)
		}
		seen[t.ID] = struct{}{}

		if t.Method == "" {
			return fmt.Errorf("api templates: templates[%d].method is required", i)
		}
		switch strings.ToUpper(t.Method) {
		case "GET", "POST", "PUT", "PATCH", "DELETE":
		default:
			return fmt.Errorf("api templates: templates[%d].method invalid: %s", i, t.Method)
		}

		if t.Path == "" || !strings.HasPrefix(t.Path, "/") {
			return fmt.Errorf("api templates: templates[%d].path must start with /", i)
		}
	}
	return nil
}
