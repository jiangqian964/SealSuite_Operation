package api

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"sealsuite-operation/internal/sealsuite"
	"sealsuite-operation/internal/storage"
)

type Executor struct {
	client    *sealsuite.Client
	templates map[string]storage.Template

	mu      sync.RWMutex
	history []HistoryItem
}

func NewExecutor(client *sealsuite.Client, templates map[string]storage.Template) *Executor {
	if templates == nil {
		templates = map[string]storage.Template{}
	}
	return &Executor{
		client:    client,
		templates: templates,
		history:   make([]HistoryItem, 0, 200),
	}
}

func (e *Executor) SetClient(client *sealsuite.Client) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.client = client
}

func (e *Executor) SetTemplates(templates map[string]storage.Template) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if templates == nil {
		templates = map[string]storage.Template{}
	}
	e.templates = templates
}

func (e *Executor) Execute(req ExecuteRequest) (*ExecuteResponse, error) {
	method := strings.ToUpper(req.Method)
	path := req.Path
	query := req.Query
	body := req.Body
	templateID := req.TemplateID

	if templateID != "" {
		e.mu.RLock()
		tpl, ok := e.templates[templateID]
		e.mu.RUnlock()
		if !ok {
			return nil, fmt.Errorf("template not found: %s", templateID)
		}
		method = strings.ToUpper(tpl.Method)
		path = tpl.Path

		// 合并模板默认 query 参数（飞连 OpenAPI 列表类接口通常要求分页参数 page/page_size）
		if len(tpl.DefaultQuery) > 0 {
			if query == nil {
				query = make(map[string]interface{}, len(tpl.DefaultQuery))
			}
			for k, v := range tpl.DefaultQuery {
				if _, exists := query[k]; !exists {
					query[k] = v
				}
			}
		}
		// 基于 query_schema 对 query 做类型强制转换（解决"字符串形式数字"等格式问题）
		if len(query) > 0 {
			query = coerceQueryBySchema(query, tpl.QuerySchema)
		}
		// 合并模板默认 body 参数
		if len(tpl.DefaultBody) > 0 {
			if body == nil {
				body = make(map[string]interface{}, len(tpl.DefaultBody))
			}
			for k, v := range tpl.DefaultBody {
				if _, exists := body[k]; !exists {
					body[k] = v
				}
			}
		}
		// 基于 body_schema 对 body 做类型强制转换（解决"字符串形式数字"等格式问题）
		if len(body) > 0 {
			body = coerceBodyBySchema(body, tpl.BodySchema)
		}
	}
	if method == "" {
		return nil, fmt.Errorf("method is required")
	}
	if path == "" {
		return nil, fmt.Errorf("path is required")
	}

	if len(req.PathParams) > 0 {
		for k, v := range req.PathParams {
			path = strings.ReplaceAll(path, "{"+k+"}", fmt.Sprintf("%v", v))
		}
	}
	if strings.Contains(path, "{") && strings.Contains(path, "}") {
		return nil, fmt.Errorf("unresolved path params in path: %s", path)
	}

	e.mu.RLock()
	client := e.client
	e.mu.RUnlock()

	start := time.Now()
	status, raw, err := client.DoRaw(method, path, toStringMap(query), body)
	latency := time.Since(start).Milliseconds()
	if err != nil {
		e.appendHistory(HistoryItem{
			Time:       time.Now().Unix(),
			TemplateID: templateID,
			Method:     method,
			Path:       path,
			HTTPStatus: status,
			LatencyMs:  latency,
			OK:         false,
		})
		return nil, err
	}

	out := &ExecuteResponse{
		HTTPStatus: status,
		LatencyMs:  latency,
		Body:       raw,
	}

	// 解析业务码（兼容当前 CommonResponse 结构：code/message/data）
	var biz struct {
		Code    *int   `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(raw, &biz); err == nil && biz.Code != nil {
		out.BusinessCode = biz.Code
		out.BusinessMessage = biz.Message
	}

	ok := status >= 200 && status < 300
	if out.BusinessCode != nil {
		ok = ok && *out.BusinessCode == 0
	}
	e.appendHistory(HistoryItem{
		Time:       time.Now().Unix(),
		TemplateID: templateID,
		Method:     method,
		Path:       path,
		HTTPStatus: status,
		LatencyMs:  latency,
		OK:         ok,
	})

	return out, nil
}

func (e *Executor) History(limit int) []HistoryItem {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if limit <= 0 || limit >= len(e.history) {
		cp := make([]HistoryItem, len(e.history))
		copy(cp, e.history)
		return cp
	}
	start := len(e.history) - limit
	cp := make([]HistoryItem, limit)
	copy(cp, e.history[start:])
	return cp
}

func (e *Executor) appendHistory(item HistoryItem) {
	e.mu.Lock()
	defer e.mu.Unlock()
	const capN = 200
	if len(e.history) >= capN {
		// drop oldest
		copy(e.history, e.history[1:])
		e.history[len(e.history)-1] = item
		return
	}
	e.history = append(e.history, item)
}

// coerceQueryBySchema 根据 query_schema 中定义的类型，对 query 字段值做类型强制转换。
// 与 coerceBodyBySchema 类似，目的是把前端 textarea 输入的"字符串形式数字"
// 转换为正确类型后再拼接到 URL query string。
// query 值最终都是 string（URL query 是字符串），但在模板 default_query 合并阶段
// 需要正确类型来匹配 default 值，且 schema 驱动的默认值可以填充数字。
func coerceQueryBySchema(query map[string]interface{}, schema map[string]interface{}) map[string]interface{} {
	if query == nil {
		query = map[string]interface{}{}
	}
	if len(schema) == 0 {
		return query
	}

	for field, schemaVal := range schema {
		var targetType string
		var defaultVal interface{}

		switch sv := schemaVal.(type) {
		case map[string]interface{}:
			if t, ok := sv["type"].(string); ok {
				targetType = t
			}
			if d, ok := sv["default"]; ok {
				defaultVal = d
			}
		case string:
			// 兼容：schema 值是字符串时，既作为类型也作为默认值的提示
			// 如果是已知类型名，只设置 targetType
			lower := strings.ToLower(sv)
			if lower == "string" || lower == "int" || lower == "integer" || lower == "number" || lower == "bool" || lower == "boolean" {
				targetType = sv
			} else {
				// 不是已知类型名，说明用户把 schema 当成了默认值来用
				defaultVal = sv
				targetType = "string"
			}
		case float64:
			// JSON number 类型，用户把 schema 当成了默认值
			defaultVal = sv
			targetType = "number"
		case bool:
			// bool 类型，用户把 schema 当成了默认值
			defaultVal = sv
			targetType = "bool"
		}

		// 默认值填充
		if _, exists := query[field]; !exists && defaultVal != nil {
			query[field] = defaultVal
		}

		// 类型转换（对于 URL query，最终都转为字符串，但这里先做值规范化）
		if val, exists := query[field]; exists && targetType != "" {
			query[field] = coerceValue(val, targetType)
		}
	}
	return query
}

// toStringMap 将 map[string]interface{} 转为 map[string]string，
// 用于将 query/path_params 值传递给 DoRaw（URL query 只能是字符串）。
// nil/空值保留为空字符串。
func toStringMap(m map[string]interface{}) map[string]string {
	if m == nil {
		return nil
	}
	result := make(map[string]string, len(m))
	for k, v := range m {
		if v == nil {
			result[k] = ""
			continue
		}
		switch val := v.(type) {
		case string:
			result[k] = val
		case float64:
			// JSON number 默认解析为 float64；如果是整数形式，用整数表示
			if val == float64(int64(val)) {
				result[k] = fmt.Sprintf("%d", int64(val))
			} else {
				result[k] = fmt.Sprintf("%g", val)
			}
		case int, int32, int64, int16, int8:
			result[k] = fmt.Sprintf("%d", val)
		case bool:
			result[k] = fmt.Sprintf("%t", val)
		default:
			// fallback：使用 fmt.Sprintf("%v")
			result[k] = fmt.Sprintf("%v", v)
		}
	}
	return result
}

// coerceBodyBySchema 根据 body_schema 中定义的类型，对 body 字段值做类型强制转换。
// 目的：解决"字符串形式数字"（如 "100"）被 json.Marshal 编码为字符串，
// 导致飞连 API 返回 code=40000 "参数错误" 的问题。
// schema 示例：{page: {type: int, default: 1}, page_size: {type: int, default: 100}}
// 特殊字段：__raw_json__（type: json）表示 body 是自由 JSON，不做字段级转换。
func coerceBodyBySchema(body map[string]interface{}, schema map[string]interface{}) map[string]interface{} {
	if len(schema) == 0 {
		return body
	}
	if body == nil {
		body = map[string]interface{}{}
	}

	// __raw_json__ 标记：整个 body 是自由 JSON，不做字段级转换
	if _, hasRaw := schema["__raw_json__"]; hasRaw {
		return body
	}

	for field, schemaVal := range schema {
		// 解析 schemaVal：可能是 map[type, default] 或 string (type) 或简单值 (默认值)
		var targetType string
		var defaultVal interface{}

		switch sv := schemaVal.(type) {
		case map[string]interface{}:
			if t, ok := sv["type"].(string); ok {
				targetType = t
			}
			if d, ok := sv["default"]; ok {
				defaultVal = d
			}
		case string:
			// 兼容：schema 值是字符串时，判断是类型名还是默认值
			lower := strings.ToLower(sv)
			if lower == "string" || lower == "int" || lower == "integer" || lower == "number" || lower == "bool" || lower == "boolean" || lower == "json" {
				targetType = sv
			} else {
				defaultVal = sv
				targetType = "string"
			}
		case float64:
			defaultVal = sv
			targetType = "number"
		case bool:
			defaultVal = sv
			targetType = "bool"
		}

		// 设置默认值（如果用户没有提供该字段）
		if _, exists := body[field]; !exists && defaultVal != nil {
			body[field] = defaultVal
		}

		// 类型转换
		if val, exists := body[field]; exists && targetType != "" {
			body[field] = coerceValue(val, targetType)
		}
	}
	return body
}

// coerceValue 将 val 转换为 targetType 对应的 Go 类型，
// 以确保 json.Marshal 产生正确的 JSON 类型。
func coerceValue(val interface{}, targetType string) interface{} {
	if val == nil {
		return val
	}

	lowerType := strings.ToLower(targetType)

	// 先处理特殊情况：val 可能是 json.Number 类型（来自 decoder.UseNumber()）
	// 但我们的代码用 map[string]interface{}，标准 json.Decoder 默认产生 float64 或 string

	switch lowerType {
	case "int", "integer", "number":
		switch v := val.(type) {
		case string:
			// 尝试先解析为 int，再尝试 float
			if n, err := strconv.ParseInt(v, 10, 64); err == nil {
				return n
			}
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				// 如果是整数形式的 float，返回 int
				if f == float64(int64(f)) {
					return int64(f)
				}
				return f
			}
			// 无法解析，保持原值（字符串）
			return val
		case float64:
			// Go json.Decoder 默认把 JSON number 解码为 float64
			// 如果是整数形式，转换为 int64 使输出为整数 JSON
			if v == float64(int64(v)) {
				return int64(v)
			}
			return v
		case json.Number:
			if n, err := v.Int64(); err == nil {
				return n
			}
			if f, err := v.Float64(); err == nil {
				return f
			}
			return val
		}

	case "float", "float64", "double":
		switch v := val.(type) {
		case string:
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				return f
			}
			return val
		case float64:
			return v
		case json.Number:
			if f, err := v.Float64(); err == nil {
				return f
			}
			return val
		}

	case "bool", "boolean":
		switch v := val.(type) {
		case string:
			if b, err := strconv.ParseBool(v); err == nil {
				return b
			}
			return val
		case bool:
			return v
		}

	case "string":
		switch v := val.(type) {
		case string:
			return v
		default:
			// 非字符串类型尝试转为字符串
			if _, err := json.Marshal(v); err == nil {
				return fmt.Sprintf("%v", v)
			}
			return val
		}

	case "json", "object":
		// 嵌套对象，不做额外转换
		return val
	}

	return val
}
