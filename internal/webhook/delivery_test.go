package webhook

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"sealsuite-operation/internal/storage"
)

func TestDeliverWebhookForSuccessDisabledSkips(t *testing.T) {
	got := DeliverWebhookForSuccess(context.Background(), &storage.WebhookItem{
		Enabled: false,
		URL:     "https://example.com/hook",
	}, Envelope{
		Event:      "task.completed",
		SourceType: "api_task",
	})
	if got.Attempted {
		t.Fatalf("expected attempted=false, got=%+v", got)
	}
}

func TestDeliverWebhookForSuccessPostsEnvelope(t *testing.T) {
	var gotMethod string
	var gotHeader string
	var gotBody map[string]interface{}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotHeader = r.Header.Get("X-Test")
		if contentType := r.Header.Get("Content-Type"); !strings.Contains(contentType, "application/json") {
			t.Fatalf("expected json content-type, got=%q", contentType)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body err=%v", err)
		}
		if err := json.Unmarshal(body, &gotBody); err != nil {
			t.Fatalf("unmarshal body err=%v body=%s", err, string(body))
		}
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"received":true}`))
	}))
	defer srv.Close()

	got := DeliverWebhookForSuccess(context.Background(), &storage.WebhookItem{
		Enabled: true,
		URL:     srv.URL,
		Method:  http.MethodPut,
		Headers: map[string]string{"X-Test": "yes"},
	}, Envelope{
		Event:      "task.completed",
		SourceType: "api_task",
		SourceID:   "task-1",
		SourceName: "Task 1",
		Timestamp:  "2026-06-06T00:00:00Z",
		Data:       map[string]interface{}{"result": "ok"},
	})

	if !got.Attempted || !got.OK {
		t.Fatalf("expected successful delivery, got=%+v", got)
	}
	if got.StatusCode != http.StatusAccepted {
		t.Fatalf("expected status=%d, got=%d", http.StatusAccepted, got.StatusCode)
	}
	if got.ResponseBody != `{"received":true}` {
		t.Fatalf("expected response body, got=%q", got.ResponseBody)
	}
	if gotMethod != http.MethodPut {
		t.Fatalf("expected method=%s, got=%s", http.MethodPut, gotMethod)
	}
	if gotHeader != "yes" {
		t.Fatalf("expected X-Test header, got=%q", gotHeader)
	}
	if gotBody["event"] != "task.completed" {
		t.Fatalf("expected event, got=%v", gotBody)
	}
	if gotBody["source_type"] != "api_task" {
		t.Fatalf("expected source_type, got=%v", gotBody)
	}
	if gotBody["source_id"] != "task-1" {
		t.Fatalf("expected source_id, got=%v", gotBody)
	}
}

func TestDeliverWebhookForSuccessGenericUsesBodyTemplate(t *testing.T) {
	var gotBody map[string]interface{}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body err=%v", err)
		}
		if err := json.Unmarshal(body, &gotBody); err != nil {
			t.Fatalf("unmarshal body err=%v body=%s", err, string(body))
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	got := DeliverWebhookForSuccess(context.Background(), &storage.WebhookItem{
		Enabled:  true,
		URL:      srv.URL,
		Method:   http.MethodPost,
		Provider: "generic",
		BodyTmpl: `{"title":"{{source_name}}","kind":"{{source_type}}","when":"{{timestamp}}","message":"{{jsonString data}}"}`,
	}, Envelope{
		Event:      "task.completed",
		SourceType: "api_task",
		SourceID:   "task-1",
		SourceName: "任务A",
		Timestamp:  "2026-06-06T00:00:00Z",
		Data: map[string]interface{}{
			"result": "ok",
			"count":  2,
		},
	})

	if !got.OK {
		t.Fatalf("expected ok=true, got=%+v", got)
	}
	if gotBody["title"] != "任务A" {
		t.Fatalf("expected title rendered, got=%v", gotBody)
	}
	if gotBody["kind"] != "api_task" {
		t.Fatalf("expected kind rendered, got=%v", gotBody)
	}
	if gotBody["when"] != "2026-06-06T00:00:00Z" {
		t.Fatalf("expected timestamp rendered, got=%v", gotBody)
	}
	message, _ := gotBody["message"].(string)
	if !strings.Contains(message, `"result":"ok"`) || !strings.Contains(message, `"count":2`) {
		t.Fatalf("expected json data rendered in message, got=%v", gotBody)
	}
}

func TestDeliverWebhookForSuccessGenericSupportsLegacyTitleMessagePlaceholders(t *testing.T) {
	var gotBody map[string]interface{}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body err=%v", err)
		}
		if err := json.Unmarshal(body, &gotBody); err != nil {
			t.Fatalf("unmarshal body err=%v body=%s", err, string(body))
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	got := DeliverWebhookForSuccess(context.Background(), &storage.WebhookItem{
		Enabled:  true,
		URL:      srv.URL,
		Method:   http.MethodPost,
		Provider: "generic",
		BodyTmpl: `{"title":"{{title}}","message":"{{message}}"}`,
	}, Envelope{
		Event:      "task.completed",
		SourceType: "api_task",
		SourceID:   "task-legacy",
		SourceName: "老模板任务",
		Timestamp:  "2026-06-06T18:00:00Z",
		Data: map[string]interface{}{
			"ok":     true,
			"result": "done",
		},
	})

	if !got.OK {
		t.Fatalf("expected ok=true, got=%+v", got)
	}
	if gotBody["title"] != "老模板任务" {
		t.Fatalf("expected legacy title rendered, got=%v", gotBody)
	}
	message, _ := gotBody["message"].(string)
	if !strings.Contains(message, `"ok":true`) || !strings.Contains(message, `"result":"done"`) {
		t.Fatalf("expected legacy message rendered with json data, got=%v", gotBody)
	}
}

func TestDeliverWebhookForSuccessFeishuBotBuildsTextPayload(t *testing.T) {
	var gotBody map[string]interface{}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body err=%v", err)
		}
		if err := json.Unmarshal(body, &gotBody); err != nil {
			t.Fatalf("unmarshal body err=%v body=%s", err, string(body))
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	got := DeliverWebhookForSuccess(context.Background(), &storage.WebhookItem{
		Enabled:  true,
		URL:      srv.URL,
		Method:   http.MethodPost,
		Provider: "feishu_bot",
		BodyTmpl: `{"ignored":true}`,
	}, Envelope{
		Event:      "task.completed",
		SourceType: "complex_task",
		SourceID:   "task-2",
		SourceName: "运营agent A",
		Timestamp:  "2026-06-06T08:00:00Z",
		Data: map[string]interface{}{
			"summary": "done",
			"ok":      true,
		},
	})

	if !got.OK {
		t.Fatalf("expected ok=true, got=%+v", got)
	}
	if gotBody["msg_type"] != "text" {
		t.Fatalf("expected msg_type=text, got=%v", gotBody)
	}
	content, _ := gotBody["content"].(map[string]interface{})
	text, _ := content["text"].(string)
	if text == "" {
		t.Fatalf("expected content.text, got=%v", gotBody)
	}
	if !strings.Contains(text, "运营agent A") {
		t.Fatalf("expected source_name in text, got=%q", text)
	}
	if !strings.Contains(text, "complex_task") {
		t.Fatalf("expected source_type in text, got=%q", text)
	}
	if !strings.Contains(text, "2026-06-06T08:00:00Z") {
		t.Fatalf("expected timestamp in text, got=%q", text)
	}
	if !strings.Contains(text, `"summary":"done"`) || !strings.Contains(text, `"ok":true`) {
		t.Fatalf("expected json data in text, got=%q", text)
	}
}

func TestDeliverWebhookForSuccessFailureDoesNotPanic(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := srv.URL
	srv.Close()

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("expected no panic, got=%v", r)
		}
	}()

	got := DeliverWebhookForSuccess(context.Background(), &storage.WebhookItem{
		Enabled: true,
		URL:     url,
	}, Envelope{
		Event:      "task.completed",
		SourceType: "api_task",
	})

	if !got.Attempted {
		t.Fatalf("expected attempted=true, got=%+v", got)
	}
	if got.OK {
		t.Fatalf("expected ok=false on request failure, got=%+v", got)
	}
	if got.Error == "" {
		t.Fatalf("expected error message, got=%+v", got)
	}
}

func TestDeliverWebhookForSuccessNon2xxReturnsStructuredFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad gateway", http.StatusBadGateway)
	}))
	defer srv.Close()

	got := DeliverWebhookForSuccess(context.Background(), &storage.WebhookItem{
		Enabled: true,
		URL:     srv.URL,
		Method:  http.MethodPost,
	}, Envelope{
		Event:      "task.completed",
		SourceType: "job_schedule",
		SourceID:   "sched-1",
	})

	if !got.Attempted {
		t.Fatalf("expected attempted=true, got=%+v", got)
	}
	if got.OK {
		t.Fatalf("expected ok=false for non-2xx response, got=%+v", got)
	}
	if got.StatusCode != http.StatusBadGateway {
		t.Fatalf("expected status=%d, got=%d", http.StatusBadGateway, got.StatusCode)
	}
	if !strings.Contains(got.ResponseBody, "bad gateway") {
		t.Fatalf("expected response body captured, got=%q", got.ResponseBody)
	}
}

// 构造一个固定的飞书投递信封，供后续消息类型测试复用
func sampleFeishuEnvelope() Envelope {
	return Envelope{
		Event:      "task.completed",
		SourceType: "complex_task",
		SourceID:   "task-9",
		SourceName: "运营agent B",
		Timestamp:  "2026-06-06T08:00:00Z",
		Data: map[string]interface{}{
			"ok":      true,
			"summary": "done",
		},
	}
}

func TestNormalizeFeishuMsgType(t *testing.T) {
	cases := map[string]string{
		"":            "text",
		"text":        "text",
		"TEXT":        "text",
		" post ":      "post",
		"post":        "post",
		"card":        "interactive",
		"interactive": "interactive",
		"unknown":     "text",
	}
	for in, want := range cases {
		if got := NormalizeFeishuMsgType(in); got != want {
			t.Fatalf("NormalizeFeishuMsgType(%q)=%q want=%q", in, got, want)
		}
	}
}

func TestGenFeishuBotSignMatchesIndependentRecompute(t *testing.T) {
	timestamp := int64(1599360473)
	secret := "abc-secret"

	got := genFeishuBotSign(timestamp, secret)

	// 使用标准库独立重算，确保实现与飞书官方算法一致：
	// key = "{timestamp}\n{secret}"，对空消息求 HMAC-SHA256 后 base64 编码。
	stringToSign := fmt.Sprintf("%d\n%s", timestamp, secret)
	mac := hmac.New(sha256.New, []byte(stringToSign))
	want := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	if got != want {
		t.Fatalf("sign mismatch got=%q want=%q", got, want)
	}
}

// 配置了 secret 时，飞书 text 消息必须携带可校验的 timestamp 与 sign
func TestDeliverWebhookForSuccessFeishuBotTextAddsValidSignature(t *testing.T) {
	var gotBody map[string]interface{}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body err=%v", err)
		}
		if err := json.Unmarshal(body, &gotBody); err != nil {
			t.Fatalf("unmarshal body err=%v body=%s", err, string(body))
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	got := DeliverWebhookForSuccess(context.Background(), &storage.WebhookItem{
		Enabled:  true,
		URL:      srv.URL,
		Provider: "feishu_bot",
		MsgType:  "text",
		Secret:   "abc-secret",
	}, sampleFeishuEnvelope())

	if !got.OK {
		t.Fatalf("expected ok=true, got=%+v", got)
	}
	ts, _ := gotBody["timestamp"].(string)
	if ts == "" {
		t.Fatalf("expected timestamp, got=%v", gotBody)
	}
	timestamp, err := strconv.ParseInt(ts, 10, 64)
	if err != nil {
		t.Fatalf("parse timestamp err=%v", err)
	}
	sign, _ := gotBody["sign"].(string)
	if sign == "" {
		t.Fatalf("expected sign, got=%v", gotBody)
	}
	if want := genFeishuBotSign(timestamp, "abc-secret"); sign != want {
		t.Fatalf("sign not valid got=%q want=%q", sign, want)
	}
}

// msg_type=post 时生成富文本结构，且未配置 secret 时不应出现 sign
func TestBuildFeishuBotPostPayload(t *testing.T) {
	body, err := BuildRequestBodyForDelivery(&storage.WebhookItem{
		Provider: "feishu_bot",
		MsgType:  "post",
	}, sampleFeishuEnvelope())
	if err != nil {
		t.Fatalf("build err=%v", err)
	}

	var m map[string]interface{}
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatalf("unmarshal err=%v body=%s", err, string(body))
	}
	if m["msg_type"] != "post" {
		t.Fatalf("expected msg_type=post, got=%v", m)
	}
	content, _ := m["content"].(map[string]interface{})
	post, _ := content["post"].(map[string]interface{})
	zhCN, _ := post["zh_cn"].(map[string]interface{})
	if zhCN == nil {
		t.Fatalf("expected content.post.zh_cn, got=%v", m)
	}
	if title, _ := zhCN["title"].(string); !strings.Contains(title, "运营agent B") {
		t.Fatalf("expected title with source name, got=%v", zhCN["title"])
	}
	if zhCN["content"] == nil {
		t.Fatalf("expected content rows, got=%v", zhCN)
	}
	if _, ok := m["sign"]; ok {
		t.Fatalf("did not expect sign when secret empty, got=%v", m)
	}
}

// msg_type=interactive 时生成卡片结构，并在配置 secret 时附带加签字段
func TestBuildFeishuBotInteractiveCardPayload(t *testing.T) {
	body, err := BuildRequestBodyForDelivery(&storage.WebhookItem{
		Provider: "feishu_bot",
		MsgType:  "interactive",
		Secret:   "card-secret",
	}, sampleFeishuEnvelope())
	if err != nil {
		t.Fatalf("build err=%v", err)
	}

	var m map[string]interface{}
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatalf("unmarshal err=%v body=%s", err, string(body))
	}
	if m["msg_type"] != "interactive" {
		t.Fatalf("expected msg_type=interactive, got=%v", m)
	}
	card, _ := m["card"].(map[string]interface{})
	if card == nil {
		t.Fatalf("expected card object, got=%v", m)
	}
	if card["header"] == nil || card["elements"] == nil {
		t.Fatalf("expected card header and elements, got=%v", card)
	}
	ts, _ := m["timestamp"].(string)
	sign, _ := m["sign"].(string)
	if ts == "" || sign == "" {
		t.Fatalf("expected timestamp and sign, got=%v", m)
	}
	timestamp, _ := strconv.ParseInt(ts, 10, 64)
	if want := genFeishuBotSign(timestamp, "card-secret"); sign != want {
		t.Fatalf("card sign invalid got=%q want=%q", sign, want)
	}
}
