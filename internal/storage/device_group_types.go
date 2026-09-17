package storage

type DeviceGroupItem struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	RawJSON   string `json:"raw_json,omitempty"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type DeviceGroupState struct {
	LastRefreshAt    string `json:"last_refresh_at"`
	LastRefreshCount int    `json:"last_refresh_count"`
}

type DeviceGroupDetail struct {
	Count   int                `json:"count"`
	GroupID string             `json:"group_id"`
	Items   []DeviceDetailItem `json:"items"`
}

type DeviceDetailItem struct {
	DeviceName         string `json:"device_name"`
	OS                 string `json:"os"`
	FullName           string `json:"full_name"`
	DID                string `json:"did"`
	UserID             string `json:"user_id"`
	DepartmentName     string `json:"department_name"`
	DeviceStatus       string `json:"device_status"`
	MacAddrs           string `json:"mac_addrs"`
	SerialNumber       string `json:"serial_number"`
	HDDSerialNumbers   string `json:"hdd_serial_numbers"`
	SSDSerialNumbers   string `json:"ssd_serial_numbers"`
	CPUSerialNumber    string `json:"cpu_serial_number"`
	MemSerialNumbers   string `json:"mem_serial_numbers"`
	GroupID            string `json:"group_id"`
	GroupName          string `json:"group_name"`
}
