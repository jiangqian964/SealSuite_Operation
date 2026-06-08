# Webhook Runtime Delivery Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在飞连API任务、飞连周期任务和运营agent 的成功执行链路中接入统一 webhook 推送能力，并保证推送失败不阻断主任务成功返回。

**Architecture:** 在后端新增统一的 webhook runtime delivery helper，负责读取 `webhook_config_id / webhook_enabled`、装配统一 payload、执行同步 HTTP 推送并返回 `webhook_delivery` 结果。三条执行链路分别在成功完成后调用该 helper；其中 API 工作台临时 Execute 通过 `webhook_push_once` 控制单次推送，不回写长期配置。

**Tech Stack:** Go（chi、runner、YAML stores）、Vanilla JS（工作台勾选项）、Node 校验脚本、Go tests。

---

## 0. Files & Responsibilities

**后端运行时**
- Create: `internal/webhook/delivery.go`
  - 统一 webhook 投递 helper、payload envelope、结果结构
- Modify: `internal/web/server.go`
  - API 执行入口、复杂任务入口、周期任务入口接入 helper
  - 支持临时 Execute 的 `webhook_push_once`
- Modify: `internal/runner/runner.go`
  - 调度器自动运行成功后接入 helper

**前端**
- Modify: `internal/web/assets/ui/index.html`
  - API 任务工作台增加“本次执行成功后也推送 webhook”勾选项
- Modify: `internal/web/assets/ui/app.js`
  - Execute 请求带上 `webhook_push_once`
  - 展示 `webhook_delivery`

**测试**
- Create: `internal/webhook/delivery_test.go`
- Modify: `internal/web/server_test.go`
- Modify: `internal/runner/runner_test.go`
- Create: `work/ui_webhook_runtime_check.js`

---

## Task 1: Add unified runtime webhook helper

**Files:**
- Create: `internal/webhook/delivery.go`
- Create: `internal/webhook/delivery_test.go`
- Create: `work/ui_webhook_runtime_check.js`

- [ ] **Step 1: Write the failing contract check**

Create `work/ui_webhook_runtime_check.js`:

```js
const fs = require('fs');
const failures = [];
const delivery = fs.existsSync('internal/webhook/delivery.go')
  ? fs.readFileSync('internal/webhook/delivery.go', 'utf8')
  : '';
[
  'type DeliveryResult struct',
  'func DeliverWebhookForSuccess',
  'event',
  'source_type',
  'webhook_delivery',
].forEach(k => { if (!delivery.includes(k)) failures.push(`missing ${k}`); });
if (failures.length) { console.error(failures.join('\\n')); process.exit(1); }
console.log('webhook runtime helper ok');
```

- [ ] **Step 2: Run the check to verify it fails**

Run:

```bash
node work/ui_webhook_runtime_check.js
```

Expected: FAIL because `internal/webhook/delivery.go` does not exist yet.

- [ ] **Step 3: Implement minimal helper**

Create `internal/webhook/delivery.go`:

```go
package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"time"

	"sealsuite-operation/internal/storage"
)

type DeliveryResult struct {
	Attempted    bool   `json:"attempted"`
	OK           bool   `json:"ok,omitempty"`
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

func DeliverWebhookForSuccess(ctx context.Context, item *storage.WebhookItem, env Envelope) DeliveryResult {
	if item == nil || !item.Enabled || item.URL == "" {
		return DeliveryResult{Attempted: false}
	}
	body, err := json.Marshal(env)
	if err != nil {
		return DeliveryResult{Attempted: true, OK: false, Error: err.Error()}
	}
	method := item.Method
	if method == "" {
		method = http.MethodPost
	}
	req, err := http.NewRequestWithContext(ctx, method, item.URL, bytes.NewReader(body))
	if err != nil {
		return DeliveryResult{Attempted: true, OK: false, Error: err.Error()}
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
		return DeliveryResult{Attempted: true, OK: false, Error: err.Error()}
	}
	defer resp.Body.Close()
	buf := new(bytes.Buffer)
	_, _ = buf.ReadFrom(resp.Body)
	return DeliveryResult{
		Attempted:    true,
		OK:           resp.StatusCode >= 200 && resp.StatusCode < 300,
		StatusCode:   resp.StatusCode,
		ResponseBody: buf.String(),
	}
}
```

- [ ] **Step 4: Add focused unit tests**

Create `internal/webhook/delivery_test.go` with at least:

```go
func TestDeliverWebhookForSuccessDisabledSkips(t *testing.T) {}
func TestDeliverWebhookForSuccessPostsEnvelope(t *testing.T) {}
func TestDeliverWebhookForSuccessFailureDoesNotPanic(t *testing.T) {}
```

- [ ] **Step 5: Run the check to verify it passes**

```bash
node work/ui_webhook_runtime_check.js
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/webhook/delivery.go internal/webhook/delivery_test.go work/ui_webhook_runtime_check.js
git commit -m "feat(webhook): add runtime delivery helper"
```

---

## Task 2: Add store lookup helpers for referenced webhook config

**Files:**
- Modify: `internal/storage/webhook_store.go`
- Modify: `internal/web/server.go`

- [ ] **Step 1: Write failing contract**

Extend `work/ui_webhook_runtime_check.js`:

```js
const store = fs.readFileSync('internal/storage/webhook_store.go', 'utf8');
if (!store.includes('func (s WebhookStore) Get(')) failures.push('missing WebhookStore.Get');
```

- [ ] **Step 2: Run the check to verify it fails**

```bash
node work/ui_webhook_runtime_check.js
```

- [ ] **Step 3: Implement `WebhookStore.Get`**

In `internal/storage/webhook_store.go`:

```go
func (s WebhookStore) Get(id string) (*WebhookItem, bool, error) {
	f, err := s.Load()
	if err != nil {
		return nil, false, err
	}
	for i := range f.Items {
		if f.Items[i].ID == id {
			item := f.Items[i]
			return &item, true, nil
		}
	}
	return nil, false, nil
}
```

- [ ] **Step 4: Run the check to verify it passes**

```bash
node work/ui_webhook_runtime_check.js
```

- [ ] **Step 5: Commit**

```bash
git add internal/storage/webhook_store.go work/ui_webhook_runtime_check.js
git commit -m "feat(webhook): add webhook store lookup helper"
```

---

## Task 3: Support one-time webhook push on API workbench Execute

**Files:**
- Modify: `internal/web/assets/ui/index.html`
- Modify: `internal/web/assets/ui/app.js`
- Modify: `internal/web/server.go`

- [ ] **Step 1: Write failing UI + server contract**

Extend `work/ui_webhook_runtime_check.js`:

```js
const html = fs.readFileSync('internal/web/assets/ui/index.html', 'utf8');
const app = fs.readFileSync('internal/web/assets/ui/app.js', 'utf8');
const server = fs.readFileSync('internal/web/server.go', 'utf8');
[
  'id="exec-webhook-push-once"',
  'webhook_push_once',
  'webhook_delivery',
].forEach(k => {
  if (!html.includes(k) && !app.includes(k) && !server.includes(k)) failures.push(`missing ${k}`);
});
```

- [ ] **Step 2: Run the check to verify it fails**

```bash
node work/ui_webhook_runtime_check.js
```

- [ ] **Step 3: Add UI control**

In `internal/web/assets/ui/index.html`, near API task Execute controls:

```html
<label class="checkbox-line">
  <input type="checkbox" id="exec-webhook-push-once" />
  <span>本次执行成功后也推送 webhook</span>
</label>
```

- [ ] **Step 4: Include fields in Execute request**

In `internal/web/assets/ui/app.js`, when building the execute payload:

```js
const webhookRef = getWebhookReferencePayload('#draft-webhook-select', '#draft-webhook-enabled');
payload.webhook_config_id = webhookRef.webhook_config_id;
payload.webhook_enabled = webhookRef.webhook_enabled;
payload.webhook_push_once = qs('#exec-webhook-push-once')?.checked === true;
```

- [ ] **Step 5: Handle one-time push in `/api/execute`**

In `internal/web/server.go`, after successful `r.Executor().Execute(in)`:

```go
delivery := map[string]interface{}{"attempted": false}
pushOnce := boolFromAny(inMap["webhook_push_once"], false)
webhookID := strings.TrimSpace(asString(inMap["webhook_config_id"]))
webhookEnabled := boolFromAny(inMap["webhook_enabled"], false)
if pushOnce && webhookEnabled && webhookID != "" {
	item, ok, err := webhookStore.Get(webhookID)
	if err == nil && ok {
		delivery = webhook.DeliveryResultToMap(webhook.DeliverWebhookForSuccess(
			req.Context(),
			item,
			webhook.Envelope{
				Event: "task.completed",
				SourceType: "api_task",
				SourceID: firstNonEmpty(asString(inMap["draft_id"]), "temporary_execute"),
				SourceName: firstNonEmpty(asString(inMap["name"]), "飞连API任务"),
				Timestamp: time.Now().Format(time.RFC3339),
				Data: out,
			},
		))
	}
}
```

Then append:

```go
resp["webhook_delivery"] = delivery
```

- [ ] **Step 6: Run checks**

```bash
node --check internal/web/assets/ui/app.js
node work/ui_binding_check.js
node work/ui_webhook_runtime_check.js
```

- [ ] **Step 7: Commit**

```bash
git add internal/web/assets/ui/index.html internal/web/assets/ui/app.js internal/web/server.go work/ui_webhook_runtime_check.js
git commit -m "feat(webhook): support one-time delivery for api execute"
```

---

## Task 4: Deliver webhook for saved task-draft executions

**Files:**
- Modify: `internal/runner/runner.go`
- Modify: `internal/web/server.go`

- [ ] **Step 1: Write failing contract**

Extend `work/ui_webhook_runtime_check.js`:

```js
const runner = fs.readFileSync('internal/runner/runner.go', 'utf8');
if (!runner.includes('webhook_delivery')) failures.push('runner should expose webhook delivery for task draft or schedule results');
```

- [ ] **Step 2: Run the check to verify it fails**

```bash
node work/ui_webhook_runtime_check.js
```

- [ ] **Step 3: Refactor `runTaskDraft` to return result + webhook delivery**

Change:

```go
func (r *Runner) runTaskDraft(d storage.TaskDraft) (interface{}, webhook.DeliveryResult, error)
```

Minimal implementation:
- execute existing logic
- if success and `d.WebhookEnabled && d.WebhookConfigID != ""`
  - load webhook config
  - send envelope with `source_type = "api_task"`
- return both the main output and delivery result

Add payload:

```go
env := webhook.Envelope{
	Event:      "task.completed",
	SourceType: "api_task",
	SourceID:   d.ID,
	SourceName: d.Name,
	Timestamp:  time.Now().Format(time.RFC3339),
	Data:       current,
}
```

- [ ] **Step 4: Update direct server callers**

Any place calling `runTaskDraft` / `executeTaskDraft` must now handle the delivery result and include:

```go
"webhook_delivery": delivery
```

- [ ] **Step 5: Run checks**

```bash
node work/ui_webhook_runtime_check.js
```

- [ ] **Step 6: Commit**

```bash
git add internal/runner/runner.go internal/web/server.go work/ui_webhook_runtime_check.js
git commit -m "feat(webhook): deliver on saved api task success"
```

---

## Task 5: Deliver webhook for job schedule manual and automatic runs

**Files:**
- Modify: `internal/runner/runner.go`
- Modify: `internal/web/server.go`
- Modify: `internal/web/server_test.go`

- [ ] **Step 1: Write failing contract**

Extend `work/ui_webhook_runtime_check.js`:

```js
const server = fs.readFileSync('internal/web/server.go', 'utf8');
const runner = fs.readFileSync('internal/runner/runner.go', 'utf8');
if (!server.includes('/job-schedules/{id}/run')) failures.push('missing manual schedule run route');
if (!runner.includes('executeSchedule')) failures.push('missing executeSchedule hook point');
```

- [ ] **Step 2: Run the check to verify it fails or remains incomplete**

```bash
node work/ui_webhook_runtime_check.js
```

- [ ] **Step 3: Refactor schedule execution to return webhook delivery**

Change:

```go
func (r *Runner) executeSchedule(s storage.JobSchedule) (interface{}, webhook.DeliveryResult, error)
```

or, if smaller:
- keep `executeSchedule` as-is
- add `deliverScheduleWebhookIfNeeded(s, result)` helper

For successful runs:

```go
env := webhook.Envelope{
	Event:      "task.completed",
	SourceType: "job_schedule",
	SourceID:   s.ID,
	SourceName: s.Name,
	Timestamp:  time.Now().Format(time.RFC3339),
	Data: map[string]interface{}{
		"schedule_id": s.ID,
		"target_type": s.TargetType,
		"target_id":   s.TargetID,
		"result":      result,
	},
}
```

- [ ] **Step 4: Include delivery in manual run response**

Change `/api/v1/job-schedules/{id}/run` from:

```go
{"ok": true}
```

to:

```go
{"ok": true, "webhook_delivery": delivery}
```

The scheduler’s automatic run path should also call the same helper, even if its delivery result only goes to logs/store for now.

- [ ] **Step 5: Run checks**

```bash
node work/ui_webhook_runtime_check.js
```

- [ ] **Step 6: Commit**

```bash
git add internal/runner/runner.go internal/web/server.go internal/web/server_test.go work/ui_webhook_runtime_check.js
git commit -m "feat(webhook): deliver on job schedule success"
```

---

## Task 6: Deliver webhook for complex task / 运营agent success

**Files:**
- Modify: `internal/runner/runner.go`
- Modify: `internal/web/server.go`
- Modify: `internal/web/server_test.go`

- [ ] **Step 1: Write failing contract**

Extend `work/ui_webhook_runtime_check.js`:

```js
const server = fs.readFileSync('internal/web/server.go', 'utf8');
if (!server.includes('/complex-tasks/run')) failures.push('missing complex task run route');
if (!server.includes('/complex-tasks/{id}/run')) failures.push('missing saved complex task run route');
```

- [ ] **Step 2: Run the check to verify it fails or is incomplete**

```bash
node work/ui_webhook_runtime_check.js
```

- [ ] **Step 3: Add delivery after successful complex task run**

In `server.go`, after `r.RunComplexTask(task)` or `r.RunComplexTask(*task)` succeeds:

```go
delivery := map[string]interface{}{"attempted": false}
if err == nil && task.WebhookEnabled && task.WebhookConfigID != "" {
	item, ok, getErr := webhookStore.Get(task.WebhookConfigID)
	if getErr == nil && ok {
		delivery = webhook.DeliveryResultToMap(webhook.DeliverWebhookForSuccess(
			req.Context(),
			item,
			webhook.Envelope{
				Event:      "task.completed",
				SourceType: "complex_task",
				SourceID:   task.ID,
				SourceName: task.Name,
				Timestamp:  time.Now().Format(time.RFC3339),
				Data: map[string]interface{}{
					"steps":        result.Steps,
					"final_output": result.FinalOutput,
					"started_at":   result.StartedAt,
					"finished_at":  result.FinishedAt,
				},
			},
		))
	}
}
resp["webhook_delivery"] = delivery
```

- [ ] **Step 4: Run checks**

```bash
node work/ui_webhook_runtime_check.js
```

- [ ] **Step 5: Commit**

```bash
git add internal/web/server.go internal/web/server_test.go work/ui_webhook_runtime_check.js
git commit -m "feat(webhook): deliver on complex task success"
```

---

## Task 7: Add focused tests and logs for delivery outcomes

**Files:**
- Modify: `internal/web/server_test.go`
- Modify: `internal/runner/runner_test.go`
- Modify: `internal/webhook/delivery_test.go`

- [ ] **Step 1: Add tests for non-blocking behavior**

Add tests covering:

```go
func TestAPIExecuteWebhookFailureDoesNotFailMainRequest(t *testing.T) {}
func TestScheduleRunWebhookSuccessReturnsDelivery(t *testing.T) {}
func TestComplexTaskRunWebhookSuccessReturnsDelivery(t *testing.T) {}
```

Core assertions:
- main path remains `ok: true`
- `webhook_delivery.attempted == true`
- webhook failure yields `ok: false` inside delivery, not top-level failure

- [ ] **Step 2: Add lightweight logs**

In each delivery call site, log:

```go
log.Printf("webhook delivery source=%s source_id=%s attempted=%v ok=%v status=%d err=%s",
  sourceType, sourceID, d.Attempted, d.OK, d.StatusCode, d.Error)
```

- [ ] **Step 3: Run checks**

```bash
node work/ui_webhook_runtime_check.js
```

- [ ] **Step 4: Commit**

```bash
git add internal/web/server_test.go internal/runner/runner_test.go internal/webhook/delivery_test.go internal/web/server.go internal/runner/runner.go
git commit -m "test(webhook): cover runtime delivery behavior"
```

---

## Self-Review Checklist

- [ ] The plan covers all three execution chains: api task, job schedule, complex task.
- [ ] Temporary Execute only pushes when `webhook_push_once` is explicitly set.
- [ ] Saved tasks use persisted `webhook_config_id / webhook_enabled`.
- [ ] Delivery failures do not flip main success responses into failures.
- [ ] `webhook_delivery` is included in all relevant responses.
- [ ] No TODO/TBD placeholders remain.

---

## Execution Handoff

Plan complete and saved to `docs/superpowers/plans/2026-06-06-webhook-runtime-delivery.md`.

Two execution options:

**1. Subagent-Driven (recommended)** - I dispatch a fresh subagent per task, review between tasks, fast iteration

**2. Inline Execution** - Execute tasks in this session using executing-plans, batch execution with checkpoints

Which approach?

