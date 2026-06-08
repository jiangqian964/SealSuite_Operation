package runner

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sealsuite-operation/internal/config"
	"sealsuite-operation/internal/sealsuite"
	"sealsuite-operation/internal/storage"
)

func withTempWorkingDir(t *testing.T, fn func(dir string)) {
	t.Helper()
	prev, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd err=%v", err)
	}
	dir := t.TempDir()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir temp dir err=%v", err)
	}
	defer func() {
		if err := os.Chdir(prev); err != nil {
			t.Fatalf("restore cwd err=%v", err)
		}
	}()
	fn(dir)
}

func TestRunScheduleAutomaticPathDeliversWebhookWithoutBreakingStatus(t *testing.T) {
	withTempWorkingDir(t, func(dir string) {
		webhookSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read webhook body err=%v", err)
			}
			text := string(body)
			if !strings.Contains(text, `"source_type":"job_schedule"`) {
				t.Fatalf("expected job_schedule source_type, got=%s", text)
			}
			if !strings.Contains(text, `"source_id":"sched_runner_auto"`) {
				t.Fatalf("expected schedule id in payload, got=%s", text)
			}
			if !strings.Contains(text, `"target_type":"task_draft"`) {
				t.Fatalf("expected task_draft target_type, got=%s", text)
			}
			http.Error(w, "upstream failed", http.StatusBadGateway)
		}))
		defer webhookSrv.Close()

		webhookStore := storage.NewWebhookStore(filepath.Join(dir, "webhooks.yaml"))
		if err := webhookStore.Upsert(storage.WebhookItem{
			ID:      "hook-runner-auto",
			Name:    "Runner Auto Hook",
			URL:     webhookSrv.URL,
			Method:  http.MethodPost,
			Enabled: true,
		}); err != nil {
			t.Fatalf("upsert webhook err=%v", err)
		}

		taskDraftsStore := storage.TaskDraftsStore{Path: filepath.Join(dir, "task-drafts.yaml")}
		if err := taskDraftsStore.Upsert(storage.TaskDraft{
			ID:    "draft_runner_auto",
			Name:  "Runner 自动调度草稿",
			Mode:  "api_only",
			InputConfig: map[string]interface{}{
				"method":      "GET",
				"path":        "/api/v1/users",
				"query":       map[string]interface{}{},
				"path_params": map[string]interface{}{},
				"body":        map[string]interface{}{},
			},
		}); err != nil {
			t.Fatalf("upsert draft err=%v", err)
		}

		cfg := &config.Config{
			Server:    config.ServerConfig{Bind: "127.0.0.1", Port: 0, Mode: "debug"},
			Scheduler: config.SchedulerConfig{Enabled: false, Timezone: "Asia/Shanghai"},
			SealSuite: config.SealSuiteConfig{BaseURL: "http://example.com", AccessKey: "ak", SecretKey: "sk", Timeout: 1, MockMode: true},
			Log:       config.LogConfig{Level: "debug", Filename: "./logs/app.log"},
		}
		client := sealsuite.NewClient(&cfg.SealSuite)
		client.SetMockMode(true)
		r := New(cfg, client, filepath.Join(dir, "jobs.yaml"), filepath.Join(dir, "api-templates.yaml"))

		s := storage.JobSchedule{
			ID:              "sched_runner_auto",
			TargetType:      "task_draft",
			TargetID:        "draft_runner_auto",
			Enabled:         true,
			WebhookConfigID: "hook-runner-auto",
			WebhookEnabled:  true,
			ScheduleType:    "cron",
			Cron:            "0 */5 * * * *",
			Timezone:        "Asia/Shanghai",
		}

		if err := r.runSchedule(s); err != nil {
			t.Fatalf("runSchedule err=%v", err)
		}

		status := r.scheduleStatus[s.ID]
		if status == nil {
			t.Fatalf("expected schedule status")
		}
		if !status.LastOK {
			t.Fatalf("expected LastOK=true despite webhook failure, got=%+v", status)
		}
		if status.LastError != "" {
			t.Fatalf("expected empty LastError, got=%q", status.LastError)
		}

		runFile, err := r.runStore.Load()
		if err != nil {
			t.Fatalf("load run store err=%v", err)
		}
		rec, ok := runFile.Items[scheduleRunKeyPrefix+s.ID]
		if !ok {
			t.Fatalf("expected run record for schedule, got=%v", runFile.Items)
		}
		if !rec.OK {
			t.Fatalf("expected run record OK=true, got=%+v", rec)
		}
		if rec.Error != "" {
			t.Fatalf("expected empty run record error, got=%q", rec.Error)
		}
	})
}

func TestRunScheduleAutomaticPathReturnsSuccessfulWebhookDeliveryWithoutBreakingStatus(t *testing.T) {
	withTempWorkingDir(t, func(dir string) {
		webhookSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read webhook body err=%v", err)
			}
			text := string(body)
			if !strings.Contains(text, `"source_type":"job_schedule"`) {
				t.Fatalf("expected job_schedule source_type, got=%s", text)
			}
			if !strings.Contains(text, `"source_id":"sched_runner_auto_success"`) {
				t.Fatalf("expected schedule id in payload, got=%s", text)
			}
			if !strings.Contains(text, `"target_type":"task_draft"`) {
				t.Fatalf("expected task_draft target_type, got=%s", text)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"received":true}`))
		}))
		defer webhookSrv.Close()

		webhookStore := storage.NewWebhookStore(filepath.Join(dir, "webhooks.yaml"))
		if err := webhookStore.Upsert(storage.WebhookItem{
			ID:      "hook-runner-auto-success",
			Name:    "Runner Auto Success Hook",
			URL:     webhookSrv.URL,
			Method:  http.MethodPost,
			Enabled: true,
		}); err != nil {
			t.Fatalf("upsert webhook err=%v", err)
		}

		taskDraftsStore := storage.TaskDraftsStore{Path: filepath.Join(dir, "task-drafts.yaml")}
		if err := taskDraftsStore.Upsert(storage.TaskDraft{
			ID:   "draft_runner_auto_success",
			Name: "Runner 自动调度草稿成功",
			Mode: "api_only",
			InputConfig: map[string]interface{}{
				"method":      "GET",
				"path":        "/api/v1/users",
				"query":       map[string]interface{}{},
				"path_params": map[string]interface{}{},
				"body":        map[string]interface{}{},
			},
		}); err != nil {
			t.Fatalf("upsert draft err=%v", err)
		}

		cfg := &config.Config{
			Server:    config.ServerConfig{Bind: "127.0.0.1", Port: 0, Mode: "debug"},
			Scheduler: config.SchedulerConfig{Enabled: false, Timezone: "Asia/Shanghai"},
			SealSuite: config.SealSuiteConfig{BaseURL: "http://example.com", AccessKey: "ak", SecretKey: "sk", Timeout: 1, MockMode: true},
			Log:       config.LogConfig{Level: "debug", Filename: "./logs/app.log"},
		}
		client := sealsuite.NewClient(&cfg.SealSuite)
		client.SetMockMode(true)
		r := New(cfg, client, filepath.Join(dir, "jobs.yaml"), filepath.Join(dir, "api-templates.yaml"))

		s := storage.JobSchedule{
			ID:              "sched_runner_auto_success",
			TargetType:      "task_draft",
			TargetID:        "draft_runner_auto_success",
			Enabled:         true,
			WebhookConfigID: "hook-runner-auto-success",
			WebhookEnabled:  true,
			ScheduleType:    "cron",
			Cron:            "0 */5 * * * *",
			Timezone:        "Asia/Shanghai",
		}

		if err := r.runSchedule(s); err != nil {
			t.Fatalf("runSchedule err=%v", err)
		}

		status := r.scheduleStatus[s.ID]
		if status == nil {
			t.Fatalf("expected schedule status")
		}
		if !status.LastOK {
			t.Fatalf("expected LastOK=true on webhook success, got=%+v", status)
		}
		if status.LastError != "" {
			t.Fatalf("expected empty LastError, got=%q", status.LastError)
		}

		runFile, err := r.runStore.Load()
		if err != nil {
			t.Fatalf("load run store err=%v", err)
		}
		rec, ok := runFile.Items[scheduleRunKeyPrefix+s.ID]
		if !ok {
			t.Fatalf("expected run record for schedule, got=%v", runFile.Items)
		}
		if !rec.OK {
			t.Fatalf("expected run record OK=true, got=%+v", rec)
		}
		if rec.Error != "" {
			t.Fatalf("expected empty run record error, got=%q", rec.Error)
		}
	})
}
