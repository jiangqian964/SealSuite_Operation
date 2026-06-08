package webhook

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
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
