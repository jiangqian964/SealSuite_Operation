# 单次/周期任务清单大模型调用绑定具体 LLM_API Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在 `单次/周期任务清单` 的 `大模型调用` 区域增加具体 `LLM_API` 模型选择，并让执行链优先按 `llm_api_id` 调用对应模型，同时兼容旧的 `role` 逻辑。

**Architecture:** 前端在任务草稿工作台增加 `LLM_API` 下拉选择，并将引用保存到 `llm_config.llm_api_id`。后端执行 `llm_inference` 时优先按 `llm_api_id` 从 `LLMAPIStore` 解析具体模型，再用 `llm_config` 中的 prompt/temperature 等字段覆盖；若未选择具体模型，则继续回退到 `planner/formatter` 角色配置。

**Tech Stack:** Vanilla JS、HTML、CSS、Go HTTP API、YAML 存储、Node.js 契约检查、Go test（如环境具备）

---

## 文件结构

### 主要前端文件

- Modify: `internal/web/assets/ui/index.html`
  - 在 `大模型调用` 区域新增 `LLM_API` 下拉框与提示文案
- Modify: `internal/web/assets/ui/app.js`
  - 加载 `LLM_API` 列表并渲染到下拉框
  - 草稿保存/回填 `llm_config.llm_api_id`
  - 执行时把 `llm_api_id` 发给后端

### 主要后端文件

- Modify: `internal/runner/runner.go`
  - 扩展 `llm_inference` 解析逻辑，优先按 `llm_api_id` 读取具体模型
- Modify: `internal/web/server.go`
  - 如需暴露更清晰的执行结果元信息或补全引用校验，可在这里最小接入
- Modify: `internal/web/server_test.go`
  - 补任务草稿/执行链回归测试

### 轻量前端契约检查

- Create: `work/ui_task_draft_llm_api_selector_contract_check.js`
  - 检查下拉框、草稿字段、执行 payload 挂点

---

### Task 1: 在任务草稿工作台增加具体 LLM_API 模型选择控件

**Files:**
- Modify: `internal/web/assets/ui/index.html`
- Modify: `internal/web/assets/ui/app.js`
- Create: `work/ui_task_draft_llm_api_selector_contract_check.js`

- [ ] **Step 1: 先写契约检查脚本**

创建 `work/ui_task_draft_llm_api_selector_contract_check.js`：

```js
const fs = require('fs');

const html = fs.readFileSync('internal/web/assets/ui/index.html', 'utf8');
const js = fs.readFileSync('internal/web/assets/ui/app.js', 'utf8');
const failures = [];

[
  'id="draft-llm-api-id"',
  '模型选择',
  'id="draft-llm-api-hint"',
].forEach((token) => {
  if (!html.includes(token)) failures.push(`html missing ${token}`);
});

[
  'function buildDraftLLMAPIOptions(',
  'function renderDraftLLMAPIHint(',
  "fetchJSON('/api/v1/settings/llm/apis')",
].forEach((token) => {
  if (!js.includes(token)) failures.push(`js missing ${token}`);
});

if (failures.length) {
  console.error(failures.join('\n'));
  process.exit(1);
}
console.log('ui task draft llm api selector contract ok');
```

- [ ] **Step 2: 先运行脚本，确认失败**

Run:

```bash
node work/ui_task_draft_llm_api_selector_contract_check.js
```

Expected:

- FAIL
- 缺少 `draft-llm-api-id` 与相关 JS 函数

- [ ] **Step 3: 修改 HTML，在大模型调用上方增加模型选择区**

在 `internal/web/assets/ui/index.html` 的 `大模型调用` 模块中，将：

```html
<div class="subtitle">大模型调用</div>
<textarea id="draft-llm" rows="5" placeholder='{"provider":"openai","prompt":"请总结结果"}'></textarea>
```

改成：

```html
<div class="subtitle">大模型调用</div>
<div class="row">
  <div class="field" style="flex:1">
    <label>模型选择</label>
    <select id="draft-llm-api-id">
      <option value="">使用 role 的默认模型</option>
    </select>
    <div id="draft-llm-api-hint" class="subtitle">未指定具体模型时，将按 role 使用运行时默认模型。</div>
  </div>
</div>
<textarea id="draft-llm" rows="5" placeholder='{"role":"formatter","prompt":"请总结结果"}'></textarea>
```

- [ ] **Step 4: 在前端加载并渲染 LLM_API 列表**

在 `internal/web/assets/ui/app.js` 中新增：

```js
window.__llmAPIItems = [];

function buildDraftLLMAPIOptions(selectedID) {
  const items = Array.isArray(window.__llmAPIItems) ? window.__llmAPIItems : [];
  const parts = [`<option value="">使用 role 的默认模型</option>`];
  items.forEach((item) => {
    const id = item.id || '';
    const active = item.active ? ' · ACTIVE' : '';
    const label = `${item.name || id} · ${item.provider || '-'} / ${item.model || '-'}${active}`;
    parts.push(`<option value="${escapeHtml(id)}" ${id === selectedID ? 'selected' : ''}>${escapeHtml(label)}</option>`);
  });
  return parts.join('');
}

function renderDraftLLMAPIHint(selectedID) {
  const host = qs('#draft-llm-api-hint');
  if (!host) return;
  const items = Array.isArray(window.__llmAPIItems) ? window.__llmAPIItems : [];
  const item = items.find((it) => (it.id || '') === selectedID);
  if (!selectedID) {
    host.textContent = '未指定具体模型时，将按 role 使用运行时默认模型。';
    return;
  }
  if (!item) {
    host.textContent = '当前草稿引用的模型不存在或已删除。';
    return;
  }
  host.textContent = `${item.provider || '-'} / ${item.model || '-'}${item.active ? ' · ACTIVE' : ''}`;
}
```

并补一个加载函数：

```js
async function refreshLLMAPIItems() {
  try {
    const data = await fetchJSON('/api/v1/settings/llm/apis');
    window.__llmAPIItems = Array.isArray(data.items) ? data.items : [];
    if (qs('#draft-llm-api-id')) {
      const selected = qs('#draft-llm-api-id').value || '';
      qs('#draft-llm-api-id').innerHTML = buildDraftLLMAPIOptions(selected);
      renderDraftLLMAPIHint(selected);
    }
  } catch (e) {
    window.__llmAPIItems = [];
    if (qs('#draft-llm-api-hint')) qs('#draft-llm-api-hint').textContent = `LLM_API 列表加载失败：${String(e)}`;
  }
}
```

- [ ] **Step 5: 绑定切换事件并回跑契约检查**

补事件绑定：

```js
qs('#draft-llm-api-id')?.addEventListener('change', () => {
  renderDraftLLMAPIHint(qs('#draft-llm-api-id').value || '');
});
```

在首次加载工作台或初始化时，调用：

```js
refreshLLMAPIItems();
```

Run:

```bash
node work/ui_task_draft_llm_api_selector_contract_check.js
```

Expected:

- 输出 `ui task draft llm api selector contract ok`

- [ ] **Step 6: 提交这一小步**

```bash
git add internal/web/assets/ui/index.html internal/web/assets/ui/app.js work/ui_task_draft_llm_api_selector_contract_check.js
git commit -m "feat: add llm api selector for task drafts"
```

如果当前目录不是 git 仓库，跳过 commit，但保留变更。

---

### Task 2: 将 llm_api_id 保存到 llm_config，并支持草稿回填

**Files:**
- Modify: `internal/web/assets/ui/app.js`
- Test: `work/ui_task_draft_llm_api_selector_contract_check.js`

- [ ] **Step 1: 扩展契约检查，锁定 llm_api_id 存储与回填**

在 `work/ui_task_draft_llm_api_selector_contract_check.js` 中追加：

```js
[
  'llm_api_id',
  "qs('#draft-llm-api-id').value",
  "draft.llm_config || {}",
].forEach((token) => {
  if (!js.includes(token)) failures.push(`js missing ${token}`);
});
```

- [ ] **Step 2: 先运行脚本，确认失败**

Run:

```bash
node work/ui_task_draft_llm_api_selector_contract_check.js
```

Expected:

- FAIL
- 缺少 `llm_api_id` 挂点

- [ ] **Step 3: 在草稿回填时读取 llm_api_id**

在 `loadTaskDraftIntoWorkbench(draft)` 中，替换原来的纯文本回填：

```js
qs('#draft-llm').value = pretty(draft.llm_config || {});
```

为：

```js
const llmConfig = cloneValue(draft.llm_config || {});
const llmAPIID = llmConfig.llm_api_id || '';
if ('llm_api_id' in llmConfig) delete llmConfig.llm_api_id;
if (qs('#draft-llm-api-id')) {
  qs('#draft-llm-api-id').innerHTML = buildDraftLLMAPIOptions(llmAPIID);
  qs('#draft-llm-api-id').value = llmAPIID;
  renderDraftLLMAPIHint(llmAPIID);
}
qs('#draft-llm').value = pretty(llmConfig);
```

- [ ] **Step 4: 在草稿保存时写入 llm_api_id**

在 `collectTaskDraftPayload()` 中，将：

```js
llm_config: parseJSONOrEmpty(qs('#draft-llm').value),
```

改成：

```js
const llmConfig = parseJSONOrEmpty(qs('#draft-llm').value);
const llmAPIID = qs('#draft-llm-api-id')?.value.trim() || '';
if (llmAPIID) {
  llmConfig.llm_api_id = llmAPIID;
} else {
  delete llmConfig.llm_api_id;
}
```

然后返回：

```js
llm_config: llmConfig,
```

- [ ] **Step 5: Execute payload 也带上 llm_api_id**

在 `buildExecutePayload()` 中，同样把下拉选择写入：

```js
const llmConfig = parseJSONOrEmpty(qs('#draft-llm')?.value || '{}');
const llmAPIID = qs('#draft-llm-api-id')?.value.trim() || '';
if (llmAPIID) {
  llmConfig.llm_api_id = llmAPIID;
} else {
  delete llmConfig.llm_api_id;
}
```

再返回：

```js
llm_config: llmConfig,
```

- [ ] **Step 6: 回跑契约检查**

Run:

```bash
node work/ui_task_draft_llm_api_selector_contract_check.js
```

Expected:

- 输出 `ui task draft llm api selector contract ok`

- [ ] **Step 7: 提交这一小步**

```bash
git add internal/web/assets/ui/app.js work/ui_task_draft_llm_api_selector_contract_check.js
git commit -m "feat: persist llm api selection in task drafts"
```

如果当前目录不是 git 仓库，跳过 commit，但保留变更。

---

### Task 3: 执行 llm_inference 时优先按 llm_api_id 解析具体模型

**Files:**
- Modify: `internal/runner/runner.go`
- Modify: `internal/web/server.go`
- Test: `internal/web/server_test.go`

- [ ] **Step 1: 先写失败测试，锁定 llm_api_id 优先级**

在 `internal/web/server_test.go` 新增一条测试：

```go
func TestAPIExecuteUsesSelectedLLMAPIIDBeforeRoleFallback(t *testing.T) {
	withTempWorkingDir(t, func(dir string) {
		llmSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/chat/completions" {
				http.NotFound(w, r)
				return
			}
			body, _ := io.ReadAll(r.Body)
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
					BaseURL:  "https://fallback.example.com",
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
			"llm_config":{"role":"formatter","llm_api_id":"llm_pro_main","prompt":"请做摘要"},
			"output_config":{"format":"text"}
		}`))
		req.Header.Set("Content-Type", "application/json")
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("execute got %d body=%s", rr.Code, rr.Body.String())
		}
		if !strings.Contains(rr.Body.String(), "模型摘要成功") {
			t.Fatalf("expected llm content in response, body=%s", rr.Body.String())
		}
	}
}
```

- [ ] **Step 2: 运行测试，确认失败**

Run:

```bash
go test ./internal/web -run TestAPIExecuteUsesSelectedLLMAPIIDBeforeRoleFallback -count=1
```

Expected:

- FAIL
- 还未读取 `llm_api_id`

如果本地没有 Go，则至少记录此步骤为待外部环境补跑。

- [ ] **Step 3: 在 Runner 中增加按 llm_api_id 解析具体模型**

在 `internal/runner/runner.go` 中新增辅助函数：

```go
func (r *Runner) resolveLLMConfigFromAPIID(id string) (config.LLMRoleConfig, error) {
	store := storage.NewLLMAPIStore("llm-apis.yaml")
	file, err := store.Load()
	if err != nil {
		return config.LLMRoleConfig{}, err
	}
	item, ok := findLLMAPIItem(file, id)
	if !ok {
		return config.LLMRoleConfig{}, fmt.Errorf("指定的 LLM_API 不存在：%s", id)
	}
	if !item.Enabled {
		return config.LLMRoleConfig{}, fmt.Errorf("指定的 LLM_API 已禁用：%s", id)
	}
	return config.LLMRoleConfig{
		Enabled:         item.Enabled,
		Provider:        item.Provider,
		BaseURL:         item.BaseURL,
		APIKey:          item.APIKey,
		Model:           item.Model,
		Timeout:         item.Timeout,
		Temperature:     item.Temperature,
		MaxTokens:       item.MaxTokens,
		Thinking:        item.Thinking,
		ReasoningEffort: item.ReasoningEffort,
		SystemPrompt:    item.SystemPrompt,
		ResponseFormat:  cloneMapAny(item.ResponseFormat),
	}, nil
}
```

然后把 `resolveLLMRoleConfig(role, overrides)` 改成：

```go
func (r *Runner) resolveLLMRoleConfig(role string, overrides map[string]interface{}) config.LLMRoleConfig {
	if id := strings.TrimSpace(asString(overrides["llm_api_id"])); id != "" {
		if llmCfg, err := r.resolveLLMConfigFromAPIID(id); err == nil {
			return applyLLMRoleOverrides(llmCfg, overrides)
		}
	}
	...
}
```

并抽出覆盖函数：

```go
func applyLLMRoleOverrides(llmCfg config.LLMRoleConfig, overrides map[string]interface{}) config.LLMRoleConfig {
  // 复用当前 provider/base_url/model/system_prompt/temperature/max_tokens/thinking/response_format 覆盖逻辑
}
```

- [ ] **Step 4: 执行结果回显 llm_api_id**

在 `llm_inference` 成功返回处，把：

```go
return map[string]interface{}{
  "role":     role,
  "provider": llmCfg.Provider,
  "model":    resp.Model,
  "prompt":   prompt,
  "content":  resp.Content,
  "raw":      resp.Raw,
}, nil
```

改成：

```go
result := map[string]interface{}{
  "role":     role,
  "provider": llmCfg.Provider,
  "model":    resp.Model,
  "prompt":   prompt,
  "content":  resp.Content,
  "raw":      resp.Raw,
}
if id := strings.TrimSpace(asString(cfg["llm_api_id"])); id != "" {
  result["llm_api_id"] = id
}
return result, nil
```

预览失败分支也同样补 `llm_api_id`。

- [ ] **Step 5: 再次运行测试，验证通过**

Run:

```bash
go test ./internal/web -run TestAPIExecuteUsesSelectedLLMAPIIDBeforeRoleFallback -count=1
```

Expected:

- PASS

- [ ] **Step 6: 提交这一小步**

```bash
git add internal/runner/runner.go internal/web/server_test.go
git commit -m "feat: resolve task draft llm by llm api id"
```

如果当前目录不是 git 仓库，跳过 commit，但保留变更。

---

### Task 4: 兼容旧 role 逻辑，并完善错误提示与前端可诊断性

**Files:**
- Modify: `internal/runner/runner.go`
- Modify: `internal/web/assets/ui/app.js`
- Test: `internal/web/server_test.go`
- Test: `work/ui_task_draft_llm_api_selector_contract_check.js`

- [ ] **Step 1: 增加“模型不存在/已禁用”的失败测试**

在 `internal/web/server_test.go` 中新增：

```go
func TestAPIExecuteReturnsReadableErrorWhenSelectedLLMAPIIsMissing(t *testing.T) {
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
			"mode":"workflow",
			"llm_config":{"role":"formatter","llm_api_id":"missing_model","prompt":"请做摘要"}
		}`))
		req.Header.Set("Content-Type", "application/json")
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d body=%s", rr.Code, rr.Body.String())
		}
		if !strings.Contains(rr.Body.String(), "指定的 LLM_API 不存在") {
			t.Fatalf("expected readable llm api missing error, body=%s", rr.Body.String())
		}
	})
}
```

- [ ] **Step 2: 运行测试，确认失败**

Run:

```bash
go test ./internal/web -run TestAPIExecuteReturnsReadableErrorWhenSelectedLLMAPIIsMissing -count=1
```

Expected:

- FAIL

- [ ] **Step 3: 让 llm_api_id 解析失败直接返回明确错误**

在 `internal/runner/runner.go` 中，不要静默忽略：

```go
if id := strings.TrimSpace(asString(overrides["llm_api_id"])); id != "" {
  if llmCfg, err := r.resolveLLMConfigFromAPIID(id); err == nil {
    return applyLLMRoleOverrides(llmCfg, overrides)
  }
}
```

改成明确分支，例如：

```go
func (r *Runner) resolveLLMConfig(role string, overrides map[string]interface{}) (config.LLMRoleConfig, error) {
  if id := strings.TrimSpace(asString(overrides["llm_api_id"])); id != "" {
    llmCfg, err := r.resolveLLMConfigFromAPIID(id)
    if err != nil {
      return config.LLMRoleConfig{}, err
    }
    return applyLLMRoleOverrides(llmCfg, overrides), nil
  }
  return applyLLMRoleOverrides(r.resolveLLMRoleConfig(role, nil), overrides), nil
}
```

然后在 `llm_inference` 里调用新的可报错版本。

- [ ] **Step 4: 前端加强已选模型提示**

在 `renderDraftLLMAPIHint(selectedID)` 中，将提示增强为：

```js
if (!item) {
  host.textContent = '当前草稿引用的模型不存在或已删除。';
  host.classList.add('danger-text');
  return;
}
host.classList.remove('danger-text');
host.textContent = `${item.provider || '-'} / ${item.model || '-'}${item.active ? ' · ACTIVE' : ''}${item.enabled === false ? ' · 已禁用' : ''}`;
```

并在 `loadTaskDraftIntoWorkbench(draft)` 后调用：

```js
renderDraftLLMAPIHint(llmAPIID);
```

- [ ] **Step 5: 回跑前端契约检查和新增 Go 测试**

Run:

```bash
node work/ui_task_draft_llm_api_selector_contract_check.js
go test ./internal/web -run 'TestAPIExecuteUsesSelectedLLMAPIIDBeforeRoleFallback|TestAPIExecuteReturnsReadableErrorWhenSelectedLLMAPIIsMissing|TestActivateSingleLLMAPIMapsToFormatterWhenNoExplicitRoleTags' -count=1
```

Expected:

- 前端契约检查 PASS
- Go 测试 PASS

- [ ] **Step 6: 提交这一小步**

```bash
git add internal/runner/runner.go internal/web/assets/ui/app.js internal/web/server_test.go work/ui_task_draft_llm_api_selector_contract_check.js
git commit -m "feat: support explicit llm api selection in drafts"
```

如果当前目录不是 git 仓库，跳过 commit，但保留变更。

---

### Task 5: 文档收口与最终回归

**Files:**
- Modify: `docs/superpowers/specs/2026-06-06-task-draft-llm-api-selector-design.md`
- Test: `work/ui_task_draft_llm_api_selector_contract_check.js`

- [ ] **Step 1: 在 spec 中补字段映射与优先级摘要**

在 `docs/superpowers/specs/2026-06-06-task-draft-llm-api-selector-design.md` 末尾追加：

```md
## 13. 字段与优先级摘要

- 下拉选择保存到：`llm_config.llm_api_id`
- 执行优先级：
  1. `llm_api_id`
  2. `llm_config` 中的显式覆盖字段
  3. `role` 对应的运行时默认模型
```

- [ ] **Step 2: 运行前端契约检查**

Run:

```bash
node work/ui_task_draft_llm_api_selector_contract_check.js
```

Expected:

- 输出 `ui task draft llm api selector contract ok`

- [ ] **Step 3: 如环境具备，跑一次目标 Go 测试集**

Run:

```bash
go test ./internal/web -run 'TestAPIExecuteUsesSelectedLLMAPIIDBeforeRoleFallback|TestAPIExecuteReturnsReadableErrorWhenSelectedLLMAPIIsMissing|TestActivateSingleLLMAPIMapsToFormatterWhenNoExplicitRoleTags' -count=1
```

Expected:

- PASS

如果环境没有 Go，则在说明中记录：

```md
请在具备 Go 工具链的环境补跑目标测试集：
go test ./internal/web -run 'TestAPIExecuteUsesSelectedLLMAPIIDBeforeRoleFallback|TestAPIExecuteReturnsReadableErrorWhenSelectedLLMAPIIsMissing|TestActivateSingleLLMAPIMapsToFormatterWhenNoExplicitRoleTags' -count=1
```

- [ ] **Step 4: 最终提交**

```bash
git add internal/web/assets/ui/index.html internal/web/assets/ui/app.js internal/runner/runner.go internal/web/server.go internal/web/server_test.go work/ui_task_draft_llm_api_selector_contract_check.js docs/superpowers/specs/2026-06-06-task-draft-llm-api-selector-design.md
git commit -m "feat: bind task draft llm calls to explicit llm api"
```

如果当前目录不是 git 仓库，跳过 commit，但保留变更。

---

## Self-Review

### Spec coverage

- UI 下拉选择具体模型：Task 1
- `llm_config.llm_api_id` 存储与回填：Task 2
- 执行优先按 `llm_api_id`：Task 3
- 兼容旧 `role` 逻辑与可读错误：Task 4
- 文档优先级摘要与回归：Task 5

无明显遗漏。

### Placeholder scan

- 计划中没有 `TODO` / `TBD`
- 每个任务都包含具体文件、代码片段、命令与预期结果

### Type consistency

- 统一使用：
  - `llm_config.llm_api_id`
  - `buildDraftLLMAPIOptions`
  - `renderDraftLLMAPIHint`
  - `resolveLLMConfigFromAPIID`
  - `applyLLMRoleOverrides`

---
