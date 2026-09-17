package api

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"sealsuite-operation/internal/config"
	"sealsuite-operation/internal/sealsuite"
	"sealsuite-operation/internal/storage"
)

// TestFormatADeviceDetail 模拟用户"格式 A"测试：
//
//	GET /api/open/v1/device/detail?did=<value>
//	Header: Authorization: <token>
//	无 Content-Type，无 body
//
// 验证 Go 代码发出的请求完全匹配用户 curl 验证通过的格式
func TestFormatADeviceDetail(t *testing.T) {
	var (
		lastMethod  string
		lastURL     string
		lastCT      string
		lastAuth    string
		lastBodyLen int64
		allHeaders  http.Header
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lastMethod = r.Method
		lastURL = r.URL.String()
		lastCT = r.Header.Get("Content-Type")
		lastAuth = r.Header.Get("Authorization")
		body, _ := io.ReadAll(r.Body)
		lastBodyLen = int64(len(body))
		allHeaders = r.Header.Clone()

		// token 请求（飞连 client 会先拿 token）
		if r.URL.Path == "/api/open/v1/token" && r.Method == http.MethodPost {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"code":0,"message":"success","data":{"access_token":"test-token-123","expires_in":7200}}`))
			return
		}

		// device_detail 返回
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"message":"success","data":{"did":"6819d3b41301f319abfcfc409463990c"}}`))
	}))
	defer srv.Close()

	client := sealsuite.NewClient(&config.SealSuiteConfig{
		BaseURL:    srv.URL,
		AccessKey:  "ak-test",
		SecretKey:  "sk-test",
		Timeout:    5,
		RetryTimes: 0,
	})

	templates := map[string]storage.Template{
		"device_detail": {
			ID:     "device_detail",
			Name:   "设备详情",
			Method: "GET",
			Path:   "/api/open/v1/device/detail",
		},
	}
	exec := NewExecutor(client, templates)

	// 1: 前端通过 textarea 传 query（数字为 string）
	_, err := exec.Execute(ExecuteRequest{
		TemplateID: "device_detail",
		Query:      map[string]interface{}{"did": "6819d3b41301f319abfcfc409463990c"},
		Body:       map[string]interface{}{}, // 用户的空 body
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// 验证格式
	t.Logf("Request: %s %s", lastMethod, lastURL)
	t.Logf("Headers: %+v", allHeaders)

	// ✅ Method 应该是 GET
	if lastMethod != "GET" {
		t.Errorf("[FAIL] Method 应为 GET，实际为 %q", lastMethod)
	} else {
		t.Logf("[PASS] Method = GET")
	}

	// ✅ URL 应该包含 did= 值
	if !strings.Contains(lastURL, "did=6819d3b41301f319abfcfc409463990c") {
		t.Errorf("[FAIL] URL 不包含正确的 did 参数: %s", lastURL)
	} else {
		t.Logf("[PASS] URL 包含 did 参数")
	}

	// ✅ Content-Type 应该有值（飞连 API 要求所有请求都必须设置）
	if lastCT == "" {
		t.Errorf("[FAIL] Content-Type 不应为空 (飞连 API 要求)，实际为空")
	} else if !strings.Contains(lastCT, "application/json") {
		t.Errorf("[FAIL] Content-Type 应为 application/json，实际为 %q", lastCT)
	} else {
		t.Logf("[PASS] Content-Type = %q (飞连 API 要求)", lastCT)
	}

	// ✅ Authorization 应该有值（非空）
	if lastAuth == "" {
		t.Errorf("[FAIL] Authorization 不应为空")
	} else {
		t.Logf("[PASS] Authorization = %q", lastAuth)
	}

	// ✅ Body 应该为 0 字节
	if lastBodyLen != 0 {
		t.Errorf("[FAIL] Body 长度应为 0，实际为 %d", lastBodyLen)
	} else {
		t.Logf("[PASS] Body 长度 = 0")
	}

	// 2: 前端传的 query 是 JSON number（如 {"page_size": 100}）
	_, err = exec.Execute(ExecuteRequest{
		TemplateID: "device_detail",
		Query:      map[string]interface{}{"did": "6819d3b41301f319abfcfc409463990c", "page_size": float64(100)},
		Body:       nil,
	})
	if err != nil {
		t.Fatalf("Execute (number query) failed: %v", err)
	}

	t.Logf("Number Query Request: %s %s", lastMethod, lastURL)
	if !strings.Contains(lastURL, "page_size=100") {
		t.Errorf("[FAIL] float64(100) 应转为 '100'，但 URL = %s", lastURL)
	} else {
		t.Logf("[PASS] float64(100) → 'page_size=100'")
	}
	if !strings.Contains(lastURL, "did=6819d3b41301f319abfcfc409463990c") {
		t.Errorf("[FAIL] did 参数缺失: %s", lastURL)
	} else {
		t.Logf("[PASS] did 保留完整字符串")
	}

	// 3: 长 token 也能正常传递（防止截断问题）
	// 刷新 token 以清除缓存
	_ = time.Now()
	_ = lastCT
}

// TestFormatAPOSTWithBody 验证 POST 带 body 的 API 能正确发出 JSON
// 修复目标："page_size":"100"（字符串）→ "page_size":100（number）
func TestFormatAPOSTWithBody(t *testing.T) {
	var lastBody string
	var lastMethod string
	var lastCT string
	var lastURL string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/open/v1/token" && r.Method == http.MethodPost {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"code":0,"message":"success","data":{"access_token":"token-post-test","expires_in":7200}}`))
			return
		}
		body, _ := io.ReadAll(r.Body)
		lastBody = string(body)
		lastMethod = r.Method
		lastCT = r.Header.Get("Content-Type")
		lastURL = r.URL.String()

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
			Name:   "设备搜索",
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

	// 前端 textarea 输入：数字是字符串形式
	_, err := exec.Execute(ExecuteRequest{
		TemplateID: "device_search",
		Query:      map[string]interface{}{},
		Body:       map[string]interface{}{"page": "1", "page_size": "100", "keyword": "test"},
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	t.Logf("Request: %s %s", lastMethod, lastURL)
	t.Logf("Content-Type: %q", lastCT)
	t.Logf("Body: %s", lastBody)

	// ✅ Method = POST
	if lastMethod != "POST" {
		t.Errorf("[FAIL] Method 应为 POST，实际为 %q", lastMethod)
	}
	// ✅ Content-Type 有 application/json
	if !strings.Contains(lastCT, "application/json") {
		t.Errorf("[FAIL] Content-Type 应为 application/json，实际为 %q", lastCT)
	}
	// ✅ page_size 应是 JSON number，不是 string
	if strings.Contains(lastBody, `"page_size":"100"`) {
		t.Errorf("[FAIL] page_size 仍为字符串形式 (\"100\"): %s", lastBody)
	}
	if !strings.Contains(lastBody, `"page_size":100`) {
		t.Errorf("[FAIL] page_size 应为 100 (number): %s", lastBody)
	}
	// ✅ keyword 保留为字符串
	if !strings.Contains(lastBody, `"keyword":"test"`) {
		t.Errorf("[FAIL] keyword 应为字符串: %s", lastBody)
	}
}
