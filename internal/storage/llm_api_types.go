package storage

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
