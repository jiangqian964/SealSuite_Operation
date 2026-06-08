# v0.2.0 SQLite 整体架构重构 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将项目从当前基于 YAML/JSON 的业务文件存储重构为以 SQLite 为唯一业务数据源的架构，并完成 Web、Runner、Scheduler 围绕 Repository + Service 的重组。

**Architecture:** 保留极小 `config.yaml` 作为启动级配置，引入 `internal/db` 负责 SQLite 初始化与 migration，新增 `internal/repository` 和 `internal/service` 分层承接业务读写与规则编排。Web API、Runner、Scheduler 通过 service 接口工作，最终删除旧业务文件存储路径。

**Tech Stack:** Go 1.21、SQLite、`modernc.org/sqlite`、Chi、Cron、Zap、YAML（仅保留启动配置）

---

## 文件结构

### 新增数据库与装配层

- Create: `internal/db/sqlite.go`
  - 初始化 SQLite、配置 WAL/busy timeout
- Create: `internal/db/migrate.go`
  - 执行 schema migration
- Create: `internal/bootstrap/app.go`
  - 装配 DB、repository、service、runner、web

### 新增 repository 层

- Create: `internal/repository/interfaces.go`
- Create: `internal/repository/sqlite/common.go`
- Create: `internal/repository/sqlite/connections_repo.go`
- Create: `internal/repository/sqlite/llm_apis_repo.go`
- Create: `internal/repository/sqlite/webhooks_repo.go`
- Create: `internal/repository/sqlite/templates_repo.go`
- Create: `internal/repository/sqlite/output_templates_repo.go`
- Create: `internal/repository/sqlite/task_drafts_repo.go`
- Create: `internal/repository/sqlite/complex_tasks_repo.go`
- Create: `internal/repository/sqlite/schedules_repo.go`
- Create: `internal/repository/sqlite/job_runs_repo.go`
- Create: `internal/repository/sqlite/audit_logs_repo.go`

### 新增 service 层

- Create: `internal/service/services.go`
- Create: `internal/service/task_drafts.go`
- Create: `internal/service/schedules.go`
- Create: `internal/service/execution.go`
- Create: `internal/service/settings.go`
- Create: `internal/service/audit.go`

### 需要改造的现有核心文件

- Modify: `cmd/main.go`
- Modify: `internal/web/server.go`
- Modify: `internal/runner/runner.go`
- Modify: `internal/config/config.go`
- Modify: `README.md`
- Modify: `config.example.yaml`

### 测试文件

- Create: `internal/db/migrate_test.go`
- Create: `internal/repository/sqlite/task_drafts_repo_test.go`
- Create: `internal/repository/sqlite/schedules_repo_test.go`
- Create: `internal/service/task_drafts_test.go`
- Create: `internal/service/schedules_test.go`
- Create: `internal/service/execution_test.go`
- Modify: `internal/web/server_test.go`
- Modify: `internal/runner/runner_test.go`

### 需要最终下线的旧文件存储入口

- Delete later: `internal/storage/connections_store.go`
- Delete later: `internal/storage/llm_api_store.go`
- Delete later: `internal/storage/webhook_store.go`
- Delete later: `internal/storage/task_drafts_store.go`
- Delete later: `internal/storage/job_schedules_store.go`
- Delete later: `internal/storage/job_runs_store.go`
- Delete later: `internal/storage/templates_store.go`
- Delete later: `internal/storage/output_templates_store.go`
- Delete later: `internal/storage/complex_tasks_store.go`

---

### Task 1: 建立 SQLite 底座与 Migration

**Files:**
- Create: `internal/db/sqlite.go`
- Create: `internal/db/migrate.go`
- Create: `internal/db/migrate_test.go`
- Modify: `go.mod`
- Modify: `config.example.yaml`
- Modify: `internal/config/config.go`

- [ ] **Step 1: 先写 migration 失败测试**

在 `internal/db/migrate_test.go` 新建测试：

```go
package db

import (
	"database/sql"
	"path/filepath"
	"testing"
)

func TestMigrateCreatesCoreTables(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "app.db")
	conn, err := OpenSQLite(dbPath)
	if err != nil {
		t.Fatalf("OpenSQLite err=%v", err)
	}
	defer conn.Close()

	if err := Migrate(conn); err != nil {
		t.Fatalf("Migrate err=%v", err)
	}

	required := []string{
		"app_connections",
		"llm_apis",
		"webhooks",
		"api_templates",
		"output_templates",
		"task_drafts",
		"complex_tasks",
		"complex_task_steps",
		"job_schedules",
		"job_runs",
		"audit_logs",
	}

	for _, table := range required {
		var name string
		err := conn.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&name)
		if err != nil {
			t.Fatalf("table %s missing err=%v", table, err)
		}
	}
}
```

- [ ] **Step 2: 运行测试，确认失败**

Run:

```bash
go test ./internal/db -run TestMigrateCreatesCoreTables -count=1
```

Expected:

- FAIL
- `OpenSQLite` 或 `Migrate` 尚不存在

- [ ] **Step 3: 引入 SQLite 驱动并补最小启动配置**

修改 `go.mod`，加入：

```go
require modernc.org/sqlite v1.34.5
```

在 `config.example.yaml` 中加入：

```yaml
database:
  path: ./data/app.db
```

在 `internal/config/config.go` 中新增：

```go
type DatabaseConfig struct {
	Path string `mapstructure:"path" yaml:"path"`
}
```

并把它挂入总配置：

```go
type Config struct {
	Server   ServerConfig   `mapstructure:"server" yaml:"server"`
	Log      LogConfig      `mapstructure:"log" yaml:"log"`
	Database DatabaseConfig `mapstructure:"database" yaml:"database"`
	...
}
```

- [ ] **Step 4: 实现 SQLite 打开与 migration**

在 `internal/db/sqlite.go` 中写：

```go
package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

func OpenSQLite(path string) (*sql.DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}
```

在 `internal/db/migrate.go` 中写：

```go
package db

import "database/sql"

func Migrate(db *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS app_connections (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			scheme TEXT NOT NULL,
			host TEXT NOT NULL,
			port INTEGER NOT NULL,
			access_key_id TEXT NOT NULL,
			secret_ref TEXT NOT NULL,
			secret_value TEXT NOT NULL,
			is_active INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS llm_apis (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			provider TEXT NOT NULL,
			base_url TEXT NOT NULL,
			api_key TEXT NOT NULL,
			model TEXT NOT NULL,
			enabled INTEGER NOT NULL DEFAULT 1,
			is_active INTEGER NOT NULL DEFAULT 0,
			tags_json TEXT NOT NULL DEFAULT '[]',
			settings_json TEXT NOT NULL DEFAULT '{}',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS webhooks (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			provider TEXT NOT NULL,
			url TEXT NOT NULL,
			method TEXT NOT NULL,
			headers_json TEXT NOT NULL DEFAULT '{}',
			auth_type TEXT NOT NULL DEFAULT '',
			body_template TEXT NOT NULL DEFAULT '',
			timeout_sec INTEGER NOT NULL DEFAULT 10,
			retry_count INTEGER NOT NULL DEFAULT 0,
			enabled INTEGER NOT NULL DEFAULT 1,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS api_templates (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			method TEXT NOT NULL,
			path TEXT NOT NULL,
			category TEXT NOT NULL DEFAULT '',
			description TEXT NOT NULL DEFAULT '',
			query_schema_json TEXT NOT NULL DEFAULT '{}',
			path_params_schema_json TEXT NOT NULL DEFAULT '{}',
			body_schema_json TEXT NOT NULL DEFAULT '{}',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS output_templates (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			format TEXT NOT NULL,
			content_template TEXT NOT NULL DEFAULT '',
			description TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS task_drafts (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			mode TEXT NOT NULL,
			cycle_mode TEXT NOT NULL DEFAULT 'once',
			run_count INTEGER NOT NULL DEFAULT 1,
			run_until TEXT NOT NULL DEFAULT '',
			source_template_id TEXT NOT NULL DEFAULT '',
			webhook_config_id TEXT NOT NULL DEFAULT '',
			webhook_enabled INTEGER NOT NULL DEFAULT 0,
			input_config_json TEXT NOT NULL DEFAULT '{}',
			transform_config_json TEXT NOT NULL DEFAULT '{}',
			llm_config_json TEXT NOT NULL DEFAULT '{}',
			output_config_json TEXT NOT NULL DEFAULT '{}',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS complex_tasks (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			goal TEXT NOT NULL DEFAULT '',
			execution_mode TEXT NOT NULL DEFAULT '',
			webhook_config_id TEXT NOT NULL DEFAULT '',
			webhook_enabled INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS complex_task_steps (
			id TEXT PRIMARY KEY,
			task_id TEXT NOT NULL,
			step_order INTEGER NOT NULL,
			type TEXT NOT NULL,
			name TEXT NOT NULL,
			config_json TEXT NOT NULL DEFAULT '{}',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS job_schedules (
			id TEXT PRIMARY KEY,
			draft_id TEXT NOT NULL DEFAULT '',
			target_type TEXT NOT NULL,
			target_id TEXT NOT NULL,
			enabled INTEGER NOT NULL DEFAULT 1,
			webhook_config_id TEXT NOT NULL DEFAULT '',
			webhook_enabled INTEGER NOT NULL DEFAULT 0,
			start_at TEXT NOT NULL DEFAULT '',
			end_at TEXT NOT NULL DEFAULT '',
			schedule_type TEXT NOT NULL DEFAULT 'interval',
			cron_expr TEXT NOT NULL DEFAULT '',
			interval_expr TEXT NOT NULL DEFAULT '',
			timezone TEXT NOT NULL DEFAULT 'Asia/Shanghai',
			post_filter_json TEXT NOT NULL DEFAULT '{}',
			field_select_json TEXT NOT NULL DEFAULT '{}',
			truncate_rules_json TEXT NOT NULL DEFAULT '{}',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS job_runs (
			id TEXT PRIMARY KEY,
			source_type TEXT NOT NULL,
			source_id TEXT NOT NULL,
			target_type TEXT NOT NULL,
			target_id TEXT NOT NULL,
			status TEXT NOT NULL,
			trigger_source TEXT NOT NULL,
			started_at TEXT NOT NULL,
			finished_at TEXT NOT NULL,
			duration_ms INTEGER NOT NULL DEFAULT 0,
			error_message TEXT NOT NULL DEFAULT '',
			result_json TEXT NOT NULL DEFAULT '{}'
		);`,
		`CREATE TABLE IF NOT EXISTS audit_logs (
			id TEXT PRIMARY KEY,
			actor TEXT NOT NULL,
			action TEXT NOT NULL,
			resource_type TEXT NOT NULL,
			resource_id TEXT NOT NULL,
			payload_json TEXT NOT NULL DEFAULT '{}',
			created_at TEXT NOT NULL
		);`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}
```

- [ ] **Step 5: 重新运行测试确认通过**

Run:

```bash
go test ./internal/db -run TestMigrateCreatesCoreTables -count=1
```

Expected:

- PASS

- [ ] **Step 6: 提交**

```bash
git add go.mod go.sum config.example.yaml internal/config/config.go internal/db/sqlite.go internal/db/migrate.go internal/db/migrate_test.go
git commit -m "feat: add sqlite bootstrap and migrations"
```

---

### Task 2: 落地 SQLite Repository，先替换任务草稿与调度核心存储

**Files:**
- Create: `internal/repository/interfaces.go`
- Create: `internal/repository/sqlite/common.go`
- Create: `internal/repository/sqlite/task_drafts_repo.go`
- Create: `internal/repository/sqlite/schedules_repo.go`
- Create: `internal/repository/sqlite/task_drafts_repo_test.go`
- Create: `internal/repository/sqlite/schedules_repo_test.go`

- [ ] **Step 1: 先写任务草稿 repository 失败测试**

在 `internal/repository/sqlite/task_drafts_repo_test.go` 中写：

```go
package sqlite

import (
	"path/filepath"
	"testing"

	appdb "sealsuite-operation/internal/db"
	"sealsuite-operation/internal/storage"
)

func TestTaskDraftRepositoryUpsertAndGet(t *testing.T) {
	db, err := appdb.OpenSQLite(filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatalf("OpenSQLite err=%v", err)
	}
	defer db.Close()
	if err := appdb.Migrate(db); err != nil {
		t.Fatalf("Migrate err=%v", err)
	}

	repo := NewTaskDraftRepository(db)
	in := storage.TaskDraft{
		ID:   "draft_1",
		Name: "draft one",
		Mode: "workflow",
	}
	if err := repo.Upsert(in); err != nil {
		t.Fatalf("Upsert err=%v", err)
	}

	got, ok, err := repo.Get("draft_1")
	if err != nil {
		t.Fatalf("Get err=%v", err)
	}
	if !ok {
		t.Fatalf("expected task draft exists")
	}
	if got.ID != "draft_1" || got.Name != "draft one" {
		t.Fatalf("unexpected draft=%+v", got)
	}
}
```

- [ ] **Step 2: 运行测试，确认失败**

Run:

```bash
go test ./internal/repository/sqlite -run TestTaskDraftRepositoryUpsertAndGet -count=1
```

Expected:

- FAIL
- 缺少 `NewTaskDraftRepository`

- [ ] **Step 3: 定义 repository 接口与 SQLite 公共辅助**

在 `internal/repository/interfaces.go` 中写：

```go
package repository

import "sealsuite-operation/internal/storage"

type TaskDraftRepository interface {
	Get(id string) (storage.TaskDraft, bool, error)
	List() ([]storage.TaskDraft, error)
	Upsert(d storage.TaskDraft) error
	Delete(id string) error
}

type ScheduleRepository interface {
	Get(id string) (storage.JobSchedule, bool, error)
	List() ([]storage.JobSchedule, error)
	Upsert(s storage.JobSchedule) error
	Delete(id string) error
}
```

在 `internal/repository/sqlite/common.go` 中写：

```go
package sqlite

import (
	"database/sql"
	"encoding/json"
)

func mustJSON(v interface{}) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func scanJSON[T any](raw string, out *T) error {
	if raw == "" {
		raw = "{}"
	}
	return json.Unmarshal([]byte(raw), out)
}

type baseRepo struct {
	db *sql.DB
}
```

- [ ] **Step 4: 实现任务草稿与调度 repository**

在 `internal/repository/sqlite/task_drafts_repo.go` 中写：

```go
package sqlite

import (
	"database/sql"
	"time"

	"sealsuite-operation/internal/storage"
)

type TaskDraftRepository struct {
	baseRepo
}

func NewTaskDraftRepository(db *sql.DB) *TaskDraftRepository {
	return &TaskDraftRepository{baseRepo{db: db}}
}

func (r *TaskDraftRepository) Upsert(d storage.TaskDraft) error {
	now := time.Now().Format(time.RFC3339)
	_, err := r.db.Exec(`
		INSERT INTO task_drafts (
			id, name, mode, cycle_mode, run_count, run_until,
			source_template_id, webhook_config_id, webhook_enabled,
			input_config_json, transform_config_json, llm_config_json, output_config_json,
			created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name=excluded.name,
			mode=excluded.mode,
			cycle_mode=excluded.cycle_mode,
			run_count=excluded.run_count,
			run_until=excluded.run_until,
			source_template_id=excluded.source_template_id,
			webhook_config_id=excluded.webhook_config_id,
			webhook_enabled=excluded.webhook_enabled,
			input_config_json=excluded.input_config_json,
			transform_config_json=excluded.transform_config_json,
			llm_config_json=excluded.llm_config_json,
			output_config_json=excluded.output_config_json,
			updated_at=excluded.updated_at
	`, d.ID, d.Name, d.Mode, d.CycleMode, d.RunCount, d.RunUntil,
		d.SourceTemplateID, d.WebhookConfigID, boolToInt(d.WebhookEnabled),
		mustJSON(d.InputConfig), mustJSON(d.TransformConfig), mustJSON(d.LLMConfig), mustJSON(d.OutputConfig),
		now, now)
	return err
}
```

同文件继续写 `Get/List/Delete`，并在 `internal/repository/sqlite/schedules_repo.go` 中用相同模式实现调度 repository。

在 `common.go` 补：

```go
func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func intToBool(v int) bool { return v != 0 }
```

- [ ] **Step 5: 再写调度 repository 失败测试并跑通**

在 `internal/repository/sqlite/schedules_repo_test.go` 中写：

```go
func TestScheduleRepositoryUpsertAndList(t *testing.T) {
	db, err := appdb.OpenSQLite(filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatalf("OpenSQLite err=%v", err)
	}
	defer db.Close()
	if err := appdb.Migrate(db); err != nil {
		t.Fatalf("Migrate err=%v", err)
	}
	repo := NewScheduleRepository(db)
	in := storage.JobSchedule{
		ID:         "job_1",
		TargetType: "task_draft",
		TargetID:   "draft_1",
		Enabled:    true,
	}
	if err := repo.Upsert(in); err != nil {
		t.Fatalf("Upsert err=%v", err)
	}
	items, err := repo.List()
	if err != nil {
		t.Fatalf("List err=%v", err)
	}
	if len(items) != 1 || items[0].ID != "job_1" {
		t.Fatalf("unexpected items=%+v", items)
	}
}
```

Run:

```bash
go test ./internal/repository/sqlite -count=1
```

Expected:

- PASS

- [ ] **Step 6: 提交**

```bash
git add internal/repository/interfaces.go internal/repository/sqlite/common.go internal/repository/sqlite/task_drafts_repo.go internal/repository/sqlite/schedules_repo.go internal/repository/sqlite/task_drafts_repo_test.go internal/repository/sqlite/schedules_repo_test.go
git commit -m "feat: add sqlite repositories for drafts and schedules"
```

---

### Task 3: 建立 Service 层并把任务草稿/调度写操作从 Handler 中抽离

**Files:**
- Create: `internal/service/services.go`
- Create: `internal/service/task_drafts.go`
- Create: `internal/service/schedules.go`
- Create: `internal/service/task_drafts_test.go`
- Create: `internal/service/schedules_test.go`
- Modify: `internal/web/server.go`

- [ ] **Step 1: 为 TaskDraftService 写失败测试**

在 `internal/service/task_drafts_test.go` 中写：

```go
package service

import (
	"testing"

	"sealsuite-operation/internal/storage"
)

type draftRepoStub struct {
	upserted storage.TaskDraft
}

func (s *draftRepoStub) Get(id string) (storage.TaskDraft, bool, error) { return storage.TaskDraft{}, false, nil }
func (s *draftRepoStub) List() ([]storage.TaskDraft, error)            { return nil, nil }
func (s *draftRepoStub) Delete(id string) error                        { return nil }
func (s *draftRepoStub) Upsert(d storage.TaskDraft) error {
	s.upserted = d
	return nil
}

func TestTaskDraftServiceSaveRequiresIDAndName(t *testing.T) {
	svc := NewTaskDraftService(&draftRepoStub{})
	err := svc.Save(storage.TaskDraft{})
	if err == nil {
		t.Fatalf("expected validation error")
	}
}
```

- [ ] **Step 2: 运行测试，确认失败**

Run:

```bash
go test ./internal/service -run TestTaskDraftServiceSaveRequiresIDAndName -count=1
```

Expected:

- FAIL

- [ ] **Step 3: 实现最小 Service 结构**

在 `internal/service/services.go` 中写：

```go
package service

import "sealsuite-operation/internal/repository"

type Services struct {
	TaskDrafts *TaskDraftService
	Schedules  *ScheduleService
}

func NewServices(taskDraftRepo repository.TaskDraftRepository, scheduleRepo repository.ScheduleRepository) *Services {
	return &Services{
		TaskDrafts: NewTaskDraftService(taskDraftRepo),
		Schedules:  NewScheduleService(scheduleRepo),
	}
}
```

在 `internal/service/task_drafts.go` 中写：

```go
package service

import (
	"errors"

	"sealsuite-operation/internal/repository"
	"sealsuite-operation/internal/storage"
)

type TaskDraftService struct {
	repo repository.TaskDraftRepository
}

func NewTaskDraftService(repo repository.TaskDraftRepository) *TaskDraftService {
	return &TaskDraftService{repo: repo}
}

func (s *TaskDraftService) Save(d storage.TaskDraft) error {
	if d.ID == "" || d.Name == "" {
		return errors.New("id/name required")
	}
	return s.repo.Upsert(d)
}
```

在 `internal/service/schedules.go` 中写：

```go
package service

import (
	"errors"

	"sealsuite-operation/internal/repository"
	"sealsuite-operation/internal/storage"
)

type ScheduleService struct {
	repo repository.ScheduleRepository
}

func NewScheduleService(repo repository.ScheduleRepository) *ScheduleService {
	return &ScheduleService{repo: repo}
}

func (s *ScheduleService) Save(in storage.JobSchedule) error {
	if in.ID == "" {
		return errors.New("id required")
	}
	if in.TargetType == "" || in.TargetID == "" {
		return errors.New("target_type/target_id required")
	}
	return s.repo.Upsert(in)
}
```

- [ ] **Step 4: 补 ScheduleService 测试并跑通**

在 `internal/service/schedules_test.go` 中写：

```go
type scheduleRepoStub struct {
	upserted storage.JobSchedule
}

func (s *scheduleRepoStub) Get(id string) (storage.JobSchedule, bool, error) { return storage.JobSchedule{}, false, nil }
func (s *scheduleRepoStub) List() ([]storage.JobSchedule, error)              { return nil, nil }
func (s *scheduleRepoStub) Delete(id string) error                            { return nil }
func (s *scheduleRepoStub) Upsert(in storage.JobSchedule) error {
	s.upserted = in
	return nil
}

func TestScheduleServiceSaveRequiresTarget(t *testing.T) {
	svc := NewScheduleService(&scheduleRepoStub{})
	err := svc.Save(storage.JobSchedule{ID: "job_1"})
	if err == nil {
		t.Fatalf("expected target validation error")
	}
}
```

Run:

```bash
go test ./internal/service -count=1
```

Expected:

- PASS

- [ ] **Step 5: 改 `server.go`，将草稿/调度写操作接到 service**

在 `internal/web/server.go` 中，把：

```go
if err := taskDraftsStore.Upsert(draft); err != nil {
```

替换成：

```go
if err := services.TaskDrafts.Save(draft); err != nil {
```

把：

```go
if err := jobSchedulesStore.Upsert(sched); err != nil {
```

替换成：

```go
if err := services.Schedules.Save(sched); err != nil {
```

其中 `services` 先用 repository 实例在 `NewRouter` 中组装：

```go
taskDraftRepo := sqliteRepo.NewTaskDraftRepository(appDB)
scheduleRepo := sqliteRepo.NewScheduleRepository(appDB)
services := service.NewServices(taskDraftRepo, scheduleRepo)
```

- [ ] **Step 6: 运行 Web 回归测试**

Run:

```bash
go test ./internal/web -run 'TestTaskDraftWebhookReferencePersistsAndReturns|TestAPIExecuteUsesSelectedLLMAPIIDBeforeRoleFallback' -count=1
```

Expected:

- PASS

- [ ] **Step 7: 提交**

```bash
git add internal/service/services.go internal/service/task_drafts.go internal/service/schedules.go internal/service/task_drafts_test.go internal/service/schedules_test.go internal/web/server.go
git commit -m "refactor: route draft and schedule writes through services"
```

---

### Task 4: 引入 Bootstrap 装配层，并让 main.go 从 SQLite 启动

**Files:**
- Create: `internal/bootstrap/app.go`
- Modify: `cmd/main.go`
- Modify: `internal/web/server.go`

- [ ] **Step 1: 先写 bootstrap 失败测试**

在 `internal/db/migrate_test.go` 末尾加一条轻量装配测试：

```go
func TestOpenSQLiteWithMigrationReady(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "app.db")
	conn, err := OpenSQLite(dbPath)
	if err != nil {
		t.Fatalf("OpenSQLite err=%v", err)
	}
	defer conn.Close()
	if err := Migrate(conn); err != nil {
		t.Fatalf("Migrate err=%v", err)
	}
}
```

Run:

```bash
go test ./internal/db -count=1
```

Expected:

- PASS

- [ ] **Step 2: 创建 bootstrap 装配层**

在 `internal/bootstrap/app.go` 中写：

```go
package bootstrap

import (
	"database/sql"

	"sealsuite-operation/internal/config"
	appdb "sealsuite-operation/internal/db"
	sqliteRepo "sealsuite-operation/internal/repository/sqlite"
	"sealsuite-operation/internal/sealsuite"
	"sealsuite-operation/internal/service"
)

type App struct {
	DB       *sql.DB
	Services *service.Services
	Client   *sealsuite.Client
}

func Build(cfg *config.Config) (*App, error) {
	db, err := appdb.OpenSQLite(cfg.Database.Path)
	if err != nil {
		return nil, err
	}
	if err := appdb.Migrate(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	client := sealsuite.NewClient(&cfg.SealSuite)
	client.SetMockMode(cfg.SealSuite.MockMode)
	services := service.NewServices(
		sqliteRepo.NewTaskDraftRepository(db),
		sqliteRepo.NewScheduleRepository(db),
	)
	return &App{
		DB:       db,
		Services: services,
		Client:   client,
	}, nil
}
```

- [ ] **Step 3: 改 `cmd/main.go` 使用 bootstrap**

将：

```go
sealSuiteClient := sealsuite.NewClient(&cfg.SealSuite)
sealSuiteClient.SetMockMode(cfg.SealSuite.MockMode)
r := runner.New(cfg, sealSuiteClient, "jobs.yaml", "api-templates.yaml")
```

替换为：

```go
app, err := bootstrap.Build(cfg)
if err != nil {
	panic(err)
}
defer app.DB.Close()

r := runner.New(cfg, app.Client, "jobs.yaml", "api-templates.yaml")
```

并在后续 Web 初始化处预留新的入参：

```go
srv, err := web.NewServer(cfg, r)
```

先保持接口不变，后续 Task 5 再进一步消掉文件依赖。

- [ ] **Step 4: 跑基础编译与回归**

Run:

```bash
go test ./cmd/... ./internal/db ./internal/service -count=1
```

Expected:

- PASS

- [ ] **Step 5: 提交**

```bash
git add internal/bootstrap/app.go cmd/main.go
git commit -m "feat: bootstrap sqlite-backed application services"
```

---

### Task 5: 重构 Runner 与执行链，让运行日志写入 SQLite 并统一执行入口

**Files:**
- Create: `internal/service/execution.go`
- Create: `internal/service/execution_test.go`
- Modify: `internal/runner/runner.go`
- Modify: `internal/runner/runner_test.go`
- Modify: `internal/web/server.go`

- [ ] **Step 1: 写失败测试，锁定执行后写入 job_runs**

在 `internal/service/execution_test.go` 中写：

```go
package service

import "testing"

type runLogWriterStub struct {
	called bool
	status string
}

func (s *runLogWriterStub) WriteRun(status string) {
	s.called = true
	s.status = status
}

func TestExecutionServiceWritesRunLogOnSuccess(t *testing.T) {
	logs := &runLogWriterStub{}
	svc := NewExecutionService(logs)
	svc.afterRun(true)
	if !logs.called || logs.status != "success" {
		t.Fatalf("expected success run log write")
	}
}
```

- [ ] **Step 2: 运行测试，确认失败**

Run:

```bash
go test ./internal/service -run TestExecutionServiceWritesRunLogOnSuccess -count=1
```

Expected:

- FAIL

- [ ] **Step 3: 先实现最小 ExecutionService 骨架**

在 `internal/service/execution.go` 中写：

```go
package service

type RunLogWriter interface {
	WriteRun(status string)
}

type ExecutionService struct {
	runLogs RunLogWriter
}

func NewExecutionService(runLogs RunLogWriter) *ExecutionService {
	return &ExecutionService{runLogs: runLogs}
}

func (s *ExecutionService) afterRun(ok bool) {
	if ok {
		s.runLogs.WriteRun("success")
		return
	}
	s.runLogs.WriteRun("failed")
}
```

- [ ] **Step 4: 在 Runner 中抽出统一入口**

在 `internal/runner/runner.go` 中新增统一入口的计划内最小形态：

```go
func (r *Runner) RunTaskDraft(id string) (interface{}, error) {
	return r.runTaskDraftByID(id, false)
}

func (r *Runner) RunSchedule(id string) (interface{}, error) {
	return r.runScheduleTarget(id)
}
```

把页面手动触发调度、任务草稿执行、定时调度执行尽量都汇总到这些入口，再由入口内部调用原有执行链。

- [ ] **Step 5: 让 job_runs 先落 SQLite repository**

新增 `internal/repository/sqlite/job_runs_repo.go`：

```go
package sqlite

import (
	"database/sql"
)

type JobRunsRepository struct {
	baseRepo
}

func NewJobRunsRepository(db *sql.DB) *JobRunsRepository {
	return &JobRunsRepository{baseRepo{db: db}}
}

func (r *JobRunsRepository) Insert(id, sourceType, sourceID, targetType, targetID, status, triggerSource, startedAt, finishedAt, errorMessage, resultJSON string, durationMS int64) error {
	_, err := r.db.Exec(`
		INSERT INTO job_runs (
			id, source_type, source_id, target_type, target_id, status, trigger_source,
			started_at, finished_at, duration_ms, error_message, result_json
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, id, sourceType, sourceID, targetType, targetID, status, triggerSource, startedAt, finishedAt, durationMS, errorMessage, resultJSON)
	return err
}
```

然后在 `runner.go` 原本写 `job-runs.json` 的位置，替换为通过 repository 写入 SQLite。

- [ ] **Step 6: 跑 runner 与 web 回归**

Run:

```bash
go test ./internal/runner ./internal/web -count=1
```

Expected:

- PASS

- [ ] **Step 7: 提交**

```bash
git add internal/service/execution.go internal/service/execution_test.go internal/repository/sqlite/job_runs_repo.go internal/runner/runner.go internal/runner/runner_test.go internal/web/server.go
git commit -m "refactor: unify execution entrypoints and persist run logs in sqlite"
```

---

### Task 6: 切换配置中心与模板存储到 SQLite，并下线旧业务文件存储路径

**Files:**
- Create: `internal/repository/sqlite/connections_repo.go`
- Create: `internal/repository/sqlite/llm_apis_repo.go`
- Create: `internal/repository/sqlite/webhooks_repo.go`
- Create: `internal/repository/sqlite/templates_repo.go`
- Create: `internal/repository/sqlite/output_templates_repo.go`
- Modify: `internal/web/server.go`
- Modify: `internal/runner/runner.go`
- Modify: `README.md`
- Modify: `config.example.yaml`
- Delete: `internal/storage/connections_store.go`
- Delete: `internal/storage/llm_api_store.go`
- Delete: `internal/storage/webhook_store.go`
- Delete: `internal/storage/task_drafts_store.go`
- Delete: `internal/storage/job_schedules_store.go`
- Delete: `internal/storage/job_runs_store.go`
- Delete: `internal/storage/templates_store.go`
- Delete: `internal/storage/output_templates_store.go`
- Delete: `internal/storage/complex_tasks_store.go`

- [ ] **Step 1: 先写一个“旧文件路径不再被引用”的失败检查**

在当前临时 shell 中先验证：

```bash
git grep -n "\"connections.yaml\"\\|\"llm-apis.yaml\"\\|\"webhooks.yaml\"\\|\"task-drafts.yaml\"\\|\"job-schedules.yaml\"\\|\"job-runs.json\"\\|\"api-templates.yaml\"\\|\"output-templates.yaml\"\\|\"complex-tasks.yaml\"" -- internal cmd
```

Expected:

- 目前会有大量命中

- [ ] **Step 2: 实现配置/模板 repository 并把 server.go 切到 SQLite**

为连接、LLM API、Webhook、模板、输出模板分别实现 SQLite repository，模式与 TaskDraftRepository 一致。

在 `internal/web/server.go` 中，将：

```go
connectionsStore := storage.ConnectionsStore{Path: "connections.yaml"}
llmAPIStore := storage.NewLLMAPIStore("llm-apis.yaml")
webhookStore := storage.NewWebhookStore("webhooks.yaml")
templatesStore := storage.TemplatesStore{Path: "api-templates.yaml"}
outputTemplatesStore := storage.OutputTemplatesStore{Path: "output-templates.yaml"}
```

替换为类似：

```go
connectionRepo := sqliteRepo.NewConnectionsRepository(appDB)
llmAPIRepo := sqliteRepo.NewLLMAPIRepository(appDB)
webhookRepo := sqliteRepo.NewWebhookRepository(appDB)
templateRepo := sqliteRepo.NewTemplateRepository(appDB)
outputTemplateRepo := sqliteRepo.NewOutputTemplateRepository(appDB)
```

- [ ] **Step 3: 更新 Runner 对 LLM/Webhook/模板的读取来源**

在 `internal/runner/runner.go` 中，替换：

```go
storage.NewLLMAPIStore("llm-apis.yaml")
storage.NewWebhookStore("webhooks.yaml")
```

为基于 repository/service 的读取逻辑。

- [ ] **Step 4: 删除旧文件存储实现与文档引用**

确认新路径已全部稳定后，删除旧文件存储实现。

更新 `README.md` 与 `config.example.yaml`，将业务文件存储说明替换为 SQLite 说明：

```yaml
database:
  path: ./data/app.db
```

README 中写明：

```md
v0.2.0 起业务状态统一存储在 SQLite 中，`*.example.yaml` 仅保留启动配置和示例说明。
```

- [ ] **Step 5: 重新运行 grep，确认旧路径已下线**

Run:

```bash
git grep -n "\"connections.yaml\"\\|\"llm-apis.yaml\"\\|\"webhooks.yaml\"\\|\"task-drafts.yaml\"\\|\"job-schedules.yaml\"\\|\"job-runs.json\"\\|\"api-templates.yaml\"\\|\"output-templates.yaml\"\\|\"complex-tasks.yaml\"" -- internal cmd
```

Expected:

- 不再命中业务运行路径

- [ ] **Step 6: 跑全量测试**

Run:

```bash
go test ./... -count=1
```

Expected:

- PASS

- [ ] **Step 7: 提交**

```bash
git add internal/repository/sqlite internal/web/server.go internal/runner/runner.go README.md config.example.yaml
git rm internal/storage/connections_store.go internal/storage/llm_api_store.go internal/storage/webhook_store.go internal/storage/task_drafts_store.go internal/storage/job_schedules_store.go internal/storage/job_runs_store.go internal/storage/templates_store.go internal/storage/output_templates_store.go internal/storage/complex_tasks_store.go
git commit -m "feat: switch business persistence to sqlite"
```

---

## Self-Review

### Spec coverage

- SQLite 作为唯一业务数据源：Task 1、Task 2、Task 6
- 保留极小启动配置文件：Task 1、Task 4、Task 6
- Repository + Service + Web/Runner/Scheduler 分层：Task 2、Task 3、Task 4、Task 5
- 配置、任务、调度、运行日志、操作日志长期保存：Task 1、Task 5、Task 6
- 删除旧业务文件存储路径：Task 6

无明显漏项。

### Placeholder scan

- 计划中没有 `TODO` / `TBD`
- 每个任务都包含明确文件、代码、命令与期望结果

### Type consistency

- 统一使用：
  - `OpenSQLite`
  - `Migrate`
  - `TaskDraftRepository`
  - `ScheduleRepository`
  - `TaskDraftService`
  - `ScheduleService`
  - `ExecutionService`
  - `job_runs`

---
