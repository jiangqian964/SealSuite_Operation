package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"sealsuite-operation/internal/config"
	"sealsuite-operation/internal/sealsuite"
	"sealsuite-operation/internal/storage"
)

func TestExecutorExecuteWithTemplateParsesBusinessCode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// token endpoint
		if r.URL.Path == "/api/open/v1/token" && r.Method == http.MethodPost {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"code":0,"message":"success","data":{"access_token":"T","expires_in":7200}}`))
			return
		}
		if r.URL.Path != "/api/v1/users" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("Authorization") != "T" {
			http.Error(w, "missing token", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"message":"success","data":{"total":1}}`))
	}))
	defer srv.Close()

	client := sealsuite.NewClient(&config.SealSuiteConfig{
		BaseURL:    srv.URL,
		AccessKey:  "ak",
		SecretKey:  "sk",
		Timeout:    5,
		RetryTimes: 0,
	})
	client.SetMockMode(false)

	templates := map[string]storage.Template{
		"users_list": {ID: "users_list", Name: "用户-列表", Category: "users", Method: "GET", Path: "/api/v1/users"},
	}
	exec := NewExecutor(client, templates)

	resp, err := exec.Execute(ExecuteRequest{TemplateID: "users_list"})
	if err != nil {
		t.Fatalf("Execute err=%v", err)
	}
	if resp.BusinessCode == nil || *resp.BusinessCode != 0 {
		t.Fatalf("expected business code 0, got=%v", resp.BusinessCode)
	}
}

// TestExecutorExecutePOSTWithStringNumbers 模拟"前端 textarea 输入数字"场景：
// body: {"page_size": "100"}（字符串形式的数字）
// 期望：经过 body_schema 转换后，发出的请求 body 应该是 {"page_size": 100}（JSON number）
// 这是用户反馈飞连 API 返回 code=40000 "参数错误" 的核心修复验证。
func TestExecutorExecutePOSTWithStringNumbers(t *testing.T) {
	var capturedBody string
	var capturedHeaders http.Header

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/open/v1/token" && r.Method == http.MethodPost {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"code":0,"message":"success","data":{"access_token":"T","expires_in":7200}}`))
			return
		}
		body, _ := io.ReadAll(r.Body)
		capturedBody = string(body)
		capturedHeaders = r.Header.Clone()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"message":"success","data":{"total":1}}`))
	}))
	defer srv.Close()

	client := sealsuite.NewClient(&config.SealSuiteConfig{
		BaseURL:    srv.URL,
		AccessKey:  "ak",
		SecretKey:  "sk",
		Timeout:    5,
		RetryTimes: 0,
	})

	templates := map[string]storage.Template{
		"device_search": {
			ID:     "device_search",
			Name:   "设备-搜索",
			Method: "POST",
			Path:   "/api/open/v1/device/search",
			BodySchema: map[string]interface{}{
				"page":      map[string]interface{}{"type": "int", "default": 1},
				"page_size": map[string]interface{}{"type": "int", "default": 100},
				"keyword":   map[string]interface{}{"type": "string"},
			},
		},
	}
	exec := NewExecutor(client, templates)

	// 模拟前端 textarea 输入：数字是字符串形式（JSON 解析后仍为 string）
	body := map[string]interface{}{
		"page":      "1",    // 字符串形式数字
		"page_size": "100",  // 字符串形式数字
		"keyword":   "test", // 正常字符串
	}

	_, err := exec.Execute(ExecuteRequest{TemplateID: "device_search", Body: body})
	if err != nil {
		t.Fatalf("Execute err=%v", err)
	}

	// 验证 Content-Type
	if ct := capturedHeaders.Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Errorf("expected Content-Type=application/json, got=%q", ct)
	}

	// 验证 body 中数字字段是 JSON number（不带引号）而非 JSON string（带引号）
	// 期望：{"keyword":"test","page":1,"page_size":100}
	// 错误：{"keyword":"test","page":"1","page_size":"100"}
	if strings.Contains(capturedBody, `"page_size":"100"`) {
		t.Errorf("page_size should be JSON number, got string-encoded in body=%q", capturedBody)
	}
	if strings.Contains(capturedBody, `"page":"1"`) {
		t.Errorf("page should be JSON number, got string-encoded in body=%q", capturedBody)
	}
	if !strings.Contains(capturedBody, `"page_size":100`) {
		t.Errorf("expected body to contain \"page_size\":100, got=%q", capturedBody)
	}
	if !strings.Contains(capturedBody, `"keyword":"test"`) {
		t.Errorf("expected body to contain \"keyword\":\"test\", got=%q", capturedBody)
	}

	// 解析 body 并验证字段类型
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(capturedBody), &parsed); err != nil {
		t.Fatalf("failed to unmarshal captured body: %v, body=%q", err, capturedBody)
	}

	// page_size 应该是 float64（JSON number → Go float64）
	if ps, ok := parsed["page_size"].(float64); !ok {
		t.Errorf("expected page_size to be JSON number (float64), got %T = %v", parsed["page_size"], parsed["page_size"])
	} else if ps != 100 {
		t.Errorf("expected page_size=100, got=%v", ps)
	}

	if p, ok := parsed["page"].(float64); !ok {
		t.Errorf("expected page to be JSON number (float64), got %T = %v", parsed["page"], parsed["page"])
	} else if p != 1 {
		t.Errorf("expected page=1, got=%v", p)
	}

	if k, ok := parsed["keyword"].(string); !ok || k != "test" {
		t.Errorf("expected keyword=\"test\", got %T = %v", parsed["keyword"], parsed["keyword"])
	}
}

// TestCoerceBodyBySchema 直接测试 coerceBodyBySchema 函数的转换逻辑
func TestCoerceBodyBySchema(t *testing.T) {
	// 场景 1: schema 为 nil，body 不变
	body := map[string]interface{}{"page_size": "100"}
	result := coerceBodyBySchema(body, nil)
	if _, ok := result["page_size"].(string); !ok {
		t.Errorf("nil schema should not change values, expected string, got %T", result["page_size"])
	}

	// 场景 2: schema 为空 map，body 不变
	body = map[string]interface{}{"page_size": "100"}
	result = coerceBodyBySchema(body, map[string]interface{}{})
	if _, ok := result["page_size"].(string); !ok {
		t.Errorf("empty schema should not change values, expected string, got %T", result["page_size"])
	}

	// 场景 3: 字符串数字 → int
	body = map[string]interface{}{"page_size": "100", "page": "1"}
	schema := map[string]interface{}{
		"page_size": map[string]interface{}{"type": "int"},
		"page":      map[string]interface{}{"type": "int"},
	}
	result = coerceBodyBySchema(body, schema)
	if _, ok := result["page_size"].(int64); !ok {
		t.Errorf("expected page_size to be int64 after coercion, got %T = %v", result["page_size"], result["page_size"])
	}
	if _, ok := result["page"].(int64); !ok {
		t.Errorf("expected page to be int64 after coercion, got %T = %v", result["page"], result["page"])
	}

	// 场景 4: float64（JSON 默认 number） → int
	body = map[string]interface{}{"page_size": 100.0} // json.Decoder 默认把 number 解析为 float64
	schema = map[string]interface{}{
		"page_size": map[string]interface{}{"type": "int"},
	}
	result = coerceBodyBySchema(body, schema)
	if _, ok := result["page_size"].(int64); !ok {
		t.Errorf("expected float64 100.0 to be int64 after coercion, got %T = %v", result["page_size"], result["page_size"])
	}

	// 场景 5: 布尔值转换
	body = map[string]interface{}{"enabled": "true", "active": "false"}
	schema = map[string]interface{}{
		"enabled": map[string]interface{}{"type": "bool"},
		"active":  map[string]interface{}{"type": "boolean"},
	}
	result = coerceBodyBySchema(body, schema)
	if v, ok := result["enabled"].(bool); !ok || !v {
		t.Errorf("expected enabled=true (bool), got %T = %v", result["enabled"], result["enabled"])
	}
	if v, ok := result["active"].(bool); !ok || v {
		t.Errorf("expected active=false (bool), got %T = %v", result["active"], result["active"])
	}

	// 场景 6: 默认值
	body = map[string]interface{}{"keyword": "test"}
	schema = map[string]interface{}{
		"page":      map[string]interface{}{"type": "int", "default": 1},
		"page_size": map[string]interface{}{"type": "int", "default": 200},
		"keyword":   map[string]interface{}{"type": "string"},
	}
	result = coerceBodyBySchema(body, schema)
	if v, ok := result["page"].(int); !ok || v != 1 {
		t.Errorf("expected default page=1 (int), got %T = %v", result["page"], result["page"])
	}
	if v, ok := result["page_size"].(int); !ok || v != 200 {
		t.Errorf("expected default page_size=200 (int), got %T = %v", result["page_size"], result["page_size"])
	}

	// 场景 7: __raw_json__ 标记 - 不做任何转换
	body = map[string]interface{}{"page_size": "100"}
	schema = map[string]interface{}{
		"__raw_json__": map[string]interface{}{"type": "json"},
	}
	result = coerceBodyBySchema(body, schema)
	if _, ok := result["page_size"].(string); !ok {
		t.Errorf("__raw_json__ should skip coercion, expected string, got %T", result["page_size"])
	}

	// 场景 8: schema 中的 type 为 string
	body = map[string]interface{}{"count": 123} // number
	schema = map[string]interface{}{
		"count": map[string]interface{}{"type": "string"},
	}
	result = coerceBodyBySchema(body, schema)
	if _, ok := result["count"].(string); !ok {
		t.Errorf("expected count to be string after coercion, got %T = %v", result["count"], result["count"])
	}

	// 场景 9: json.Marshal 后验证 - 这是最关键的测试
	body = map[string]interface{}{"page": "1", "page_size": "100", "keyword": "test"}
	schema = map[string]interface{}{
		"page":      map[string]interface{}{"type": "int"},
		"page_size": map[string]interface{}{"type": "int"},
		"keyword":   map[string]interface{}{"type": "string"},
	}
	result = coerceBodyBySchema(body, schema)
	rawJSON, _ := json.Marshal(result)
	rawStr := string(rawJSON)
	if strings.Contains(rawStr, `"page_size":"100"`) {
		t.Errorf("JSON output should encode page_size as number, got=%q", rawStr)
	}
	if !strings.Contains(rawStr, `"page_size":100`) {
		t.Errorf("JSON output should contain \"page_size\":100, got=%q", rawStr)
	}
}

// TestExecutorExecuteGETWithEmptyBody 验证 GET 请求带空 body 时：
// - 不发送 body
// - 设置 Content-Type（飞连 API 要求所有请求都必须设置）
// 这是之前已修复 GET 请求带 body 问题的回归测试
func TestExecutorExecuteGETWithEmptyBody(t *testing.T) {
	var capturedBody string
	var capturedCT string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/open/v1/token" && r.Method == http.MethodPost {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"code":0,"message":"success","data":{"access_token":"T","expires_in":7200}}`))
			return
		}
		body, _ := io.ReadAll(r.Body)
		capturedBody = string(body)
		capturedCT = r.Header.Get("Content-Type")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"message":"success"}`))
	}))
	defer srv.Close()

	client := sealsuite.NewClient(&config.SealSuiteConfig{
		BaseURL:    srv.URL,
		AccessKey:  "ak",
		SecretKey:  "sk",
		Timeout:    5,
		RetryTimes: 0,
	})

	templates := map[string]storage.Template{
		"device_detail": {
			ID:     "device_detail",
			Name:   "设备-详情",
			Method: "GET",
			Path:   "/api/open/v1/device/detail",
			BodySchema: map[string]interface{}{
				"page":      map[string]interface{}{"type": "int"},
				"page_size": map[string]interface{}{"type": "int"},
			},
		},
	}
	exec := NewExecutor(client, templates)

	// 测试 1: 传空 body
	_, err := exec.Execute(ExecuteRequest{
		TemplateID: "device_detail",
		Query:      map[string]interface{}{"did": "dev-001"},
		Body:       map[string]interface{}{},
	})
	if err != nil {
		t.Fatalf("Execute err=%v", err)
	}
	if capturedCT == "" {
		t.Errorf("GET request should have Content-Type (飞连 API 要求), got empty")
	}
	if capturedBody != "" {
		t.Errorf("GET request with empty body should send empty body, got=%q", capturedBody)
	}

	// 测试 2: 传带字符串数字的 body（GET 应该完全忽略 body）
	_, err = exec.Execute(ExecuteRequest{
		TemplateID: "device_detail",
		Query:      map[string]interface{}{"did": "dev-002"},
		Body:       map[string]interface{}{"page": "1", "page_size": "100"},
	})
	if err != nil {
		t.Fatalf("Execute err=%v", err)
	}
	if capturedCT == "" {
		t.Errorf("GET request should have Content-Type even with body params (飞连 API 要求), got empty")
	}
	if capturedBody != "" {
		t.Errorf("GET request should NOT send body regardless of input body, got=%q", capturedBody)
	}
}
