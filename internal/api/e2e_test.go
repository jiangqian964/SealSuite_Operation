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

// TestE2E_TemplateTest_ExactPath 模拟用户在"API 工具箱"页面点击"测试"按钮：
// 前端发送 POST /api/templates/{id}/test → 后端 executor.Execute → DoRaw → 飞连网关
// 这是用户最常操作的真实路径，必须完全正确。
func TestE2E_TemplateTest_ExactPath(t *testing.T) {
	// 1. 启动模拟飞连网关（检测格式）
	var lastMethod, lastURL, lastCT, lastBody string
	var lastBodyBytes int

	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		lastMethod = r.Method
		lastURL = r.URL.String()
		lastCT = r.Header.Get("Content-Type")
		lastBody = string(body)
		lastBodyBytes = len(body)

		// token API
		if r.URL.Path == "/api/open/v1/token" && r.Method == http.MethodPost {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"code":0,"message":"success","data":{"access_token":"T1","expires_in":7200}}`))
			return
		}

		// 模拟飞连网关的严格校验
		ok := true
		if r.Method == "GET" && len(body) > 0 {
			ok = false
		}
		if r.Method == "POST" && len(body) > 0 {
			var parsed map[string]interface{}
			if err := json.Unmarshal(body, &parsed); err != nil {
				ok = false
			} else {
				if ps, exists := parsed["page_size"]; exists {
					if _, isNumber := ps.(float64); !isNumber {
						ok = false
					}
				}
				if p, exists := parsed["page"]; exists {
					if _, isNumber := p.(float64); !isNumber {
						ok = false
					}
				}
			}
		}

		w.Header().Set("Content-Type", "application/json")
		if ok {
			_, _ = w.Write([]byte(`{"code":0,"message":"ok","data":{"items":[]}}`))
		} else {
			_, _ = w.Write([]byte(`{"code":40000,"message":"参数错误"}`))
		}
	}))
	defer gateway.Close()

	// 2. 配置 client（指向模拟网关）
	client := sealsuite.NewClient(&config.SealSuiteConfig{
		BaseURL:    gateway.URL,
		AccessKey:  "AK_TEST",
		SecretKey:  "SK_TEST",
		Timeout:    10,
		RetryTimes: 0,
	})
	tpls := map[string]storage.Template{
		"device_detail": {
			ID:     "device_detail",
			Name:   "设备详情",
			Method: "GET",
			Path:   "/api/open/v1/device/detail",
		},
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
	exec := NewExecutor(client, tpls)

	// ============== 3. 真实场景 A ==============
	// 前端 textarea 输入:
	//   query: {"did": "6819d3b41301f319abfcfc409463990c"}
	//   body:  <空>
	// Go 端解析后 -> map[string]interface{}{"did": "6819..."} -> 经 toStringMap -> map[string]string
	// 实际上: template test endpoint 会先把 body 解析为 map[string]interface{}，再给 Execute
	t.Run("GET device_detail (前端 textarea 输入)", func(t *testing.T) {
		// 模拟 /api/templates/{id}/test 端的请求体:
		//   {"path_params":{}, "query":{"did":"6819d3b41301f319abfcfc409463990c"},"body":{}}
		// 后端解析后会变成 map[string]interface{}
		out, err := exec.Execute(ExecuteRequest{
			TemplateID: "device_detail",
			Query:      map[string]interface{}{"did": "6819d3b41301f319abfcfc409463990c"},
			Body:       map[string]interface{}{},
		})
		if err != nil {
			t.Fatalf("exec failed: %v", err)
		}

		// 检查:
		if lastMethod != "GET" {
			t.Errorf("expected GET, got %s", lastMethod)
		}
		if lastCT == "" {
			t.Errorf("expected Content-Type (飞连 API 要求), got empty")
		}
		if lastBodyBytes != 0 {
			t.Errorf("expected empty body, got %d bytes: %q", lastBodyBytes, lastBody)
		}
		if !strings.Contains(lastURL, "did=6819d3b41301f319abfcfc409463990c") {
			t.Errorf("expected URL to contain did param, got %s", lastURL)
		}
		var parsed map[string]interface{}
		if err := json.Unmarshal(out.Body, &parsed); err != nil {
			t.Fatalf("response not JSON: %v", err)
		}
		if code, ok := parsed["code"].(float64); ok && code != 0 {
			t.Errorf("got code=%d (expected 0), resp=%s", int(code), string(out.Body))
		}
		t.Logf("  请求: %s %s", lastMethod, lastURL)
		t.Logf("  Content-Type: %q", lastCT)
		t.Logf("  body: %q (%d bytes)", lastBody, lastBodyBytes)
		t.Logf("  响应: %s", string(out.Body))
	})

	// ============== 4. 真实场景 B ==============
	// 前端 textarea 输入 POST body（数字写成字符串形式是常见场景）:
	//   body: {"page":"1","page_size":"100","keyword":"test"}
	t.Run("POST device_search (字符串数字形式)", func(t *testing.T) {
		out, err := exec.Execute(ExecuteRequest{
			TemplateID: "device_search",
			Query:      map[string]interface{}{},
			Body: map[string]interface{}{
				"page":      "1",
				"page_size": "100",
				"keyword":   "test",
			},
		})
		if err != nil {
			t.Fatalf("exec failed: %v", err)
		}

		if lastMethod != "POST" {
			t.Errorf("expected POST, got %s", lastMethod)
		}
		if !strings.Contains(lastCT, "application/json") {
			t.Errorf("expected Content-Type=application/json, got %q", lastCT)
		}
		// **关键验证**：body 中 page_size 应该是 number（不带引号），不是 string
		if strings.Contains(lastBody, `"page_size":"100"`) {
			t.Errorf("page_size still encoded as string in request body: %s", lastBody)
		}
		if !strings.Contains(lastBody, `"page_size":100`) {
			t.Errorf("expected page_size to be JSON number, got body: %s", lastBody)
		}
		var parsed map[string]interface{}
		if err := json.Unmarshal(out.Body, &parsed); err != nil {
			t.Fatalf("response not JSON: %v", err)
		}
		if code, ok := parsed["code"].(float64); ok && code != 0 {
			t.Errorf("got code=%d (expected 0), resp=%s", int(code), string(out.Body))
		}
		t.Logf("  请求: %s %s", lastMethod, lastURL)
		t.Logf("  Content-Type: %q", lastCT)
		t.Logf("  body: %s", lastBody)
		t.Logf("  响应: %s", string(out.Body))
	})

	// ============== 5. 真实场景 C ==============
	// 前端 textarea 输入（数字写成 number）:
	//   body: {"page":1,"page_size":100,"keyword":"test"}
	// Go json.Decoder 把 number 解析为 float64
	t.Run("POST device_search (float64 数字形式)", func(t *testing.T) {
		out, err := exec.Execute(ExecuteRequest{
			TemplateID: "device_search",
			Query:      map[string]interface{}{},
			Body: map[string]interface{}{
				// Go json.Decoder 把 JSON number 解析为 float64
				"page":      float64(1),
				"page_size": float64(100),
				"keyword":   "test",
			},
		})
		if err != nil {
			t.Fatalf("exec failed: %v", err)
		}

		if strings.Contains(lastBody, `"page_size":"100"`) {
			t.Errorf("page_size should not be string: %s", lastBody)
		}
		// json.Marshal(float64) → "100"
		if !strings.Contains(lastBody, `"page_size":100`) {
			t.Errorf("expected page_size to be JSON number, got body: %s", lastBody)
		}
		t.Logf("  body: %s", lastBody)
		t.Logf("  响应: %s", string(out.Body))
	})

	// ============== 6. 场景 D ==============
	// 缺省值（不传 page/page_size），依赖 body_schema 中的 default
	t.Run("POST device_search (使用 body_schema default 值)", func(t *testing.T) {
		out, err := exec.Execute(ExecuteRequest{
			TemplateID: "device_search",
			Query:      map[string]interface{}{},
			Body:       map[string]interface{}{"keyword": "test"},
		})
		if err != nil {
			t.Fatalf("exec failed: %v", err)
		}

		// body_schema 定义了 default: 1, default: 100
		if !strings.Contains(lastBody, `"page":1`) {
			t.Errorf("expected default page=1, got body: %s", lastBody)
		}
		if !strings.Contains(lastBody, `"page_size":100`) {
			t.Errorf("expected default page_size=100, got body: %s", lastBody)
		}
		t.Logf("  body: %s", lastBody)
		t.Logf("  响应: %s", string(out.Body))
	})
}
