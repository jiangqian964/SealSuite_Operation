package storage

type ComplexTasksFile struct {
	Version int           `yaml:"version" json:"version"`
	Items   []ComplexTask `yaml:"items" json:"items"`
}

type ComplexTask struct {
	ID              string            `yaml:"id" json:"id"`
	Name            string            `yaml:"name" json:"name"`
	Goal            string            `yaml:"goal,omitempty" json:"goal,omitempty"`
	ExecutionMode   string            `yaml:"execution_mode" json:"execution_mode"`
	WebhookConfigID string            `yaml:"webhook_config_id,omitempty" json:"webhook_config_id,omitempty"`
	WebhookEnabled  bool              `yaml:"webhook_enabled,omitempty" json:"webhook_enabled,omitempty"`
	Steps           []ComplexTaskStep `yaml:"steps" json:"steps"`
}

type ComplexTaskStep struct {
	ID     string                 `yaml:"id" json:"id"`
	Type   string                 `yaml:"type" json:"type"`
	Name   string                 `yaml:"name" json:"name"`
	Config map[string]interface{} `yaml:"config,omitempty" json:"config,omitempty"`
}
