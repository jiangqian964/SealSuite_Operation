package storage

const (
	FeishuResourceCreateModeSelect = "select"
	FeishuResourceCreateModeAdd    = "add"
)

type FeishuResourceItem struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Type      string `json:"type"`
	TagIDs    string `json:"tag_ids,omitempty"`
	TagNames  string `json:"tag_names,omitempty"`
	RawJSON   string `json:"raw_json,omitempty"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type FeishuResourceState struct {
	LastRefreshAt    string `json:"last_refresh_at"`
	LastRefreshCount int    `json:"last_refresh_count"`
	ScheduleEnabled  bool   `json:"schedule_enabled"`
}

type FeishuResourcesFile struct {
	Items []FeishuResourceItem `json:"items"`
	State FeishuResourceState  `json:"state"`
}
