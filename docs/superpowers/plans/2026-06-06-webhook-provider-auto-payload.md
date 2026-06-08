# Webhook 类型驱动自动 Payload 匹配 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让 `飞连API任务`、`飞连周期任务`、`运营agent` 在绑定某个 `webhook-id` 后，按该 webhook 的 `provider` 自动生成匹配的 payload，第一版优先支持飞书机器人，并保留通用 webhook 的自定义模板能力。

**Architecture:** 在 `WebhookItem` 中增加 `provider`，由 webhook 配置中心声明目标平台；运行时 `DeliverWebhookForSuccess(...)` 不再直接发送 envelope，而是先按 `provider` 构造最终请求体。前端配置页补充 provider 选择与只读提示，任务页只保留 webhook 引用并展示“已自动匹配 payload”的状态文案。

**Tech Stack:** Go、Chi、YAML Store、Vanilla JS、Go test、Node.js 轻量契约检查

---

## 文件结构

### 核心后端文件

- Modify: `internal/storage/webhook_store.go`
  - 给 `WebhookItem` 增加 `Provider`
  - 统一 provider 默认值与归一化逻辑
- Modify: `internal/webhook/delivery.go`
  - 新增 provider-aware payload 构造逻辑
  - 让 `generic` 真正使用 `body_template`
  - 让 `feishu_bot` 自动生成飞书文本消息格式
- Modify: `internal/web/server.go`
  - webhook 保存/返回结构增加 `provider`
  - webhook 测试接口改为按 provider 生成请求体
- Modify: `internal/runner/runner.go`
  - 如果需要，仅保持调用入口不变，确保新 `DeliverWebhookForSuccess(...)` 能被现有三条运行链路复用

### 测试文件

- Modify: `internal/webhook/delivery_test.go`
  - 覆盖 `generic` 模板渲染
  - 覆盖 `feishu_bot` 自动 payload
- Modify: `internal/web/server_test.go`
  - 覆盖 webhook save/list/delete 的 `provider`
  - 覆盖 webhook test endpoint 的飞书预览与实际发送格式

### 前端文件

- Modify: `internal/web/assets/ui/index.html`
  - webhook 配置区新增 provider 下拉和提示文案
  - 静态任务区补只读 hint 容器
- Modify: `internal/web/assets/ui/app.js`
  - webhook 草稿默认值增加 `provider`
  - 按 provider 切换 body_template / test payload 的 UI 行为
  - 任务引用区展示 provider 感知提示
  - webhook 列表和下拉选项展示 provider 标签

### 轻量验证脚本

- Create: `work/ui_webhook_provider_contract_check.js`
  - 用 Node 静态检查前端关键选择器与提示文案是否接入

---

### Task 1: 给 webhook 配置模型补上 `provider`

**Files:**
- Modify: `internal/storage/webhook_store.go`
- Modify: `internal/web/server.go`
- Modify: `internal/web/server_test.go`

- [ ] **Step 1: 先写保存/列表接口的失败测试**

在 `internal/web/server_test.go` 的 `TestWebhookRoutesSaveListAndDelete` 附近追加一个新用例，确认 `provider` 可保存、可列表返回，且缺省时回退为 `generic`。

```go
func TestWebhookRoutesPersistProviderAndDefaultGeneric(t *testing.T) {
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
			"id":"hook-feishu",
			"name":"飞书推送",
			"url":"https://open.feishu.cn/open-apis/bot/v2/hook/abc",
			"method":"POST",
			"provider":"feishu_bot",
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
		if len(file.Items) != 1 || file.Items[0].Provider != "feishu_bot" {
			t.Fatalf("expected provider=feishu_bot, got=%v", file.Items)
		}

		rr = httptest.NewRecorder()
		req = httptest.NewRequest(http.MethodGet, "/api/v1/settings/webhooks", nil)
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("list got %d body=%s", rr.Code, rr.Body.String())
		}
		if !strings.Contains(rr.Body.String(), `"provider":"feishu_bot"`) {
			t.Fatalf("expected provider in response, body=%s", rr.Body.String())
		}
	})
}
```

- [ ] **Step 2: 跑定向测试，先确认失败**

Run:

```bash
go test ./internal/web -run 'TestWebhookRoutes(PersistProviderAndDefaultGeneric|SaveListAndDelete)' -count=1
```

Expected:

- 新增断言失败
- 或返回体中不包含 `provider`

- [ ] **Step 3: 在 store 和 server 中补最小实现**

先在 `internal/storage/webhook_store.go` 给结构体和归一化逻辑加字段。

```go
type WebhookItem struct {
	ID         string            `yaml:"id" json:"id"`
	Name       string            `yaml:"name" json:"name"`
	URL        string            `yaml:"url" json:"url"`
	Method     string            `yaml:"method" json:"method"`
	Headers    map[string]string `yaml:"headers" json:"headers"`
	AuthType   string            `yaml:"auth_type" json:"auth_type"`
	Provider   string            `yaml:"provider" json:"provider"`
	BodyTmpl   string            `yaml:"body_template" json:"body_template"`
	TimeoutSec int               `yaml:"timeout_sec" json:"timeout_sec"`
	RetryCount int               `yaml:"retry_count" json:"retry_count"`
	Enabled    bool              `yaml:"enabled" json:"enabled"`
	CreatedAt  string            `yaml:"created_at" json:"created_at"`
}

func normalizeWebhookProvider(provider string) string {
	switch strings.TrimSpace(strings.ToLower(provider)) {
	case "", "generic":
		return "generic"
	case "feishu_bot":
		return "feishu_bot"
	default:
		return "generic"
	}
}

func normalizeWebhookItem(item WebhookItem) WebhookItem {
	item.ID = strings.TrimSpace(item.ID)
	item.Name = strings.TrimSpace(item.Name)
	item.URL = strings.TrimSpace(item.URL)
	item.Method = normalizeWebhookMethod(item.Method)
	item.AuthType = strings.TrimSpace(item.AuthType)
	item.Provider = normalizeWebhookProvider(item.Provider)
	item.BodyTmpl = strings.TrimSpace(item.BodyTmpl)
	item.Headers = normalizeWebhookHeaders(item.Headers)
	return item
}
```

再在 `internal/web/server.go` 的 save / map 输出路径补上 `provider`。

```go
item := storage.WebhookItem{
	ID:         strings.TrimSpace(firstNonEmptyString(in["id"], "")),
	Name:       strings.TrimSpace(firstNonEmptyString(in["name"], "")),
	URL:        strings.TrimSpace(firstNonEmptyString(in["url"], "")),
	Method:     firstNonEmptyString(in["method"], ""),
	Headers:    toStringMap(in["headers"]),
	AuthType:   firstNonEmptyString(in["auth_type"], ""),
	Provider:   firstNonEmptyString(in["provider"], "generic"),
	BodyTmpl:   firstNonEmptyString(in["body_template"], ""),
	TimeoutSec: intFromAny(in["timeout_sec"], 0),
	RetryCount: intFromAny(in["retry_count"], 0),
	Enabled:    boolFromAny(in["enabled"], true),
}

func webhookItemToMap(item storage.WebhookItem) map[string]interface{} {
	return map[string]interface{}{
		"id":            item.ID,
		"name":          item.Name,
		"url":           item.URL,
		"method":        item.Method,
		"headers":       cloneStringMap(item.Headers),
		"auth_type":     item.AuthType,
		"provider":      item.Provider,
		"body_template": item.BodyTmpl,
		"timeout_sec":   item.TimeoutSec,
		"retry_count":   item.RetryCount,
		"enabled":       item.Enabled,
		"created_at":    item.CreatedAt,
	}
}
```

- [ ] **Step 4: 重新跑测试，确认通过**

Run:

```bash
go test ./internal/web -run 'TestWebhookRoutes(PersistProviderAndDefaultGeneric|SaveListAndDelete)' -count=1
```

Expected:

- PASS
- 返回体中带 `provider`
- 未指定 provider 的老逻辑仍不报错

- [ ] **Step 5: 提交这一小步**

```bash
git add internal/storage/webhook_store.go internal/web/server.go internal/web/server_test.go
git commit -m "feat: add webhook provider model"
```

如果当前目录不是 git 仓库，跳过 commit，但保留变更。

---

### Task 2: 让运行时按 `provider` 生成最终请求体

**Files:**
- Modify: `internal/webhook/delivery.go`
- Modify: `internal/webhook/delivery_test.go`

- [ ] **Step 1: 先写两个失败测试**

在 `internal/webhook/delivery_test.go` 追加：

1. `generic` 使用 `body_template`
2. `feishu_bot` 自动生成飞书文本消息

```go
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
		BodyTmpl: `{"title":"{{source_name}}","message":"{{jsonString data}}"}`,
	}, Envelope{
		Event:      "task.completed",
		SourceType: "api_task",
		SourceID:   "task-1",
		SourceName: "任务A",
		Timestamp:  "2026-06-06T00:00:00Z",
		Data:       map[string]interface{}{"result": "ok"},
	})

	if !got.OK {
		t.Fatalf("expected ok=true, got=%+v", got)
	}
	if gotBody["title"] != "任务A" {
		t.Fatalf("expected title rendered, got=%v", gotBody)
	}
	if !strings.Contains(gotBody["message"].(string), `"result":"ok"`) {
		t.Fatalf("expected json string in message, got=%v", gotBody)
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
	}, Envelope{
		Event:      "task.completed",
		SourceType: "complex_task",
		SourceID:   "task-2",
		SourceName: "运营agent A",
		Timestamp:  "2026-06-06T08:00:00Z",
		Data:       map[string]interface{}{"summary": "done"},
	})

	if !got.OK {
		t.Fatalf("expected ok=true, got=%+v", got)
	}
	if gotBody["msg_type"] != "text" {
		t.Fatalf("expected msg_type=text, got=%v", gotBody)
	}
	content, _ := gotBody["content"].(map[string]interface{})
	if content["text"] == "" {
		t.Fatalf("expected content.text, got=%v", gotBody)
	}
	if !strings.Contains(content["text"].(string), "运营agent A") {
		t.Fatalf("expected source_name in text, got=%v", content["text"])
	}
}
```

- [ ] **Step 2: 先跑 webhook 包测试，确认失败**

Run:

```bash
go test ./internal/webhook -run 'TestDeliverWebhookForSuccess(GenericUsesBodyTemplate|FeishuBotBuildsTextPayload|PostsEnvelope)' -count=1
```

Expected:

- `generic` 用例会拿到原始 envelope
- `feishu_bot` 用例不会拿到 `msg_type=text`

- [ ] **Step 3: 在 `delivery.go` 中实现最小 provider-aware 渲染**

把 `json.Marshal(env)` 替换为“先构造 body，再发送”。优先用小函数拆开，避免 `DeliverWebhookForSuccess(...)` 继续膨胀。

```go
func DeliverWebhookForSuccess(ctx context.Context, item *storage.WebhookItem, env Envelope) DeliveryResult {
	if item == nil || !item.Enabled || item.URL == "" {
		return DeliveryResult{Attempted: false}
	}

	body, err := buildWebhookRequestBody(item, env)
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
	// 其余逻辑保持不变
}

func buildWebhookRequestBody(item *storage.WebhookItem, env Envelope) ([]byte, error) {
	if item == nil {
		return nil, fmt.Errorf("webhook item is nil")
	}
	switch storage.NormalizeWebhookProviderForRuntime(item.Provider) {
	case "feishu_bot":
		return buildFeishuBotBody(env)
	default:
		return buildGenericWebhookBody(item, env)
	}
}

func buildGenericWebhookBody(item *storage.WebhookItem, env Envelope) ([]byte, error) {
	if strings.TrimSpace(item.BodyTmpl) == "" {
		return json.Marshal(env)
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
```

模板渲染只做 spec 里定义的最小变量集：

```go
func renderWebhookTemplate(tmpl string, env Envelope) (string, error) {
	replacer := strings.NewReplacer(
		"{{source_type}}", env.SourceType,
		"{{source_id}}", env.SourceID,
		"{{source_name}}", env.SourceName,
		"{{timestamp}}", env.Timestamp,
		"{{jsonString data}}", mustJSONString(env.Data),
		"{{jsonString envelope}}", mustJSONString(env),
	)
	return replacer.Replace(tmpl), nil
}
```

如果你不想把 provider 归一化函数暴露到 `storage` 包外，就直接在 `delivery.go` 内部写一个同名小 helper。

- [ ] **Step 4: 重跑 webhook 测试，确认通过**

Run:

```bash
go test ./internal/webhook -count=1
```

Expected:

- PASS
- 原有 `PostsEnvelope` 测试仍通过，或按新断言改成 `PostsGenericEnvelopeWhenTemplateEmpty`
- 新增 `generic` / `feishu_bot` 测试通过

- [ ] **Step 5: 提交这一小步**

```bash
git add internal/webhook/delivery.go internal/webhook/delivery_test.go
git commit -m "feat: render webhook payload by provider"
```

如果当前目录不是 git 仓库，跳过 commit，但保留变更。

---

### Task 3: 让 webhook 测试接口也走 provider 自动匹配

**Files:**
- Modify: `internal/web/server.go`
- Modify: `internal/web/server_test.go`

- [ ] **Step 1: 先写测试接口的失败测试**

在 `internal/web/server_test.go` 的 `TestWebhookTestEndpointReturnsStatusBodyAndPreview` 附近追加一个飞书用例，确认测试接口收到的是飞书结构，不是用户手工拼的原始 body。

```go
func TestWebhookTestEndpointBuildsFeishuPayloadFromProvider(t *testing.T) {
	webhookSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body err=%v", err)
		}
		if !strings.Contains(string(body), `"msg_type":"text"`) {
			t.Fatalf("expected feishu text payload, got=%s", string(body))
		}
		if !strings.Contains(string(body), `"content"`) {
			t.Fatalf("expected content field, got=%s", string(body))
		}
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
		"url":"`+webhookSrv.URL+`",
		"method":"POST",
		"provider":"feishu_bot",
		"headers":{"X-Test":"yes"},
		"payload":{"summary":"hello"}
	}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("test got %d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"provider":"feishu_bot"`) {
		t.Fatalf("expected preview provider in body=%s", rr.Body.String())
	}
}
```

- [ ] **Step 2: 跑测试，确认当前实现失败**

Run:

```bash
go test ./internal/web -run 'TestWebhookTestEndpoint(BuildsFeishuPayloadFromProvider|ReturnsStatusBodyAndPreview)' -count=1
```

Expected:

- 当前会发送原始 payload
- 或响应 preview 中不含 provider / provider-aware body

- [ ] **Step 3: 在 `server.go` 中把测试接口接到统一 body builder**

不要在测试接口里再写一套飞书逻辑，直接复用 `internal/webhook` 的构造函数，避免运行时和测试接口分叉。

可以把 `delivery.go` 中的内部函数稍微抬升成可复用函数，例如：

```go
func BuildPreviewRequestBody(item storage.WebhookItem, payload interface{}) ([]byte, error) {
	env := Envelope{
		Event:      "webhook.test",
		SourceType: "webhook_test",
		SourceID:   item.ID,
		SourceName: item.Name,
		Timestamp:  time.Now().Format(time.RFC3339),
		Data:       payload,
	}
	return buildWebhookRequestBody(&item, env)
}
```

然后在 `internal/web/server.go` 的 `/api/v1/settings/webhooks/test` 中改成：

```go
item := storage.WebhookItem{
	URL:      firstNonEmptyString(in["url"], ""),
	Method:   firstNonEmptyString(in["method"], "POST"),
	Headers:  toStringMap(in["headers"]),
	Provider: firstNonEmptyString(in["provider"], "generic"),
	BodyTmpl: firstNonEmptyString(in["body_template"], ""),
	Enabled:  true,
}
payload := in["payload"]
body, err := webhook.BuildPreviewRequestBody(item, payload)
if err != nil {
	respondError(w, http.StatusBadRequest, err)
	return
}
```

测试接口返回 preview 时也把 provider 和最终 body 一并回显：

```go
respondJSON(w, map[string]interface{}{
	"ok":            delivery.OK,
	"provider":      item.Provider,
	"status_code":   delivery.StatusCode,
	"response_body": delivery.ResponseBody,
	"request_body":  json.RawMessage(body),
})
```

- [ ] **Step 4: 重新跑测试**

Run:

```bash
go test ./internal/web -run 'TestWebhook(TestEndpointBuildsFeishuPayloadFromProvider|RoutesPersistProviderAndDefaultGeneric|TestEndpointReturnsStatusBodyAndPreview)' -count=1
```

Expected:

- PASS
- webhook test 接口与真实运行时共享同一套 provider-aware body 构造逻辑

- [ ] **Step 5: 提交这一小步**

```bash
git add internal/web/server.go internal/web/server_test.go internal/webhook/delivery.go
git commit -m "feat: use provider-aware body in webhook test endpoint"
```

如果当前目录不是 git 仓库，跳过 commit，但保留变更。

---

### Task 4: 改造 webhook 配置页，暴露 `provider` 并收起飞书模板编辑

**Files:**
- Modify: `internal/web/assets/ui/index.html`
- Modify: `internal/web/assets/ui/app.js`
- Create: `work/ui_webhook_provider_contract_check.js`

- [ ] **Step 1: 先写前端契约检查脚本**

新增 `work/ui_webhook_provider_contract_check.js`，只做静态检查，不跑浏览器。

```js
const fs = require('fs');

const html = fs.readFileSync('internal/web/assets/ui/index.html', 'utf8');
const js = fs.readFileSync('internal/web/assets/ui/app.js', 'utf8');
const failures = [];

[
  'id="webhook-provider"',
  'id="webhook-provider-hint"',
].forEach((token) => {
  if (!html.includes(token)) failures.push(`missing html token: ${token}`);
});

[
  'provider: \'generic\'',
  'function renderWebhookProviderState(',
  '系统将自动按飞书机器人格式生成 payload',
].forEach((token) => {
  if (!js.includes(token)) failures.push(`missing js token: ${token}`);
});

if (failures.length) {
  console.error(failures.join('\n'));
  process.exit(1);
}
console.log('ui webhook provider contract ok');
```

- [ ] **Step 2: 先跑脚本，确认失败**

Run:

```bash
node work/ui_webhook_provider_contract_check.js
```

Expected:

- FAIL
- 提示缺少 `webhook-provider` 和相关状态处理函数

- [ ] **Step 3: 在 HTML/JS 中补 provider 表单状态**

在 `internal/web/assets/ui/index.html` 的 webhook 配置区增加 provider 下拉和 hint：

```html
<label>类型 / 协议</label>
<select id="webhook-provider">
  <option value="generic">通用 webhook</option>
  <option value="feishu_bot">飞书机器人</option>
</select>
<div id="webhook-provider-hint" class="subtitle">
  通用 webhook 使用自定义 body_template。
</div>
```

在 `internal/web/assets/ui/app.js` 中把默认草稿、表单填充和保存 payload 改成带 `provider`：

```js
function buildDefaultWebhookDraft() {
  return {
    id: '',
    name: '',
    url: '',
    method: 'POST',
    provider: 'generic',
    headers: defaultWebhookHeaders(),
    body_template: defaultWebhookBodyTemplate(),
    enabled: true
  };
}

function fillWebhookForm(item, opts) {
  const options = opts || {};
  const cfg = Object.assign({}, buildDefaultWebhookDraft(), item || {});
  if (qs('#webhook-provider')) qs('#webhook-provider').value = cfg.provider || 'generic';
  if (qs('#webhook-body-template')) qs('#webhook-body-template').value = cfg.body_template || defaultWebhookBodyTemplate();
  renderWebhookProviderState();
  // 其余逻辑保持原样
}

function getWebhookPayload() {
  const current = currentWebhookItem();
  return {
    id: qs('#webhook-id')?.value.trim() || '',
    name: qs('#webhook-name')?.value.trim() || '',
    url: qs('#webhook-url')?.value.trim() || '',
    method: (qs('#webhook-method')?.value || 'POST').toUpperCase(),
    provider: qs('#webhook-provider')?.value || 'generic',
    headers: parseJSONObjectSafe(qs('#webhook-headers')?.value || '{}', 'webhook headers', {}),
    body_template: qs('#webhook-body-template')?.value || '',
    enabled: current ? current.enabled !== false : true
  };
}

function renderWebhookProviderState() {
  const provider = qs('#webhook-provider')?.value || 'generic';
  const isFeishu = provider === 'feishu_bot';
  const bodyField = qs('#webhook-body-template');
  const hint = qs('#webhook-provider-hint');
  if (bodyField) bodyField.disabled = isFeishu;
  if (hint) {
    hint.textContent = isFeishu
      ? '系统将自动按飞书机器人格式生成 payload，无需手工填写 body 模板。'
      : '通用 webhook 使用自定义 body_template。';
  }
  if (isFeishu && bodyField && !bodyField.value.trim()) {
    bodyField.value = '';
  }
}

qs('#webhook-provider')?.addEventListener('change', () => renderWebhookProviderState());
```

同时把列表展示和保存结果回显补上 `provider`：

```js
<div class="subtitle">
  <span class="pill">${escapeHtml(it.provider || 'generic')}</span>
  headers: ${Object.keys(it.headers || {}).length}
</div>
```

- [ ] **Step 4: 重新跑契约检查**

Run:

```bash
node work/ui_webhook_provider_contract_check.js
```

Expected:

- 输出 `ui webhook provider contract ok`

- [ ] **Step 5: 提交这一小步**

```bash
git add internal/web/assets/ui/index.html internal/web/assets/ui/app.js work/ui_webhook_provider_contract_check.js
git commit -m "feat: add webhook provider form state"
```

如果当前目录不是 git 仓库，跳过 commit，但保留变更。

---

### Task 5: 在任务侧显示“已自动匹配 payload”的只读提示

**Files:**
- Modify: `internal/web/assets/ui/index.html`
- Modify: `internal/web/assets/ui/app.js`
- Test: `work/ui_webhook_provider_contract_check.js`

- [ ] **Step 1: 先扩展契约检查，要求存在任务侧提示 token**

在 `work/ui_webhook_provider_contract_check.js` 追加检查：

```js
[
  'id="draft-webhook-auto-hint"',
  '已按飞书机器人协议自动匹配 payload',
  'function renderWebhookReferenceHint(',
].forEach((token) => {
  if (!(html + '\n' + js).includes(token)) {
    failures.push(`missing task hint token: ${token}`);
  }
});
```

- [ ] **Step 2: 先跑脚本，确认失败**

Run:

```bash
node work/ui_webhook_provider_contract_check.js
```

Expected:

- FAIL
- 缺少任务侧 hint 相关 token

- [ ] **Step 3: 给任务引用区补 provider 感知提示**

在 `internal/web/assets/ui/index.html` 的静态任务区加 hint 容器：

```html
<div id="draft-webhook-auto-hint" class="subtitle"></div>
<div id="complex-task-webhook-auto-hint" class="subtitle"></div>
```

在 `internal/web/assets/ui/app.js` 加一个通用 helper，并在草稿、周期任务、运营agent 渲染时调用：

```js
function getWebhookCatalogItem(id) {
  return (window.__connCenterState.webhooks || []).find((it) => (it.id || '') === (id || '')) || null;
}

function renderWebhookReferenceHint(selectSelector, hostSelector) {
  const host = qs(hostSelector);
  if (!host) return;
  const webhookID = qs(selectSelector)?.value || '';
  const item = getWebhookCatalogItem(webhookID);
  if (!item) {
    host.textContent = '';
    return;
  }
  if ((item.provider || 'generic') === 'feishu_bot') {
    host.textContent = '已按飞书机器人协议自动匹配 payload。';
    return;
  }
  host.textContent = '当前 webhook 使用自定义 body_template。';
}
```

对静态表单直接挂监听：

```js
qs('#draft-webhook-config-id')?.addEventListener('change', () => {
  renderWebhookReferenceHint('#draft-webhook-config-id', '#draft-webhook-auto-hint');
});
qs('#complex-task-webhook-config-id')?.addEventListener('change', () => {
  renderWebhookReferenceHint('#complex-task-webhook-config-id', '#complex-task-webhook-auto-hint');
});
```

对动态生成的周期任务表单，在渲染 HTML 时插入说明块：

```js
const webhookItem = getWebhookCatalogItem(job.webhook_config_id || '');
const webhookHint = webhookItem
  ? ((webhookItem.provider || 'generic') === 'feishu_bot'
      ? '已按飞书机器人协议自动匹配 payload。'
      : '当前 webhook 使用自定义 body_template。')
  : '';
```

并把该文案插到 job drawer / form 的 webhook 选择器下方。

- [ ] **Step 4: 重跑契约检查**

Run:

```bash
node work/ui_webhook_provider_contract_check.js
```

Expected:

- 输出 `ui webhook provider contract ok`
- 静态检查同时覆盖 provider 表单和任务侧提示

- [ ] **Step 5: 提交这一小步**

```bash
git add internal/web/assets/ui/index.html internal/web/assets/ui/app.js work/ui_webhook_provider_contract_check.js
git commit -m "feat: show auto payload hint for webhook bindings"
```

如果当前目录不是 git 仓库，跳过 commit，但保留变更。

---

### Task 6: 做一次最小回归验证并补文档说明

**Files:**
- Modify: `internal/web/server_test.go`
- Modify: `internal/webhook/delivery_test.go`
- Modify: `docs/superpowers/specs/2026-06-06-webhook-provider-auto-payload-design.md`

- [ ] **Step 1: 回顾 spec 覆盖面，补一个文档内显式示例**

在 `docs/superpowers/specs/2026-06-06-webhook-provider-auto-payload-design.md` 的飞书章节末尾追加一个“配置示例”，让实现后的页面文案和文档一致：

```md
### 配置示例

- webhook 类型：`飞书机器人`
- webhook-url：`https://open.feishu.cn/open-apis/bot/v2/hook/...`
- 任务侧：只勾选该 `webhook-id`
- 运行时：系统自动将任务结果 JSON 放入 `content.text`
```

- [ ] **Step 2: 跑后端测试**

Run:

```bash
go test ./internal/webhook ./internal/web -count=1
```

Expected:

- 全部 PASS

- [ ] **Step 3: 跑前端契约检查**

Run:

```bash
node work/ui_webhook_provider_contract_check.js
```

Expected:

- 输出 `ui webhook provider contract ok`

- [ ] **Step 4: 记录已知环境差异**

如果当前执行环境没有 Go，可在本次变更说明里记录：

```md
本地实现后请在具备 Go 工具链的环境补跑：

go test ./... -count=1
```

同时不要因为当前环境无 Go 就跳过前端契约检查。

- [ ] **Step 5: 最终提交**

```bash
git add internal/storage/webhook_store.go internal/webhook/delivery.go internal/webhook/delivery_test.go internal/web/server.go internal/web/server_test.go internal/web/assets/ui/index.html internal/web/assets/ui/app.js work/ui_webhook_provider_contract_check.js docs/superpowers/specs/2026-06-06-webhook-provider-auto-payload-design.md
git commit -m "feat: auto match webhook payload by provider"
```

如果当前目录不是 git 仓库，跳过 commit，但保留变更。

---

## Self-Review

### Spec coverage

- `provider` 模型与兼容策略：Task 1
- `generic` / `feishu_bot` 运行时自动请求体：Task 2
- webhook 测试接口复用 provider-aware 逻辑：Task 3
- webhook 配置页 provider 下拉与飞书只读提示：Task 4
- 任务页“已自动匹配 payload”提示：Task 5
- 文档与最小回归：Task 6

没有遗漏 spec 中的核心要求。

### Placeholder scan

- 计划中没有 `TODO` / `TBD`
- 每个改动任务都给了目标文件、测试命令和最小代码骨架

### Type consistency

- 统一使用 `provider`
- provider 取值统一为 `generic` / `feishu_bot`
- 统一复用 `DeliverWebhookForSuccess(...)` / `buildWebhookRequestBody(...)`

---
