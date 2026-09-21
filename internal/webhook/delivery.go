package webhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"sealsuite-operation/internal/storage"
)

type DeliveryResult struct {
	Attempted    bool   `json:"attempted"`
	OK           bool   `json:"ok"`
	StatusCode   int    `json:"status_code,omitempty"`
	ResponseBody string `json:"response_body,omitempty"`
	Error        string `json:"error,omitempty"`
}

type Envelope struct {
	Event      string      `json:"event"`
	SourceType string      `json:"source_type"`
	SourceID   string      `json:"source_id"`
	SourceName string      `json:"source_name"`
	Timestamp  string      `json:"timestamp"`
	Data       interface{} `json:"data"`
}

// DeliverWebhookForSuccess returns a structured result that callers can expose as webhook_delivery.
// 支持重试机制：根据 item.RetryCount 配置进行多次投递尝试，使用指数退避策略。
func DeliverWebhookForSuccess(ctx context.Context, item *storage.WebhookItem, env Envelope) DeliveryResult {
	if item == nil || !item.Enabled || item.URL == "" {
		return DeliveryResult{Attempted: false}
	}

	body, err := BuildRequestBodyForDelivery(item, env)
	if err != nil {
		return DeliveryResult{Attempted: true, Error: err.Error()}
	}

	method := item.Method
	if method == "" {
		method = http.MethodPost
	}

	timeout := 10 * time.Second
	if item.TimeoutSec > 0 {
		timeout = time.Duration(item.TimeoutSec) * time.Second
	}
	clt := &http.Client{Timeout: timeout}

	maxAttempts := 1
	if item.RetryCount > 0 {
		maxAttempts = item.RetryCount + 1
	}

	var lastResult DeliveryResult
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(1<<uint(attempt-1)) * time.Second
			if backoff > 30*time.Second {
				backoff = 30 * time.Second
			}
			select {
			case <-ctx.Done():
				return lastResult
			case <-time.After(backoff):
			}
		}

		req, err := http.NewRequestWithContext(ctx, method, item.URL, bytes.NewReader(body))
		if err != nil {
			lastResult = DeliveryResult{Attempted: true, Error: err.Error()}
			continue
		}
		req.Header.Set("Content-Type", "application/json")
		for k, v := range item.Headers {
			req.Header.Set(k, v)
		}

		resp, err := clt.Do(req)
		if err != nil {
			lastResult = DeliveryResult{Attempted: true, Error: err.Error()}
			continue
		}
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		lastResult = DeliveryResult{
			Attempted:    true,
			OK:           resp.StatusCode >= 200 && resp.StatusCode < 300,
			StatusCode:   resp.StatusCode,
			ResponseBody: string(respBody),
		}

		if lastResult.OK {
			return lastResult
		}
	}

	return lastResult
}

func BuildRequestBodyForDelivery(item *storage.WebhookItem, env Envelope) ([]byte, error) {
	return buildWebhookRequestBody(item, env, env)
}

func BuildRequestBodyForTest(item *storage.WebhookItem, payload interface{}) ([]byte, error) {
	env := Envelope{
		Event:      "webhook.test",
		SourceType: "webhook_test",
		SourceID:   "manual_test",
		SourceName: "Webhook Test",
		Timestamp:  time.Now().Format(time.RFC3339),
		Data:       payload,
	}
	return buildWebhookRequestBody(item, env, payload)
}

func NormalizeProvider(provider string) string {
	return normalizeWebhookProvider(provider)
}

func buildWebhookRequestBody(item *storage.WebhookItem, env Envelope, genericPayload interface{}) ([]byte, error) {
	if item == nil {
		return nil, fmt.Errorf("webhook item is nil")
	}
	switch normalizeWebhookProvider(item.Provider) {
	case "feishu_bot":
		return buildFeishuBotBody(item, env)
	default:
		return buildGenericWebhookBody(item, env, genericPayload)
	}
}

func buildGenericWebhookBody(item *storage.WebhookItem, env Envelope, payload interface{}) ([]byte, error) {
	if strings.TrimSpace(item.BodyTmpl) == "" {
		switch v := payload.(type) {
		case nil:
			return nil, nil
		case string:
			return []byte(v), nil
		default:
			return json.Marshal(v)
		}
	}

	rendered, err := renderWebhookTemplate(item.BodyTmpl, env)
	if err != nil {
		return nil, err
	}
	return []byte(rendered), nil
}

// NormalizeFeishuMsgType 将飞书机器人消息类型归一化为 text / post / interactive，默认 text。
func NormalizeFeishuMsgType(msgType string) string {
	switch strings.ToLower(strings.TrimSpace(msgType)) {
	case "post":
		return "post"
	case "interactive", "card":
		return "interactive"
	default:
		return "text"
	}
}

// buildFeishuBotBody 按配置生成飞书自定义群机器人消息体。
// 根据 item.MsgType 生成 text / post / interactive 三种消息；
// 当 item.Secret 非空（机器人启用了“签名校验”）时，补充 timestamp 与 sign 字段。
func buildFeishuBotBody(item *storage.WebhookItem, env Envelope) ([]byte, error) {
	msgType := NormalizeFeishuMsgType(item.MsgType)

	var payload map[string]interface{}
	switch msgType {
	case "post":
		payload = buildFeishuPostPayload(env)
	case "interactive":
		payload = buildFeishuCardPayload(env)
	default:
		payload = buildFeishuTextPayload(env)
	}

	// 加签：飞书机器人安全设置启用“签名校验”后，每次请求都必须携带 timestamp 与 sign。
	if secret := strings.TrimSpace(item.Secret); secret != "" {
		timestamp := time.Now().Unix()
		payload["timestamp"] = strconv.FormatInt(timestamp, 10)
		payload["sign"] = genFeishuBotSign(timestamp, secret)
	}
	return json.Marshal(payload)
}

// buildFeishuTextPayload 构造 text 纯文本消息。
func buildFeishuTextPayload(env Envelope) map[string]interface{} {
	text := fmt.Sprintf("【%s】执行通知\n类型: %s\n时间: %s\n结果:\n%s",
		env.SourceName,
		env.SourceType,
		env.Timestamp,
		mustJSONString(env.Data),
	)
	return map[string]interface{}{
		"msg_type": "text",
		"content": map[string]interface{}{
			"text": text,
		},
	}
}

// buildFeishuPostPayload 构造 post 富文本消息（zh_cn），标题+分段正文。
func buildFeishuPostPayload(env Envelope) map[string]interface{} {
	// content 为二维数组：每个子数组代表一行，行内可包含多个不同 tag 的元素。
	content := [][]interface{}{
		{
			map[string]interface{}{
				"tag":  "text",
				"text": fmt.Sprintf("类型：%s\n时间：%s", env.SourceType, env.Timestamp),
			},
		},
		{
			map[string]interface{}{
				"tag":  "text",
				"text": "执行结果：",
			},
		},
		{
			map[string]interface{}{
				"tag":  "text",
				"text": mustJSONString(env.Data),
			},
		},
	}
	return map[string]interface{}{
		"msg_type": "post",
		"content": map[string]interface{}{
			"post": map[string]interface{}{
				"zh_cn": map[string]interface{}{
					"title":   fmt.Sprintf("【%s】执行通知", env.SourceName),
					"content": content,
				},
			},
		},
	}
}

// buildFeishuCardPayload 构造 interactive 交互卡片消息：蓝色标题 + 类型/时间字段 + 分隔线 + 结果代码块。
func buildFeishuCardPayload(env Envelope) map[string]interface{} {
	return map[string]interface{}{
		"msg_type": "interactive",
		"card": map[string]interface{}{
			"config": map[string]interface{}{
				"wide_screen_mode": true,
			},
			"header": map[string]interface{}{
				"template": "blue",
				"title": map[string]interface{}{
					"tag":     "plain_text",
					"content": fmt.Sprintf("【%s】执行通知", env.SourceName),
				},
			},
			"elements": []interface{}{
				map[string]interface{}{
					"tag": "div",
					"fields": []interface{}{
						map[string]interface{}{
							"is_short": true,
							"text": map[string]interface{}{
								"tag":     "lark_md",
								"content": fmt.Sprintf("**类型**\n%s", env.SourceType),
							},
						},
						map[string]interface{}{
							"is_short": true,
							"text": map[string]interface{}{
								"tag":     "lark_md",
								"content": fmt.Sprintf("**时间**\n%s", env.Timestamp),
							},
						},
					},
				},
				map[string]interface{}{
					"tag": "hr",
				},
				map[string]interface{}{
					"tag": "div",
					"text": map[string]interface{}{
						"tag":     "lark_md",
						"content": fmt.Sprintf("**执行结果**\n```\n%s\n```", mustJSONString(env.Data)),
					},
				},
			},
		},
	}
}

// genFeishuBotSign 计算飞书自定义机器人加签签名。
// 算法：string_to_sign = "{timestamp}\n{secret}"，将其作为 HMAC-SHA256 的密钥、对空消息求摘要，再做 base64 编码。
func genFeishuBotSign(timestamp int64, secret string) string {
	stringToSign := fmt.Sprintf("%d\n%s", timestamp, secret)
	mac := hmac.New(sha256.New, []byte(stringToSign))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func renderWebhookTemplate(tmpl string, env Envelope) (string, error) {
	replacer := strings.NewReplacer(
		"{{title}}", env.SourceName,
		"{{message}}", mustJSONEscapedString(env.Data),
		"{{source_type}}", env.SourceType,
		"{{source_id}}", env.SourceID,
		"{{source_name}}", env.SourceName,
		"{{timestamp}}", env.Timestamp,
		"{{json data}}", mustJSONString(env.Data),
		"{{json envelope}}", mustJSONString(env),
		"{{jsonString data}}", mustJSONEscapedString(env.Data),
		"{{jsonString envelope}}", mustJSONEscapedString(env),
	)
	return replacer.Replace(tmpl), nil
}

func mustJSONString(v interface{}) string {
	data, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(data)
}

func mustJSONEscapedString(v interface{}) string {
	encoded, err := json.Marshal(mustJSONString(v))
	if err != nil {
		return "{}"
	}
	if len(encoded) >= 2 {
		return string(encoded[1 : len(encoded)-1])
	}
	return ""
}

func normalizeWebhookProvider(provider string) string {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "feishu_bot":
		return "feishu_bot"
	default:
		return "generic"
	}
}
