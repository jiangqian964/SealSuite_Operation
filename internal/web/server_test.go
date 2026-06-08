package web

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sealsuite-operation/internal/config"
	"sealsuite-operation/internal/runner"
	"sealsuite-operation/internal/sealsuite"
	"sealsuite-operation/internal/storage"
)

func TestIndexServed(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{Bind: "127.0.0.1", Port: 0, Mode: "debug"},
		Scheduler: config.SchedulerConfig{Enabled: false, Timezone: "Asia/Shanghai"},
		SealSuite: config.SealSuiteConfig{BaseURL: "http://example.com", AccessKey: "ak", SecretKey: "sk", Timeout: 1},
		Log: config.LogConfig{Level: "debug", Filename: "./logs/app.log"},
	}
	client := sealsuite.NewClient(&cfg.SealSuite)
	r := runner.New(cfg, client, "jobs.yaml", "api-templates.yaml")
	h, err := NewRouter(cfg, r)
	if err != nil {
		t.Fatalf("NewRouter err=%v", err)
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if len(rr.Body.Bytes()) == 0 {
		t.Fatalf("expected body")
	}
}

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

func TestConnectionTestReturnsTokenFields(t *testing.T) {
	// Contract test: /connection/test should explicitly tell whether token is valid.
	feilian := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/open/v1/token" && r.Method == http.MethodPost {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"code":0,"message":"success","data":{"access_token":"T","expires_in":7200}}`))
			return
		}
		if r.URL.Path == "/api/v1/users" && r.Method == http.MethodGet {
			if r.Header.Get("Authorization") != "T" {
				http.Error(w, "missing token", http.StatusUnauthorized)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"code":0,"message":"success","data":{}}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer feilian.Close()

	cfg := &config.Config{
		Server:    config.ServerConfig{Bind: "127.0.0.1", Port: 0, Mode: "debug"},
		Scheduler: config.SchedulerConfig{Enabled: false, Timezone: "Asia/Shanghai"},
		SealSuite: config.SealSuiteConfig{BaseURL: feilian.URL, AccessKey: "ak", SecretKey: "sk", Timeout: 1},
		Log:       config.LogConfig{Level: "debug", Filename: "./logs/app.log"},
	}
	client := sealsuite.NewClient(&cfg.SealSuite)
	r := runner.New(cfg, client, "jobs.yaml", "api-templates.yaml")
	h, err := NewRouter(cfg, r)
	if err != nil {
		t.Fatalf("NewRouter err=%v", err)
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/connection/test", strings.NewReader(`{"base_url":"`+feilian.URL+`","access_key":"ak","secret_key":"sk"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}

	var out map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("invalid json: %v body=%s", err, rr.Body.String())
	}
	if _, ok := out["token_ok"]; !ok {
		t.Fatalf("expected token_ok in response, got=%v", out)
	}
}

func TestDeleteInactiveConnectionEndpoint(t *testing.T) {
	withTempWorkingDir(t, func(dir string) {
		store := storage.ConnectionsStore{Path: filepath.Join(dir, "connections.yaml")}
		if err := store.Save(&storage.ConnectionsFile{
			Version:  1,
			ActiveID: "conn-active",
			Items: []storage.ConnectionItem{
				{ID: "conn-inactive", Name: "inactive"},
				{ID: "conn-active", Name: "active"},
			},
		}); err != nil {
			t.Fatalf("Save err=%v", err)
		}

		cfg := &config.Config{
			Server:    config.ServerConfig{Bind: "127.0.0.1", Port: 0, Mode: "debug"},
			Scheduler: config.SchedulerConfig{Enabled: false, Timezone: "Asia/Shanghai"},
			SealSuite: config.SealSuiteConfig{BaseURL: "http://example.com", AccessKey: "ak", SecretKey: "sk", Timeout: 1},
			Log:       config.LogConfig{Level: "debug", Filename: "./logs/app.log"},
		}
		client := sealsuite.NewClient(&cfg.SealSuite)
		r := runner.New(cfg, client, "jobs.yaml", "api-templates.yaml")
		h, err := NewRouter(cfg, r)
		if err != nil {
			t.Fatalf("NewRouter err=%v", err)
		}

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodDelete, "/api/v1/connections/conn-inactive", nil)
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
		}

		got, err := store.Load()
		if err != nil {
			t.Fatalf("Load err=%v", err)
		}
		if len(got.Items) != 1 || got.Items[0].ID != "conn-active" {
			t.Fatalf("expected only active item left, got=%+v", got.Items)
		}
	})
}

func TestDeleteActiveConnectionEndpointRejected(t *testing.T) {
	withTempWorkingDir(t, func(dir string) {
		store := storage.ConnectionsStore{Path: filepath.Join(dir, "connections.yaml")}
		if err := store.Save(&storage.ConnectionsFile{
			Version:  1,
			ActiveID: "conn-active",
			Items: []storage.ConnectionItem{
				{ID: "conn-inactive", Name: "inactive"},
				{ID: "conn-active", Name: "active"},
			},
		}); err != nil {
			t.Fatalf("Save err=%v", err)
		}

		cfg := &config.Config{
			Server:    config.ServerConfig{Bind: "127.0.0.1", Port: 0, Mode: "debug"},
			Scheduler: config.SchedulerConfig{Enabled: false, Timezone: "Asia/Shanghai"},
			SealSuite: config.SealSuiteConfig{BaseURL: "http://example.com", AccessKey: "ak", SecretKey: "sk", Timeout: 1},
			Log:       config.LogConfig{Level: "debug", Filename: "./logs/app.log"},
		}
		client := sealsuite.NewClient(&cfg.SealSuite)
		r := runner.New(cfg, client, "jobs.yaml", "api-templates.yaml")
		h, err := NewRouter(cfg, r)
		if err != nil {
			t.Fatalf("NewRouter err=%v", err)
		}

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodDelete, "/api/v1/connections/conn-active", nil)
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d body=%s", rr.Code, rr.Body.String())
		}

		var out map[string]interface{}
		if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
			t.Fatalf("invalid json: %v body=%s", err, rr.Body.String())
		}
		if out["error"] != "cannot delete active connection" {
			t.Fatalf("expected active delete rejection, got=%v", out)
		}
	})
}

func TestJobsCRUDEndpoints(t *testing.T) {
	cfg := &config.Config{
		Server:    config.ServerConfig{Bind: "127.0.0.1", Port: 0, Mode: "debug"},
		Scheduler: config.SchedulerConfig{Enabled: false, Timezone: "Asia/Shanghai"},
		SealSuite: config.SealSuiteConfig{BaseURL: "http://example.com", AccessKey: "ak", SecretKey: "sk", Timeout: 1},
		Log:       config.LogConfig{Level: "debug", Filename: "./logs/app.log"},
	}
	client := sealsuite.NewClient(&cfg.SealSuite)
	r := runner.New(cfg, client, "jobs.yaml", "api-templates.yaml")
	h, err := NewRouter(cfg, r)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/jobs", strings.NewReader(`{"name":"job-a","enabled":true,"cron":"0 */1 * * * *","type":"api_poll","params":{"template_id":"users_list"}}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("create got %d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/v1/jobs/job-a", nil)
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("get got %d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodDelete, "/api/v1/jobs/job-a", nil)
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("delete got %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestTemplateCRUDEndpoints(t *testing.T) {
	cfg := &config.Config{
		Server:    config.ServerConfig{Bind: "127.0.0.1", Port: 0, Mode: "debug"},
		Scheduler: config.SchedulerConfig{Enabled: false, Timezone: "Asia/Shanghai"},
		SealSuite: config.SealSuiteConfig{BaseURL: "http://example.com", AccessKey: "ak", SecretKey: "sk", Timeout: 1},
		Log:       config.LogConfig{Level: "debug", Filename: "./logs/app.log"},
	}
	client := sealsuite.NewClient(&cfg.SealSuite)
	r := runner.New(cfg, client, "jobs.yaml", "api-templates.yaml")
	h, err := NewRouter(cfg, r)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/api/templates", strings.NewReader(`{"id":"users_list_tmp","name":"用户-列表","category":"users","method":"GET","path":"/api/open/v1/users"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("create got %d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/v1/api/templates/users_list_tmp", nil)
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("get got %d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodDelete, "/api/v1/api/templates/users_list_tmp", nil)
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("delete got %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestTaskDraftCRUDEndpoints(t *testing.T) {
	cfg := &config.Config{
		Server:    config.ServerConfig{Bind: "127.0.0.1", Port: 0, Mode: "debug"},
		Scheduler: config.SchedulerConfig{Enabled: false, Timezone: "Asia/Shanghai"},
		SealSuite: config.SealSuiteConfig{BaseURL: "http://example.com", AccessKey: "ak", SecretKey: "sk", Timeout: 1},
		Log:       config.LogConfig{Level: "debug", Filename: "./logs/app.log"},
	}
	client := sealsuite.NewClient(&cfg.SealSuite)
	r := runner.New(cfg, client, "jobs.yaml", "api-templates.yaml")
	h, err := NewRouter(cfg, r)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/task-drafts", strings.NewReader(`{"id":"draft_users_sync","name":"用户同步草稿","mode":"api_only","source_template_id":"users_list"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("create got %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestJobScheduleCRUDEndpoints(t *testing.T) {
	cfg := &config.Config{
		Server:    config.ServerConfig{Bind: "127.0.0.1", Port: 0, Mode: "debug"},
		Scheduler: config.SchedulerConfig{Enabled: false, Timezone: "Asia/Shanghai"},
		SealSuite: config.SealSuiteConfig{BaseURL: "http://example.com", AccessKey: "ak", SecretKey: "sk", Timeout: 1},
		Log:       config.LogConfig{Level: "debug", Filename: "./logs/app.log"},
	}
	client := sealsuite.NewClient(&cfg.SealSuite)
	r := runner.New(cfg, client, "jobs.yaml", "api-templates.yaml")
	h, err := NewRouter(cfg, r)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/task-drafts", strings.NewReader(`{"id":"draft_users_sync","name":"用户同步草稿","mode":"api_only","source_template_id":"users_list"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(rr, req)

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/v1/job-schedules", strings.NewReader(`{"id":"sched_users_daily","target_type":"task_draft","target_id":"draft_users_sync","enabled":true,"schedule_type":"cron","cron":"0 0 9 * * *","timezone":"Asia/Shanghai"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("create got %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestComplexTaskCRUDEndpoints(t *testing.T) {
	cfg := &config.Config{
		Server:    config.ServerConfig{Bind: "127.0.0.1", Port: 0, Mode: "debug"},
		Scheduler: config.SchedulerConfig{Enabled: false, Timezone: "Asia/Shanghai"},
		SealSuite: config.SealSuiteConfig{BaseURL: "http://example.com", AccessKey: "ak", SecretKey: "sk", Timeout: 1},
		Log:       config.LogConfig{Level: "debug", Filename: "./logs/app.log"},
	}
	client := sealsuite.NewClient(&cfg.SealSuite)
	r := runner.New(cfg, client, "jobs.yaml", "api-templates.yaml")
	h, err := NewRouter(cfg, r)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/complex-tasks", strings.NewReader(`{"id":"weekly-security-summary","name":"每周安全汇总","execution_mode":"manual","steps":[{"id":"s1","type":"api_call","name":"查询设备"}]}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("create got %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestJobScheduleRunEndpointAndDetail(t *testing.T) {
	cfg := &config.Config{
		Server:    config.ServerConfig{Bind: "127.0.0.1", Port: 0, Mode: "debug"},
		Scheduler: config.SchedulerConfig{Enabled: false, Timezone: "Asia/Shanghai"},
		SealSuite: config.SealSuiteConfig{BaseURL: "http://example.com", AccessKey: "ak", SecretKey: "sk", Timeout: 1, MockMode: true},
		Log:       config.LogConfig{Level: "debug", Filename: "./logs/app.log"},
	}
	client := sealsuite.NewClient(&cfg.SealSuite)
	client.SetMockMode(true)
	r := runner.New(cfg, client, "jobs.yaml", "api-templates.yaml")
	h, err := NewRouter(cfg, r)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/task-drafts", strings.NewReader(`{
		"id":"draft_runtime_users",
		"name":"运行态用户查询",
		"mode":"api_only",
		"input_config":{"method":"GET","path":"/api/v1/users","query":{},"path_params":{},"body":{}}
	}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("draft create got %d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/v1/job-schedules", strings.NewReader(`{
		"id":"sched_runtime_users",
		"target_type":"task_draft",
		"target_id":"draft_runtime_users",
		"enabled":true,
		"schedule_type":"cron",
		"cron":"0 */5 * * * *",
		"timezone":"Asia/Shanghai"
	}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("schedule create got %d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/v1/job-schedules/sched_runtime_users/run", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("schedule run got %d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/v1/job-schedules/sched_runtime_users", nil)
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("schedule detail got %d body=%s", rr.Code, rr.Body.String())
	}

	var out map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("invalid json: %v body=%s", err, rr.Body.String())
	}
	if _, ok := out["run"]; !ok {
		t.Fatalf("expected run in schedule detail, got=%v", out)
	}
}

func TestComplexTaskRunEndpoint(t *testing.T) {
	cfg := &config.Config{
		Server:    config.ServerConfig{Bind: "127.0.0.1", Port: 0, Mode: "debug"},
		Scheduler: config.SchedulerConfig{Enabled: false, Timezone: "Asia/Shanghai"},
		SealSuite: config.SealSuiteConfig{BaseURL: "http://example.com", AccessKey: "ak", SecretKey: "sk", Timeout: 1, MockMode: true},
		Log:       config.LogConfig{Level: "debug", Filename: "./logs/app.log"},
	}
	client := sealsuite.NewClient(&cfg.SealSuite)
	client.SetMockMode(true)
	r := runner.New(cfg, client, "jobs.yaml", "api-templates.yaml")
	h, err := NewRouter(cfg, r)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/complex-tasks", strings.NewReader(`{
		"id":"complex_runtime_demo",
		"name":"复杂任务试运行",
		"execution_mode":"workflow",
		"steps":[
			{"id":"s1","type":"api_call","name":"查用户","config":{"method":"GET","path":"/api/v1/users","query":{},"path_params":{},"body":{}}},
			{"id":"s2","type":"output","name":"整理输出","config":{"format":"markdown","title":"运行结果"}}
		]
	}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("complex task create got %d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/v1/complex-tasks/complex_runtime_demo/run", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("complex task run got %d body=%s", rr.Code, rr.Body.String())
	}

	var out map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("invalid json: %v body=%s", err, rr.Body.String())
	}
	if _, ok := out["steps"]; !ok {
		t.Fatalf("expected steps in run response, got=%v", out)
	}
	if _, ok := out["final_output"]; !ok {
		t.Fatalf("expected final_output in run response, got=%v", out)
	}
}

func TestOutputTemplateCRUDEndpoints(t *testing.T) {
	cfg := &config.Config{
		Server:    config.ServerConfig{Bind: "127.0.0.1", Port: 0, Mode: "debug"},
		Scheduler: config.SchedulerConfig{Enabled: false, Timezone: "Asia/Shanghai"},
		SealSuite: config.SealSuiteConfig{BaseURL: "http://example.com", AccessKey: "ak", SecretKey: "sk", Timeout: 1},
		Log:       config.LogConfig{Level: "debug", Filename: "./logs/app.log"},
	}
	client := sealsuite.NewClient(&cfg.SealSuite)
	r := runner.New(cfg, client, "jobs.yaml", "api-templates.yaml")
	h, err := NewRouter(cfg, r)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/output-templates", strings.NewReader(`{
		"id":"out_users_summary",
		"name":"用户汇总输出模板",
		"format":"markdown",
		"title":"用户汇总",
		"content":"# 用户汇总"
	}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("create got %d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/v1/output-templates/out_users_summary", nil)
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("get got %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestLLMTestEndpoint(t *testing.T) {
	llmSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"model":"gpt-test","choices":[{"message":{"content":"测试成功"}}]}`))
	}))
	defer llmSrv.Close()

	cfg := &config.Config{
		Server:    config.ServerConfig{Bind: "127.0.0.1", Port: 0, Mode: "debug"},
		Scheduler: config.SchedulerConfig{Enabled: false, Timezone: "Asia/Shanghai"},
		SealSuite: config.SealSuiteConfig{BaseURL: "http://example.com", AccessKey: "ak", SecretKey: "sk", Timeout: 1},
		Log:       config.LogConfig{Level: "debug", Filename: "./logs/app.log"},
	}
	client := sealsuite.NewClient(&cfg.SealSuite)
	r := runner.New(cfg, client, "jobs.yaml", "api-templates.yaml")
	h, err := NewRouter(cfg, r)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/settings/llm/test", strings.NewReader(`{
		"role":"planner",
		"enabled":true,
		"provider":"deepseek",
		"base_url":"`+llmSrv.URL+`",
		"api_key":"KEY",
		"model":"gpt-test",
		"thinking":true
	}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("llm test got %d body=%s", rr.Code, rr.Body.String())
	}
	var out map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if out["ok"] != true {
		t.Fatalf("expected ok=true got=%v body=%s", out["ok"], rr.Body.String())
	}
}

func TestLLMAPIsRoutesSaveListAndActivateRuntime(t *testing.T) {
	withTempWorkingDir(t, func(dir string) {
		if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(`sealsuite:
  base_url: "http://example.com"
  access_key: "ak"
  secret_key: "sk"
  timeout: 1
llm:
  mock_mode: false
  planner:
    provider: "old-planner"
    model: "old-planner-model"
  formatter:
    provider: "old-formatter"
    model: "old-formatter-model"
scheduler:
  enabled: false
  timezone: "Asia/Shanghai"
log:
  level: "debug"
  filename: "./logs/app.log"
server:
  bind: "127.0.0.1"
  port: 0
  mode: "debug"
`), 0o644); err != nil {
			t.Fatalf("write config err=%v", err)
		}

		cfg := &config.Config{
			Server:    config.ServerConfig{Bind: "127.0.0.1", Port: 0, Mode: "debug"},
			Scheduler: config.SchedulerConfig{Enabled: false, Timezone: "Asia/Shanghai"},
			SealSuite: config.SealSuiteConfig{BaseURL: "http://example.com", AccessKey: "ak", SecretKey: "sk", Timeout: 1},
			Log:       config.LogConfig{Level: "debug", Filename: "./logs/app.log"},
		}
		client := sealsuite.NewClient(&cfg.SealSuite)
		r := runner.New(cfg, client, "jobs.yaml", "api-templates.yaml")
		h, err := NewRouter(cfg, r)
		if err != nil {
			t.Fatalf("NewRouter err=%v", err)
		}

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/settings/llm/apis", strings.NewReader(`{
			"id":"api-a",
			"name":"API A",
			"tags":[" planner ","default","planner"],
			"provider":"deepseek",
			"base_url":"https://api.example.com/v1",
			"api_key":"SECRET-KEY",
			"model":"deepseek-v3",
			"timeout":30,
			"temperature":0.3,
			"max_tokens":2048,
			"thinking":true,
			"reasoning_effort":"medium",
			"response_format":{"type":"json_object"},
			"system_prompt":"hello",
			"activate":true
		}`))
		req.Header.Set("Content-Type", "application/json")
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("save got %d body=%s", rr.Code, rr.Body.String())
		}

		apiStore := storage.NewLLMAPIStore(filepath.Join(dir, "llm-apis.yaml"))
		file, err := apiStore.Load()
		if err != nil {
			t.Fatalf("load llm api store err=%v", err)
		}
		if file.ActiveID != "api-a" {
			t.Fatalf("expected active_id=api-a, got=%q", file.ActiveID)
		}

		cfgStore := storage.ConfigStore{Path: filepath.Join(dir, "config.yaml")}
		llmCfg, err := cfgStore.GetLLMConfig()
		if err != nil {
			t.Fatalf("GetLLMConfig err=%v", err)
		}
		planner := llmCfg["planner"].(map[string]interface{})
		formatter := llmCfg["formatter"].(map[string]interface{})
		if planner["provider"] != "deepseek" || planner["model"] != "deepseek-v3" {
			t.Fatalf("expected planner updated from active llm api, got=%v", planner)
		}
		if formatter["provider"] != "old-formatter" || formatter["model"] != "old-formatter-model" {
			t.Fatalf("expected formatter untouched, got=%v", formatter)
		}

		rr = httptest.NewRecorder()
		req = httptest.NewRequest(http.MethodGet, "/api/v1/settings/llm/apis", nil)
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("list got %d body=%s", rr.Code, rr.Body.String())
		}

		var listOut map[string]interface{}
		if err := json.Unmarshal(rr.Body.Bytes(), &listOut); err != nil {
			t.Fatalf("invalid list json: %v body=%s", err, rr.Body.String())
		}
		items, _ := listOut["items"].([]interface{})
		if len(items) != 1 {
			t.Fatalf("expected one item, got=%v", listOut["items"])
		}
		item, _ := items[0].(map[string]interface{})
		if item["active"] != true {
			t.Fatalf("expected active item, got=%v", item)
		}
		if item["api_key"] != "****" {
			t.Fatalf("expected masked api_key, got=%v", item["api_key"])
		}
	})
}

func TestActivateSingleLLMAPIMapsToFormatterWhenNoExplicitRoleTags(t *testing.T) {
	withTempWorkingDir(t, func(dir string) {
		if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(`sealsuite:
  base_url: "http://example.com"
  access_key: "ak"
  secret_key: "sk"
  timeout: 1
llm:
  mock_mode: false
  planner:
    provider: "old-planner"
    model: "old-planner-model"
  formatter:
    provider: "old-formatter"
    model: "old-formatter-model"
scheduler:
  enabled: false
  timezone: "Asia/Shanghai"
log:
  level: "debug"
  filename: "./logs/app.log"
server:
  bind: "127.0.0.1"
  port: 0
  mode: "debug"
`), 0o644); err != nil {
			t.Fatalf("write config err=%v", err)
		}

		cfg := &config.Config{
			Server:    config.ServerConfig{Bind: "127.0.0.1", Port: 0, Mode: "debug"},
			Scheduler: config.SchedulerConfig{Enabled: false, Timezone: "Asia/Shanghai"},
			SealSuite: config.SealSuiteConfig{BaseURL: "http://example.com", AccessKey: "ak", SecretKey: "sk", Timeout: 1},
			Log:       config.LogConfig{Level: "debug", Filename: "./logs/app.log"},
		}
		client := sealsuite.NewClient(&cfg.SealSuite)
		r := runner.New(cfg, client, "jobs.yaml", "api-templates.yaml")
		h, err := NewRouter(cfg, r)
		if err != nil {
			t.Fatal(err)
		}

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/settings/llm/apis", strings.NewReader(`{
			"id":"llm_main",
			"name":"主模型",
			"tags":["Main"],
			"enabled":true,
			"provider":"deepseek",
			"base_url":"https://api.deepseek.com",
			"api_key":"KEY",
			"model":"deepseek-v3",
			"activate":true
		}`))
		req.Header.Set("Content-Type", "application/json")
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("create got %d body=%s", rr.Code, rr.Body.String())
		}

		cfgStore := storage.ConfigStore{Path: filepath.Join(dir, "config.yaml")}
		llmCfg, err := cfgStore.GetLLMConfig()
		if err != nil {
			t.Fatalf("GetLLMConfig err=%v", err)
		}
		planner := llmCfg["planner"].(map[string]interface{})
		formatter := llmCfg["formatter"].(map[string]interface{})
		if planner["provider"] != "deepseek" || planner["model"] != "deepseek-v3" {
			t.Fatalf("expected planner updated from main llm api, got=%v", planner)
		}
		if formatter["provider"] != "deepseek" || formatter["model"] != "deepseek-v3" {
			t.Fatalf("expected formatter updated from main llm api, got=%v", formatter)
		}
	})
}

func TestAPIExecuteUsesSelectedLLMAPIIDBeforeRoleFallback(t *testing.T) {
	withTempWorkingDir(t, func(dir string) {
		llmSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/chat/completions" {
				http.NotFound(w, r)
				return
			}
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read llm body err=%v", err)
			}
			if !strings.Contains(string(body), `"model":"deepseek-v4-flash"`) {
				t.Fatalf("expected selected llm api model in request, got=%s", string(body))
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"model":"deepseek-v4-flash","choices":[{"message":{"content":"模型摘要成功"}}]}`))
		}))
		defer llmSrv.Close()

		cfg := &config.Config{
			Server:    config.ServerConfig{Bind: "127.0.0.1", Port: 0, Mode: "debug"},
			Scheduler: config.SchedulerConfig{Enabled: false, Timezone: "Asia/Shanghai"},
			SealSuite: config.SealSuiteConfig{BaseURL: "http://example.com", AccessKey: "ak", SecretKey: "sk", Timeout: 1, MockMode: true},
			Log:       config.LogConfig{Level: "debug", Filename: "./logs/app.log"},
			LLM: config.LLMConfig{
				Formatter: config.LLMRoleConfig{
					Enabled:  true,
					Provider: "fallback-provider",
					BaseURL:  llmSrv.URL,
					APIKey:   "fallback-key",
					Model:    "fallback-model",
				},
			},
		}
		client := sealsuite.NewClient(&cfg.SealSuite)
		client.SetMockMode(true)
		r := runner.New(cfg, client, "jobs.yaml", "api-templates.yaml")
		h, err := NewRouter(cfg, r)
		if err != nil {
			t.Fatalf("NewRouter err=%v", err)
		}

		store := storage.NewLLMAPIStore(filepath.Join(dir, "llm-apis.yaml"))
		if err := store.UpsertAndMaybeActivate(storage.LLMAPIItem{
			ID:       "llm_pro_main",
			Name:     "主模型",
			Tags:     []string{"Main"},
			Enabled:  true,
			Provider: "deepseek",
			BaseURL:  llmSrv.URL,
			APIKey:   "KEY",
			Model:    "deepseek-v4-flash",
		}, true); err != nil {
			t.Fatalf("upsert llm api err=%v", err)
		}

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/api/execute", strings.NewReader(`{
			"method":"GET",
			"path":"/api/v1/users",
			"mode":"workflow",
			"llm_config":{"role":"formatter","llm_api_id":"llm_pro_main","prompt":"请做摘要"}
		}`))
		req.Header.Set("Content-Type", "application/json")
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("execute got %d body=%s", rr.Code, rr.Body.String())
		}

		var out map[string]interface{}
		if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
			t.Fatalf("invalid json: %v body=%s", err, rr.Body.String())
		}
		if out["content"] != "模型摘要成功" {
			t.Fatalf("expected llm content in response, got=%v", out["content"])
		}
		if out["llm_api_id"] != "llm_pro_main" {
			t.Fatalf("expected llm_api_id echoed in response, got=%v", out["llm_api_id"])
		}
		if out["model"] != "deepseek-v4-flash" {
			t.Fatalf("expected selected llm api model echoed in response, got=%v", out["model"])
		}
	})
}

func TestAPIExecuteReturnsReadableErrorWhenSelectedLLMAPIIsMissing(t *testing.T) {
	withTempWorkingDir(t, func(dir string) {
		llmSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/chat/completions" {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"model":"fallback-model","choices":[{"message":{"content":"fallback should not be used"}}]}`))
		}))
		defer llmSrv.Close()

		cfg := &config.Config{
			Server:    config.ServerConfig{Bind: "127.0.0.1", Port: 0, Mode: "debug"},
			Scheduler: config.SchedulerConfig{Enabled: false, Timezone: "Asia/Shanghai"},
			SealSuite: config.SealSuiteConfig{BaseURL: "http://example.com", AccessKey: "ak", SecretKey: "sk", Timeout: 1, MockMode: true},
			Log:       config.LogConfig{Level: "debug", Filename: "./logs/app.log"},
			LLM: config.LLMConfig{
				Formatter: config.LLMRoleConfig{
					Enabled:  true,
					Provider: "fallback-provider",
					BaseURL:  llmSrv.URL,
					APIKey:   "fallback-key",
					Model:    "fallback-model",
				},
			},
		}
		client := sealsuite.NewClient(&cfg.SealSuite)
		client.SetMockMode(true)
		r := runner.New(cfg, client, "jobs.yaml", "api-templates.yaml")
		h, err := NewRouter(cfg, r)
		if err != nil {
			t.Fatalf("NewRouter err=%v", err)
		}

		store := storage.NewLLMAPIStore(filepath.Join(dir, "llm-apis.yaml"))
		if err := store.UpsertAndMaybeActivate(storage.LLMAPIItem{
			ID:       "llm_existing",
			Name:     "已存在模型",
			Enabled:  true,
			Provider: "deepseek",
			BaseURL:  llmSrv.URL,
			APIKey:   "KEY",
			Model:    "deepseek-v4-flash",
		}, false); err != nil {
			t.Fatalf("upsert llm api err=%v", err)
		}

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/api/execute", strings.NewReader(`{
			"method":"GET",
			"path":"/api/v1/users",
			"mode":"workflow",
			"llm_config":{"role":"formatter","llm_api_id":"llm_deleted","prompt":"请做摘要"}
		}`))
		req.Header.Set("Content-Type", "application/json")
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for missing llm_api_id, got %d body=%s", rr.Code, rr.Body.String())
		}

		var out map[string]interface{}
		if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
			t.Fatalf("invalid json: %v body=%s", err, rr.Body.String())
		}
		if !strings.Contains(strings.TrimSpace(out["error"].(string)), "指定的 LLM_API 不存在或已删除: llm_deleted") {
			t.Fatalf("expected readable missing llm api error, got=%v", out["error"])
		}
	})
}

func TestAPIExecuteReturnsReadableErrorWhenSelectedLLMAPIIsDisabled(t *testing.T) {
	withTempWorkingDir(t, func(dir string) {
		cfg := &config.Config{
			Server:    config.ServerConfig{Bind: "127.0.0.1", Port: 0, Mode: "debug"},
			Scheduler: config.SchedulerConfig{Enabled: false, Timezone: "Asia/Shanghai"},
			SealSuite: config.SealSuiteConfig{BaseURL: "http://example.com", AccessKey: "ak", SecretKey: "sk", Timeout: 1, MockMode: true},
			Log:       config.LogConfig{Level: "debug", Filename: "./logs/app.log"},
			LLM: config.LLMConfig{
				Formatter: config.LLMRoleConfig{
					Enabled:  true,
					Provider: "fallback-provider",
					BaseURL:  "http://fallback.invalid",
					APIKey:   "fallback-key",
					Model:    "fallback-model",
				},
			},
		}
		client := sealsuite.NewClient(&cfg.SealSuite)
		client.SetMockMode(true)
		r := runner.New(cfg, client, "jobs.yaml", "api-templates.yaml")
		h, err := NewRouter(cfg, r)
		if err != nil {
			t.Fatalf("NewRouter err=%v", err)
		}

		store := storage.NewLLMAPIStore(filepath.Join(dir, "llm-apis.yaml"))
		if err := store.UpsertAndMaybeActivate(storage.LLMAPIItem{
			ID:       "llm_disabled",
			Name:     "已禁用模型",
			Enabled:  false,
			Provider: "deepseek",
			BaseURL:  "https://api.deepseek.example",
			APIKey:   "KEY",
			Model:    "deepseek-v4-flash",
		}, false); err != nil {
			t.Fatalf("upsert llm api err=%v", err)
		}

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/api/execute", strings.NewReader(`{
			"method":"GET",
			"path":"/api/v1/users",
			"mode":"workflow",
			"llm_config":{"role":"formatter","llm_api_id":"llm_disabled","prompt":"请做摘要"}
		}`))
		req.Header.Set("Content-Type", "application/json")
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for disabled llm_api_id, got %d body=%s", rr.Code, rr.Body.String())
		}

		var out map[string]interface{}
		if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
			t.Fatalf("invalid json: %v body=%s", err, rr.Body.String())
		}
		if !strings.Contains(strings.TrimSpace(out["error"].(string)), "指定的 LLM_API 已禁用: llm_disabled") {
			t.Fatalf("expected readable disabled llm api error, got=%v", out["error"])
		}
	})
}

func TestWebhookRoutesPersistProviderAndDefaultGeneric(t *testing.T) {
	withTempWorkingDir(t, func(dir string) {
		if err := os.WriteFile(filepath.Join(dir, "webhooks.yaml"), []byte(`items:
  - id: hook-legacy
    name: Legacy Hook
    provider: feishu
    url: https://example.com/legacy
    method: POST
    enabled: true
`), 0o644); err != nil {
			t.Fatalf("write legacy webhook store err=%v", err)
		}

		cfg := &config.Config{
			Server:    config.ServerConfig{Bind: "127.0.0.1", Port: 0, Mode: "debug"},
			Scheduler: config.SchedulerConfig{Enabled: false, Timezone: "Asia/Shanghai"},
			SealSuite: config.SealSuiteConfig{BaseURL: "http://example.com", AccessKey: "ak", SecretKey: "sk", Timeout: 1},
			Log:       config.LogConfig{Level: "debug", Filename: "./logs/app.log"},
		}
		client := sealsuite.NewClient(&cfg.SealSuite)
		r := runner.New(cfg, client, "jobs.yaml", "api-templates.yaml")
		h, err := NewRouter(cfg, r)
		if err != nil {
			t.Fatalf("NewRouter err=%v", err)
		}

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/settings/webhooks", strings.NewReader(`{
			"id":"hook-provider",
			"name":"Hook Provider",
			"url":"https://example.com/provider",
			"provider":"feishu_bot",
			"method":"post",
			"enabled":true
		}`))
		req.Header.Set("Content-Type", "application/json")
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("save got %d body=%s", rr.Code, rr.Body.String())
		}

		store := storage.NewWebhookStore(filepath.Join(dir, "webhooks.yaml"))
		file, err := store.Load()
		if err != nil {
			t.Fatalf("load webhook store err=%v", err)
		}
		if len(file.Items) != 2 {
			t.Fatalf("expected two webhook items, got=%v", file.Items)
		}

		var legacyItem *storage.WebhookItem
		var providerItem *storage.WebhookItem
		for i := range file.Items {
			switch file.Items[i].ID {
			case "hook-legacy":
				legacyItem = &file.Items[i]
			case "hook-provider":
				providerItem = &file.Items[i]
			}
		}
		if legacyItem == nil {
			t.Fatalf("expected legacy item in store, got=%v", file.Items)
		}
		if legacyItem.Provider != "generic" {
			t.Fatalf("expected legacy provider default generic, got=%q", legacyItem.Provider)
		}
		if providerItem == nil {
			t.Fatalf("expected provider item in store, got=%v", file.Items)
		}
		if providerItem.Provider != "feishu_bot" {
			t.Fatalf("expected provider persisted as feishu_bot, got=%q", providerItem.Provider)
		}

		rr = httptest.NewRecorder()
		req = httptest.NewRequest(http.MethodGet, "/api/v1/settings/webhooks", nil)
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("list got %d body=%s", rr.Code, rr.Body.String())
		}

		var listOut map[string]interface{}
		if err := json.Unmarshal(rr.Body.Bytes(), &listOut); err != nil {
			t.Fatalf("invalid list json: %v body=%s", err, rr.Body.String())
		}
		items, _ := listOut["items"].([]interface{})
		if len(items) != 2 {
			t.Fatalf("expected two webhook items, got=%v", listOut["items"])
		}

		gotProviders := map[string]string{}
		for _, raw := range items {
			item, _ := raw.(map[string]interface{})
			id, _ := item["id"].(string)
			provider, _ := item["provider"].(string)
			gotProviders[id] = provider
		}
		if gotProviders["hook-legacy"] != "generic" {
			t.Fatalf("expected listed legacy provider generic, got=%q", gotProviders["hook-legacy"])
		}
		if gotProviders["hook-provider"] != "feishu_bot" {
			t.Fatalf("expected listed provider feishu_bot, got=%q", gotProviders["hook-provider"])
		}
	})
}

func TestTaskDraftRoutesPersistWebhookReferenceState(t *testing.T) {
	withTempWorkingDir(t, func(dir string) {
		cfg := &config.Config{
			Server:    config.ServerConfig{Bind: "127.0.0.1", Port: 0, Mode: "debug"},
			Scheduler: config.SchedulerConfig{Enabled: false, Timezone: "Asia/Shanghai"},
			SealSuite: config.SealSuiteConfig{BaseURL: "http://example.com", AccessKey: "ak", SecretKey: "sk", Timeout: 1},
			Log:       config.LogConfig{Level: "debug", Filename: "./logs/app.log"},
		}
		client := sealsuite.NewClient(&cfg.SealSuite)
		r := runner.New(cfg, client, "jobs.yaml", "api-templates.yaml")
		h, err := NewRouter(cfg, r)
		if err != nil {
			t.Fatalf("NewRouter err=%v", err)
		}

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/task-drafts", strings.NewReader(`{
			"id":"draft_webhook_enabled",
			"name":"Webhook Enabled Draft",
			"mode":"api_only",
			"source_template_id":"department_list",
			"webhook_config_id":"test1",
			"webhook_enabled":true,
			"input_config":{"method":"GET","path":"/api/open/v1/department/list","query":{},"path_params":{},"body":{}}
		}`))
		req.Header.Set("Content-Type", "application/json")
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("task draft save got %d body=%s", rr.Code, rr.Body.String())
		}

		store := storage.TaskDraftsStore{Path: filepath.Join(dir, "task-drafts.yaml")}
		tf, err := store.Load()
		if err != nil {
			t.Fatalf("load task drafts err=%v", err)
		}
		if len(tf.Items) != 1 {
			t.Fatalf("expected one task draft, got=%v", tf.Items)
		}
		if tf.Items[0].WebhookConfigID != "test1" {
			t.Fatalf("expected webhook_config_id persisted, got=%q", tf.Items[0].WebhookConfigID)
		}
		if !tf.Items[0].WebhookEnabled {
			t.Fatalf("expected webhook_enabled persisted true, got=%+v", tf.Items[0])
		}

		rr = httptest.NewRecorder()
		req = httptest.NewRequest(http.MethodGet, "/api/v1/task-drafts/draft_webhook_enabled", nil)
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("task draft get got %d body=%s", rr.Code, rr.Body.String())
		}
		if !strings.Contains(rr.Body.String(), `"webhook_enabled":true`) {
			t.Fatalf("expected webhook_enabled=true in get response, body=%s", rr.Body.String())
		}
	})
}

func TestTaskDraftRoutesPersistCycleFields(t *testing.T) {
	withTempWorkingDir(t, func(dir string) {
		cfg := &config.Config{
			Server:    config.ServerConfig{Bind: "127.0.0.1", Port: 0, Mode: "debug"},
			Scheduler: config.SchedulerConfig{Enabled: false, Timezone: "Asia/Shanghai"},
			SealSuite: config.SealSuiteConfig{BaseURL: "http://example.com", AccessKey: "ak", SecretKey: "sk", Timeout: 1},
			Log:       config.LogConfig{Level: "debug", Filename: "./logs/app.log"},
		}
		client := sealsuite.NewClient(&cfg.SealSuite)
		r := runner.New(cfg, client, "jobs.yaml", "api-templates.yaml")
		h, err := NewRouter(cfg, r)
		if err != nil {
			t.Fatalf("NewRouter err=%v", err)
		}

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/task-drafts", strings.NewReader(`{
			"id":"draft_cycle_fields",
			"name":"周期字段草稿",
			"mode":"api_only",
			"source_template_id":"users_list",
			"cycle_mode":"30min",
			"run_count":5,
			"run_until":"2026-06-30",
			"input_config":{"method":"GET","path":"/api/open/v1/users","query":{},"path_params":{},"body":{}}
		}`))
		req.Header.Set("Content-Type", "application/json")
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("task draft save got %d body=%s", rr.Code, rr.Body.String())
		}

		store := storage.TaskDraftsStore{Path: filepath.Join(dir, "task-drafts.yaml")}
		tf, err := store.Load()
		if err != nil {
			t.Fatalf("load task drafts err=%v", err)
		}
		if len(tf.Items) != 1 {
			t.Fatalf("expected one task draft, got=%v", tf.Items)
		}
		if tf.Items[0].CycleMode != "30min" {
			t.Fatalf("expected cycle_mode persisted, got=%q", tf.Items[0].CycleMode)
		}
		if tf.Items[0].RunCount != 5 {
			t.Fatalf("expected run_count persisted as 5, got=%d", tf.Items[0].RunCount)
		}
		if tf.Items[0].RunUntil != "2026-06-30" {
			t.Fatalf("expected run_until persisted, got=%q", tf.Items[0].RunUntil)
		}

		rr = httptest.NewRecorder()
		req = httptest.NewRequest(http.MethodGet, "/api/v1/task-drafts/draft_cycle_fields", nil)
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("task draft get got %d body=%s", rr.Code, rr.Body.String())
		}
		if !strings.Contains(rr.Body.String(), `"cycle_mode":"30min"`) {
			t.Fatalf("expected cycle_mode in get response, body=%s", rr.Body.String())
		}
		if !strings.Contains(rr.Body.String(), `"run_count":5`) {
			t.Fatalf("expected run_count in get response, body=%s", rr.Body.String())
		}
		if !strings.Contains(rr.Body.String(), `"run_until":"2026-06-30"`) {
			t.Fatalf("expected run_until in get response, body=%s", rr.Body.String())
		}
	})
}

func TestWebhookRoutesSaveListAndDelete(t *testing.T) {
	withTempWorkingDir(t, func(dir string) {
		cfg := &config.Config{
			Server:    config.ServerConfig{Bind: "127.0.0.1", Port: 0, Mode: "debug"},
			Scheduler: config.SchedulerConfig{Enabled: false, Timezone: "Asia/Shanghai"},
			SealSuite: config.SealSuiteConfig{BaseURL: "http://example.com", AccessKey: "ak", SecretKey: "sk", Timeout: 1},
			Log:       config.LogConfig{Level: "debug", Filename: "./logs/app.log"},
		}
		client := sealsuite.NewClient(&cfg.SealSuite)
		r := runner.New(cfg, client, "jobs.yaml", "api-templates.yaml")
		h, err := NewRouter(cfg, r)
		if err != nil {
			t.Fatalf("NewRouter err=%v", err)
		}

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/settings/webhooks", strings.NewReader(`{
			"id":"hook-a",
			"name":"Hook A",
			"url":"https://example.com/hook",
			"method":"post",
			"headers":{"Content-Type":"application/json"},
			"body_template":"{\"title\":\"hello\"}",
			"enabled":true
		}`))
		req.Header.Set("Content-Type", "application/json")
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("save got %d body=%s", rr.Code, rr.Body.String())
		}

		store := storage.NewWebhookStore(filepath.Join(dir, "webhooks.yaml"))
		file, err := store.Load()
		if err != nil {
			t.Fatalf("load webhook store err=%v", err)
		}
		if len(file.Items) != 1 || file.Items[0].Method != "POST" {
			t.Fatalf("expected stored POST webhook, got=%v", file.Items)
		}
		if file.Items[0].Provider != "generic" {
			t.Fatalf("expected default provider generic, got=%q", file.Items[0].Provider)
		}

		rr = httptest.NewRecorder()
		req = httptest.NewRequest(http.MethodGet, "/api/v1/settings/webhooks", nil)
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("list got %d body=%s", rr.Code, rr.Body.String())
		}

		var listOut map[string]interface{}
		if err := json.Unmarshal(rr.Body.Bytes(), &listOut); err != nil {
			t.Fatalf("invalid list json: %v body=%s", err, rr.Body.String())
		}
		items, _ := listOut["items"].([]interface{})
		if len(items) != 1 {
			t.Fatalf("expected one webhook item, got=%v", listOut["items"])
		}
		item, _ := items[0].(map[string]interface{})
		if item["provider"] != "generic" {
			t.Fatalf("expected listed provider generic, got=%v", item["provider"])
		}

		rr = httptest.NewRecorder()
		req = httptest.NewRequest(http.MethodDelete, "/api/v1/settings/webhooks/hook-a", nil)
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("delete got %d body=%s", rr.Code, rr.Body.String())
		}

		file, err = store.Load()
		if err != nil {
			t.Fatalf("reload webhook store err=%v", err)
		}
		if len(file.Items) != 0 {
			t.Fatalf("expected store empty after delete, got=%v", file.Items)
		}
	})
}

func TestWebhookTestEndpointReturnsStatusBodyAndPreview(t *testing.T) {
	webhookSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/hook" {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got=%s", r.Method)
		}
		if got := r.Header.Get("X-Test"); got != "yes" {
			t.Fatalf("expected X-Test header, got=%q", got)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body err=%v", err)
		}
		if !strings.Contains(string(body), `"title":"测试推送"`) {
			t.Fatalf("expected payload in body, got=%s", string(body))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"received":true}`))
	}))
	defer webhookSrv.Close()

	cfg := &config.Config{
		Server:    config.ServerConfig{Bind: "127.0.0.1", Port: 0, Mode: "debug"},
		Scheduler: config.SchedulerConfig{Enabled: false, Timezone: "Asia/Shanghai"},
		SealSuite: config.SealSuiteConfig{BaseURL: "http://example.com", AccessKey: "ak", SecretKey: "sk", Timeout: 1},
		Log:       config.LogConfig{Level: "debug", Filename: "./logs/app.log"},
	}
	client := sealsuite.NewClient(&cfg.SealSuite)
	r := runner.New(cfg, client, "jobs.yaml", "api-templates.yaml")
	h, err := NewRouter(cfg, r)
	if err != nil {
		t.Fatalf("NewRouter err=%v", err)
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/settings/webhooks/test", strings.NewReader(`{
		"url":"`+webhookSrv.URL+`/hook",
		"method":"POST",
		"headers":{"X-Test":"yes","Content-Type":"application/json"},
		"payload":{"title":"测试推送"}
	}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("webhook test got %d body=%s", rr.Code, rr.Body.String())
	}

	var out map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("invalid json: %v body=%s", err, rr.Body.String())
	}
	if out["ok"] != true {
		t.Fatalf("expected ok=true, got=%v", out)
	}
	if out["status_code"] != float64(http.StatusOK) {
		t.Fatalf("expected status_code=200, got=%v", out["status_code"])
	}
	if out["provider"] != "generic" {
		t.Fatalf("expected provider=generic, got=%v", out["provider"])
	}
	if !strings.Contains(out["response_body"].(string), `"received":true`) {
		t.Fatalf("expected response body, got=%v", out["response_body"])
	}
	if !strings.Contains(out["request_body"].(string), `"title":"测试推送"`) {
		t.Fatalf("expected request_body preview, got=%v", out["request_body"])
	}
	preview, _ := out["request_preview"].(map[string]interface{})
	if preview["url"] != webhookSrv.URL+"/hook" {
		t.Fatalf("expected request preview url, got=%v", preview)
	}
	if preview["provider"] != "generic" {
		t.Fatalf("expected request preview provider generic, got=%v", preview)
	}
	if !strings.Contains(firstNonEmptyString(preview["request_body"], ""), `"title":"测试推送"`) {
		t.Fatalf("expected request preview body, got=%v", preview["request_body"])
	}
}

func TestWebhookTestEndpointBuildsFeishuPayloadFromProvider(t *testing.T) {
	webhookSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/hook" {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got=%s", r.Method)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Fatalf("expected application/json content type, got=%q", got)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body err=%v", err)
		}
		var payload map[string]interface{}
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatalf("expected json body, got err=%v body=%s", err, string(body))
		}
		if payload["msg_type"] != "text" {
			t.Fatalf("expected feishu text payload, got=%v", payload)
		}
		content, _ := payload["content"].(map[string]interface{})
		text, _ := content["text"].(string)
		if !strings.Contains(text, "测试推送") {
			t.Fatalf("expected text payload include test title, got=%q", text)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"received":true}`))
	}))
	defer webhookSrv.Close()

	cfg := &config.Config{
		Server:    config.ServerConfig{Bind: "127.0.0.1", Port: 0, Mode: "debug"},
		Scheduler: config.SchedulerConfig{Enabled: false, Timezone: "Asia/Shanghai"},
		SealSuite: config.SealSuiteConfig{BaseURL: "http://example.com", AccessKey: "ak", SecretKey: "sk", Timeout: 1},
		Log:       config.LogConfig{Level: "debug", Filename: "./logs/app.log"},
	}
	client := sealsuite.NewClient(&cfg.SealSuite)
	r := runner.New(cfg, client, "jobs.yaml", "api-templates.yaml")
	h, err := NewRouter(cfg, r)
	if err != nil {
		t.Fatalf("NewRouter err=%v", err)
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/settings/webhooks/test", strings.NewReader(`{
		"url":"`+webhookSrv.URL+`/hook",
		"method":"POST",
		"provider":"feishu_bot",
		"payload":{"title":"测试推送","detail":"来自 provider"}
	}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("webhook test got %d body=%s", rr.Code, rr.Body.String())
	}

	var out map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("invalid json: %v body=%s", err, rr.Body.String())
	}
	if out["ok"] != true {
		t.Fatalf("expected ok=true, got=%v", out)
	}
	if out["provider"] != "feishu_bot" {
		t.Fatalf("expected provider=feishu_bot, got=%v", out["provider"])
	}
	requestBody, _ := out["request_body"].(string)
	if !strings.Contains(requestBody, `"msg_type":"text"`) {
		t.Fatalf("expected feishu request_body preview, got=%v", out["request_body"])
	}
	if !strings.Contains(requestBody, "测试推送") {
		t.Fatalf("expected request_body include payload text, got=%v", out["request_body"])
	}
	preview, _ := out["request_preview"].(map[string]interface{})
	if preview["provider"] != "feishu_bot" {
		t.Fatalf("expected request preview provider feishu_bot, got=%v", preview)
	}
	if !strings.Contains(firstNonEmptyString(preview["request_body"], ""), `"msg_type":"text"`) {
		t.Fatalf("expected request preview body feishu payload, got=%v", preview["request_body"])
	}
}

func TestAPIExecuteWebhookFailureDoesNotFailMainRequest(t *testing.T) {
	withTempWorkingDir(t, func(dir string) {
		webhookSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				t.Fatalf("expected POST, got=%s", r.Method)
			}
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read webhook body err=%v", err)
			}
			text := string(body)
			if !strings.Contains(text, `"source_type":"api_task"`) {
				t.Fatalf("expected api_task source_type, got=%s", text)
			}
			if !strings.Contains(text, `"source_id":"temporary_execute"`) {
				t.Fatalf("expected temporary_execute source_id, got=%s", text)
			}
			if !strings.Contains(text, `"event":"task.completed"`) {
				t.Fatalf("expected task.completed event, got=%s", text)
			}
			http.Error(w, "upstream unavailable", http.StatusBadGateway)
		}))
		defer webhookSrv.Close()

		webhookStore := storage.NewWebhookStore(filepath.Join(dir, "webhooks.yaml"))
		if err := webhookStore.Upsert(storage.WebhookItem{
			ID:      "hook-api-execute",
			Name:    "API Execute Hook",
			URL:     webhookSrv.URL,
			Method:  http.MethodPost,
			Enabled: true,
		}); err != nil {
			t.Fatalf("upsert webhook err=%v", err)
		}

		cfg := &config.Config{
			Server:    config.ServerConfig{Bind: "127.0.0.1", Port: 0, Mode: "debug"},
			Scheduler: config.SchedulerConfig{Enabled: false, Timezone: "Asia/Shanghai"},
			SealSuite: config.SealSuiteConfig{BaseURL: "http://example.com", AccessKey: "ak", SecretKey: "sk", Timeout: 1, MockMode: true},
			Log:       config.LogConfig{Level: "debug", Filename: "./logs/app.log"},
		}
		client := sealsuite.NewClient(&cfg.SealSuite)
		client.SetMockMode(true)
		r := runner.New(cfg, client, "jobs.yaml", "api-templates.yaml")
		h, err := NewRouter(cfg, r)
		if err != nil {
			t.Fatalf("NewRouter err=%v", err)
		}

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/api/execute", strings.NewReader(`{
			"method":"GET",
			"path":"/api/v1/users",
			"query":{},
			"path_params":{},
			"body":{},
			"name":"临时执行 webhook",
			"webhook_config_id":"hook-api-execute",
			"webhook_enabled":true,
			"webhook_push_once":true
		}`))
		req.Header.Set("Content-Type", "application/json")
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("api execute got %d body=%s", rr.Code, rr.Body.String())
		}

		var out map[string]interface{}
		if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
			t.Fatalf("invalid json: %v body=%s", err, rr.Body.String())
		}
		if _, ok := out["error"]; ok {
			t.Fatalf("expected main request success despite webhook failure, got=%v", out)
		}
		delivery, _ := out["webhook_delivery"].(map[string]interface{})
		if delivery["attempted"] != true {
			t.Fatalf("expected attempted delivery, got=%v", out["webhook_delivery"])
		}
		if delivery["ok"] != false {
			t.Fatalf("expected delivery ok=false on webhook failure, got=%v", out["webhook_delivery"])
		}
		if delivery["status_code"] != float64(http.StatusBadGateway) {
			t.Fatalf("expected status 502, got=%v", out["webhook_delivery"])
		}
		if out["http_status"] != float64(http.StatusOK) {
			t.Fatalf("expected main api execute response preserved, got=%v", out)
		}
	})
}

func TestAPIExecuteRunsTaskDraftPostProcessChain(t *testing.T) {
	withTempWorkingDir(t, func(dir string) {
		cfg := &config.Config{
			Server:    config.ServerConfig{Bind: "127.0.0.1", Port: 0, Mode: "debug"},
			Scheduler: config.SchedulerConfig{Enabled: false, Timezone: "Asia/Shanghai"},
			SealSuite: config.SealSuiteConfig{BaseURL: "http://example.com", AccessKey: "ak", SecretKey: "sk", Timeout: 1, MockMode: true},
			Log:       config.LogConfig{Level: "debug", Filename: "./logs/app.log"},
		}
		client := sealsuite.NewClient(&cfg.SealSuite)
		client.SetMockMode(true)
		r := runner.New(cfg, client, "jobs.yaml", "api-templates.yaml")
		h, err := NewRouter(cfg, r)
		if err != nil {
			t.Fatalf("NewRouter err=%v", err)
		}

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/api/execute", strings.NewReader(`{
			"method":"GET",
			"path":"/api/v1/users",
			"query":{},
			"path_params":{},
			"body":{},
			"draft_id":"draft_exec_chain",
			"name":"即时处理链路",
			"mode":"api_only",
			"transform_config":{"type":"wrap","key":"wrapped"},
			"output_config":{"format":"markdown","title":"即时处理结果"}
		}`))
		req.Header.Set("Content-Type", "application/json")
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("api execute got %d body=%s", rr.Code, rr.Body.String())
		}

		var out map[string]interface{}
		if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
			t.Fatalf("invalid json: %v body=%s", err, rr.Body.String())
		}
		result, _ := out["result"].(string)
		if !strings.Contains(result, "# 即时处理结果") {
			t.Fatalf("expected markdown output title, got=%v", out["result"])
		}
		if !strings.Contains(result, `"wrapped"`) {
			t.Fatalf("expected wrapped transform output, got=%v", out["result"])
		}
		delivery, _ := out["webhook_delivery"].(map[string]interface{})
		if delivery["attempted"] != false {
			t.Fatalf("expected no webhook delivery by default, got=%v", out["webhook_delivery"])
		}
		if _, ok := out["http_status"]; ok {
			t.Fatalf("expected processed response to use result envelope, got=%v", out)
		}
	})
}

func TestJobScheduleRunReturnsTaskDraftWebhookDeliveryWithoutBlockingSuccess(t *testing.T) {
	withTempWorkingDir(t, func(dir string) {
		webhookSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				t.Fatalf("expected POST, got=%s", r.Method)
			}
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read webhook body err=%v", err)
			}
			if !strings.Contains(string(body), `"source_type":"job_schedule"`) {
				t.Fatalf("expected job_schedule source_type, got=%s", string(body))
			}
			if !strings.Contains(string(body), `"source_id":"sched_runtime_webhook"`) {
				t.Fatalf("expected schedule id in payload, got=%s", string(body))
			}
			if !strings.Contains(string(body), `"target_type":"task_draft"`) {
				t.Fatalf("expected task_draft target_type in payload, got=%s", string(body))
			}
			http.Error(w, "upstream failed", http.StatusBadGateway)
		}))
		defer webhookSrv.Close()

		webhookStore := storage.NewWebhookStore(filepath.Join(dir, "webhooks.yaml"))
		if err := webhookStore.Upsert(storage.WebhookItem{
			ID:      "hook-runtime-draft",
			Name:    "Runtime Draft Hook",
			URL:     webhookSrv.URL,
			Method:  http.MethodPost,
			Enabled: true,
		}); err != nil {
			t.Fatalf("upsert webhook err=%v", err)
		}

		cfg := &config.Config{
			Server:    config.ServerConfig{Bind: "127.0.0.1", Port: 0, Mode: "debug"},
			Scheduler: config.SchedulerConfig{Enabled: false, Timezone: "Asia/Shanghai"},
			SealSuite: config.SealSuiteConfig{BaseURL: "http://example.com", AccessKey: "ak", SecretKey: "sk", Timeout: 1, MockMode: true},
			Log:       config.LogConfig{Level: "debug", Filename: "./logs/app.log"},
		}
		client := sealsuite.NewClient(&cfg.SealSuite)
		client.SetMockMode(true)
		r := runner.New(cfg, client, "jobs.yaml", "api-templates.yaml")
		h, err := NewRouter(cfg, r)
		if err != nil {
			t.Fatalf("NewRouter err=%v", err)
		}

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/task-drafts", strings.NewReader(`{
			"id":"draft_runtime_webhook",
			"name":"运行态 webhook 草稿",
			"mode":"api_only",
			"webhook_config_id":"hook-runtime-draft",
			"webhook_enabled":true,
			"input_config":{"method":"GET","path":"/api/v1/users","query":{},"path_params":{},"body":{}}
		}`))
		req.Header.Set("Content-Type", "application/json")
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("draft create got %d body=%s", rr.Code, rr.Body.String())
		}

		rr = httptest.NewRecorder()
		req = httptest.NewRequest(http.MethodPost, "/api/v1/job-schedules", strings.NewReader(`{
			"id":"sched_runtime_webhook",
			"target_type":"task_draft",
			"target_id":"draft_runtime_webhook",
			"enabled":true,
			"webhook_config_id":"hook-runtime-draft",
			"webhook_enabled":true,
			"schedule_type":"cron",
			"cron":"0 */5 * * * *",
			"timezone":"Asia/Shanghai"
		}`))
		req.Header.Set("Content-Type", "application/json")
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("schedule create got %d body=%s", rr.Code, rr.Body.String())
		}

		rr = httptest.NewRecorder()
		req = httptest.NewRequest(http.MethodPost, "/api/v1/job-schedules/sched_runtime_webhook/run", strings.NewReader(`{}`))
		req.Header.Set("Content-Type", "application/json")
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("schedule run got %d body=%s", rr.Code, rr.Body.String())
		}

		var out map[string]interface{}
		if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
			t.Fatalf("invalid json: %v body=%s", err, rr.Body.String())
		}
		if out["ok"] != true {
			t.Fatalf("expected ok=true, got=%v", out)
		}
		delivery, _ := out["webhook_delivery"].(map[string]interface{})
		if delivery["attempted"] != true {
			t.Fatalf("expected attempted delivery, got=%v", out["webhook_delivery"])
		}
		if delivery["ok"] != false {
			t.Fatalf("expected delivery ok=false on webhook failure, got=%v", out["webhook_delivery"])
		}
		if delivery["status_code"] != float64(http.StatusBadGateway) {
			t.Fatalf("expected status 502, got=%v", out["webhook_delivery"])
		}
	})
}

func TestScheduleRunWebhookSuccessReturnsDelivery(t *testing.T) {
	withTempWorkingDir(t, func(dir string) {
		webhookSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				t.Fatalf("expected POST, got=%s", r.Method)
			}
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read webhook body err=%v", err)
			}
			text := string(body)
			if !strings.Contains(text, `"source_type":"job_schedule"`) {
				t.Fatalf("expected job_schedule source_type, got=%s", text)
			}
			if !strings.Contains(text, `"source_id":"sched_complex_webhook"`) {
				t.Fatalf("expected schedule id in payload, got=%s", text)
			}
			if !strings.Contains(text, `"target_type":"complex_task"`) {
				t.Fatalf("expected complex_task target_type in payload, got=%s", text)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"received":true}`))
		}))
		defer webhookSrv.Close()

		webhookStore := storage.NewWebhookStore(filepath.Join(dir, "webhooks.yaml"))
		if err := webhookStore.Upsert(storage.WebhookItem{
			ID:      "hook-schedule-complex",
			Name:    "Schedule Complex Hook",
			URL:     webhookSrv.URL,
			Method:  http.MethodPost,
			Enabled: true,
		}); err != nil {
			t.Fatalf("upsert webhook err=%v", err)
		}

		cfg := &config.Config{
			Server:    config.ServerConfig{Bind: "127.0.0.1", Port: 0, Mode: "debug"},
			Scheduler: config.SchedulerConfig{Enabled: false, Timezone: "Asia/Shanghai"},
			SealSuite: config.SealSuiteConfig{BaseURL: "http://example.com", AccessKey: "ak", SecretKey: "sk", Timeout: 1, MockMode: true},
			Log:       config.LogConfig{Level: "debug", Filename: "./logs/app.log"},
		}
		client := sealsuite.NewClient(&cfg.SealSuite)
		client.SetMockMode(true)
		r := runner.New(cfg, client, "jobs.yaml", "api-templates.yaml")
		h, err := NewRouter(cfg, r)
		if err != nil {
			t.Fatalf("NewRouter err=%v", err)
		}

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/complex-tasks", strings.NewReader(`{
			"id":"complex_schedule_target",
			"name":"定时复杂任务",
			"execution_mode":"workflow",
			"steps":[
				{"id":"s1","type":"api_call","name":"查用户","config":{"method":"GET","path":"/api/v1/users","query":{},"path_params":{},"body":{}}},
				{"id":"s2","type":"output","name":"整理输出","config":{"format":"markdown","title":"运行结果"}}
			]
		}`))
		req.Header.Set("Content-Type", "application/json")
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("complex task create got %d body=%s", rr.Code, rr.Body.String())
		}

		rr = httptest.NewRecorder()
		req = httptest.NewRequest(http.MethodPost, "/api/v1/job-schedules", strings.NewReader(`{
			"id":"sched_complex_webhook",
			"target_type":"complex_task",
			"target_id":"complex_schedule_target",
			"enabled":true,
			"webhook_config_id":"hook-schedule-complex",
			"webhook_enabled":true,
			"schedule_type":"cron",
			"cron":"0 */5 * * * *",
			"timezone":"Asia/Shanghai"
		}`))
		req.Header.Set("Content-Type", "application/json")
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("schedule create got %d body=%s", rr.Code, rr.Body.String())
		}

		rr = httptest.NewRecorder()
		req = httptest.NewRequest(http.MethodPost, "/api/v1/job-schedules/sched_complex_webhook/run", strings.NewReader(`{}`))
		req.Header.Set("Content-Type", "application/json")
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("schedule run got %d body=%s", rr.Code, rr.Body.String())
		}

		var out map[string]interface{}
		if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
			t.Fatalf("invalid json: %v body=%s", err, rr.Body.String())
		}
		if out["ok"] != true {
			t.Fatalf("expected ok=true, got=%v", out)
		}
		delivery, _ := out["webhook_delivery"].(map[string]interface{})
		if delivery["attempted"] != true {
			t.Fatalf("expected attempted delivery, got=%v", out["webhook_delivery"])
		}
		if delivery["ok"] != true {
			t.Fatalf("expected successful delivery, got=%v", out["webhook_delivery"])
		}
		if delivery["status_code"] != float64(http.StatusOK) {
			t.Fatalf("expected status 200, got=%v", out["webhook_delivery"])
		}
	})
}

func TestComplexTaskRunWebhookSuccessReturnsDelivery(t *testing.T) {
	withTempWorkingDir(t, func(dir string) {
		webhookSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				t.Fatalf("expected POST, got=%s", r.Method)
			}
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read webhook body err=%v", err)
			}
			text := string(body)
			if !strings.Contains(text, `"source_type":"complex_task"`) {
				t.Fatalf("expected complex_task source_type, got=%s", text)
			}
			if !strings.Contains(text, `"source_id":"complex_runtime_hook"`) {
				t.Fatalf("expected complex task id in payload, got=%s", text)
			}
			if !strings.Contains(text, `"event":"task.completed"`) {
				t.Fatalf("expected task.completed event, got=%s", text)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"received":true}`))
		}))
		defer webhookSrv.Close()

		webhookStore := storage.NewWebhookStore(filepath.Join(dir, "webhooks.yaml"))
		if err := webhookStore.Upsert(storage.WebhookItem{
			ID:      "hook-complex-runtime",
			Name:    "Complex Runtime Hook",
			URL:     webhookSrv.URL,
			Method:  http.MethodPost,
			Enabled: true,
		}); err != nil {
			t.Fatalf("upsert webhook err=%v", err)
		}

		cfg := &config.Config{
			Server:    config.ServerConfig{Bind: "127.0.0.1", Port: 0, Mode: "debug"},
			Scheduler: config.SchedulerConfig{Enabled: false, Timezone: "Asia/Shanghai"},
			SealSuite: config.SealSuiteConfig{BaseURL: "http://example.com", AccessKey: "ak", SecretKey: "sk", Timeout: 1, MockMode: true},
			Log:       config.LogConfig{Level: "debug", Filename: "./logs/app.log"},
		}
		client := sealsuite.NewClient(&cfg.SealSuite)
		client.SetMockMode(true)
		r := runner.New(cfg, client, "jobs.yaml", "api-templates.yaml")
		h, err := NewRouter(cfg, r)
		if err != nil {
			t.Fatalf("NewRouter err=%v", err)
		}

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/complex-tasks/run", strings.NewReader(`{
			"id":"complex_runtime_hook",
			"name":"复杂任务试运行 webhook",
			"execution_mode":"workflow",
			"webhook_config_id":"hook-complex-runtime",
			"webhook_enabled":true,
			"steps":[
				{"id":"s1","type":"api_call","name":"查用户","config":{"method":"GET","path":"/api/v1/users","query":{},"path_params":{},"body":{}}},
				{"id":"s2","type":"output","name":"整理输出","config":{"format":"markdown","title":"运行结果"}}
			]
		}`))
		req.Header.Set("Content-Type", "application/json")
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("complex task run got %d body=%s", rr.Code, rr.Body.String())
		}

		var out map[string]interface{}
		if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
			t.Fatalf("invalid json: %v body=%s", err, rr.Body.String())
		}
		if out["ok"] != true {
			t.Fatalf("expected ok=true, got=%v", out)
		}
		delivery, _ := out["webhook_delivery"].(map[string]interface{})
		if delivery["attempted"] != true {
			t.Fatalf("expected attempted delivery, got=%v", out["webhook_delivery"])
		}
		if delivery["ok"] != true {
			t.Fatalf("expected successful delivery, got=%v", out["webhook_delivery"])
		}
		if delivery["status_code"] != float64(http.StatusOK) {
			t.Fatalf("expected status 200, got=%v", out["webhook_delivery"])
		}
	})
}

func TestSavedComplexTaskRunWebhookFailureDoesNotFailMainSuccess(t *testing.T) {
	withTempWorkingDir(t, func(dir string) {
		webhookSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				t.Fatalf("expected POST, got=%s", r.Method)
			}
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read webhook body err=%v", err)
			}
			text := string(body)
			if !strings.Contains(text, `"source_type":"complex_task"`) {
				t.Fatalf("expected complex_task source_type, got=%s", text)
			}
			if !strings.Contains(text, `"source_id":"complex_saved_hook"`) {
				t.Fatalf("expected saved complex task id in payload, got=%s", text)
			}
			http.Error(w, "upstream unavailable", http.StatusBadGateway)
		}))
		defer webhookSrv.Close()

		webhookStore := storage.NewWebhookStore(filepath.Join(dir, "webhooks.yaml"))
		if err := webhookStore.Upsert(storage.WebhookItem{
			ID:      "hook-complex-saved",
			Name:    "Complex Saved Hook",
			URL:     webhookSrv.URL,
			Method:  http.MethodPost,
			Enabled: true,
		}); err != nil {
			t.Fatalf("upsert webhook err=%v", err)
		}

		cfg := &config.Config{
			Server:    config.ServerConfig{Bind: "127.0.0.1", Port: 0, Mode: "debug"},
			Scheduler: config.SchedulerConfig{Enabled: false, Timezone: "Asia/Shanghai"},
			SealSuite: config.SealSuiteConfig{BaseURL: "http://example.com", AccessKey: "ak", SecretKey: "sk", Timeout: 1, MockMode: true},
			Log:       config.LogConfig{Level: "debug", Filename: "./logs/app.log"},
		}
		client := sealsuite.NewClient(&cfg.SealSuite)
		client.SetMockMode(true)
		r := runner.New(cfg, client, "jobs.yaml", "api-templates.yaml")
		h, err := NewRouter(cfg, r)
		if err != nil {
			t.Fatalf("NewRouter err=%v", err)
		}

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/complex-tasks", strings.NewReader(`{
			"id":"complex_saved_hook",
			"name":"复杂任务已保存 webhook",
			"execution_mode":"workflow",
			"webhook_config_id":"hook-complex-saved",
			"webhook_enabled":true,
			"steps":[
				{"id":"s1","type":"api_call","name":"查用户","config":{"method":"GET","path":"/api/v1/users","query":{},"path_params":{},"body":{}}},
				{"id":"s2","type":"output","name":"整理输出","config":{"format":"markdown","title":"运行结果"}}
			]
		}`))
		req.Header.Set("Content-Type", "application/json")
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("complex task create got %d body=%s", rr.Code, rr.Body.String())
		}

		rr = httptest.NewRecorder()
		req = httptest.NewRequest(http.MethodPost, "/api/v1/complex-tasks/complex_saved_hook/run", strings.NewReader(`{}`))
		req.Header.Set("Content-Type", "application/json")
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("complex task run got %d body=%s", rr.Code, rr.Body.String())
		}

		var out map[string]interface{}
		if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
			t.Fatalf("invalid json: %v body=%s", err, rr.Body.String())
		}
		if out["ok"] != true {
			t.Fatalf("expected ok=true despite webhook failure, got=%v", out)
		}
		delivery, _ := out["webhook_delivery"].(map[string]interface{})
		if delivery["attempted"] != true {
			t.Fatalf("expected attempted delivery, got=%v", out["webhook_delivery"])
		}
		if delivery["ok"] != false {
			t.Fatalf("expected delivery ok=false on webhook failure, got=%v", out["webhook_delivery"])
		}
		if delivery["status_code"] != float64(http.StatusBadGateway) {
			t.Fatalf("expected status 502, got=%v", out["webhook_delivery"])
		}
	})
}
