// Package sealsuite 提供 SealSuite API 的客户端封装
// 支持 HTTP 请求、认证、模拟模式等功能
package sealsuite

import (
	"bytes"        // 字节缓冲区
	"encoding/json" // JSON 编码解码
	"fmt"          // 格式化输出
	"io"           // I/O 操作
	"net/http"     // HTTP 客户端
	"net/url"
	"strings"
	"sync"
	"time"         // 时间处理

	// 项目内部包
	"sealsuite-operation/internal/config"  // 配置管理
	"sealsuite-operation/internal/logger"  // 日志系统

	// 第三方库
	"go.uber.org/zap"  // 结构化日志
)

// Client 是 SealSuite API 的客户端结构体
// 封装了 HTTP 请求、认证信息和模拟模式
type Client struct {
	baseURL    string        // API 基础地址
	accessKey  string        // API 访问密钥 ID
	secretKey  string        // API 访问密钥密码
	httpClient *http.Client  // HTTP 客户端实例
	mockMode   bool          // 是否启用模拟模式

	// access_token 相关（根据飞连 OpenAPI 文档）
	tokenMu   sync.Mutex
	token     string
	expiresAt time.Time
	expiresIn int
}

// CommonResponse 是 SealSuite API 的通用响应结构
// 所有 API 响应都遵循此格式
type CommonResponse struct {
	Code    int         `json:"code"`     // 响应状态码，0 表示成功
	Message string      `json:"message"`  // 响应消息
	Data    interface{} `json:"data,omitempty"`  // 响应数据，可选
}

// NewClient 创建一个新的 SealSuite API 客户端
// 参数:
//   cfg - SealSuite 配置对象，包含 API 地址、密钥等信息
// 返回:
//   *Client - 新创建的客户端实例
func NewClient(cfg *config.SealSuiteConfig) *Client {
	baseURL := strings.TrimSpace(cfg.BaseURL)
	// 优先使用结构化字段组装 base_url（若提供）
	if cfg.Host != "" {
		scheme := cfg.Scheme
		if scheme == "" {
			scheme = "https"
		}
		if cfg.Port > 0 {
			baseURL = fmt.Sprintf("%s://%s:%d", scheme, cfg.Host, cfg.Port)
		} else {
			baseURL = fmt.Sprintf("%s://%s", scheme, cfg.Host)
		}
	}
	return &Client{
		baseURL:   baseURL,        // 设置 API 基础地址
		accessKey: cfg.AccessKey,  // 设置访问密钥 ID
		secretKey: cfg.SecretKey,  // 设置访问密钥密码
		httpClient: &http.Client{  // 创建 HTTP 客户端
			Timeout: time.Duration(cfg.Timeout) * time.Second,  // 设置请求超时
		},
		mockMode: false,  // 默认不启用模拟模式
	}
}

// SetMockMode 设置是否启用模拟模式
// 在模拟模式下，不会发起真实的 HTTP 请求，而是返回预定义的模拟数据
// 参数:
//   enabled - 是否启用模拟模式
func (c *Client) SetMockMode(enabled bool) {
	c.mockMode = enabled
	logger.Info("mock mode set", zap.Bool("enabled", enabled))
}

// getMockResponse 根据请求路径返回模拟数据
// 仅在 mockMode 为 true 时被调用
// 参数:
//   path - API 请求路径
// 返回:
//   *CommonResponse - 模拟的响应数据
func (c *Client) getMockResponse(path string) *CommonResponse {
	// 定义不同 API 路径对应的模拟数据
	mockData := map[string]interface{}{
		// 用户列表 API 的模拟数据
		"/api/v1/users": map[string]interface{}{
			"total": 100,  // 总用户数
			"items": []map[string]interface{}{  // 用户列表
				{"id": 1, "name": "张三", "email": "zhangsan@example.com", "status": "active"},
				{"id": 2, "name": "李四", "email": "lisi@example.com", "status": "active"},
			},
		},
		// 设备列表 API 的模拟数据
		"/api/v1/devices": map[string]interface{}{
			"total": 50,  // 总设备数
			"items": []map[string]interface{}{  // 设备列表
				{"id": 101, "name": "MacBook Pro", "type": "laptop", "status": "online"},
				{"id": 102, "name": "iPhone 15", "type": "mobile", "status": "online"},
			},
		},
		// 示例 API 的模拟数据
		"/api/v1/example/endpoint": map[string]interface{}{
			"timestamp": time.Now().Unix(),  // 当前时间戳
			"status":    "ok",               // 状态
			"message":   "模拟数据响应成功",   // 消息
		},
	}

	// 查找对应路径的模拟数据
	data, exists := mockData[path]
	if !exists {
		// 如果没有找到预定义数据，返回默认的模拟响应
		data = map[string]interface{}{
			"message": "这是一个模拟的API响应",
			"path":    path,
		}
	}

	// 返回通用响应结构
	return &CommonResponse{
		Code:    0,        // 成功状态码
		Message: "success",  // 成功消息
		Data:    data,     // 模拟数据
	}
}

// ensureAccessToken 确保 access_token 可用：
// - 若 token 为空 / 已过期 / 剩余时间 < 30 分钟，则重新获取
func (c *Client) ensureAccessToken() (string, error) {
	c.tokenMu.Lock()
	defer c.tokenMu.Unlock()

	// mock 模式不需要 token
	if c.mockMode {
		return "MOCK_TOKEN", nil
	}

	now := time.Now()
	if c.token != "" && now.Add(30*time.Minute).Before(c.expiresAt) {
		return c.token, nil
	}

	token, expiresIn, err := c.FetchAccessToken()
	if err != nil {
		return "", err
	}
	c.token = token
	c.expiresIn = expiresIn
	c.expiresAt = time.Now().Add(time.Duration(expiresIn) * time.Second)
	logger.Info("access_token refreshed", zap.Int("expires_in", expiresIn))
	return c.token, nil
}

// FetchAccessToken 主动获取 access_token（不使用缓存），用于“测试连接”显示 token 获取结果。
func (c *Client) FetchAccessToken() (token string, expiresIn int, err error) {
	// mock 模式
	if c.mockMode {
		return "MOCK_TOKEN", 7200, nil
	}

	type reqBody struct {
		AccessKeyID     string `json:"access_key_id"`
		AccessKeySecret string `json:"access_key_secret"`
	}
	type respData struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	type respBody struct {
		Code    int      `json:"code"`
		Message string   `json:"message"`
		Data    respData `json:"data"`
	}

	bodyBytes, _ := json.Marshal(reqBody{
		AccessKeyID:     c.accessKey,
		AccessKeySecret: c.secretKey,
	})

	tokenURL := fmt.Sprintf("%s%s", c.baseURL, "/api/open/v1/token")
	req, err := http.NewRequest(http.MethodPost, tokenURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", 0, fmt.Errorf("create token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json;charset=utf-8")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", 0, fmt.Errorf("token request failed: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", 0, fmt.Errorf("read token response: %w", err)
	}

	var parsed respBody
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "", 0, fmt.Errorf("unmarshal token response: %w", err)
	}
	if parsed.Code != 0 {
		return "", 0, fmt.Errorf("get access_token failed: code=%d message=%s", parsed.Code, parsed.Message)
	}
	if parsed.Data.AccessToken == "" || parsed.Data.ExpiresIn <= 0 {
		return "", 0, fmt.Errorf("get access_token failed: empty token or expires_in")
	}
	return parsed.Data.AccessToken, parsed.Data.ExpiresIn, nil
}

// DoRaw 执行 HTTP 请求并返回原始响应内容（不做业务 code 校验）。
// 该方法用于“API 工具箱/调试器”与预览能力：即使业务 code != 0，也需要把响应返回给上层展示。
func (c *Client) DoRaw(method, path string, query map[string]string, body interface{}) (int, []byte, error) {
	// ---------------------------
	// 1. 模拟模式处理
	// ---------------------------
	if c.mockMode {
		logger.Info("returning mock raw response", zap.String("method", method), zap.String("path", path))
		b, _ := json.Marshal(c.getMockResponse(path))
		return http.StatusOK, b, nil
	}

	// ---------------------------
	// 2. 准备请求体
	// ---------------------------
	var reqBody io.Reader
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return 0, nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		reqBody = bytes.NewReader(jsonBody)
	}

	// ---------------------------
	// 3. 组装 URL（含 query）
	// ---------------------------
	rawURL := fmt.Sprintf("%s%s", c.baseURL, path)
	u, err := url.Parse(rawURL)
	if err != nil {
		return 0, nil, fmt.Errorf("failed to parse url: %w", err)
	}
	if len(query) > 0 {
		q := u.Query()
		for k, v := range query {
			q.Set(k, v)
		}
		u.RawQuery = q.Encode()
	}

	// ---------------------------
	// 4. 创建 HTTP 请求
	// ---------------------------
	req, err := http.NewRequest(method, u.String(), reqBody)
	if err != nil {
		return 0, nil, fmt.Errorf("failed to create request: %w", err)
	}

	// ---------------------------
	// 5. 设置请求头
	// ---------------------------
	req.Header.Set("Content-Type", "application/json")

	// 根据飞连 OpenAPI 文档：业务 API 需要在 Header 中携带 Authorization: <access_token>
	// 获取 token 的接口本身不需要 Authorization，避免递归
	if path != "/api/open/v1/token" {
		token, err := c.ensureAccessToken()
		if err != nil {
			return 0, nil, err
		}
		req.Header.Set("Authorization", token)
	}

	logger.Debug("making API request", zap.String("method", method), zap.String("url", u.String()))

	// ---------------------------
	// 6. 发送请求
	// ---------------------------
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// ---------------------------
	// 7. 读取响应
	// ---------------------------
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, nil, fmt.Errorf("failed to read response body: %w", err)
	}
	logger.Debug("API response", zap.Int("status_code", resp.StatusCode), zap.String("body", string(respBody)))
	return resp.StatusCode, respBody, nil
}

// doRequest 执行通用的 HTTP 请求
// 参数:
//   method - HTTP 方法：GET, POST, PUT, DELETE 等
//   path - API 路径，会拼接到 baseURL 后面
//   body - 请求体，会被序列化为 JSON，可为 nil
// 返回:
//   *CommonResponse - API 响应数据
//   error - 请求失败时的错误信息
func (c *Client) doRequest(method, path string, body interface{}) (*CommonResponse, error) {
	status, respBody, err := c.DoRaw(method, path, nil, body)
	if err != nil {
		return nil, err
	}

	var result CommonResponse
	// 将 JSON 响应反序列化为结构体
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	// ---------------------------
	// 8. 检查业务状态码
	// ---------------------------
	if result.Code != 0 {
		// 如果状态码不为 0，表示 API 返回了业务错误
		return nil, fmt.Errorf("API error: http=%d, code=%d, message=%s", status, result.Code, result.Message)
	}

	// 返回成功的响应
	return &result, nil
}

// Get 发送 GET 请求
// 参数:
//   path - API 路径
// 返回:
//   *CommonResponse - API 响应
//   error - 错误信息
func (c *Client) Get(path string) (*CommonResponse, error) {
	return c.doRequest(http.MethodGet, path, nil)
}

// Post 发送 POST 请求
// 参数:
//   path - API 路径
//   body - 请求体对象
// 返回:
//   *CommonResponse - API 响应
//   error - 错误信息
func (c *Client) Post(path string, body interface{}) (*CommonResponse, error) {
	return c.doRequest(http.MethodPost, path, body)
}

// Put 发送 PUT 请求
// 参数:
//   path - API 路径
//   body - 请求体对象
// 返回:
//   *CommonResponse - API 响应
//   error - 错误信息
func (c *Client) Put(path string, body interface{}) (*CommonResponse, error) {
	return c.doRequest(http.MethodPut, path, body)
}

// Delete 发送 DELETE 请求
// 参数:
//   path - API 路径
// 返回:
//   *CommonResponse - API 响应
//   error - 错误信息
func (c *Client) Delete(path string) (*CommonResponse, error) {
	return c.doRequest(http.MethodDelete, path, nil)
}
