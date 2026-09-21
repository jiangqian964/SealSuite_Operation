package storage

type FeishuDeviceItem struct {
	ID                  string `json:"id"`
	DID                 string `json:"did"`
	UserID              string `json:"user_id"`
	DeviceName          string `json:"device_name"`
	FullName            string `json:"full_name"`
	DepartmentName      string `json:"department_name"`
	OS                  string `json:"os"`
	ClientIP            string `json:"client_ip"`
	ClientIPLocation    string `json:"client_ip_location"`
	DeviceStatus        string `json:"device_status"`
	NICType             string `json:"nic_type"`
	MacAddrs            string `json:"mac_addrs"`
	IsVM                bool   `json:"is_vm"`
	SerialNumber        string `json:"serial_number"`
	MacAddr             string `json:"mac_addr"`
	IsVirtual           bool   `json:"is_virtual"`
	IsDefault           bool   `json:"is_default"`
	HDDSerialNumbers    string `json:"hdd_serial_numbers"`
	SSDSerialNumbers    string `json:"ssd_serial_numbers"`
	CPUSerialNumber     string `json:"cpu_serial_number"`
	MemSerialNumbers    string `json:"mem_serial_numbers"`
	WindowsADDomainName string `json:"windows_ad_domain_name"`
	GroupsID            string `json:"groups_id"`
	GroupsName          string `json:"groups_name"`
	GroupsMode          string `json:"groups_mode"`
	DeviceType          string `json:"device_type"`
	TrustedStatus       string `json:"trusted_status"`
	// FeishuDeviceRecordID 飞书设备记录 ID，非空表示该设备已导入飞书，可用于更新接口
	FeishuDeviceRecordID string `json:"feishu_device_record_id"`
	CreatedAt            string `json:"created_at"`
	UpdatedAt            string `json:"updated_at"`
}

type FeishuDeviceSyncState struct {
	LastSyncAt             string `json:"last_sync_at"`
	LastSyncCount          int    `json:"last_sync_count"`
	ScheduleEnabled        bool   `json:"schedule_enabled"`
	ScheduleInterval       string `json:"schedule_interval"`
	LastImportAt           string `json:"last_import_at"`
	LastImportCount        int    `json:"last_import_count"`
	ImportScheduleEnabled  bool   `json:"import_schedule_enabled"`
	ImportScheduleInterval string `json:"import_schedule_interval"`
}

type FieldMappingItem struct {
	ID          string `json:"id"`
	SourceField string `json:"source_field"`
	SourceLabel string `json:"source_label"`
	TargetField string `json:"target_field"`
	TargetLabel string `json:"target_label"`
	Enabled     bool   `json:"enabled"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

// DefaultFieldMappings 默认字段映射（飞连字段 -> 飞书设备记录字段）
// 注意：device_system / device_ownership / device_status 为必填 int 字段，
// 在导入逻辑中根据飞连 os / trusted_status 自动转换，不在此映射表中配置。
var DefaultFieldMappings = []FieldMappingItem{
	{SourceField: "serial_number", SourceLabel: "序列号", TargetField: "serial_number", TargetLabel: "序列号", Enabled: true},
	{SourceField: "hdd_serial_numbers", SourceLabel: "硬盘序列号", TargetField: "disk_serial_number", TargetLabel: "磁盘序列号", Enabled: true},
	{SourceField: "mac_addr", SourceLabel: "MAC地址", TargetField: "mac_address", TargetLabel: "MAC地址", Enabled: true},
	{SourceField: "cpu_serial_number", SourceLabel: "CPU序列号", TargetField: "uuid", TargetLabel: "主板UUID", Enabled: false},
}
