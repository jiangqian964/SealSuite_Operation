package storage

type WebhookItem struct {
	ID       string            `yaml:"id" json:"id"`
	Name     string            `yaml:"name" json:"name"`
	Provider string            `yaml:"provider" json:"provider"`
	URL      string            `yaml:"url" json:"url"`
	Method   string            `yaml:"method" json:"method"`
	Headers  map[string]string `yaml:"headers" json:"headers"`
	AuthType string            `yaml:"auth_type" json:"auth_type"`
	BodyTmpl string            `yaml:"body_template" json:"body_template"`

	// Secret 飞书自定义群机器人“签名校验（加签）”密钥。
	// 当机器人安全设置启用了签名校验时必填，系统会据此计算 timestamp 与 sign；未启用可留空。
	Secret string `yaml:"secret" json:"secret"`
	// MsgType 飞书机器人消息类型：text（纯文本）/ post（富文本）/ interactive（交互卡片）。
	// 仅在 provider=feishu_bot 时生效。
	MsgType string `yaml:"msg_type" json:"msg_type"`

	TimeoutSec int    `yaml:"timeout_sec" json:"timeout_sec"`
	RetryCount int    `yaml:"retry_count" json:"retry_count"`
	Enabled    bool   `yaml:"enabled" json:"enabled"`
	CreatedAt  string `yaml:"created_at" json:"created_at"`
}

type WebhookFile struct {
	Items []WebhookItem `yaml:"items" json:"items"`
}
