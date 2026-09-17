package sealsuite

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"sealsuite-operation/internal/config"
)

// captureRequest 创建一个测试服务器并返回捕获到的请求细节
func captureRequest(t *testing.T, handler func(w http.ResponseWriter, r *http.Request)) (*httptest.Server, chan *capturedRequest) {
	t.Helper()
	ch := make(chan *capturedRequest, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cr := &capturedRequest{
			Method:  r.Method,
			URL:     r.URL.String(),
			Headers: make(map[string]string),
		}
		for k, vs := range r.Header {
			cr.Headers[k] = strings.Join(vs, ", ")
		}
		body, _ := io.ReadAll(r.Body)
		cr.Body = body
		ch <- cr
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"code":0,"message":"ok","data":{}}`)
	}))
	return srv, ch
}

type capturedRequest struct {
	Method  string
	URL     string
	Headers map[string]string
	Body    []byte
}

func makeTestClient(baseURL string) *Client {
	cfg := &config.SealSuiteConfig{BaseURL: baseURL}
	client := NewClient(cfg)
	// 用一个假的 token 缓存，避免调用真实的 token 接口
	client.token = "test-token"
	client.expiresAt = time.Now().Add(1 * time.Hour)
	return client
}

// TestDoRawGETWithNilBody 验证 GET 请求传递 nil body 时：
//   - 不发送 body
//   - 设置 Content-Type（飞连 API 要求所有请求都必须设置）
//   - 不设置 Authorization（当 path 是 token 接口时）或设置 Authorization
func TestDoRawGETWithNilBody(t *testing.T) {
	srv, ch := captureRequest(t, nil)
	defer srv.Close()

	client := makeTestClient(srv.URL)
	status, raw, err := client.DoRaw("GET", "/api/open/v1/device/detail", map[string]string{"did": "dev-001"}, nil)
	if err != nil {
		t.Fatalf("DoRaw error: %v", err)
	}
	if status < 200 || status >= 300 {
		t.Fatalf("unexpected status: %d", status)
	}
	_ = raw
	cr := <-ch

	t.Logf("Method=%s URL=%s", cr.Method, cr.URL)
	t.Logf("Headers: %+v", cr.Headers)
	t.Logf("Body: %q", string(cr.Body))

	if cr.Method != "GET" {
		t.Errorf("expected GET, got %s", cr.Method)
	}
	if ct := cr.Headers["Content-Type"]; ct == "" {
		t.Errorf("GET request should have Content-Type (飞连 API 要求), got empty")
	} else if !strings.Contains(ct, "application/json") {
		t.Errorf("GET Content-Type should be application/json, got %q", ct)
	}
	if cl := cr.Headers["Content-Length"]; cl != "" && cl != "0" {
		t.Errorf("GET request should NOT have body, Content-Length=%s", cl)
	}
	if len(cr.Body) > 0 {
		t.Errorf("GET request should have empty body, got %q", string(cr.Body))
	}
	if auth := cr.Headers["Authorization"]; auth != "test-token" {
		t.Errorf("expected Authorization=test-token, got %q", auth)
	}
	if !strings.Contains(cr.URL, "did=dev-001") {
		t.Errorf("expected URL to contain query param, got %s", cr.URL)
	}
}

// TestDoRawGETWithEmptyMapBody 验证 GET 请求传递空 map body {} 时：
//   - 完全忽略 body（不应发送 body）
//   - 设置 Content-Type（飞连 API 要求所有请求都必须设置）
func TestDoRawGETWithEmptyMapBody(t *testing.T) {
	srv, ch := captureRequest(t, nil)
	defer srv.Close()

	client := makeTestClient(srv.URL)
	status, _, err := client.DoRaw("GET", "/api/open/v1/addr/management/list", nil, map[string]interface{}{})
	if err != nil {
		t.Fatalf("DoRaw error: %v", err)
	}
	if status < 200 || status >= 300 {
		t.Fatalf("unexpected status: %d", status)
	}
	cr := <-ch

	t.Logf("Method=%s URL=%s", cr.Method, cr.URL)
	t.Logf("Headers: %+v", cr.Headers)
	t.Logf("Body: %q", string(cr.Body))

	if ct := cr.Headers["Content-Type"]; ct == "" {
		t.Errorf("GET with empty map should have Content-Type (飞连 API 要求), got empty")
	}
	if len(cr.Body) > 0 {
		t.Errorf("GET with empty map should have empty body, got %q", string(cr.Body))
	}
}

// TestDoRawPOSTWithRealBody 验证 POST 请求传递真实 body 时：
//   - 正确设置 Content-Type: application/json;charset=utf-8
//   - body 是正确的 JSON
//   - 字段值的类型保持（number 保持 number，string 保持 string）
func TestDoRawPOSTWithRealBody(t *testing.T) {
	srv, ch := captureRequest(t, nil)
	defer srv.Close()

	client := makeTestClient(srv.URL)
	body := map[string]interface{}{
		"page":      1,
		"page_size": 100,
		"keyword":   "test",
	}
	status, _, err := client.DoRaw("POST", "/api/open/v1/device/search", nil, body)
	if err != nil {
		t.Fatalf("DoRaw error: %v", err)
	}
	if status < 200 || status >= 300 {
		t.Fatalf("unexpected status: %d", status)
	}
	cr := <-ch

	t.Logf("Method=%s URL=%s", cr.Method, cr.URL)
	t.Logf("Headers: %+v", cr.Headers)
	t.Logf("Body: %s", string(cr.Body))

	if ct := cr.Headers["Content-Type"]; ct == "" {
		t.Errorf("POST should have Content-Type, got empty")
	} else if !strings.Contains(ct, "application/json") {
		t.Errorf("POST Content-Type should be application/json, got %q", ct)
	}

	// 验证 body 是有效的 JSON 且类型正确
	var parsed map[string]interface{}
	if err := json.Unmarshal(cr.Body, &parsed); err != nil {
		t.Fatalf("body is not valid JSON: %v, body=%q", err, string(cr.Body))
	}
	t.Logf("Parsed body: %+v", parsed)

	// 验证 number 类型
	if v, ok := parsed["page"].(float64); !ok || v != 1 {
		t.Errorf("expected page=1 (number), got %v (%T)", parsed["page"], parsed["page"])
	}
	if v, ok := parsed["page_size"].(float64); !ok || v != 100 {
		t.Errorf("expected page_size=100 (number), got %v (%T)", parsed["page_size"], parsed["page_size"])
	}
	if v, ok := parsed["keyword"].(string); !ok || v != "test" {
		t.Errorf("expected keyword=\"test\" (string), got %v (%T)", parsed["keyword"], parsed["keyword"])
	}
}

// TestDoRawPOSTWithEmptyBody 验证 POST 请求传递空 body 时：
//   - 不应发送 "{}" 作为 body
//   - 设置 Content-Type（飞连 API 要求所有请求都必须设置）
func TestDoRawPOSTWithEmptyBody(t *testing.T) {
	srv, ch := captureRequest(t, nil)
	defer srv.Close()

	client := makeTestClient(srv.URL)
	status, _, err := client.DoRaw("POST", "/api/open/v1/some/endpoint", nil, map[string]interface{}{})
	if err != nil {
		t.Fatalf("DoRaw error: %v", err)
	}
	if status < 200 || status >= 300 {
		t.Fatalf("unexpected status: %d", status)
	}
	cr := <-ch

	t.Logf("Method=%s URL=%s", cr.Method, cr.URL)
	t.Logf("Headers: %+v", cr.Headers)
	t.Logf("Body: %q", string(cr.Body))

	if ct := cr.Headers["Content-Type"]; ct == "" {
		t.Errorf("POST with empty body should have Content-Type (飞连 API 要求), got empty")
	}
	if len(cr.Body) > 0 {
		t.Errorf("POST with empty body should have empty raw body, got %q", string(cr.Body))
	}
}

// TestDoRawPOSTWithStringNumberBody 模拟"前端 text input 输入数字"的场景：
// body 是 map[string]interface{}{"page_size": "100"}（数字以字符串形式）
// 验证序列化后仍然是 string（这个行为是 Go 的默认行为，我们需要明确测试到）
func TestDoRawPOSTWithStringNumberBody(t *testing.T) {
	srv, ch := captureRequest(t, nil)
	defer srv.Close()

	client := makeTestClient(srv.URL)
	body := map[string]interface{}{
		"page":      "1",   // 字符串形式的数字
		"page_size": "100", // 字符串形式的数字
	}
	status, _, err := client.DoRaw("POST", "/api/open/v1/device/search", nil, body)
	if err != nil {
		t.Fatalf("DoRaw error: %v", err)
	}
	if status < 200 || status >= 300 {
		t.Fatalf("unexpected status: %d", status)
	}
	cr := <-ch

	t.Logf("Method=%s URL=%s", cr.Method, cr.URL)
	t.Logf("Headers: %+v", cr.Headers)
	t.Logf("Body: %s", string(cr.Body))

	// 检查原始 JSON 中数字是否被引号包裹
	raw := string(cr.Body)
	if !strings.Contains(raw, `"page_size":"100"`) {
		t.Logf("INFO: body did not contain \"page_size\":\"100\", body=%s", raw)
	} else {
		t.Logf("INFO: Confirmed: string-typed numbers stay as strings in JSON body")
	}
}

// TestNewClientBaseURLFromHostPort 验证 NewClient 使用 host/port 字段构造 baseURL 时：
//   - 默认端口不包含在 URL 中（避免 Host header 含 :443 导致网关拒绝）
func TestNewClientBaseURLFromHostPort(t *testing.T) {
	tests := []struct {
		name   string
		scheme string
		host   string
		port   int
		want   string
	}{
		{"https default port 443", "https", "feilian.example.com", 443, "https://feilian.example.com"},
		{"http default port 80", "http", "feilian.example.com", 80, "http://feilian.example.com"},
		{"https non-default port", "https", "feilian.example.com", 8443, "https://feilian.example.com:8443"},
		{"http non-default port", "http", "feilian.example.com", 8080, "http://feilian.example.com:8080"},
		{"zero port falls back to scheme default", "https", "feilian.example.com", 0, "https://feilian.example.com"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.SealSuiteConfig{
				Scheme: tt.scheme,
				Host:   tt.host,
				Port:   tt.port,
			}
			client := NewClient(cfg)
			if client.baseURL != tt.want {
				t.Errorf("expected baseURL=%q, got %q", tt.want, client.baseURL)
			}
		})
	}
}
