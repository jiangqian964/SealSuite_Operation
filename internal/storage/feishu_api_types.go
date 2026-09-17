package storage

type FeishuAPIItem struct {
	ID        string `yaml:"id" json:"id"`
	Name      string `yaml:"name" json:"name"`
	AppID     string `yaml:"app_id" json:"app_id"`
	AppSecret string `yaml:"app_secret" json:"app_secret,omitempty"`
	BaseURL   string `yaml:"base_url" json:"base_url"`
	Enabled   bool   `yaml:"enabled" json:"enabled"`
	CreatedAt string `yaml:"created_at" json:"created_at"`
}

type FeishuAPIFile struct {
	ActiveID string          `yaml:"active_id" json:"active_id"`
	Items    []FeishuAPIItem `yaml:"items" json:"items"`
}
