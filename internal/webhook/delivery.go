package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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

	req, err := http.NewRequestWithContext(ctx, method, item.URL, bytes.NewReader(body))
	if err != nil {
		return DeliveryResult{Attempted: true, Error: err.Error()}
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range item.Headers {
		req.Header.Set(k, v)
	}

	timeout := 10 * time.Second
	if item.TimeoutSec > 0 {
		timeout = time.Duration(item.TimeoutSec) * time.Second
	}
	client := &http.Client{Timeout: timeout}

	resp, err := client.Do(req)
	if err != nil {
		return DeliveryResult{Attempted: true, Error: err.Error()}
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	return DeliveryResult{
		Attempted:    true,
		OK:           resp.StatusCode >= 200 && resp.StatusCode < 300,
		StatusCode:   resp.StatusCode,
		ResponseBody: string(respBody),
	}
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
		return buildFeishuBotBody(env)
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

func buildFeishuBotBody(env Envelope) ([]byte, error) {
	text := fmt.Sprintf("【%s】执行成功\n类型: %s\n时间: %s\n结果:\n%s",
		env.SourceName,
		env.SourceType,
		env.Timestamp,
		mustJSONString(env.Data),
	)
	return json.Marshal(map[string]interface{}{
		"msg_type": "text",
		"content": map[string]interface{}{
			"text": text,
		},
	})
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
