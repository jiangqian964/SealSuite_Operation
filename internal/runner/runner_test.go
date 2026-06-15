package runner

import (
	"database/sql"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"sealsuite-operation/internal/config"
	appdb "sealsuite-operation/internal/db"
	sqliteRepo "sealsuite-operation/internal/repository/sqlite"
	"sealsuite-operation/internal/sealsuite"
	"sealsuite-operation/internal/service"
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

func openTestSQLiteDB(t *testing.T, cfg *config.Config, dir string) *sql.DB {
	t.Helper()
	cfg.Database.Path = filepath.Join(dir, "app.db")
	db, err := appdb.OpenSQLite(cfg.Database.Path)
	if err != nil {
		t.Fatalf("open sqlite err=%v", err)
	}
	if err := appdb.Migrate(db); err != nil {
		_ = db.Close()
		t.Fatalf("migrate sqlite err=%v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func latestRunStatusRecord(t *testing.T, dbPath, sourceType, sourceID string) (string, string) {
	t.Helper()
	db, err := appdb.OpenSQLite(dbPath)
	if err != nil {
		t.Fatalf("OpenSQLite err=%v", err)
	}
	defer db.Close()

	var status string
	var errorMessage string
	err = db.QueryRow(`
		SELECT status, error_message
		FROM job_runs
		WHERE source_type = ? AND source_id = ?
		ORDER BY started_at DESC, id DESC
		LIMIT 1
	`, sourceType, sourceID).Scan(&status, &errorMessage)
	if err != nil {
		t.Fatalf("query job_runs err=%v", err)
	}
	return status, errorMessage
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

		cfg := &config.Config{
			Server:    config.ServerConfig{Bind: "127.0.0.1", Port: 0, Mode: "debug"},
			Scheduler: config.SchedulerConfig{Enabled: false, Timezone: "Asia/Shanghai"},
			SealSuite: config.SealSuiteConfig{BaseURL: "http://example.com", AccessKey: "ak", SecretKey: "sk", Timeout: 1, MockMode: true},
			Log:       config.LogConfig{Level: "debug", Filename: "./logs/app.log"},
		}
		db := openTestSQLiteDB(t, cfg, dir)
		if err := sqliteRepo.NewTaskDraftRepository(db).Upsert(storage.TaskDraft{
			ID:   "draft_runner_auto",
			Name: "Runner 自动调度草稿",
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
		if err := sqliteRepo.NewWebhookRepository(db).Upsert(storage.WebhookItem{
			ID:      "hook-runner-auto",
			Name:    "Runner Auto Hook",
			URL:     webhookSrv.URL,
			Method:  http.MethodPost,
			Enabled: true,
		}); err != nil {
			t.Fatalf("upsert webhook err=%v", err)
		}
		client := sealsuite.NewClient(&cfg.SealSuite)
		client.SetMockMode(true)
		r := New(cfg, client)

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

		runStatus, runError := latestRunStatusRecord(t, cfg.Database.Path, "job_schedule", s.ID)
		if runStatus != "success" {
			t.Fatalf("expected sqlite run status=success, got=%q", runStatus)
		}
		if runError != "" {
			t.Fatalf("expected empty sqlite run error, got=%q", runError)
		}
	})
}

func TestNewBootstrapsLegacyJobsFromYAMLIntoSQLite(t *testing.T) {
	withTempWorkingDir(t, func(dir string) {
		if err := os.WriteFile(filepath.Join(dir, "jobs.yaml"), []byte(`version: 1
jobs:
  - name: yaml_runner_job
    enabled: true
    cron: "0 */5 * * * *"
    type: api_poll
    params:
      template_id: users_list
`), 0o644); err != nil {
			t.Fatalf("write jobs yaml err=%v", err)
		}

		cfg := &config.Config{
			Server:    config.ServerConfig{Bind: "127.0.0.1", Port: 0, Mode: "debug"},
			Scheduler: config.SchedulerConfig{Enabled: false, Timezone: "Asia/Shanghai"},
			Database:  config.DatabaseConfig{Path: filepath.Join(dir, "app.db")},
			SealSuite: config.SealSuiteConfig{BaseURL: "http://example.com", AccessKey: "ak", SecretKey: "sk", Timeout: 1},
			Log:       config.LogConfig{Level: "debug", Filename: "./logs/app.log"},
		}
		client := sealsuite.NewClient(&cfg.SealSuite)
		r := New(cfg, client)
		defer r.Stop()

		jf, _, _, err := r.LoadSnapshot()
		if err != nil {
			t.Fatalf("LoadSnapshot err=%v", err)
		}
		if len(jf.Jobs) != 1 || jf.Jobs[0].Name != "yaml_runner_job" {
			t.Fatalf("expected jobs.yaml imported into sqlite, got=%+v", jf.Jobs)
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

		cfg := &config.Config{
			Server:    config.ServerConfig{Bind: "127.0.0.1", Port: 0, Mode: "debug"},
			Scheduler: config.SchedulerConfig{Enabled: false, Timezone: "Asia/Shanghai"},
			SealSuite: config.SealSuiteConfig{BaseURL: "http://example.com", AccessKey: "ak", SecretKey: "sk", Timeout: 1, MockMode: true},
			Log:       config.LogConfig{Level: "debug", Filename: "./logs/app.log"},
		}
		db := openTestSQLiteDB(t, cfg, dir)
		if err := sqliteRepo.NewTaskDraftRepository(db).Upsert(storage.TaskDraft{
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
		if err := sqliteRepo.NewWebhookRepository(db).Upsert(storage.WebhookItem{
			ID:      "hook-runner-auto-success",
			Name:    "Runner Auto Success Hook",
			URL:     webhookSrv.URL,
			Method:  http.MethodPost,
			Enabled: true,
		}); err != nil {
			t.Fatalf("upsert webhook err=%v", err)
		}
		client := sealsuite.NewClient(&cfg.SealSuite)
		client.SetMockMode(true)
		r := New(cfg, client)

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

		runStatus, runError := latestRunStatusRecord(t, cfg.Database.Path, "job_schedule", s.ID)
		if runStatus != "success" {
			t.Fatalf("expected sqlite run status=success, got=%q", runStatus)
		}
		if runError != "" {
			t.Fatalf("expected empty sqlite run error, got=%q", runError)
		}
	})
}

func TestRunTaskDraftEntryWritesSQLiteRunLog(t *testing.T) {
	withTempWorkingDir(t, func(dir string) {
		dbPath := filepath.Join(dir, "app.db")
		cfg := &config.Config{
			Server:    config.ServerConfig{Bind: "127.0.0.1", Port: 0, Mode: "debug"},
			Scheduler: config.SchedulerConfig{Enabled: false, Timezone: "Asia/Shanghai"},
			Database:  config.DatabaseConfig{Path: dbPath},
			SealSuite: config.SealSuiteConfig{BaseURL: "http://example.com", AccessKey: "ak", SecretKey: "sk", Timeout: 1, MockMode: true},
			Log:       config.LogConfig{Level: "debug", Filename: "./logs/app.log"},
		}
		db := openTestSQLiteDB(t, cfg, dir)
		if err := sqliteRepo.NewTaskDraftRepository(db).Upsert(storage.TaskDraft{
			ID:   "draft_runner_entry",
			Name: "Runner 统一入口草稿",
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
		client := sealsuite.NewClient(&cfg.SealSuite)
		client.SetMockMode(true)
		r := New(cfg, client)

		out, err := r.RunTaskDraft("draft_runner_entry")
		if err != nil {
			t.Fatalf("RunTaskDraft err=%v", err)
		}
		if out == nil {
			t.Fatalf("expected task draft output")
		}

		verifyDB, err := appdb.OpenSQLite(dbPath)
		if err != nil {
			t.Fatalf("OpenSQLite err=%v", err)
		}
		defer verifyDB.Close()

		var (
			status     string
			sourceType string
			sourceID   string
			targetType string
			targetID   string
		)
		err = verifyDB.QueryRow(`
			SELECT status, source_type, source_id, target_type, target_id
			FROM job_runs
			WHERE source_type = ? AND source_id = ?
			ORDER BY started_at DESC
			LIMIT 1
		`, "task_draft", "draft_runner_entry").Scan(&status, &sourceType, &sourceID, &targetType, &targetID)
		if err != nil {
			t.Fatalf("query job_runs err=%v", err)
		}
		if status != "success" || sourceType != "task_draft" || sourceID != "draft_runner_entry" {
			t.Fatalf("unexpected run log status/source: status=%q sourceType=%q sourceID=%q", status, sourceType, sourceID)
		}
		if targetType != "task_draft" || targetID != "draft_runner_entry" {
			t.Fatalf("unexpected run log target: targetType=%q targetID=%q", targetType, targetID)
		}
	})
}

func TestExecuteTargetOutputRoutesExternalIPSync(t *testing.T) {
	r := &Runner{}
	called := false
	r.executeExternalIPSync = func(id string) (*service.ExternalIPSyncExecutionSummary, error) {
		called = id == "google_ipv4_sync"
		return &service.ExternalIPSyncExecutionSummary{TaskID: id, Status: "success"}, nil
	}

	out, err := r.executeTargetOutput("external_ip_sync", "google_ipv4_sync")
	if err != nil {
		t.Fatalf("executeTargetOutput err=%v", err)
	}
	summary, ok := out.(*service.ExternalIPSyncExecutionSummary)
	if !ok {
		t.Fatalf("expected external ip sync summary, got=%T", out)
	}
	if summary.TaskID != "google_ipv4_sync" {
		t.Fatalf("unexpected summary: %+v", summary)
	}
	if !called {
		t.Fatal("expected external ip sync executor called")
	}
}

func TestRunExternalIPSyncTaskErrorsWhenRepoNil(t *testing.T) {
	r := &Runner{}

	_, err := r.runExternalIPSyncTask("google_ipv4_sync")
	if err == nil {
		t.Fatal("expected error when repository is nil")
	}
	if !strings.Contains(err.Error(), "external ip sync repository is nil") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunExternalIPSyncTaskErrorsWhenTaskNotFound(t *testing.T) {
	withTempWorkingDir(t, func(dir string) {
		cfg := &config.Config{
			Server:    config.ServerConfig{Bind: "127.0.0.1", Port: 0, Mode: "debug"},
			Scheduler: config.SchedulerConfig{Enabled: false, Timezone: "Asia/Shanghai"},
			Log:       config.LogConfig{Level: "debug", Filename: "./logs/app.log"},
		}
		db := openTestSQLiteDB(t, cfg, dir)
		r := &Runner{
			externalIPSyncRepo: sqliteRepo.NewExternalIPSyncTaskRepository(db),
		}

		_, err := r.runExternalIPSyncTask("missing_task")
		if err == nil {
			t.Fatal("expected not found error")
		}
		if !strings.Contains(err.Error(), "external ip sync task not found: missing_task") {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestFilterGooglePrefixesAndDiff(t *testing.T) {
	source := googleIPRanges{
		Prefixes: []googlePrefix{
			{IPv4Prefix: "1.1.1.0/24"},
			{IPv4Prefix: " 2.2.2.0/24 "},
			{IPv4Prefix: "1.1.1.0/24"},
			{IPv6Prefix: "2001:db8::/32"},
		},
	}

	gotIPv4 := filterGoogleCIDRs(source, "ipv4")
	if want := []string{"1.1.1.0/24", "2.2.2.0/24"}; !reflect.DeepEqual(gotIPv4, want) {
		t.Fatalf("unexpected ipv4 cidrs: got=%v want=%v", gotIPv4, want)
	}

	gotAll := filterGoogleCIDRs(source, "all")
	if want := []string{"1.1.1.0/24", "2.2.2.0/24", "2001:db8::/32"}; !reflect.DeepEqual(gotAll, want) {
		t.Fatalf("unexpected all cidrs: got=%v want=%v", gotAll, want)
	}

	toAdd := diffCIDRs(
		[]string{"1.1.1.0/24", "2.2.2.0/24", "1.1.1.0/24", " "},
		[]string{"2.2.2.0/24"},
	)
	if want := []string{"1.1.1.0/24"}; !reflect.DeepEqual(toAdd, want) {
		t.Fatalf("unexpected diff: got=%v want=%v", toAdd, want)
	}
}

func TestRunExternalIPSyncTaskWithTaskDryRunSkipsWrite(t *testing.T) {
	r := &Runner{
		fetchGoogleCIDRs: func(task storage.ExternalIPSyncTask) ([]string, error) {
			return []string{"1.1.1.0/24"}, nil
		},
		loadFeilianResourceCIDRs: func(resourceID string) ([]string, error) {
			return []string{}, nil
		},
	}
	wrote := false
	r.writeFeilianCIDRs = func(task storage.ExternalIPSyncTask, cidrs []string) error {
		wrote = true
		return nil
	}

	summary, err := r.runExternalIPSyncTaskWithTask(storage.ExternalIPSyncTask{
		ID:            "google_ipv4_sync",
		IPVersion:     "ipv4",
		ResourceID:    "res_google",
		DryRun:        true,
		SkipWhenEmpty: true,
	})
	if err != nil {
		t.Fatalf("runExternalIPSyncTaskWithTask err=%v", err)
	}
	if wrote {
		t.Fatal("expected no write in dry-run")
	}
	if summary == nil || summary.ToAddTotal != 1 || summary.AddedTotal != 0 {
		t.Fatalf("unexpected summary: %+v", summary)
	}
	if summary.Status != "dry_run" {
		t.Fatalf("expected dry_run status, got=%+v", summary)
	}
}

func TestRunExternalIPSyncTaskWithTaskSkipWhenEmpty(t *testing.T) {
	r := &Runner{
		fetchGoogleCIDRs: func(task storage.ExternalIPSyncTask) ([]string, error) {
			return []string{"1.1.1.0/24"}, nil
		},
		loadFeilianResourceCIDRs: func(resourceID string) ([]string, error) {
			return []string{"1.1.1.0/24"}, nil
		},
	}
	wrote := false
	r.writeFeilianCIDRs = func(task storage.ExternalIPSyncTask, cidrs []string) error {
		wrote = true
		return nil
	}

	summary, err := r.runExternalIPSyncTaskWithTask(storage.ExternalIPSyncTask{
		ID:            "google_ipv4_sync",
		IPVersion:     "ipv4",
		ResourceID:    "res_google",
		SkipWhenEmpty: true,
	})
	if err != nil {
		t.Fatalf("runExternalIPSyncTaskWithTask err=%v", err)
	}
	if wrote {
		t.Fatal("expected no write when diff is empty and skip_when_empty=true")
	}
	if summary == nil || summary.ToAddTotal != 0 || summary.AddedTotal != 0 {
		t.Fatalf("unexpected summary: %+v", summary)
	}
	if summary.Status != "skipped" {
		t.Fatalf("expected skipped status, got=%+v", summary)
	}
}

func TestRunExternalIPSyncTaskWithTaskWritesOnlyDiff(t *testing.T) {
	r := &Runner{
		fetchGoogleCIDRs: func(task storage.ExternalIPSyncTask) ([]string, error) {
			return []string{"1.1.1.0/24", "2.2.2.0/24"}, nil
		},
		loadFeilianResourceCIDRs: func(resourceID string) ([]string, error) {
			return []string{"2.2.2.0/24"}, nil
		},
	}
	var gotCIDRs []string
	r.writeFeilianCIDRs = func(task storage.ExternalIPSyncTask, cidrs []string) error {
		gotCIDRs = append([]string(nil), cidrs...)
		return nil
	}

	summary, err := r.runExternalIPSyncTaskWithTask(storage.ExternalIPSyncTask{
		ID:         "google_ipv4_sync",
		IPVersion:  "ipv4",
		ResourceID: "res_google",
	})
	if err != nil {
		t.Fatalf("runExternalIPSyncTaskWithTask err=%v", err)
	}
	if want := []string{"1.1.1.0/24"}; !reflect.DeepEqual(gotCIDRs, want) {
		t.Fatalf("unexpected cidrs to write: got=%v want=%v", gotCIDRs, want)
	}
	if summary == nil || summary.ToAddTotal != 1 || summary.AddedTotal != 1 {
		t.Fatalf("unexpected summary: %+v", summary)
	}
	if summary.Status != "success" {
		t.Fatalf("expected success status, got=%+v", summary)
	}
}
