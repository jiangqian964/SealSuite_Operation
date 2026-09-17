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
	CreatedAt           string `json:"created_at"`
	UpdatedAt           string `json:"updated_at"`
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

var DefaultFieldMappings = []FieldMappingItem{
	{SourceField: "os", SourceLabel: "操作系统", TargetField: "device_system", TargetLabel: "设备系统", Enabled: false},
	{SourceField: "serial_number", SourceLabel: "序列号", TargetField: "serial_number", TargetLabel: "序列号", Enabled: false},
	{SourceField: "mac_addr", SourceLabel: "MAC地址", TargetField: "mac_address", TargetLabel: "MAC地址", Enabled: false},
	{SourceField: "device_type", SourceLabel: "设备类型", TargetField: "device_type", TargetLabel: "设备类型", Enabled: false},
	{SourceField: "trusted_status", SourceLabel: "信任状态", TargetField: "trusted_status", TargetLabel: "信任状态", Enabled: false},
	{SourceField: "user_id", SourceLabel: "用户ID", TargetField: "user_id", TargetLabel: "用户ID", Enabled: false},
	{SourceField: "client_ip", SourceLabel: "客户端IP", TargetField: "ip_address", TargetLabel: "IP地址", Enabled: false},
	{SourceField: "groups_name", SourceLabel: "分组名称", TargetField: "group_name", TargetLabel: "分组名称", Enabled: false},
}
