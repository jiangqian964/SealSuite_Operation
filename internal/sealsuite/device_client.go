package sealsuite

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

// DupDeviceSearchResult 设备搜索结果
type DupDeviceSearchResult struct {
	DID          string   `json:"did"`
	DeviceName   string   `json:"device_name"`
	SerialNumber string   `json:"serial_number"`
	HDDSerials   []string `json:"hdd_serials"`
	SSDSerials   []string `json:"ssd_serials"`
	CPUSerial    string   `json:"cpu_serial"`
	MemSerials   []string `json:"mem_serials"`
	MacAddresses []string `json:"mac_addresses"`
	UpdatedTime  string   `json:"updated_time"`
	RawJSON      string   `json:"raw_json"`
}

// SearchAllDevices 分页获取全量设备信息
// 使用 offset + limit 分页，循环获取直到结果为空
func (c *Client) SearchAllDevices() ([]DupDeviceSearchResult, error) {
	var allDevices []DupDeviceSearchResult
	offset := 0
	limit := 100
	maxPages := 1000 // 防止无限循环

	for i := 0; i < maxPages; i++ {
		params := map[string]string{
			"limit":  fmt.Sprintf("%d", limit),
			"offset": fmt.Sprintf("%d", offset),
		}

		statusCode, body, err := c.DoRaw(http.MethodGet, "/api/open/v1/device/search", params, nil)
		if err != nil {
			return nil, fmt.Errorf("device search offset=%d: %w", offset, err)
		}
		if statusCode != http.StatusOK {
			return nil, fmt.Errorf("device search offset=%d: status=%d body=%s", offset, statusCode, string(body))
		}

		var resp struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
			Data    struct {
				Items   []map[string]interface{} `json:"items"`
				Devices []map[string]interface{} `json:"devices"`
				Total   int                      `json:"total"`
			} `json:"data"`
			Items   []map[string]interface{} `json:"items"`
			Devices []map[string]interface{} `json:"devices"`
			Total   int                      `json:"total"`
		}
		if err := json.Unmarshal(body, &resp); err != nil {
			return nil, fmt.Errorf("device search parse offset=%d: %w", offset, err)
		}

		// 检查业务状态码
		if resp.Code != 0 {
			return nil, fmt.Errorf("device search offset=%d: code=%d message=%s", offset, resp.Code, resp.Message)
		}

		// 获取设备列表（兼容多种返回结构）
		rawItems := resp.Data.Items
		if len(rawItems) == 0 {
			rawItems = resp.Data.Devices
		}
		if len(rawItems) == 0 {
			rawItems = resp.Items
		}
		if len(rawItems) == 0 {
			rawItems = resp.Devices
		}

		if len(rawItems) == 0 {
			break // 没有更多数据了
		}

		// 解析每条设备
		for _, raw := range rawItems {
			device := parseDupDevice(raw)
			if device.DID != "" {
				allDevices = append(allDevices, device)
			}
		}

		// 如果返回数量少于 limit，说明已经到最后一页
		if len(rawItems) < limit {
			break
		}

		offset += limit
	}

	return allDevices, nil
}

// parseDupDevice 解析单条设备数据
func parseDupDevice(raw map[string]interface{}) DupDeviceSearchResult {
	result := DupDeviceSearchResult{}

	// 保存原始 JSON
	rawJSON, _ := json.Marshal(raw)
	result.RawJSON = string(rawJSON)

	// 提取 device_info 嵌套结构
	deviceInfo, _ := raw["device_info"].(map[string]interface{})

	// 辅助函数：从 raw 或 device_info 中获取字段
	getField := func(key string) interface{} {
		if v, ok := raw[key]; ok && v != nil {
			return v
		}
		if deviceInfo != nil {
			if v, ok := deviceInfo[key]; ok && v != nil {
				return v
			}
		}
		return nil
	}

	// DID
	if v := getField("did"); v != nil {
		result.DID = fmt.Sprintf("%v", v)
	} else if v := getField("devices_did"); v != nil {
		result.DID = fmt.Sprintf("%v", v)
	}

	// 设备名称
	if v := getField("device_name"); v != nil {
		result.DeviceName = fmt.Sprintf("%v", v)
	} else if v := getField("name"); v != nil {
		result.DeviceName = fmt.Sprintf("%v", v)
	}

	// 序列号
	if v := getField("serial_number"); v != nil {
		result.SerialNumber = fmt.Sprintf("%v", v)
	} else if v := getField("device_info_serial_number"); v != nil {
		result.SerialNumber = fmt.Sprintf("%v", v)
	}

	// CPU 序列号
	if v := getField("cpu_serial_number"); v != nil {
		result.CPUSerial = fmt.Sprintf("%v", v)
	} else if v := getField("device_info_cpu_serial_number"); v != nil {
		result.CPUSerial = fmt.Sprintf("%v", v)
	}

	// HDD 序列号列表
	if v := getField("hdd_serial_numbers"); v != nil {
		result.HDDSerials = toStringSlice(v)
	} else if v := getField("device_info_hdd_serial_numbers"); v != nil {
		result.HDDSerials = toStringSlice(v)
	}

	// SSD 序列号列表
	if v := getField("ssd_serial_numbers"); v != nil {
		result.SSDSerials = toStringSlice(v)
	} else if v := getField("device_info_ssd_serial_numbers"); v != nil {
		result.SSDSerials = toStringSlice(v)
	}

	// 内存序列号列表
	if v := getField("mem_serial_numbers"); v != nil {
		result.MemSerials = toStringSlice(v)
	} else if v := getField("device_info_mem_serial_numbers"); v != nil {
		result.MemSerials = toStringSlice(v)
	}

	// MAC 地址列表
	if v := getField("mac_addrs"); v != nil {
		result.MacAddresses = toStringSlice(v)
	} else if v := getField("nic_detail_list_mac_addr"); v != nil {
		result.MacAddresses = toStringSlice(v)
	} else if v := getField("nic_detail_list"); v != nil {
		// 从网卡详情列表中提取 MAC 地址
		result.MacAddresses = extractMacFromNicList(v)
	}

	// 更新时间
	if v := getField("updated_time"); v != nil {
		result.UpdatedTime = formatUnixTime(v)
	} else if v := getField("devices_updated_time"); v != nil {
		result.UpdatedTime = formatUnixTime(v)
	}

	return result
}

// toStringSlice 将 interface{} 转换为 []string
func toStringSlice(v interface{}) []string {
	if v == nil {
		return []string{}
	}

	// 如果已经是字符串数组
	if slice, ok := v.([]string); ok {
		return slice
	}

	// 如果是 []interface{}
	if slice, ok := v.([]interface{}); ok {
		result := make([]string, 0, len(slice))
		for _, item := range slice {
			if item != nil {
				result = append(result, fmt.Sprintf("%v", item))
			}
		}
		return result
	}

	// 如果是字符串（逗号分隔）
	if s, ok := v.(string); ok {
		if s == "" {
			return []string{}
		}
		// 尝试解析 JSON 数组
		var arr []string
		if err := json.Unmarshal([]byte(s), &arr); err == nil {
			return arr
		}
		return []string{s}
	}

	return []string{}
}

// extractMacFromNicList 从网卡详情列表中提取 MAC 地址
func extractMacFromNicList(v interface{}) []string {
	if v == nil {
		return []string{}
	}

	slice, ok := v.([]interface{})
	if !ok {
		return []string{}
	}

	var macs []string
	for _, nic := range slice {
		if nicMap, ok := nic.(map[string]interface{}); ok {
			if mac, ok := nicMap["mac_addr"].(string); ok && mac != "" {
				macs = append(macs, mac)
			} else if mac, ok := nicMap["mac_address"].(string); ok && mac != "" {
				macs = append(macs, mac)
			}
		}
	}
	return macs
}

// BatchUpdateDeviceStatus 批量修改设备状态
// status: 目标状态（如 "invalid" 表示失效）
func (c *Client) BatchUpdateDeviceStatus(dids []string, status string) error {
	if len(dids) == 0 {
		return nil
	}

	body := map[string]interface{}{
		"dids":   dids,
		"status": status,
	}

	statusCode, respBody, err := c.DoRaw(http.MethodPost, "/api/open/v1/device/batch/status/update", nil, body)
	if err != nil {
		return fmt.Errorf("batch update device status: %w", err)
	}
	if statusCode != http.StatusOK {
		return fmt.Errorf("batch update device status: status=%d body=%s", statusCode, string(respBody))
	}

	var resp struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return fmt.Errorf("batch update device status parse: %w", err)
	}
	if resp.Code != 0 {
		return fmt.Errorf("batch update device status: code=%d message=%s", resp.Code, resp.Message)
	}

	return nil
}

// BatchDeleteInvalidDevices 批量删除失效设备
// clean_range: 0=终端列表和终端分组信息, 1=软件统计信息, 2=软件分发记录, 3=终端登记工单, 4=补丁统计信息
// 返回: 成功的设备ID列表, 失败的设备ID->错误信息 map, 整体错误
func (c *Client) BatchDeleteInvalidDevices(dids []string) ([]string, map[string]string, error) {
	if len(dids) == 0 {
		return []string{}, map[string]string{}, nil
	}

	body := map[string]interface{}{
		"dids":        dids,
		"clean_range": []int{0, 1, 2, 3, 4},
	}

	statusCode, respBody, err := c.DoRaw(http.MethodPost, "/api/open/v1/device/batch/invalid/delete", nil, body)
	if err != nil {
		return []string{}, map[string]string{}, fmt.Errorf("batch delete invalid devices: %w", err)
	}
	if statusCode != http.StatusOK {
		return []string{}, map[string]string{}, fmt.Errorf("batch delete invalid devices: status=%d body=%s", statusCode, string(respBody))
	}

	var resp struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			Success struct {
				DeviceIDs []string `json:"device_ids"`
			} `json:"success"`
			Failed []struct {
				DeviceIDs []string `json:"device_ids"`
				ErrorMsg  string   `json:"error_msg"`
			} `json:"failed"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return []string{}, map[string]string{}, fmt.Errorf("batch delete invalid devices parse: %w", err)
	}
	if resp.Code != 0 {
		return []string{}, map[string]string{}, fmt.Errorf("batch delete invalid devices: code=%d message=%s", resp.Code, resp.Message)
	}

	successDIDs := resp.Data.Success.DeviceIDs
	failedMap := make(map[string]string)
	for _, f := range resp.Data.Failed {
		for _, did := range f.DeviceIDs {
			failedMap[did] = f.ErrorMsg
		}
	}

	return successDIDs, failedMap, nil
}

// formatUnixTime 将 Unix 时间戳转换为 RFC3339 格式字符串
func formatUnixTime(v interface{}) string {
	if v == nil {
		return ""
	}

	var ts int64
	switch val := v.(type) {
	case float64:
		ts = int64(val)
	case float32:
		ts = int64(val)
	case int:
		ts = int64(val)
	case int64:
		ts = val
	case string:
		parsed, err := strconv.ParseFloat(val, 64)
		if err != nil {
			return val
		}
		ts = int64(parsed)
	default:
		return fmt.Sprintf("%v", v)
	}

	if ts == 0 {
		return ""
	}

	return time.Unix(ts, 0).Format(time.RFC3339)
}
