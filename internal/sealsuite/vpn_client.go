package sealsuite

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"sealsuite-operation/internal/logger"
	"sealsuite-operation/internal/storage"

	"go.uber.org/zap"
)

type VPNConntrackLogItem struct {
	LogID          string `json:"log_id"`
	UserID         string `json:"user_id"`
	UserFullName   string `json:"user_full_name"`
	DepartmentID   string `json:"department_id"`
	DepartmentPath string `json:"department_path"`
	RolesName      string `json:"roles_name"`
	RolesID        string `json:"roles_id"`
	DestIP         string `json:"items_dip"`
	DestPort       int    `json:"items_dport"`
	Action         string `json:"items_action"`
	Protocol       string `json:"protocol"`
	EventTime      string `json:"event_time"`
	RawJSON        string `json:"-"`
}

type VPNConntrackListResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    struct {
		Items []map[string]interface{} `json:"items"`
		Total int                      `json:"total"`
	} `json:"data"`
	Items []map[string]interface{} `json:"items"`
	Total int                      `json:"total"`
}

func (c *Client) ListVPNConntrackLogs(startTime, endTime string, limit, offset int) ([]VPNConntrackLogItem, int, error) {
	requestID := fmt.Sprintf("vpn_conntrack_%d", time.Now().UnixNano())

	logger.Info("[VPN API] 获取连接跟踪日志",
		zap.String("request_id", requestID),
		zap.String("start_time", startTime),
		zap.String("end_time", endTime),
		zap.Int("limit", limit),
		zap.Int("offset", offset),
	)

	params := map[string]string{
		"limit":  fmt.Sprintf("%d", limit),
		"offset": fmt.Sprintf("%d", offset),
	}
	if startTime != "" {
		params["start_time"] = startTime
	}
	if endTime != "" {
		params["end_time"] = endTime
	}

	statusCode, body, err := c.DoRaw(http.MethodGet, "/api/open/v1/vpn/log/conntrack/list", params, nil)
	if err != nil {
		logger.Error("[VPN API] 获取连接跟踪日志失败",
			zap.String("request_id", requestID),
			zap.Error(err),
		)
		return nil, 0, fmt.Errorf("vpn conntrack list offset=%d: %w", offset, err)
	}

	if statusCode != http.StatusOK {
		logger.Error("[VPN API] 获取连接跟踪日志非200",
			zap.String("request_id", requestID),
			zap.Int("status_code", statusCode),
			zap.String("body", truncateString(string(body), 500)),
		)
		return nil, 0, fmt.Errorf("vpn conntrack list offset=%d: status=%d body=%s", offset, statusCode, string(body))
	}

	var resp VPNConntrackListResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		logger.Error("[VPN API] 解析响应失败",
			zap.String("request_id", requestID),
			zap.Error(err),
		)
		return nil, 0, fmt.Errorf("vpn conntrack list parse offset=%d: %w", offset, err)
	}

	if resp.Code != 0 {
		logger.Error("[VPN API] 业务错误",
			zap.String("request_id", requestID),
			zap.Int("code", resp.Code),
			zap.String("message", resp.Message),
		)
		return nil, 0, fmt.Errorf("vpn conntrack list offset=%d: code=%d message=%s", offset, resp.Code, resp.Message)
	}

	rawItems := resp.Data.Items
	if len(rawItems) == 0 {
		rawItems = resp.Items
	}

	total := resp.Data.Total
	if total == 0 {
		total = resp.Total
	}

	items := make([]VPNConntrackLogItem, 0, len(rawItems))
	for _, raw := range rawItems {
		item := parseVPNConntrackLog(raw)
		if item.LogID != "" {
			items = append(items, item)
		}
	}

	logger.Info("[VPN API] 获取连接跟踪日志成功",
		zap.String("request_id", requestID),
		zap.Int("count", len(items)),
		zap.Int("total", total),
	)

	return items, total, nil
}

func (c *Client) FetchAllVPNConntrackLogs(startTime, endTime string, maxItems int) ([]VPNConntrackLogItem, error) {
	var allLogs []VPNConntrackLogItem
	offset := 0
	limit := 100
	maxPages := 1000

	for i := 0; i < maxPages; i++ {
		if maxItems > 0 && len(allLogs) >= maxItems {
			break
		}

		batchLimit := limit
		if maxItems > 0 && len(allLogs)+limit > maxItems {
			batchLimit = maxItems - len(allLogs)
		}

		logs, total, err := c.ListVPNConntrackLogs(startTime, endTime, batchLimit, offset)
		if err != nil {
			return allLogs, fmt.Errorf("fetch vpn logs page %d: %w", i, err)
		}

		if len(logs) == 0 {
			break
		}

		allLogs = append(allLogs, logs...)

		if total > 0 && len(allLogs) >= total {
			break
		}

		if len(logs) < batchLimit {
			break
		}

		offset += limit
	}

	return allLogs, nil
}

func parseVPNConntrackLog(raw map[string]interface{}) VPNConntrackLogItem {
	result := VPNConntrackLogItem{}

	rawJSON, _ := json.Marshal(raw)
	result.RawJSON = string(rawJSON)

	getStr := func(key string) string {
		if v, ok := raw[key]; ok && v != nil {
			return fmt.Sprintf("%v", v)
		}
		return ""
	}

	result.LogID = getStr("log_id")
	if result.LogID == "" {
		result.LogID = getStr("id")
	}
	result.UserID = getStr("user_id")
	result.UserFullName = getStr("user_full_name")
	if result.UserFullName == "" {
		result.UserFullName = getStr("user_name")
	}
	result.DepartmentID = getStr("department_id")
	result.DepartmentPath = getStr("department_path")
	if result.DepartmentPath == "" {
		result.DepartmentPath = getStr("department_name")
	}
	result.RolesName = getStr("roles_name")
	if result.RolesName == "" {
		result.RolesName = getStr("role_name")
	}
	result.RolesID = getStr("roles_id")
	if result.RolesID == "" {
		result.RolesID = getStr("role_id")
	}
	result.Protocol = getStr("protocol")
	result.EventTime = getStr("event_time")
	if result.EventTime == "" {
		result.EventTime = getStr("time")
	}

	if v, ok := raw["items_dip"]; ok && v != nil {
		result.DestIP = fmt.Sprintf("%v", v)
	} else if v, ok := raw["dest_ip"]; ok && v != nil {
		result.DestIP = fmt.Sprintf("%v", v)
	}

	if v, ok := raw["items_dport"]; ok && v != nil {
		result.DestPort = toInt(v)
	} else if v, ok := raw["dest_port"]; ok && v != nil {
		result.DestPort = toInt(v)
	}

	if v, ok := raw["items_action"]; ok && v != nil {
		result.Action = fmt.Sprintf("%v", v)
	} else if v, ok := raw["action"]; ok && v != nil {
		result.Action = fmt.Sprintf("%v", v)
	}

	if result.EventTime != "" && isUnixTimestamp(result.EventTime) {
		result.EventTime = formatUnixTimeStr(result.EventTime)
	}

	return result
}

func toInt(v interface{}) int {
	switch val := v.(type) {
	case float64:
		return int(val)
	case float32:
		return int(val)
	case int:
		return val
	case int64:
		return int(val)
	case string:
		if n, err := strconv.Atoi(val); err == nil {
			return n
		}
	}
	return 0
}

func isUnixTimestamp(s string) bool {
	if len(s) < 8 || len(s) > 13 {
		return false
	}
	_, err := strconv.ParseFloat(s, 64)
	return err == nil
}

func formatUnixTimeStr(s string) string {
	ts, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return s
	}
	if ts == 0 {
		return ""
	}
	return time.Unix(int64(ts), 0).Format(time.RFC3339)
}

func VPNLogToStorageLog(items []VPNConntrackLogItem) []storage.ZTNAAccessLog {
	result := make([]storage.ZTNAAccessLog, 0, len(items))
	now := time.Now().Format(time.RFC3339)
	for _, item := range items {
		logItem := storage.ZTNAAccessLog{
			ID:             item.LogID,
			LogID:          item.LogID,
			UserID:         item.UserID,
			UserFullName:   item.UserFullName,
			DepartmentID:   item.DepartmentID,
			DepartmentPath: item.DepartmentPath,
			RolesName:      item.RolesName,
			RolesID:        item.RolesID,
			DestIP:         item.DestIP,
			DestPort:       item.DestPort,
			Action:         item.Action,
			Protocol:       item.Protocol,
			EventTime:      item.EventTime,
			ImportedAt:     now,
			RawJSON:        item.RawJSON,
		}
		if logItem.ID == "" {
			logItem.ID = fmt.Sprintf("vpn_%s_%s_%d", item.UserID, item.DestIP, item.DestPort)
		}
		result = append(result, logItem)
	}
	return result
}
