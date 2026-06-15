package storage

type ConnectionsFile struct {
	Version  int              `yaml:"version"`
	ActiveID string           `yaml:"active_id"`
	Items    []ConnectionItem `yaml:"items"`
}

type ConnectionItem struct {
	ID          string `yaml:"id"`
	Name        string `yaml:"name"`
	Scheme      string `yaml:"scheme"`
	Host        string `yaml:"host"`
	Port        int    `yaml:"port"`
	AccessKeyID string `yaml:"access_key_id"`
	SecretRef   string `yaml:"secret_ref"`
	CreatedAt   string `yaml:"created_at"`
}
