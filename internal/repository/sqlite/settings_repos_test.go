package sqlite

import (
	"database/sql"
	"path/filepath"
	"testing"

	appdb "sealsuite-operation/internal/db"
	"sealsuite-operation/internal/storage"
)

func openSettingsTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := appdb.OpenSQLite(filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatalf("OpenSQLite err=%v", err)
	}
	if err := appdb.Migrate(db); err != nil {
		_ = db.Close()
		t.Fatalf("Migrate err=%v", err)
	}
	return db
}

func TestConnectionsRepositoryAddActivateDelete(t *testing.T) {
	db := openSettingsTestDB(t)
	defer db.Close()

	repo := NewConnectionsRepository(db)
	if err := repo.AddAndActivate(storage.ConnectionItem{
		ID:          "conn_1",
		Name:        "主连接",
		Scheme:      "https",
		Host:        "feilian.example.com",
		Port:        443,
		AccessKeyID: "ak",
		SecretRef:   "config",
		CreatedAt:   "2026-06-08T10:00:00+08:00",
	}); err != nil {
		t.Fatalf("AddAndActivate err=%v", err)
	}

	file, err := repo.Load()
	if err != nil {
		t.Fatalf("Load err=%v", err)
	}
	if file.ActiveID != "conn_1" || len(file.Items) != 1 {
		t.Fatalf("unexpected connections file=%+v", file)
	}
	if err := repo.Delete("conn_1"); err == nil {
		t.Fatalf("expected delete active connection rejected")
	}
}

func TestLLMAPIRepositoryCRUD(t *testing.T) {
	db := openSettingsTestDB(t)
	defer db.Close()

	repo := NewLLMAPIRepository(db)
	if err := repo.UpsertAndMaybeActivate(storage.LLMAPIItem{
		ID:             "llm_1",
		Name:           "主 LLM",
		Enabled:        true,
		Provider:       "deepseek",
		BaseURL:        "https://api.deepseek.com",
		APIKey:         "secret",
		Model:          "deepseek-chat",
		Tags:           []string{"prod"},
		Timeout:        30,
		SystemPrompt:   "你是助手",
		ResponseFormat: map[string]interface{}{"type": "json_object"},
	}, true); err != nil {
		t.Fatalf("UpsertAndMaybeActivate err=%v", err)
	}

	got, ok, err := repo.Get("llm_1")
	if err != nil {
		t.Fatalf("Get err=%v", err)
	}
	if !ok || got.APIKey != "secret" || got.SystemPrompt != "你是助手" {
		t.Fatalf("unexpected llm api=%+v", got)
	}

	file, err := repo.Load()
	if err != nil {
		t.Fatalf("Load err=%v", err)
	}
	if file.ActiveID != "llm_1" || len(file.Items) != 1 {
		t.Fatalf("unexpected llm api file=%+v", file)
	}
	if err := repo.Delete("llm_1"); err == nil {
		t.Fatalf("expected delete active llm api rejected")
	}
}

func TestWebhookTemplateAndOutputRepositoriesCRUD(t *testing.T) {
	db := openSettingsTestDB(t)
	defer db.Close()

	webhookRepo := NewWebhookRepository(db)
	templateRepo := NewTemplateRepository(db)
	outputRepo := NewOutputTemplateRepository(db)

	if err := webhookRepo.Upsert(storage.WebhookItem{
		ID:        "hook_1",
		Name:      "通知",
		Provider:  "generic",
		URL:       "https://example.com/hook",
		Method:    "POST",
		Headers:   map[string]string{"Content-Type": "application/json"},
		Enabled:   true,
		CreatedAt: "2026-06-08T10:00:00+08:00",
	}); err != nil {
		t.Fatalf("webhook upsert err=%v", err)
	}
	if err := templateRepo.Upsert(storage.Template{
		ID:               "users_list",
		Name:             "用户列表",
		Category:         "users",
		Method:           "GET",
		Path:             "/api/v1/users",
		DryRunQueryParam: "dry_run",
	}); err != nil {
		t.Fatalf("template upsert err=%v", err)
	}
	if err := outputRepo.Upsert(storage.OutputTemplate{
		ID:      "out_1",
		Name:    "输出模板",
		Format:  "markdown",
		Title:   "用户汇总",
		Content: "# 用户汇总",
		Meta:    map[string]interface{}{"scope": "daily"},
	}); err != nil {
		t.Fatalf("output upsert err=%v", err)
	}

	hook, ok, err := webhookRepo.Get("hook_1")
	if err != nil || !ok || hook.URL != "https://example.com/hook" {
		t.Fatalf("unexpected webhook item ok=%v err=%v item=%+v", ok, err, hook)
	}
	tpl, ok, err := templateRepo.Get("users_list")
	if err != nil || !ok || tpl.DryRunQueryParam != "dry_run" {
		t.Fatalf("unexpected template ok=%v err=%v item=%+v", ok, err, tpl)
	}
	out, ok, err := outputRepo.Get("out_1")
	if err != nil || !ok || out.Title != "用户汇总" || out.Content != "# 用户汇总" {
		t.Fatalf("unexpected output template ok=%v err=%v item=%+v", ok, err, out)
	}
}
