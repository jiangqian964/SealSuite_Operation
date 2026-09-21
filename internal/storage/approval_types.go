package storage

// ApprovalConfig 自动化审批配置：白名单分组与待审批推送 Webhook
// 数据库中以单行形式存储（id 固定为 1）。
type ApprovalConfig struct {
	// GroupIDs 白名单设备分组 ID 列表（来自飞连 device_groups）
	GroupIDs []string `json:"group_ids"`
	// WebhookID 待审批任务推送使用的 Webhook 配置 ID
	WebhookID string `json:"webhook_id"`
	// Enabled 是否启用自动化审批
	Enabled bool `json:"enabled"`
}

// ApprovalTaskStatus 审批任务状态
type ApprovalTaskStatus string

const (
	// ApprovalStatusPending 待审批（未命中白名单，等待人工处理）
	ApprovalStatusPending ApprovalTaskStatus = "pending"
	// ApprovalStatusAutoApproved 自动通过（命中白名单，已自动调用飞书新增设备）
	ApprovalStatusAutoApproved ApprovalTaskStatus = "auto_approved"
	// ApprovalStatusApproved 人工通过
	ApprovalStatusApproved ApprovalTaskStatus = "approved"
	// ApprovalStatusRejected 人工驳回
	ApprovalStatusRejected ApprovalTaskStatus = "rejected"
)

// ApprovalTask 设备申报审批任务流水
type ApprovalTask struct {
	// ID 任务唯一 ID
	ID string `json:"id"`
	// EventID 飞书事件 ID，用于幂等去重
	EventID string `json:"event_id"`
	// DeviceIdentifier 用于在飞连中匹配设备的标识（device_id/serial_number/mac_address）
	DeviceIdentifier string `json:"device_identifier"`
	// RawPayload 飞书事件原始 payload JSON
	RawPayload string `json:"raw_payload"`
	// Status 任务状态
	Status ApprovalTaskStatus `json:"status"`
	// FeishuDeviceRecordID 飞书新增设备接口返回的 device_record_id
	FeishuDeviceRecordID string `json:"feishu_device_record_id"`
	// WebhookDelivery Webhook 投递结果（JSON 字符串，仅未命中白名单时有值）
	WebhookDelivery string `json:"webhook_delivery"`
	// ErrorMessage 处理过程中的错误信息
	ErrorMessage string `json:"error_message"`
	// CreatedAt 创建时间（RFC3339）
	CreatedAt string `json:"created_at"`
	// UpdatedAt 更新时间（RFC3339）
	UpdatedAt string `json:"updated_at"`
}
