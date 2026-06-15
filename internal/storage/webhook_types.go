package storage

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
