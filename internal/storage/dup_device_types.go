package storage

const (
	DupDeviceTable             = "dup_devices"
	DupDeviceGroupTable        = "dup_device_groups"
	DupDeviceGroupMemberTable  = "dup_device_group_members"
	DupDeviceCleanupLogTable   = "dup_device_cleanup_logs"
	DupDeviceTaskStateTable    = "dup_device_task_state"
)

// 重复设备匹配级别
const (
	DupMatchLevelHighMatch   = "high_match"   // 高匹配：序列号 + 至少2个硬件标识一致
	DupMatchLevelSerialOnly  = "serial_only"  // 仅序列号一致
)

// 重复分组状态
const (
	DupGroupStatusPending        = "pending"         // 待处理
	DupGroupStatusAutoResolved   = "auto_resolved"   // 自动清理
	DupGroupStatusManualResolved = "manual_resolved" // 手动清理
)

// 清理类型
const (
	DupCleanupTypeAuto   = "auto"   // 自动清理
	DupCleanupTypeManual = "manual" // 手动清理
)

// 清理状态
const (
	DupCleanupStatusSuccess = "success" // 成功
	DupCleanupStatusFailed  = "failed"  // 失败
)

type DupDevice struct {
	DID            string   `json:"did"`
	DeviceName     string   `json:"device_name"`
	SerialNumber   string   `json:"serial_number"`
	HDDSerials     []string `json:"hdd_serials"`
	SSDSerials     []string `json:"ssd_serials"`
	CPUSerial      string   `json:"cpu_serial"`
	MemSerials     []string `json:"mem_serials"`
	MacAddresses   []string `json:"mac_addresses"`
	UpdatedTime    string   `json:"updated_time"`
	RawJSON        string   `json:"raw_json"`
	SyncedAt       string   `json:"synced_at"`
	IsValid        bool     `json:"is_valid"`
}

type DupDeviceGroup struct {
	ID          string `json:"id"`
	MatchKey    string `json:"match_key"`
	MatchValue  string `json:"match_value"`
	MatchLevel  string `json:"match_level"`
	DeviceCount int    `json:"device_count"`
	Status      string `json:"status"`
	DetectedAt  string `json:"detected_at"`
	ResolvedAt  string `json:"resolved_at"`
	RetainedDID string `json:"retained_did"`
}

type DupDeviceGroupMember struct {
	ID           string   `json:"id"`
	GroupID      string   `json:"group_id"`
	DID          string   `json:"did"`
	DeviceName   string   `json:"device_name"`
	SerialNumber string   `json:"serial_number"`
	HDDSerials   []string `json:"hdd_serials"`
	SSDSerials   []string `json:"ssd_serials"`
	CPUSerial    string   `json:"cpu_serial"`
	MemSerials   []string `json:"mem_serials"`
	MacAddresses []string `json:"mac_addresses"`
	UpdatedTime  string   `json:"updated_time"`
	Retained     bool     `json:"retained"`
}

type DupDeviceCleanupLog struct {
	ID            string `json:"id"`
	GroupID       string `json:"group_id"`
	DID           string `json:"did"`
	DeviceName    string `json:"device_name"`
	SerialNumber  string `json:"serial_number"`
	CleanupType   string `json:"cleanup_type"`
	CleanupStatus string `json:"cleanup_status"`
	ErrorMsg      string `json:"error_msg"`
	CleanedAt     string `json:"cleaned_at"`
}

type DupDeviceTaskState struct {
	ID               string `json:"id"`
	LastSyncAt       string `json:"last_sync_at"`
	LastSyncCount    int    `json:"last_sync_count"`
	LastSyncError    string `json:"last_sync_error"`
	LastDetectAt     string `json:"last_detect_at"`
	LastDetectGroups int    `json:"last_detect_groups"`
	LastDetectError  string `json:"last_detect_error"`
	SyncEnabled      bool   `json:"sync_enabled"`
	DetectEnabled    bool   `json:"detect_enabled"`
}

type DupSyncResult struct {
	SyncedCount int    `json:"synced_count"`
	SyncAt      string `json:"sync_at"`
	Error       string `json:"error,omitempty"`
}

type DupDetectResult struct {
	DetectedGroups int    `json:"detected_groups"`
	HighMatchCount int    `json:"high_match_count"`
	SerialOnlyCount int   `json:"serial_only_count"`
	DetectAt       string `json:"detect_at"`
	Error          string `json:"error,omitempty"`
}

type DupCleanupResult struct {
	CleanedCount int    `json:"cleaned_count"`
	FailedCount  int    `json:"failed_count"`
	CleanupAt    string `json:"cleanup_at"`
	Error        string `json:"error,omitempty"`
}

type DupFullResult struct {
	SyncedCount    int    `json:"synced_count"`
	DetectedGroups int    `json:"detected_groups"`
	CleanedCount   int    `json:"cleaned_count"`
	Error          string `json:"error,omitempty"`
}
