function qs(sel) { return document.querySelector(sel); }
function qsa(sel) { return Array.from(document.querySelectorAll(sel)); }

function openModal() {
  qs('#modal').classList.remove('hidden');
}
function closeModal() {
  qs('#modal').classList.add('hidden');
}

function toast(title, msg, kind) {
  const t = qs('#toast');
  const dot = qs('#health-dot');
  qs('#toast-title').textContent = title || '提示';
  qs('#toast-msg').textContent = msg || '';
  t.classList.add('show');
  clearTimeout(window.__toastTimer);
  window.__toastTimer = setTimeout(() => t.classList.remove('show'), 3200);
  if (kind === 'ok') dot.classList.add('ok');
}

async function fetchJSON(url, opts) {
  const res = await fetch(url, Object.assign({
    headers: { 'Content-Type': 'application/json' }
  }, opts || {}));
  const text = await res.text();
  let json;
  try { json = JSON.parse(text); } catch { json = { raw: text }; }
  if (!res.ok) {
    const err = new Error(`HTTP ${res.status}: ${text}`);
    err.status = res.status;
    err.payload = json;
    throw err;
  }
  return json;
}

function parseJSONOrEmpty(s) {
  const t = (s || '').trim();
  if (!t) return {};
  return JSON.parse(t);
}

function setView(name) {
  qsa('.navbtn').forEach(b => b.classList.toggle('active', b.dataset.view === name));
  qsa('.view').forEach(v => v.classList.add('hidden'));
  const view = qs(`#view-${name}`);
  if (!view) return;
  view.classList.remove('hidden');
  if (name === 'tasks') switchTaskSubpage(window.__taskSubpage || 'once-cycle');
}

qsa('.navbtn').forEach(b => b.addEventListener('click', () => setView(b.dataset.view)));

window.__taskSubpage = 'once-cycle';

function getTaskSubpageMeta(name) {
  if (name === 'schedule') {
    return {
      title: '定时任务清单',
      subtitle: '调度中心：绑定单次/周期任务或运营agent，配置开始时间、结束时间、执行周期或间隔，并支持轻量二次处理。'
    };
  }
  return {
    title: '单次/周期任务清单',
    subtitle: '配置单次或周期飞连 API 请求任务、输入参数、轻量处理、大模型调用与输出格式，并保存为任务草稿。'
  };
}

function renderTaskSubpageHeader(name) {
  const meta = getTaskSubpageMeta(name);
  if (qs('#task-page-title')) qs('#task-page-title').textContent = meta.title;
  if (qs('#task-page-subtitle')) qs('#task-page-subtitle').textContent = meta.subtitle;
}

function switchTaskSubpage(name) {
  const next = name === 'schedule' ? 'schedule' : 'once-cycle';
  window.__taskSubpage = next;
  qs('#task-subpage-once-cycle')?.classList.toggle('hidden', next !== 'once-cycle');
  qs('#task-subpage-schedule')?.classList.toggle('hidden', next !== 'schedule');
  qs('#btn-task-sub-once-cycle')?.classList.toggle('active', next === 'once-cycle');
  qs('#btn-task-sub-schedule')?.classList.toggle('active', next === 'schedule');
  qs('#btn-task-sub-once-cycle')?.setAttribute('aria-pressed', String(next === 'once-cycle'));
  qs('#btn-task-sub-schedule')?.setAttribute('aria-pressed', String(next === 'schedule'));
  renderTaskSubpageHeader(next);
  return next;
}

function openTaskCenterSubpage(name) {
  setView('tasks');
  switchTaskSubpage(name);
}

window.__connSubpage = 'feilian';

window.__connCenterState = {
  currentType: 'connection',
  currentMode: 'view',
  currentConnectionID: '',
  currentConnectionMode: 'view',
  currentLLMAPIID: '',
  currentLLMMode: 'view',
  currentWebhookID: '',
  currentWebhookMode: 'create',
  connList: [],
  llmAPIs: [],
  webhooks: []
};
window.__webhookCatalogLoaded = false;

async function initVersion() {
  try {
    const v = await fetchJSON('/api/v1/version');
    qs('#version').textContent = v.version ? `v${v.version}` : '';
  } catch {}
}
initVersion();

async function refreshHealth(quiet) {
  try {
    const data = await fetchJSON('/api/v1/health');
    qs('#health').textContent = JSON.stringify(data, null, 2);
    qs('#health-text').textContent = data.ok ? 'healthy' : 'unknown';
    qs('#health-big').textContent = data.ok ? 'Healthy' : 'Unknown';
    qs('#health-dot').classList.toggle('ok', !!data.ok);
    const pill = qs('#health-pill');
    pill.textContent = data.ok ? 'OK' : 'UNKNOWN';
    pill.className = 'pill ' + (data.ok ? 'ok' : '');
    if (!quiet) toast('健康状态', data.ok ? '服务正常' : '服务未知', data.ok ? 'ok' : '');
  } catch (e) {
    qs('#health-text').textContent = 'error';
    qs('#health-big').textContent = 'Error';
    qs('#health-dot').classList.remove('ok');
    const pill = qs('#health-pill');
    pill.textContent = 'ERROR';
    pill.className = 'pill bad';
    if (!quiet) toast('健康状态', String(e), '');
  }
}

qs('#btn-health').addEventListener('click', async () => {
  await refreshHealth(false);
});

qs('#btn-health-quick').addEventListener('click', async () => refreshHealth(false));
qs('#btn-reload-quick').addEventListener('click', async () => {
  try {
    await fetchJSON('/api/v1/reload', { method: 'POST', body: '{}' });
    toast('Reload', '已触发 Reload', 'ok');
  } catch (e) {
    toast('Reload 失败', String(e), '');
  }
});

qs('#btn-open-connection').addEventListener('click', () => {
  setView('connection');
  setConnectionSubpage(window.__connSubpage || 'feilian');
});
qs('#btn-open-api').addEventListener('click', () => openTaskCenterSubpage('once-cycle'));
qs('#btn-open-jobs').addEventListener('click', () => openTaskCenterSubpage('schedule'));
qs('#btn-open-templates')?.addEventListener('click', () => setView('templates'));
qs('#btn-api-go-templates')?.addEventListener('click', () => setView('templates'));
qs('#btn-task-sub-once-cycle')?.addEventListener('click', () => switchTaskSubpage('once-cycle'));
qs('#btn-task-sub-schedule')?.addEventListener('click', () => switchTaskSubpage('schedule'));

// --- Connection ---
function setReadonlyFieldState(selectors, readonly) {
  selectors.forEach((sel) => {
    const el = qs(sel);
    if (!el) return;
    const isSelect = el.tagName === 'SELECT';
    const isCheckbox = el.tagName === 'INPUT' && (el.type === 'checkbox' || el.type === 'radio');
    if (isSelect || isCheckbox) {
      el.disabled = !!readonly;
      return;
    }
    if ('readOnly' in el) el.readOnly = !!readonly;
  });
}

function currentConnectionItem() {
  const id = window.__connCenterState.currentConnectionID || '';
  return (window.__connCenterState.connList || []).find((it) => it.id === id) || null;
}

function currentLLMAPIItem() {
  const id = window.__connCenterState.currentLLMAPIID || '';
  return (window.__connCenterState.llmAPIs || []).find((it) => it.id === id) || null;
}

function currentWebhookItem() {
  const id = window.__connCenterState.currentWebhookID || '';
  return (window.__connCenterState.webhooks || []).find((it) => it.id === id) || null;
}

function normalizeConnectionSubpage(name) {
  return ['feilian', 'llm', 'webhook'].includes(name) ? name : 'feilian';
}

function getConnectionWorkbenchMode(type) {
  const state = window.__connCenterState;
  if (type === 'llm') return state.currentLLMMode || (state.currentType === 'llm' ? state.currentMode : 'view');
  return state.currentConnectionMode || (state.currentType === 'connection' ? state.currentMode : 'view');
}

function syncConnectionSubpageUI(name) {
  const current = normalizeConnectionSubpage(name);
  qs('#subpage-feilian')?.classList.toggle('hidden', current !== 'feilian');
  qs('#subpage-llm')?.classList.toggle('hidden', current !== 'llm');
  qs('#subpage-webhook')?.classList.toggle('hidden', current !== 'webhook');
  qs('#btn-sub-feilian')?.classList.toggle('active', current === 'feilian');
  qs('#btn-sub-llm')?.classList.toggle('active', current === 'llm');
  qs('#btn-sub-webhook')?.classList.toggle('active', current === 'webhook');
}

function setConnectionSubpage(name) {
  const current = normalizeConnectionSubpage(name);
  window.__connSubpage = current;
  syncConnectionSubpageUI(current);
  if (current === 'feilian') {
    setConnectionWorkbenchMode('connection', getConnectionWorkbenchMode('connection'), window.__connCenterState.currentConnectionID || '');
    return current;
  }
  if (current === 'llm') {
    setConnectionWorkbenchMode('llm', getConnectionWorkbenchMode('llm'), window.__connCenterState.currentLLMAPIID || '');
    return current;
  }
  renderWebhookSubpage();
  return current;
}

function getWorkbenchModeText(type, mode) {
  const labelMap = {
    view: '查看态',
    edit: '编辑态',
    create: '新建态'
  };
  const base = labelMap[mode] || '查看态';
  const current = type === 'llm' ? currentLLMAPIItem() : currentConnectionItem();
  const suffix = current ? (current.name || current.id || '') : '';
  return suffix ? `${base} · ${suffix}` : base;
}

async function getConnPayload() {
  const name = qs('#conn-name') ? qs('#conn-name').value.trim() : '';
  const scheme = qs('#conn-scheme').value.trim();
  const host = qs('#conn-host').value.trim();
  const portStr = qs('#conn-port').value.trim();
  const access_key = qs('#conn-ak').value.trim();
  const secret_key = qs('#conn-sk').value.trim();
  const payload = {};
  if (name) payload.name = name;
  if (scheme) payload.scheme = scheme;
  if (host) payload.host = host;
  if (portStr) payload.port = Number(portStr);
  if (access_key) payload.access_key = access_key;
  payload.secret_key = secret_key;
  return payload;
}

function parseTagInput(v) {
  return String(v || '').split(',').map((s) => s.trim()).filter(Boolean);
}

function parseJSONValueSafe(raw, label, defaultValue) {
  const text = String(raw || '').trim();
  if (!text) return cloneValue(defaultValue);
  try {
    return JSON.parse(text);
  } catch (e) {
    throw new Error(`${label} 必须是合法 JSON: ${e.message}`);
  }
}

function parseJSONObjectSafe(raw, label, defaultValue) {
  const value = parseJSONValueSafe(raw, label, defaultValue);
  if (value === null || Array.isArray(value) || typeof value !== 'object') {
    throw new Error(`${label} 必须是 JSON 对象`);
  }
  return value;
}

function getWebhookProviderValue(provider) {
  return String(provider || '').trim() === 'feishu_bot' ? 'feishu_bot' : 'generic';
}

function getWebhookProviderLabel(provider) {
  return getWebhookProviderValue(provider) === 'feishu_bot' ? '飞书机器人' : '通用 webhook';
}

function getWebhookCatalogItem(id) {
  return (window.__connCenterState.webhooks || []).find((item) => (item.id || '') === (id || '')) || null;
}

function getWebhookReferenceHintText(item) {
  if (!item) return '';
  return getWebhookProviderValue(item.provider) === 'feishu_bot'
    ? '已按飞书机器人协议自动匹配 payload。'
    : '当前 webhook 使用自定义 body_template。';
}

function getWebhookReferenceHintHostSelector(selectSelector) {
  return {
    '#draft-webhook-config-id': '#draft-webhook-auto-hint',
    '#complex-task-webhook-config-id': '#complex-task-webhook-auto-hint',
    '#job-drawer-webhook-config-id': '#job-drawer-webhook-auto-hint',
    '#job-form-webhook-config-id': '#job-form-webhook-auto-hint'
  }[selectSelector] || '';
}

function renderWebhookReferenceHint(selectSelector, hostSelector) {
  const host = qs(hostSelector);
  if (!host) return;
  const webhookID = qs(selectSelector)?.value.trim() || '';
  host.textContent = getWebhookReferenceHintText(getWebhookCatalogItem(webhookID));
}

function buildWebhookReferenceOptions(selectedID) {
  const options = ['<option value="">不绑定 webhook</option>'];
  (window.__connCenterState.webhooks || []).forEach((item) => {
    const id = item.id || '';
    const selected = id === (selectedID || '') ? 'selected' : '';
    const status = item.enabled === false ? '（已停用）' : '';
    const provider = getWebhookProviderLabel(item.provider);
    const label = item.name ? `${item.name} · ${id} · ${provider}${status}` : `${id} · ${provider}${status}`;
    options.push(`<option value="${escapeHtml(id)}" ${selected}>${escapeHtml(label)}</option>`);
  });
  return options.join('');
}

function syncWebhookReferenceSelectOptions() {
  [
    '#draft-webhook-config-id',
    '#complex-task-webhook-config-id',
    '#job-form-webhook-config-id',
    '#job-drawer-webhook-config-id'
  ].forEach((sel) => {
    const el = qs(sel);
    if (!el) return;
    const selectedID = el.value || '';
    el.innerHTML = buildWebhookReferenceOptions(selectedID);
    el.value = selectedID;
    const hintSelector = getWebhookReferenceHintHostSelector(sel);
    if (hintSelector) renderWebhookReferenceHint(sel, hintSelector);
  });
}

async function ensureWebhookCatalogLoaded() {
  if (window.__webhookCatalogLoaded) {
    return window.__connCenterState.webhooks || [];
  }
  const data = await fetchJSON('/api/v1/settings/webhooks');
  window.__connCenterState.webhooks = data.items || [];
  window.__webhookCatalogLoaded = true;
  syncWebhookReferenceSelectOptions();
  return window.__connCenterState.webhooks;
}

function getWebhookReferencePayload(selectSelector, enabledSelector) {
  const webhookConfigID = qs(selectSelector)?.value.trim() || '';
  return {
    webhook_config_id: webhookConfigID,
    webhook_enabled: webhookConfigID ? (qs(enabledSelector)?.checked === true) : false
  };
}

function fillWebhookReferenceFields(selectSelector, enabledSelector, payload) {
  const webhookConfigID = (payload && payload.webhook_config_id) || '';
  const select = qs(selectSelector);
  if (select) {
    select.innerHTML = buildWebhookReferenceOptions(webhookConfigID);
    select.value = webhookConfigID;
  }
  if (qs(enabledSelector)) {
    qs(enabledSelector).checked = !!(webhookConfigID && payload && payload.webhook_enabled);
  }
  const hintSelector = getWebhookReferenceHintHostSelector(selectSelector);
  if (hintSelector) renderWebhookReferenceHint(selectSelector, hintSelector);
}

function syncWebhookReferenceEnabledState(selectSelector, enabledSelector, opts) {
  const options = opts || {};
  const webhookConfigID = qs(selectSelector)?.value.trim() || '';
  const input = qs(enabledSelector);
  if (!input) return;
  if (!webhookConfigID) {
    input.checked = false;
    return;
  }
  if (options.autoEnable === true && input.checked !== true) {
    input.checked = true;
  }
}

function getLLMAPIPayload() {
  const timeout = qs('#llm-api-timeout')?.value.trim() || '';
  const temperature = qs('#llm-api-temperature')?.value.trim() || '';
  const maxTokens = qs('#llm-api-max-tokens')?.value.trim() || '';
  const responseFormat = qs('#llm-api-response-format')?.value.trim() || '';
  const payload = {
    id: qs('#llm-api-id')?.value.trim() || '',
    name: qs('#llm-api-name')?.value.trim() || '',
    tags: parseTagInput(qs('#llm-api-tags')?.value || ''),
    activate: qs('#llm-api-activate')?.checked === true,
    enabled: qs('#llm-api-enabled')?.value === 'true',
    provider: qs('#llm-api-provider')?.value.trim() || '',
    base_url: qs('#llm-api-base-url')?.value.trim() || '',
    api_key: qs('#llm-api-api-key')?.value.trim() || '',
    model: qs('#llm-api-model')?.value.trim() || '',
    thinking: qs('#llm-api-thinking')?.value === 'true',
    reasoning_effort: qs('#llm-api-reasoning-effort')?.value.trim() || '',
    system_prompt: qs('#llm-api-system-prompt')?.value || ''
  };
  if (timeout) payload.timeout = Number(timeout);
  if (temperature) payload.temperature = Number(temperature);
  if (maxTokens) payload.max_tokens = Number(maxTokens);
  if (responseFormat) payload.response_format = { type: responseFormat };
  return payload;
}

function fillLLMAPIForm(item) {
  const cfg = item || {};
  if (qs('#llm-api-id')) qs('#llm-api-id').value = cfg.id || '';
  if (qs('#llm-api-name')) qs('#llm-api-name').value = cfg.name || '';
  if (qs('#llm-api-tags')) qs('#llm-api-tags').value = Array.isArray(cfg.tags) ? cfg.tags.join(', ') : '';
  if (qs('#llm-api-enabled')) qs('#llm-api-enabled').value = String(cfg.enabled !== false);
  if (qs('#llm-api-provider')) qs('#llm-api-provider').value = cfg.provider || 'deepseek';
  if (qs('#llm-api-base-url')) qs('#llm-api-base-url').value = cfg.base_url || '';
  if (qs('#llm-api-api-key')) qs('#llm-api-api-key').value = '';
  if (qs('#llm-api-model')) qs('#llm-api-model').value = cfg.model || '';
  if (qs('#llm-api-timeout')) qs('#llm-api-timeout').value = cfg.timeout || '';
  if (qs('#llm-api-temperature')) qs('#llm-api-temperature').value = cfg.temperature ?? '';
  if (qs('#llm-api-max-tokens')) qs('#llm-api-max-tokens').value = cfg.max_tokens || '';
  if (qs('#llm-api-thinking')) qs('#llm-api-thinking').value = String(!!cfg.thinking);
  if (qs('#llm-api-reasoning-effort')) qs('#llm-api-reasoning-effort').value = cfg.reasoning_effort || '';
  if (qs('#llm-api-response-format')) qs('#llm-api-response-format').value = ((cfg.response_format || {}).type) || '';
  if (qs('#llm-api-system-prompt')) qs('#llm-api-system-prompt').value = cfg.system_prompt || '';
  if (qs('#llm-api-activate')) qs('#llm-api-activate').checked = !!cfg.active;
}

function currentActiveLLMAPIItem() {
  return (window.__connCenterState.llmAPIs || []).find((it) => it.active) || null;
}

function buildLLMRuntimeFallbackFromConfig(data) {
  const planner = (data || {}).planner || {};
  return {
    id: '',
    name: '',
    tags: [],
    active: true,
    enabled: planner.enabled !== false,
    provider: planner.provider || '',
    base_url: planner.base_url || '',
    model: planner.model || '',
    timeout: planner.timeout || '',
    temperature: planner.temperature ?? '',
    max_tokens: planner.max_tokens || '',
    thinking: !!planner.thinking,
    reasoning_effort: planner.reasoning_effort || '',
    response_format: planner.response_format || {},
    system_prompt: planner.system_prompt || ''
  };
}

function setConnectionWorkbenchMode(type, mode, id) {
  const state = window.__connCenterState;
  state.currentType = type || state.currentType || 'connection';
  const nextMode = mode || (state.currentType === 'llm' ? state.currentLLMMode : state.currentConnectionMode) || 'view';
  state.currentMode = nextMode;
  if (state.currentType === 'connection') {
    state.currentConnectionID = id || state.currentConnectionID || '';
    state.currentConnectionMode = nextMode;
  }
  if (state.currentType === 'llm') {
    state.currentLLMAPIID = id || state.currentLLMAPIID || '';
    state.currentLLMMode = nextMode;
  }
  renderConnectionWorkbench();
}

function renderConnectionWorkbench() {
  const host = qs('#conn-workbench');
  if (!host) return;
  const state = window.__connCenterState;
  const isConnection = state.currentType !== 'llm';
  const isLLM = !isConnection;
  host.dataset.currentType = state.currentType || '';
  host.dataset.currentMode = state.currentMode || '';
  host.dataset.currentConnectionId = state.currentConnectionID || '';
  host.dataset.currentLlmApiId = state.currentLLMAPIID || '';

  if (qs('#conn-workbench-type')) qs('#conn-workbench-type').textContent = isConnection ? '飞连_API' : 'LLM_API';
  if (qs('#conn-workbench-mode')) qs('#conn-workbench-mode').textContent = getWorkbenchModeText(state.currentType, state.currentMode);

  qs('#conn-form-feilian')?.classList.toggle('hidden', !isConnection);
  qs('#conn-form-llm-api')?.classList.toggle('hidden', !isLLM);

  const connReadonly = !isConnection || state.currentMode === 'view';
  setReadonlyFieldState(['#conn-name', '#conn-scheme', '#conn-host', '#conn-port', '#conn-ak', '#conn-sk'], connReadonly);
  if (qs('#btn-conn-load')) qs('#btn-conn-load').disabled = !isConnection;
  if (qs('#btn-conn-test')) qs('#btn-conn-test').disabled = !isConnection;
  if (qs('#btn-conn-save')) {
    qs('#btn-conn-save').disabled = !isConnection || state.currentMode === 'view';
    qs('#btn-conn-save').textContent = state.currentMode === 'edit' ? '保存为新 ACTIVE 快照' : '保存并热更新';
  }
  if (qs('#btn-conn-edit')) {
    qs('#btn-conn-edit').disabled = !isConnection;
    qs('#btn-conn-edit').classList.toggle('hidden', !isConnection || state.currentMode === 'create');
  }

  const llmReadonly = !isLLM || state.currentMode === 'view';
  setReadonlyFieldState([
    '#llm-api-name',
    '#llm-api-tags',
    '#llm-api-provider',
    '#llm-api-base-url',
    '#llm-api-api-key',
    '#llm-api-model',
    '#llm-api-timeout',
    '#llm-api-temperature',
    '#llm-api-max-tokens',
    '#llm-api-reasoning-effort',
    '#llm-api-response-format',
    '#llm-api-system-prompt'
  ], llmReadonly);
  setReadonlyFieldState([
    '#llm-api-enabled',
    '#llm-api-thinking',
    '#llm-api-activate'
  ], llmReadonly);
  setReadonlyFieldState(['#llm-api-id'], !isLLM || state.currentMode !== 'create');
  if (qs('#btn-llm-load')) qs('#btn-llm-load').disabled = !isLLM;
  if (qs('#btn-llm-edit')) {
    qs('#btn-llm-edit').disabled = !isLLM;
    qs('#btn-llm-edit').classList.toggle('hidden', !isLLM || state.currentMode === 'create');
  }
  if (qs('#btn-llm-save')) qs('#btn-llm-save').disabled = !isLLM || state.currentMode === 'view';
  if (qs('#btn-llm-test')) qs('#btn-llm-test').disabled = !isLLM;
}

function renderConnectionCenter() {
  const currentSubpage = window.__connSubpage || 'feilian';
  renderFeilianSubpage();
  renderLLMSubpage();
  renderWebhookSubpage();
  syncConnectionSubpageUI(currentSubpage);
  if (currentSubpage === 'llm') renderConnectionWorkbench();
}

function resolveConnectionSelection(items, activeId) {
  const currentID = window.__connCenterState.currentConnectionID || '';
  if (currentID && items.some((it) => it.id === currentID)) return currentID;
  if (activeId && items.some((it) => it.id === activeId)) return activeId;
  return (items[0] || {}).id || '';
}

function resolveLLMAPISelection(items, activeId) {
  const currentID = window.__connCenterState.currentLLMAPIID || '';
  if (currentID && items.some((it) => it.id === currentID)) return currentID;
  if (activeId && items.some((it) => it.id === activeId)) return activeId;
  return (items[0] || {}).id || '';
}

function resolveWebhookSelection(items) {
  const currentID = window.__connCenterState.currentWebhookID || '';
  if (currentID && items.some((it) => it.id === currentID)) return currentID;
  return (items[0] || {}).id || '';
}

async function loadCurrentConnectionConfig(quiet, preserveWorkbench) {
  try {
    const data = await fetchJSON('/api/v1/connection');
    const selected = currentConnectionItem();
    if (qs('#conn-name')) qs('#conn-name').value = (selected && selected.name) || '';
    qs('#conn-scheme').value = data.scheme || '';
    qs('#conn-host').value = data.host || '';
    qs('#conn-port').value = data.port || '';
    qs('#conn-ak').value = data.access_key || '';
    qs('#conn-sk').value = '';
    qs('#conn-out').textContent = JSON.stringify(data, null, 2);
    if (!preserveWorkbench) setConnectionWorkbenchMode('connection', 'view', window.__connCenterState.currentConnectionID || '');
    if (!quiet) toast('连接配置', '已加载当前 ACTIVE 配置', 'ok');
    return data;
  } catch (e) {
    qs('#conn-out').textContent = String(e);
    if (!quiet) toast('连接配置', String(e), '');
    throw e;
  }
}

async function openConnectionRecord(item) {
  if (!item) return;
  setView('connection');
  setConnectionSubpage('feilian');
  window.__connCenterState.currentConnectionID = item.id || '';
  if (qs('#conn-name')) qs('#conn-name').value = item.name || '';
  qs('#conn-scheme').value = item.scheme || '';
  qs('#conn-host').value = item.host || '';
  qs('#conn-port').value = item.port || '';
  qs('#conn-sk').value = '';
  setConnectionWorkbenchMode('connection', 'view', item.id || '');
  if (item.active) {
    await loadCurrentConnectionConfig(true, true);
    qs('#conn-out').textContent = pretty(Object.assign({}, item, {
      note: '当前 ACTIVE 配置已同步加载到工作台，secret 不会回显。点击“切换编辑态”后可修改并保存为新快照。'
    }));
  } else {
    qs('#conn-ak').value = '';
    qs('#conn-out').textContent = pretty(Object.assign({}, item, {
      note: '历史记录仅回填基础连接信息；鉴权字段保持空白，避免将脱敏值误保存回配置。'
    }));
  }
  renderConnectionCenter();
  toast('飞连_API', `已打开 ${item.name || item.id || '连接记录'}`, 'ok');
}

function renderFeilianSubpage() {
  renderFeilianConfigList(window.__connCenterState.connList || []);
  if ((window.__connSubpage || 'feilian') === 'feilian') {
    setConnectionWorkbenchMode('connection', getConnectionWorkbenchMode('connection'), window.__connCenterState.currentConnectionID || '');
  }
}

function openFeilianCreateMode() {
  setView('connection');
  setConnectionSubpage('feilian');
  window.__connCenterState.currentConnectionID = '';
  if (qs('#conn-name')) qs('#conn-name').value = '';
  qs('#conn-scheme').value = 'https';
  qs('#conn-host').value = '';
  qs('#conn-port').value = '443';
  qs('#conn-ak').value = '';
  qs('#conn-sk').value = '';
  qs('#conn-out').textContent = pretty({
    mode: 'create',
    type: 'connection',
    hint: '请填写新的飞连_API 连接信息，然后点击“保存并热更新”。'
  });
  setConnectionWorkbenchMode('connection', 'create', '');
  renderConnectionCenter();
}

function openConnectionCreateMode() {
  openFeilianCreateMode();
}

function openConnectionEditMode() {
  setView('connection');
  setConnectionSubpage('feilian');
  setConnectionWorkbenchMode('connection', 'edit', window.__connCenterState.currentConnectionID || '');
  toast('飞连_API', '已切换到编辑态', 'ok');
}

async function refreshConnections() {
  try {
    const data = await fetchJSON('/api/v1/connections');
    const items = data.items || [];
    window.__connCenterState.connList = items;
    window.__connCenterState.currentConnectionID = resolveConnectionSelection(items, data.active_id || '');
    renderConnectionCenter();
  } catch (e) {
    toast('连接历史', String(e), '');
  }
}

function renderFeilianConfigList(items) {
  const host = qs('#conn-list-feilian');
  if (!host) return;
  if (!items.length) {
    host.className = 'subtitle';
    host.innerHTML = '<div class="subtitle">暂无飞连_API 配置</div>';
    return;
  }
  const currentID = window.__connCenterState.currentConnectionID || '';
  host.className = 'api-config-list';
  host.innerHTML = items.map((it) => `
    <div class="api-config-item ${it.active ? 'api-config-item-active' : ''} ${currentID === it.id ? 'api-config-item-current' : ''}" data-id="${escapeHtml(it.id || '')}">
      <div style="font-weight:750">${escapeHtml(it.name || it.id || '')} ${it.active ? '<span class="pill ok">ACTIVE</span>' : ''}</div>
      <div class="subtitle"><code>${escapeHtml(`${it.scheme || 'https'}://${it.host || ''}:${it.port || ''}`)}</code></div>
      <div class="subtitle">AK: <code>${escapeHtml(it.access_key_id || '')}</code></div>
      <div class="api-task-actions">
        <button class="btn" data-act="open" data-id="${escapeHtml(it.id || '')}">查看</button>
        <button class="btn" data-act="activate" data-id="${escapeHtml(it.id || '')}" ${(it.active || !it.id) ? 'disabled' : ''}>设为 ACTIVE</button>
        <button class="btn danger" data-act="delete" data-id="${escapeHtml(it.id || '')}" ${(it.active || !it.id) ? 'disabled' : ''}>删除</button>
      </div>
    </div>
  `).join('');
  qsa('#conn-list-feilian [data-act="open"]').forEach((btn) => btn.addEventListener('click', async () => {
    const item = (window.__connCenterState.connList || []).find((it) => it.id === (btn.dataset.id || ''));
    await openConnectionRecord(item);
  }));
  qsa('#conn-list-feilian [data-act="activate"]').forEach((btn) => btn.addEventListener('click', async () => {
    const id = btn.dataset.id || '';
    if (!id) return;
    try {
      await fetchJSON(`/api/v1/connections/${encodeURIComponent(id)}/activate`, { method: 'POST', body: '{}' });
      window.__connCenterState.currentConnectionID = id;
      await refreshConnections();
      await loadCurrentConnectionConfig(true, true);
      setConnectionWorkbenchMode('connection', 'view', id);
      toast('飞连_API', `已切换 ACTIVE: ${id}`, 'ok');
    } catch (e) {
      toast('切换失败', String(e), '');
    }
  }));
  qsa('#conn-list-feilian [data-act="delete"]').forEach((btn) => btn.addEventListener('click', async () => {
    const id = btn.dataset.id || '';
    if (!id || !confirm(`确认删除飞连_API 配置 ${id}？`)) return;
    try {
      await fetchJSON(`/api/v1/connections/${encodeURIComponent(id)}`, { method: 'DELETE' });
      if (window.__connCenterState.currentConnectionID === id) {
        window.__connCenterState.currentConnectionID = '';
      }
      await refreshConnections();
      toast('飞连_API', `已删除 ${id}`, 'ok');
    } catch (e) {
      toast('删除失败', String(e), '');
    }
  }));
}

qs('#btn-conn-load')?.addEventListener('click', async () => {
  try {
    await loadCurrentConnectionConfig(false);
    await refreshConnections();
  } catch {}
});

qs('#btn-conn-edit')?.addEventListener('click', () => openConnectionEditMode());

qs('#btn-conn-test')?.addEventListener('click', async () => {
  try {
    const payload = await getConnPayload();
    const data = await fetchJSON('/api/v1/connection/test', { method: 'POST', body: JSON.stringify(payload) });
    qs('#conn-out').textContent = JSON.stringify(data, null, 2);
    qs('#modal-token').textContent = JSON.stringify({
      token_ok: data.token_ok,
      token_preview: data.token_preview,
      token_expires_in: data.token_expires_in,
      token_error: data.token_error || '',
      token_request_preview: data.token_request_preview || null
    }, null, 2);
    qs('#modal-probe').textContent = JSON.stringify({
      probe_ok: data.probe_ok,
      probe_error: data.probe_error || '',
      probe_result: data.probe_result || null
    }, null, 2);
    openModal();
    toast('测试连接', data.token_ok ? 'Token 获取成功' : 'Token 获取失败', data.token_ok ? 'ok' : '');
  } catch (e) {
    qs('#conn-out').textContent = String(e);
    toast('测试连接', String(e), '');
  }
});

qs('#btn-conn-save')?.addEventListener('click', async () => {
  try {
    const payload = await getConnPayload();
    const data = await fetchJSON('/api/v1/connections', { method: 'POST', body: JSON.stringify(payload) });
    qs('#conn-out').textContent = JSON.stringify(data, null, 2);
    qs('#conn-sk').value = '';
    window.__connCenterState.currentConnectionID = data.id || '';
    await refreshConnections();
    await loadCurrentConnectionConfig(true, true);
    setConnectionWorkbenchMode('connection', 'view', data.id || '');
    toast('保存成功', '已写入 config.yaml 并热更新', 'ok');
  } catch (e) {
    qs('#conn-out').textContent = String(e);
    toast('保存失败', String(e), '');
  }
});

qs('#btn-conn-refresh')?.addEventListener('click', () => refreshConnections());
qs('#btn-conn-create')?.addEventListener('click', () => openFeilianCreateMode());
qs('#btn-conn-create-inline')?.addEventListener('click', () => openFeilianCreateMode());
qs('#btn-sub-feilian')?.addEventListener('click', () => setConnectionSubpage('feilian'));
qs('#btn-sub-llm')?.addEventListener('click', () => setConnectionSubpage('llm'));
qs('#btn-sub-webhook')?.addEventListener('click', () => setConnectionSubpage('webhook'));

async function loadCurrentLLMConfig(quiet, preserveWorkbench) {
  try {
    const data = await fetchJSON('/api/v1/settings/llm');
    const selected = currentLLMAPIItem() || currentActiveLLMAPIItem() || buildLLMRuntimeFallbackFromConfig(data);
    fillLLMAPIForm(selected);
    qs('#llm-out').textContent = JSON.stringify(data, null, 2);
    if (!preserveWorkbench) setConnectionWorkbenchMode('llm', 'view', window.__connCenterState.currentLLMAPIID || '');
    if (!quiet) toast('LLM 配置', '已加载当前 ACTIVE 配置', 'ok');
    return data;
  } catch (e) {
    qs('#llm-out').textContent = String(e);
    if (!quiet) toast('LLM 配置', String(e), '');
    throw e;
  }
}

async function openLLMAPIRecord(item) {
  if (!item) return;
  setView('connection');
  setConnectionSubpage('llm');
  window.__connCenterState.currentLLMAPIID = item.id || '';
  fillLLMAPIForm(item);
  setConnectionWorkbenchMode('llm', 'view', item.id || '');
  if (item.active) {
    await loadCurrentLLMConfig(true, true);
    qs('#llm-out').textContent = pretty(Object.assign({}, item, {
      note: '当前 ACTIVE LLM_API 已同步加载到工作台，api_key 不会回显。点击“切换编辑态”后可修改并保存。'
    }));
  } else {
    qs('#llm-out').textContent = pretty(Object.assign({}, item, {
      note: '查看态不回填 api_key；如需重新启用，请切换编辑态补全密钥后保存。'
    }));
  }
  renderConnectionCenter();
  toast('LLM_API', `已打开 ${item.name || item.id || '配置'}`, 'ok');
}

function renderLLMSubpage() {
  renderLLMAPIList(window.__connCenterState.llmAPIs || []);
  if ((window.__connSubpage || 'feilian') === 'llm') {
    setConnectionWorkbenchMode('llm', getConnectionWorkbenchMode('llm'), window.__connCenterState.currentLLMAPIID || '');
  }
}

function openLLMAPICreateMode() {
  setView('connection');
  setConnectionSubpage('llm');
  window.__connCenterState.currentLLMAPIID = '';
  fillLLMAPIForm({
    id: '',
    name: '',
    tags: [],
    active: true,
    enabled: true,
    provider: 'deepseek',
    base_url: '',
    model: '',
    thinking: false,
    response_format: {}
  });
  qs('#llm-out').textContent = pretty({
    mode: 'create',
    type: 'llm',
    hint: '请填写 LLM_API ID / 名称 / 标签，并配置 provider、base_url、api_key、model 等字段后保存。'
  });
  setConnectionWorkbenchMode('llm', 'create', '');
  renderConnectionCenter();
}

function openLLMAPIEditMode() {
  setView('connection');
  setConnectionSubpage('llm');
  setConnectionWorkbenchMode('llm', 'edit', window.__connCenterState.currentLLMAPIID || '');
  toast('LLM_API', '已切换到编辑态', 'ok');
}

async function refreshLLMAPIs() {
  try {
    const data = await fetchJSON('/api/v1/settings/llm/apis');
    const items = data.items || [];
    window.__connCenterState.llmAPIs = items;
    window.__llmAPIItems = Array.isArray(items) ? items : [];
    window.__connCenterState.currentLLMAPIID = resolveLLMAPISelection(items, data.active_id || '');
    if (qs('#draft-llm-api-id')) {
      const selected = qs('#draft-llm-api-id').value || '';
      qs('#draft-llm-api-id').innerHTML = buildDraftLLMAPIOptions(selected);
      renderDraftLLMAPIHint(selected);
    }
    renderConnectionCenter();
  } catch (e) {
    toast('LLM_API', String(e), '');
  }
}

function renderLLMAPIList(items) {
  const host = qs('#conn-list-llm');
  if (!host) return;
  if (!items.length) {
    host.className = 'subtitle';
    host.innerHTML = '<div class="subtitle">暂无 LLM_API 配置</div>';
    return;
  }
  const currentID = window.__connCenterState.currentLLMAPIID || '';
  host.className = 'api-config-list';
  host.innerHTML = items.map((it) => `
    <div class="api-config-item ${it.active ? 'api-config-item-active' : ''} ${currentID === it.id ? 'api-config-item-current' : ''}" data-id="${escapeHtml(it.id || '')}">
      <div style="font-weight:750">${escapeHtml(it.name || it.id || '')} ${it.active ? '<span class="pill ok">ACTIVE</span>' : ''}</div>
      <div class="subtitle">${escapeHtml(it.provider || '')} / ${escapeHtml(it.model || '')}</div>
      <div class="subtitle">${(Array.isArray(it.tags) && it.tags.length) ? it.tags.map((tag) => `<span class="pill">${escapeHtml(tag)}</span>`).join(' ') : '无标签'}</div>
      <div class="api-task-actions">
        <button class="btn" data-act="open-llm" data-id="${escapeHtml(it.id || '')}">查看</button>
        <button class="btn" data-act="activate-llm" data-id="${escapeHtml(it.id || '')}" ${(it.active || !it.id) ? 'disabled' : ''}>设为 ACTIVE</button>
        <button class="btn danger" data-act="delete-llm" data-id="${escapeHtml(it.id || '')}" ${(it.active || !it.id) ? 'disabled' : ''}>删除</button>
      </div>
    </div>
  `).join('');
  qsa('#conn-list-llm [data-act="open-llm"]').forEach((btn) => btn.addEventListener('click', async () => {
    const item = (window.__connCenterState.llmAPIs || []).find((it) => it.id === (btn.dataset.id || ''));
    await openLLMAPIRecord(item);
  }));
  qsa('#conn-list-llm [data-act="activate-llm"]').forEach((btn) => btn.addEventListener('click', async () => {
    const id = btn.dataset.id || '';
    if (!id) return;
    try {
      await fetchJSON(`/api/v1/settings/llm/apis/${encodeURIComponent(id)}/activate`, { method: 'POST', body: '{}' });
      window.__connCenterState.currentLLMAPIID = id;
      await refreshLLMAPIs();
      await loadCurrentLLMConfig(true, true);
      setConnectionWorkbenchMode('llm', 'view', id);
      toast('LLM_API', `已切换 ACTIVE: ${id}`, 'ok');
    } catch (e) {
      toast('LLM_API 切换失败', String(e), '');
    }
  }));
  qsa('#conn-list-llm [data-act="delete-llm"]').forEach((btn) => btn.addEventListener('click', async () => {
    const id = btn.dataset.id || '';
    if (!id || !confirm(`确认删除 LLM_API 配置 ${id}？`)) return;
    try {
      await fetchJSON(`/api/v1/settings/llm/apis/${encodeURIComponent(id)}`, { method: 'DELETE' });
      if (window.__connCenterState.currentLLMAPIID === id) {
        window.__connCenterState.currentLLMAPIID = '';
      }
      await refreshLLMAPIs();
      toast('LLM_API', `已删除 ${id}`, 'ok');
    } catch (e) {
      toast('LLM_API 删除失败', String(e), '');
    }
  }));
}

qs('#btn-llm-load')?.addEventListener('click', async () => {
  try {
    await loadCurrentLLMConfig(false);
  } catch {}
});

qs('#btn-llm-edit')?.addEventListener('click', () => openLLMAPIEditMode());

qs('#btn-llm-test')?.addEventListener('click', async () => {
  try {
    const payload = getLLMAPIPayload();
    const data = await fetchJSON('/api/v1/settings/llm/test', {
      method: 'POST',
      body: JSON.stringify({
        role: 'planner',
        enabled: payload.enabled,
        provider: payload.provider,
        base_url: payload.base_url,
        api_key: payload.api_key,
        model: payload.model,
        timeout: payload.timeout,
        temperature: payload.temperature,
        max_tokens: payload.max_tokens,
        thinking: payload.thinking,
        reasoning_effort: payload.reasoning_effort,
        response_format: payload.response_format,
        system_prompt: payload.system_prompt
      })
    });
    qs('#llm-out').textContent = JSON.stringify(data, null, 2);
    toast('LLM 测试', data.ok ? 'LLM_API 测试成功' : 'LLM_API 测试失败', data.ok ? 'ok' : '');
  } catch (e) {
    qs('#llm-out').textContent = String(e);
    toast('LLM 测试', String(e), '');
  }
});

qs('#btn-llm-save')?.addEventListener('click', async () => {
  try {
    const payload = getLLMAPIPayload();
    const data = await fetchJSON('/api/v1/settings/llm/apis', { method: 'POST', body: JSON.stringify(payload) });
    qs('#llm-api-api-key').value = '';
    window.__connCenterState.currentLLMAPIID = data.id || payload.id || '';
    await refreshLLMAPIs();
    if (payload.activate) {
      await loadCurrentLLMConfig(true, true);
    }
    const current = (window.__connCenterState.llmAPIs || []).find((it) => it.id === (data.id || payload.id || ''));
    if (current) fillLLMAPIForm(current);
    qs('#llm-out').textContent = pretty({
      ok: true,
      id: data.id || payload.id,
      active: !!payload.activate,
      tags: payload.tags,
      note: payload.activate ? 'LLM_API 已保存并切换为 ACTIVE。' : 'LLM_API 已保存，可稍后再切换 ACTIVE。'
    });
    setConnectionWorkbenchMode('llm', 'view', data.id || payload.id || '');
    toast('LLM 配置', payload.activate ? 'LLM_API 已保存并热更新' : 'LLM_API 已保存', 'ok');
  } catch (e) {
    qs('#llm-out').textContent = String(e);
    toast('LLM 保存失败', String(e), '');
  }
});

qs('#btn-llm-api-refresh')?.addEventListener('click', () => refreshLLMAPIs());
qs('#btn-llm-api-create')?.addEventListener('click', () => openLLMAPICreateMode());
qs('#btn-llm-api-create-inline')?.addEventListener('click', () => openLLMAPICreateMode());

function defaultWebhookHeaders() {
  return { 'Content-Type': 'application/json' };
}

function defaultWebhookBodyTemplate() {
  return '{\n  "title": "{{source_name}}",\n  "message": "{{jsonString data}}"\n}';
}

function defaultWebhookTestPayload(item) {
  return {
    title: `${item && (item.name || item.id) ? (item.name || item.id) : 'webhook'} 测试推送`,
    message: 'hello webhook',
    webhook_id: (item && item.id) || ''
  };
}

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

function setWebhookFormMode(mode, id) {
  window.__connCenterState.currentWebhookMode = mode || 'create';
  window.__connCenterState.currentWebhookID = id || '';
  const isCreate = window.__connCenterState.currentWebhookMode === 'create';
  setReadonlyFieldState(['#webhook-id'], !isCreate);
  if (qs('#btn-webhook-delete')) {
    qs('#btn-webhook-delete').disabled = !(window.__connCenterState.currentWebhookID || qs('#webhook-id')?.value.trim());
  }
}

function fillWebhookForm(item, opts) {
  const options = opts || {};
  const cfg = Object.assign({}, buildDefaultWebhookDraft(), item || {});
  if (qs('#webhook-id')) qs('#webhook-id').value = cfg.id || '';
  if (qs('#webhook-name')) qs('#webhook-name').value = cfg.name || '';
  if (qs('#webhook-url')) qs('#webhook-url').value = cfg.url || '';
  if (qs('#webhook-provider')) qs('#webhook-provider').value = getWebhookProviderValue(cfg.provider);
  if (qs('#webhook-method')) qs('#webhook-method').value = (cfg.method || 'POST').toUpperCase();
  if (qs('#webhook-headers')) qs('#webhook-headers').value = pretty(cfg.headers || defaultWebhookHeaders());
  if (qs('#webhook-body-template')) qs('#webhook-body-template').value = cfg.body_template || defaultWebhookBodyTemplate();
  if (qs('#webhook-test-payload')) {
    const keepCurrent = options.preserveTestPayload && qs('#webhook-test-payload').value.trim();
    qs('#webhook-test-payload').value = keepCurrent
      ? qs('#webhook-test-payload').value
      : pretty(defaultWebhookTestPayload(cfg));
  }
  renderWebhookProviderState();
}

function getWebhookPayload() {
  const current = currentWebhookItem();
  return {
    id: qs('#webhook-id')?.value.trim() || '',
    name: qs('#webhook-name')?.value.trim() || '',
    url: qs('#webhook-url')?.value.trim() || '',
    method: (qs('#webhook-method')?.value || 'POST').toUpperCase(),
    provider: getWebhookProviderValue(qs('#webhook-provider')?.value || 'generic'),
    headers: parseJSONObjectSafe(qs('#webhook-headers')?.value || '{}', 'webhook headers', {}),
    body_template: qs('#webhook-body-template')?.value || '',
    enabled: current ? current.enabled !== false : true
  };
}

function renderWebhookProviderState() {
  const provider = getWebhookProviderValue(qs('#webhook-provider')?.value || 'generic');
  const isFeishu = provider === 'feishu_bot';
  const bodyField = qs('#webhook-body-template');
  const hint = qs('#webhook-provider-hint');
  if (bodyField) {
    bodyField.readOnly = isFeishu;
    bodyField.setAttribute('aria-readonly', isFeishu ? 'true' : 'false');
    bodyField.placeholder = isFeishu
      ? '{"msg_type":"text","content":{"text":"系统自动生成"}}'
      : '{"title":"{{source_name}}","message":"{{jsonString data}}"}';
  }
  if (hint) {
    hint.textContent = isFeishu
      ? '系统将自动按飞书机器人格式生成 payload，无需手工填写 body_template。'
      : '通用 webhook 使用自定义 body_template。';
  }
}

function renderWebhookSubpage() {
  renderWebhookList(window.__connCenterState.webhooks || []);
  const current = currentWebhookItem();
  if (current) {
    fillWebhookForm(current, { preserveTestPayload: true });
    setWebhookFormMode('update', current.id || '');
    return;
  }
  if (!(window.__connCenterState.webhooks || []).length) {
    setWebhookFormMode('create', '');
    if (!(qs('#webhook-id')?.value || '').trim()) {
      fillWebhookForm(buildDefaultWebhookDraft(), { preserveTestPayload: true });
    }
    return;
  }
  setWebhookFormMode(window.__connCenterState.currentWebhookMode || 'create', window.__connCenterState.currentWebhookID || '');
}

function openWebhookCreateMode(quiet) {
  setWebhookFormMode('create', '');
  fillWebhookForm(buildDefaultWebhookDraft());
  setView('connection');
  setConnectionSubpage('webhook');
  qs('#webhook-out').textContent = pretty({
    mode: 'create',
    type: 'webhook',
    hint: '请填写 webhook-id、name、url、provider、method、headers；generic 可自定义 body-template，feishu_bot 会自动生成 payload。'
  });
  if (!quiet) toast('webhook', '已切换到新建态', 'ok');
}

function openWebhookRecord(item, quiet) {
  if (!item) return;
  setWebhookFormMode('update', item.id || '');
  fillWebhookForm(item);
  setView('connection');
  setConnectionSubpage('webhook');
  qs('#webhook-out').textContent = pretty(Object.assign({}, item, {
    note: '可直接在右侧修改并保存；测试推送会使用当前表单中的 URL / provider / method / headers / test-payload。'
  }));
  if (!quiet) toast('webhook', `已打开 ${item.name || item.id || '配置'}`, 'ok');
}

async function refreshWebhooks(quiet) {
  try {
    const data = await fetchJSON('/api/v1/settings/webhooks');
    const items = data.items || [];
    window.__connCenterState.webhooks = items;
    window.__webhookCatalogLoaded = true;
    syncWebhookReferenceSelectOptions();
    const nextID = resolveWebhookSelection(items);
    if (nextID) {
      const current = items.find((it) => it.id === nextID) || null;
      setWebhookFormMode('update', nextID);
      if (current) fillWebhookForm(current, { preserveTestPayload: true });
    } else {
      setWebhookFormMode('create', '');
      fillWebhookForm(buildDefaultWebhookDraft(), { preserveTestPayload: true });
    }
    renderWebhookSubpage();
    if (!quiet) toast('webhook', '已刷新 webhook 列表', 'ok');
  } catch (e) {
    if (qs('#webhook-out')) qs('#webhook-out').textContent = String(e);
    toast('webhook', String(e), '');
  }
}

function renderWebhookList(items) {
  const host = qs('#webhook-list');
  if (!host) return;
  if (!items.length) {
    host.className = 'config-record-list subtitle';
    host.innerHTML = '<div class="subtitle">暂无 webhook 配置，点击“新增”创建第一条记录。</div>';
    return;
  }
  const currentID = window.__connCenterState.currentWebhookID || '';
  host.className = 'config-record-list';
  host.innerHTML = items.map((it) => `
    <div class="config-record-item ${currentID === it.id ? 'config-record-item-active' : ''}" data-webhook-id="${escapeHtml(it.id || '')}">
      <div style="display:flex;justify-content:space-between;gap:12px;align-items:flex-start">
        <div style="min-width:0">
          <div style="font-weight:750">${escapeHtml(it.name || it.id || '')}</div>
          <div class="subtitle"><code>${escapeHtml(it.id || '')}</code></div>
        </div>
        <span class="pill ${it.enabled === false ? '' : 'ok'}">${it.enabled === false ? 'DISABLED' : 'ENABLED'}</span>
      </div>
      <div class="subtitle" style="margin-top:8px"><span class="pill">${escapeHtml((it.method || 'POST').toUpperCase())}</span> <code>${escapeHtml(it.url || '')}</code></div>
      <div class="subtitle"><span class="pill">${escapeHtml(getWebhookProviderLabel(it.provider))}</span> headers: ${Object.keys(it.headers || {}).length} · body-template: ${(it.body_template || '').trim() ? '已配置' : '空'}</div>
      <div class="api-task-actions">
        <button class="btn" data-act="open-webhook" data-id="${escapeHtml(it.id || '')}">查看</button>
      </div>
    </div>
  `).join('');
  qsa('#webhook-list [data-webhook-id]').forEach((card) => card.addEventListener('click', async (evt) => {
    if (evt.target.closest('button')) return;
    const item = (window.__connCenterState.webhooks || []).find((it) => it.id === (card.dataset.webhookId || ''));
    openWebhookRecord(item, true);
  }));
  qsa('#webhook-list [data-act="open-webhook"]').forEach((btn) => btn.addEventListener('click', async () => {
    const item = (window.__connCenterState.webhooks || []).find((it) => it.id === (btn.dataset.id || ''));
    openWebhookRecord(item, false);
  }));
}

qs('#btn-webhook-refresh')?.addEventListener('click', () => refreshWebhooks(false));
qs('#btn-webhook-create')?.addEventListener('click', () => openWebhookCreateMode(false));
qs('#webhook-provider')?.addEventListener('change', () => renderWebhookProviderState());
qs('#btn-webhook-save')?.addEventListener('click', async () => {
  try {
    const payload = getWebhookPayload();
    const data = await fetchJSON('/api/v1/settings/webhooks', {
      method: 'POST',
      body: JSON.stringify(payload)
    });
    setWebhookFormMode('update', data.id || payload.id || '');
    await refreshWebhooks(true);
    qs('#webhook-out').textContent = pretty({
      ok: true,
      action: 'save',
      id: data.id || payload.id || '',
      url: payload.url,
      provider: payload.provider,
      method: payload.method,
      headers: payload.headers,
      body_template: payload.body_template
    });
    toast('webhook', '配置已保存', 'ok');
  } catch (e) {
    if (qs('#webhook-out')) qs('#webhook-out').textContent = String(e);
    toast('webhook 保存失败', String(e), '');
  }
});

qs('#btn-webhook-delete')?.addEventListener('click', async () => {
  const id = window.__connCenterState.currentWebhookID || qs('#webhook-id')?.value.trim() || '';
  if (!id) {
    toast('webhook 删除失败', '请先从左侧列表选择一条 webhook 配置', '');
    return;
  }
  if (!confirm(`确认删除 webhook 配置 ${id}？`)) return;
  try {
    await fetchJSON(`/api/v1/settings/webhooks/${encodeURIComponent(id)}`, { method: 'DELETE' });
    setWebhookFormMode('create', '');
    fillWebhookForm(buildDefaultWebhookDraft());
    qs('#webhook-out').textContent = pretty({ ok: true, action: 'delete', id });
    await refreshWebhooks(true);
    toast('webhook', `已删除 ${id}`, 'ok');
  } catch (e) {
    if (qs('#webhook-out')) qs('#webhook-out').textContent = String(e);
    toast('webhook 删除失败', String(e), '');
  }
});

qs('#btn-webhook-test')?.addEventListener('click', async () => {
  try {
    const payload = parseJSONValueSafe(qs('#webhook-test-payload')?.value || '{}', 'webhook test-payload', {});
    const data = await fetchJSON('/api/v1/settings/webhooks/test', {
      method: 'POST',
      body: JSON.stringify({
        url: qs('#webhook-url')?.value.trim() || '',
        provider: getWebhookProviderValue(qs('#webhook-provider')?.value || 'generic'),
        method: (qs('#webhook-method')?.value || 'POST').toUpperCase(),
        headers: parseJSONObjectSafe(qs('#webhook-headers')?.value || '{}', 'webhook headers', {}),
        body_template: qs('#webhook-body-template')?.value || '',
        payload
      })
    });
    qs('#webhook-out').textContent = pretty({
      ok: data.ok,
      provider: data.provider || getWebhookProviderValue(qs('#webhook-provider')?.value || 'generic'),
      status_code: data.status_code,
      response_body: data.response_body,
      request_preview: data.request_preview,
      error: data.error || ''
    });
    toast('webhook 测试', data.ok ? '测试推送完成' : '测试推送返回非成功状态', data.ok ? 'ok' : '');
  } catch (e) {
    if (qs('#webhook-out')) qs('#webhook-out').textContent = String(e);
    toast('webhook 测试失败', String(e), '');
  }
});

qs('#modal-close')?.addEventListener('click', closeModal);
qs('#modal-backdrop')?.addEventListener('click', closeModal);

window.__taskDraftCache = [];
window.__currentDraftID = '';
window.__llmAPIItems = [];
window.__jobsCache = [];
window.__jobRunCache = {};
window.__currentJobName = '';
window.__jobEditorMode = 'form';
window.__jobEditorIsCreate = false;
window.__jobEditorOriginalName = '';
window.__jobDrawerIsCreate = false;
window.__jobDrawerOriginalID = '';
window.__complexTaskCache = [];
window.__currentComplexTaskID = '';
window.__currentComplexTask = createEmptyComplexTask();
window.__currentComplexStepID = '';
window.__currentComplexRunPayload = null;
window.__outputTemplateCache = [];
window.__pendingJobDraft = null;

function buildDraftLLMAPIOptions(selectedID) {
  const items = Array.isArray(window.__llmAPIItems) ? window.__llmAPIItems : [];
  const parts = ['<option value="">使用 role 的默认模型</option>'];
  const hasSelected = !!selectedID && items.some((item) => (item.id || '') === selectedID);
  if (selectedID && !hasSelected) {
    parts.push(`<option value="${escapeHtml(selectedID)}" selected>${escapeHtml(`${selectedID} · 已删除`)}</option>`);
  }
  items.forEach((item) => {
    const id = item.id || '';
    const disabled = item.enabled === false ? ' · 已禁用' : '';
    const active = item.active ? ' · ACTIVE' : '';
    const label = `${item.name || id} · ${item.provider || '-'} / ${item.model || '-'}${disabled}${active}`;
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
    host.textContent = '当前草稿引用的模型已删除，请重新选择可用的 LLM_API。';
    return;
  }
  if (item.enabled === false) {
    host.textContent = `当前草稿引用的模型已禁用：${item.provider || '-'} / ${item.model || '-'}，执行时会返回错误，请重新选择。`;
    return;
  }
  host.textContent = `${item.provider || '-'} / ${item.model || '-'}${item.active ? ' · ACTIVE' : ''}`;
}

function splitDraftLLMConfigSelection(llmConfig) {
  const next = cloneValue(llmConfig || {});
  const llm_api_id = typeof next.llm_api_id === 'string' ? next.llm_api_id.trim() : '';
  delete next.llm_api_id;
  return {
    llm_api_id,
    llm_config: next
  };
}

function readTaskDraftLLMConfig() {
  const llm = parseJSONOrEmpty(qs('#draft-llm')?.value || '{}');
  const llm_api_id = qs('#draft-llm-api-id')?.value || '';
  if (llm_api_id) llm.llm_api_id = llm_api_id;
  else delete llm.llm_api_id;
  return llm;
}

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

async function refreshJobs(quiet) {
  try {
    const data = await fetchJSON('/api/v1/job-schedules');
    qs('#jobs').textContent = JSON.stringify(data, null, 2);
    window.__jobsCache = data.items || [];
    window.__jobRunCache = data.schedule_status || {};
    renderJobsTable();
    if (!quiet) toast('定时任务清单', '已刷新任务列表', 'ok');
  } catch (e) {
    toast('定时任务清单', String(e), '');
  }
}

qs('#btn-jobs').addEventListener('click', async () => refreshJobs(false));
qs('#btn-reload').addEventListener('click', async () => {
  try {
    const data = await fetchJSON('/api/v1/reload', { method: 'POST', body: '{}' });
    qs('#jobs').textContent = JSON.stringify(data, null, 2);
    toast('Reload', '已触发 Reload', 'ok');
    refreshJobs(true);
  } catch (e) {
    toast('Reload', String(e), '');
  }
});
qs('#jobs-search')?.addEventListener('input', () => renderJobsTable());
qs('#jobs-filter-type')?.addEventListener('change', () => renderJobsTable());
qs('#jobs-filter-enabled')?.addEventListener('change', () => renderJobsTable());
qs('#btn-job-create')?.addEventListener('click', () => {
  openCreateJobEditor();
});
qs('#btn-job-editor-form')?.addEventListener('click', () => switchJobEditorMode('form'));
qs('#btn-job-editor-json-btn')?.addEventListener('click', () => switchJobEditorMode('json'));
qs('#btn-job-editor-back')?.addEventListener('click', () => {
  setView('tasks');
  switchTaskSubpage('schedule');
  if (window.__currentJobName) loadJobDetail(window.__currentJobName);
});
qs('#btn-job-editor-save')?.addEventListener('click', saveAdvancedJobEditor);
qs('#job-drawer-close')?.addEventListener('click', closeJobDrawer);
qs('#job-drawer-backdrop')?.addEventListener('click', closeJobDrawer);
qs('#btn-draft-save')?.addEventListener('click', saveTaskDraft);
qs('#draft-id')?.addEventListener('input', () => setDraftFieldError('draft-id', ''));
qs('#draft-name')?.addEventListener('input', () => setDraftFieldError('draft-name', ''));
qs('#draft-cycle-mode')?.addEventListener('change', () => syncTaskDraftCycleFields());
qs('#draft-llm-api-id')?.addEventListener('change', () => {
  renderDraftLLMAPIHint(qs('#draft-llm-api-id').value || '');
});
qs('#btn-api-tasks-refresh')?.addEventListener('click', () => refreshTaskDrafts(false));
qs('#api-task-filter')?.addEventListener('input', () => renderAPITaskList());
qs('#btn-complex-create')?.addEventListener('click', () => {
  window.__currentComplexTaskID = '';
  window.__currentComplexStepID = '';
  window.__currentComplexTask = createEmptyComplexTask();
  renderComplexTaskEditor(window.__currentComplexTask);
  toast('运营agent', '已创建空白运营agent 草稿', 'ok');
});
qs('#btn-step-api')?.addEventListener('click', () => appendComplexTaskStep('api_call'));
qs('#btn-step-transform')?.addEventListener('click', () => appendComplexTaskStep('data_transform'));
qs('#btn-step-llm')?.addEventListener('click', () => appendComplexTaskStep('llm_inference'));
qs('#btn-step-output')?.addEventListener('click', () => appendComplexTaskStep('output'));
qs('#btn-complex-run')?.addEventListener('click', runComplexTask);
qs('#btn-complex-save-draft')?.addEventListener('click', saveComplexTaskAsDraft);
qs('#btn-complex-save-output-template')?.addEventListener('click', saveComplexRunAsOutputTemplate);
qs('#btn-output-templates-refresh')?.addEventListener('click', () => refreshOutputTemplates(false));
qs('#btn-complex-save')?.addEventListener('click', saveComplexTask);
qs('#btn-complex-delete')?.addEventListener('click', deleteCurrentComplexTask);
qs('#complex-task-id')?.addEventListener('input', () => renderComplexTaskConfig());
qs('#complex-task-name')?.addEventListener('input', () => renderComplexTaskConfig());
qs('#complex-task-goal')?.addEventListener('input', () => renderComplexTaskConfig());
qs('#complex-task-mode')?.addEventListener('change', () => renderComplexTaskConfig());
qs('#draft-webhook-config-id')?.addEventListener('change', () => {
  syncWebhookReferenceEnabledState('#draft-webhook-config-id', '#draft-webhook-enabled', { autoEnable: true });
  renderWebhookReferenceHint('#draft-webhook-config-id', '#draft-webhook-auto-hint');
});
qs('#complex-task-webhook-config-id')?.addEventListener('change', () => {
  syncWebhookReferenceEnabledState('#complex-task-webhook-config-id', '#complex-task-webhook-enabled', { autoEnable: true });
  renderWebhookReferenceHint('#complex-task-webhook-config-id', '#complex-task-webhook-auto-hint');
  renderComplexTaskConfig();
});
qs('#complex-task-webhook-enabled')?.addEventListener('change', () => renderComplexTaskConfig());
syncTaskDraftCycleFields();

async function refreshTaskDrafts(quiet) {
  try {
    const data = await fetchJSON('/api/v1/task-drafts');
    window.__taskDraftCache = Array.isArray(data.items) ? data.items : [];
    renderAPITaskList();
    if (!quiet) toast('任务草稿', '已刷新任务草稿列表', 'ok');
  } catch (e) {
    toast('任务草稿', String(e), '');
  }
}

function normalizeTaskDraftRunUntilForInput(value) {
  const raw = String(value || '').trim();
  if (!raw) return '';
  if (/^\d{4}-\d{2}-\d{2}$/.test(raw)) return raw;
  const date = new Date(raw);
  if (Number.isNaN(date.getTime())) return raw.slice(0, 10);
  const year = date.getFullYear();
  const month = String(date.getMonth() + 1).padStart(2, '0');
  const day = String(date.getDate()).padStart(2, '0');
  return `${year}-${month}-${day}`;
}

function normalizeTaskDraftRunUntilForPayload(value) {
  const raw = String(value || '').trim();
  if (!raw) return '';
  if (/^\d{4}-\d{2}-\d{2}$/.test(raw)) return raw;
  const date = new Date(raw);
  if (Number.isNaN(date.getTime())) return raw.slice(0, 10);
  return date.toISOString().slice(0, 10);
}

function syncTaskDraftCycleFields(cycleMode) {
  const cycleModeEl = qs('#draft-cycle-mode');
  const runCountEl = qs('#draft-run-count');
  const runUntilEl = qs('#draft-run-until');
  if (!cycleModeEl || !runCountEl || !runUntilEl) return;
  const nextMode = normalizeTaskDraftCycleMode(cycleMode || cycleModeEl.value || 'once');
  cycleModeEl.value = nextMode;
  const isOnce = nextMode === 'once';
  qs('#draft-run-count').disabled = isOnce;
  qs('#draft-run-until').disabled = isOnce;
  if (isOnce) {
    qs('#draft-run-count').value = '1';
    qs('#draft-run-until').value = '';
    return;
  }
  qs('#draft-run-count').value = String(Math.max(1, Number(runCountEl.value || 1) || 1));
}

function buildTaskDraftCycleModeOptions() {
  return ['once', '5min', '30min', '1h', '6h', '24h', '7day', '1month', '1year'];
}

function normalizeTaskDraftCycleMode(value) {
  const raw = String(value || '').trim();
  return buildTaskDraftCycleModeOptions().includes(raw) ? raw : 'once';
}

function clearTaskDraftWorkbench() {
  window.__currentDraftID = '';
  if (qs('#draft-id')) qs('#draft-id').value = '';
  if (qs('#draft-name')) qs('#draft-name').value = '';
  if (qs('#draft-mode')) qs('#draft-mode').value = 'api_only';
  if (qs('#draft-cycle-mode')) qs('#draft-cycle-mode').value = 'once';
  if (qs('#draft-run-count')) qs('#draft-run-count').value = '1';
  if (qs('#draft-run-until')) qs('#draft-run-until').value = '';
  fillWebhookReferenceFields('#draft-webhook-config-id', '#draft-webhook-enabled', {});
  if (qs('#exec-webhook-push-once')) qs('#exec-webhook-push-once').checked = false;
  if (qs('#exec-template')) qs('#exec-template').value = '';
  if (qs('#exec-method')) qs('#exec-method').value = '';
  if (qs('#exec-path')) qs('#exec-path').value = '';
  if (qs('#exec-query')) qs('#exec-query').value = '';
  if (qs('#exec-path-params')) qs('#exec-path-params').value = '';
  if (qs('#exec-body')) qs('#exec-body').value = '';
  if (qs('#draft-transform')) qs('#draft-transform').value = '';
  if (qs('#draft-llm-api-id')) {
    qs('#draft-llm-api-id').innerHTML = buildDraftLLMAPIOptions('');
    qs('#draft-llm-api-id').value = '';
    renderDraftLLMAPIHint('');
  }
  if (qs('#draft-llm')) qs('#draft-llm').value = '';
  if (qs('#draft-output')) qs('#draft-output').value = '';
  syncTaskDraftCycleFields('once');
  clearTaskDraftFieldErrors();
  renderAPITaskList();
}

async function deleteTaskDraftByID(id) {
  if (!id) return;
  if (!confirm(`确认删除任务草稿 ${id}？`)) return;
  try {
    await fetchJSON(`/api/v1/task-drafts/${encodeURIComponent(id)}`, { method: 'DELETE' });
    if (window.__currentDraftID === id) clearTaskDraftWorkbench();
    await refreshTaskDrafts(true);
    await refreshJobs(true);
    toast('删除成功', id, 'ok');
  } catch (e) {
    toast('删除失败', String(e), '');
  }
}

function buildTaskDraftExecutePayload(draft) {
  const input = draft && draft.input_config ? draft.input_config : {};
  return {
    template_id: draft.source_template_id || input.template_id || '',
    method: input.method || '',
    path: input.path || '',
    query: input.query || {},
    path_params: input.path_params || {},
    body: input.body || {}
  };
}

async function runTaskDraftFromList(id) {
  const draft = (window.__taskDraftCache || []).find(it => it.id === id);
  if (!draft) throw new Error(`未找到任务草稿 ${id}`);
  loadTaskDraftIntoWorkbench(draft);
  const data = await fetchJSON('/api/v1/api/execute', {
    method: 'POST',
    body: JSON.stringify(buildTaskDraftExecutePayload(draft))
  });
  qs('#api-out').textContent = pretty(data);
  focusApiOut();
  return data;
}

function buildHourOptions(selected) {
  return Array.from({ length: 24 }, (_, i) => {
    const value = String(i).padStart(2, '0');
    return `<option value="${value}" ${value === selected ? 'selected' : ''}>${value}</option>`;
  }).join('');
}

function buildMinuteOptions(selected) {
  return Array.from({ length: 60 }, (_, i) => {
    const value = String(i).padStart(2, '0');
    return `<option value="${value}" ${value === selected ? 'selected' : ''}>${value}</option>`;
  }).join('');
}

function buildIntervalPresetOptions(selected) {
  return ['5min', '30min', '1h', '6h', '24h', '7day', '1month', '1year']
    .map((value) => `<option value="${value}" ${value === selected ? 'selected' : ''}>${value}</option>`)
    .join('');
}

function normalizeIntervalPreset(value) {
  const raw = String(value || '').trim().toLowerCase();
  if (!raw) return '1h';
  if (raw === '5m' || raw === '5min') return '5min';
  if (raw === '30m' || raw === '30min') return '30min';
  if (raw === '1h' || raw === '60m') return '1h';
  if (raw === '6h') return '6h';
  if (raw === '24h' || raw === '1d') return '24h';
  if (raw === '168h' || raw === '7d' || raw === '7day') return '7day';
  if (raw === '720h' || raw === '30d' || raw === '1month') return '1month';
  if (raw === '8760h' || raw === '365d' || raw === '1year') return '1year';
  return '1h';
}

function parseScheduleClockFromCron(cronExpr) {
  const parts = String(cronExpr || '').trim().split(/\s+/).filter(Boolean);
  const minuteIndex = parts.length >= 6 ? 1 : 0;
  const hourIndex = parts.length >= 6 ? 2 : 1;
  const minute = /^\d+$/.test(parts[minuteIndex] || '') ? String(Number(parts[minuteIndex])).padStart(2, '0') : '00';
  const hour = /^\d+$/.test(parts[hourIndex] || '') ? String(Number(parts[hourIndex])).padStart(2, '0') : '09';
  return { hour, minute };
}

function inferIntervalPresetFromCron(cronExpr) {
  const cron = String(cronExpr || '').trim();
  if (!cron) return '';
  if (/^0 \*\/5 \* \* \* \*$/.test(cron)) return '5min';
  if (/^0 \*\/30 \* \* \* \*$/.test(cron)) return '30min';
  if (/^0 0 (\*|\*\/1) \* \* \*$/.test(cron)) return '1h';
  if (/^0 0 \*\/6 \* \* \*$/.test(cron)) return '6h';
  if (/^0 \d{1,2} \d{1,2} \* \* [\d,\-*/A-Z]+$/i.test(cron)) return '7day';
  if (/^0 \d{1,2} \d{1,2} \d{1,2} \* \*$/.test(cron)) return '1month';
  if (/^0 \d{1,2} \d{1,2} \d{1,2} \d{1,2} \*$/.test(cron)) return '1year';
  return '';
}

function intervalPresetToDuration(preset) {
  switch (preset) {
    case '5min': return '5m';
    case '30min': return '30m';
    case '6h': return '6h';
    case '24h': return '24h';
    case '7day': return '168h';
    case '1month': return '720h';
    case '1year': return '8760h';
    case '1h':
    default:
      return '1h';
  }
}

function buildIntervalPresetLabel(preset) {
  switch (normalizeIntervalPreset(preset)) {
    case '5min': return '5 分钟';
    case '30min': return '30 分钟';
    case '6h': return '6 小时';
    case '24h': return '24 小时';
    case '7day': return '7 天';
    case '1month': return '1 个月';
    case '1year': return '1 年';
    case '1h':
    default:
      return '1 小时';
  }
}

function getScheduleAnchorParts(startAt) {
  const match = String(startAt || '').trim().match(/^(\d{4})-(\d{2})-(\d{2})/);
  if (!match) return { month: 1, day: 1 };
  return {
    month: Math.max(1, Math.min(12, Number(match[2]) || 1)),
    day: Math.max(1, Math.min(31, Number(match[3]) || 1))
  };
}

function buildJobScheduleSummary(job) {
  const cron = String(job.cron || '').trim();
  const parts = cron.split(/\s+/).filter(Boolean);
  const dayIndex = parts.length >= 6 ? 3 : 2;
  const monthIndex = parts.length >= 6 ? 4 : 3;
  const day = /^\d+$/.test(parts[dayIndex] || '') ? String(Number(parts[dayIndex])) : '';
  const month = /^\d+$/.test(parts[monthIndex] || '') ? String(Number(parts[monthIndex])) : '';
  const clock = parseScheduleClockFromCron(cron);
  const inferredPreset = inferIntervalPresetFromCron(cron);
  const normalizedInterval = normalizeIntervalPreset(job.interval || inferredPreset || '');
  const timezone = String(job.timezone || 'Asia/Shanghai').trim() || 'Asia/Shanghai';
  const serverHint = `默认使用服务器时区（${timezone}）`;

  if ((job.schedule_type || '') === 'interval') {
    const label = buildIntervalPresetLabel(normalizedInterval);
    return {
      modeLabel: '固定间隔',
      short: `每 ${label} 执行`,
      detail: `系统会按固定间隔触发任务，每 ${label} 执行一次；${serverHint}`
    };
  }

  if (/^0 \d{1,2} \d{1,2} \d{1,2} \d{1,2} \*$/.test(cron)) {
    return {
      modeLabel: '年度定时',
      short: `每年 ${month || '1'} 月 ${day || '1'} 日 ${clock.hour}:${clock.minute} 执行`,
      detail: `系统会在每年 ${month || '1'} 月 ${day || '1'} 日 ${clock.hour}:${clock.minute} 执行；${serverHint}`
    };
  }

  if (/^0 \d{1,2} \d{1,2} \d{1,2} \* \*$/.test(cron)) {
    return {
      modeLabel: '每月定时',
      short: `每月 ${day || '1'} 日 ${clock.hour}:${clock.minute} 执行`,
      detail: `系统会在每月 ${day || '1'} 日 ${clock.hour}:${clock.minute} 执行；${serverHint}`
    };
  }

  if (/^0 \d{1,2} \d{1,2} \* \* \*$/.test(cron)) {
    return {
      modeLabel: '每日定时',
      short: `每天 ${clock.hour}:${clock.minute} 执行`,
      detail: `系统会在每天 ${clock.hour}:${clock.minute} 执行；${serverHint}`
    };
  }

  if (inferredPreset) {
    const label = buildIntervalPresetLabel(inferredPreset);
    return {
      modeLabel: '固定间隔',
      short: `每 ${label} 执行`,
      detail: `当前调度与快捷调度的固定间隔规则一致，每 ${label} 执行一次；${serverHint}`
    };
  }

  if (cron) {
    return {
      modeLabel: '自定义 Cron',
      short: '按自定义 Cron 执行',
      detail: `当前使用自定义 Cron 表达式：${cron}；${serverHint}`
    };
  }

  return {
    modeLabel: '未设置',
    short: '尚未配置调度方式',
    detail: `当前尚未配置调度方式；${serverHint}`
  };
}

function inferScheduleUIState(job) {
  const clock = parseScheduleClockFromCron(job.cron || '');
  if ((job.schedule_type || '') === 'interval') {
    return { mode: 'interval', intervalPreset: normalizeIntervalPreset(job.interval || ''), hour: clock.hour, minute: clock.minute };
  }
  const cronPreset = inferIntervalPresetFromCron(job.cron || '');
  if (cronPreset) {
    return { mode: 'interval', intervalPreset: cronPreset, hour: clock.hour, minute: clock.minute };
  }
  return {
    mode: 'daily',
    intervalPreset: '1h',
    minute: clock.minute,
    hour: clock.hour,
  };
}

function buildSchedulePayloadFromUI(base, ids) {
  const payload = cloneValue(base || {});
  const mode = qs(ids.mode)?.value || 'daily';
  const hour = qs(ids.hour)?.value || '09';
  const minute = qs(ids.minute)?.value || '00';
  payload.timezone = 'Asia/Shanghai';
  if (mode === 'interval') {
    const preset = qs(ids.interval)?.value || '1h';
    if (preset === '1month' || preset === '1year') {
      const anchor = getScheduleAnchorParts(payload.start_at || (base || {}).start_at || '');
      payload.schedule_type = 'cron';
      payload.cron = preset === '1year'
        ? `0 ${Number(minute)} ${Number(hour)} ${anchor.day} ${anchor.month} *`
        : `0 ${Number(minute)} ${Number(hour)} ${anchor.day} * *`;
      payload.interval = '';
      return payload;
    }
    payload.schedule_type = 'interval';
    payload.interval = intervalPresetToDuration(preset);
    payload.cron = '';
    return payload;
  }
  payload.schedule_type = 'cron';
  payload.cron = `0 ${Number(minute)} ${Number(hour)} * * *`;
  payload.interval = '';
  return payload;
}

function buildDefaultJobDraft() {
  return {
    id: '',
    target_type: (window.__taskDraftCache[0] || {}).id ? 'task_draft' : 'complex_task',
    target_id: (window.__taskDraftCache[0] || {}).id || (window.__complexTaskCache[0] || {}).id || '',
    enabled: true,
    webhook_config_id: '',
    webhook_enabled: false,
    start_at: '',
    end_at: '',
    schedule_type: 'cron',
    cron: '0 0 9 * * *',
    interval: '',
    timezone: 'Asia/Shanghai'
  };
}

function takePendingJobDraft(baseDraft) {
  const pending = cloneValue(window.__pendingJobDraft || {});
  window.__pendingJobDraft = null;
  if (!Object.keys(pending).length) return baseDraft;
  const draft = Object.assign({}, baseDraft, pending);
  draft.target_type = pending.target_type || draft.target_type || 'task_draft';
  draft.target_id = pending.target_id || pending.draft_id || draft.target_id || '';
  draft.draft_id = draft.target_type === 'task_draft' ? (pending.draft_id || draft.target_id || '') : '';
  if (!draft.id) {
    const suffix = slugifyDraftPart(draft.target_id || pending.name || 'task');
    draft.id = `job_${suffix || 'task'}`;
  }
  return draft;
}

function openCreateJobEditor(prefill) {
  if (prefill) window.__pendingJobDraft = cloneValue(prefill);
  const hasPending = !!(window.__pendingJobDraft && Object.keys(window.__pendingJobDraft).length);
  if (!hasPending && !(window.__taskDraftCache || []).length && !(window.__complexTaskCache || []).length) {
    toast('新建定时任务', '请先创建至少一个单次/周期任务草稿或运营agent', '');
    return false;
  }
  const draft = takePendingJobDraft(buildDefaultJobDraft());
  setView('tasks');
  switchTaskSubpage('schedule');
  if (typeof openAdvancedJobEditor === 'function') {
    openAdvancedJobEditor(draft, { isCreate: true });
    return true;
  }
  if (typeof openJobDrawer === 'function') {
    openJobDrawer(draft, { isCreate: true });
    return true;
  }
  toast('进入定时任务清单失败', '未找到可用的新建调度入口', '');
  return false;
}

function createScheduleFromTaskDraft(id) {
  const draft = (window.__taskDraftCache || []).find(it => it.id === id);
  if (!draft) {
    toast('进入定时任务清单失败', `未找到任务草稿 ${id}`, '');
    return;
  }
  loadTaskDraftIntoWorkbench(draft);
  const created = openCreateJobEditor({
    id: `job_${slugifyDraftPart(draft.id || draft.name || 'task') || 'task'}`,
    target_type: 'task_draft',
    target_id: draft.id || '',
    draft_id: draft.id || '',
    enabled: false,
    webhook_config_id: draft.webhook_config_id || '',
    webhook_enabled: !!(draft.webhook_config_id && draft.webhook_enabled),
    timezone: 'Asia/Shanghai',
    name: draft.name || ''
  });
  if (created) {
    toast('定时任务清单', `已为 ${draft.id || draft.name || '当前任务'} 预填新建调度`, 'ok');
  }
}

function renderAPITaskList() {
  const host = qs('#api-task-list');
  if (!host) return;
  const keyword = (qs('#api-task-filter')?.value || '').trim().toLowerCase();
  const drafts = (window.__taskDraftCache || []).filter((draft) => {
    const hay = `${draft.id || ''} ${draft.name || ''} ${draft.source_template_id || (draft.input_config || {}).template_id || ''}`.toLowerCase();
    return !keyword || hay.includes(keyword);
  });
  if (!drafts.length) {
    host.className = 'subtitle';
    host.innerHTML = '<div class="subtitle">暂无任务草稿，请先在右侧保存第一条任务。</div>';
    return;
  }
  host.className = 'api-task-list';
  const cards = [];
  drafts.forEach((draft) => {
    cards.push(`
      <div class="api-task-item ${window.__currentDraftID === draft.id ? 'api-task-item-active' : ''}" data-id="${escapeHtml(draft.id || '')}">
        <div style="font-weight:750">${escapeHtml(draft.name || draft.id || '')}</div>
        <div class="subtitle"><code>${escapeHtml(draft.id || '')}</code> · mode=${escapeHtml(draft.mode || '')}</div>
        <div class="subtitle">template=${escapeHtml(draft.source_template_id || (draft.input_config || {}).template_id || '')}</div>
        <div class="subtitle">周期=${escapeHtml(normalizeTaskDraftCycleMode(draft.cycle_mode || 'once'))} · 次数=${escapeHtml(String(Number(draft.run_count || 1)))} · 结束=${escapeHtml(draft.run_until || '未设置')}</div>
        <div class="subtitle">webhook=${escapeHtml(draft.webhook_config_id || '未绑定')} · push=${draft.webhook_enabled ? 'on' : 'off'}</div>
        <div class="api-task-actions">
          <button class="btn" data-act="test" data-id="${escapeHtml(draft.id || '')}">测试</button>
          <button class="btn" data-act="schedule" data-id="${escapeHtml(draft.id || '')}">进入定时任务清单</button>
          <button class="btn" data-act="edit" data-id="${escapeHtml(draft.id || '')}">编辑</button>
          <button class="btn danger" data-act="delete" data-id="${escapeHtml(draft.id || '')}">删除</button>
        </div>
      </div>
    `);
  });
  host.innerHTML = cards.join('');
  qsa('#api-task-list [data-act="edit"]').forEach(btn => btn.addEventListener('click', () => {
    const id = btn.dataset.id || '';
    const draft = (window.__taskDraftCache || []).find(it => it.id === id);
    if (!draft) return;
    loadTaskDraftIntoWorkbench(draft);
    toast('任务列表', `已加载 ${id}`, 'ok');
  }));
  qsa('#api-task-list [data-act="delete"]').forEach(btn => btn.addEventListener('click', async () => {
    await deleteTaskDraftByID(btn.dataset.id || '');
  }));
  qsa('#api-task-list [data-act="test"]').forEach(btn => btn.addEventListener('click', async () => {
    try {
      await runTaskDraftFromList(btn.dataset.id || '');
      toast('任务列表', '任务测试完成', 'ok');
    } catch (e) {
      renderAPIError('任务测试失败', e);
      toast('任务测试失败', String(e), '');
    }
  }));
  qsa('#api-task-list [data-act="schedule"]').forEach(btn => btn.addEventListener('click', () => {
    createScheduleFromTaskDraft(btn.dataset.id || '');
  }));
}

function loadTaskDraftIntoWorkbench(draft) {
  window.__currentDraftID = draft.id || '';
  const llmSelection = splitDraftLLMConfigSelection(draft.llm_config || {});
  qs('#draft-id').value = draft.id || '';
  qs('#draft-name').value = draft.name || '';
  qs('#draft-mode').value = draft.mode || 'api_only';
  qs('#draft-cycle-mode').value = normalizeTaskDraftCycleMode(draft.cycle_mode || 'once');
  qs('#draft-run-count').value = String(Math.max(1, Number(draft.run_count || 1) || 1));
  qs('#draft-run-until').value = normalizeTaskDraftRunUntilForInput(draft.run_until || '');
  syncTaskDraftCycleFields(normalizeTaskDraftCycleMode(draft.cycle_mode || 'once'));
  fillWebhookReferenceFields('#draft-webhook-config-id', '#draft-webhook-enabled', draft);
  if (qs('#exec-webhook-push-once')) qs('#exec-webhook-push-once').checked = false;
  const input = draft.input_config || {};
  qs('#exec-template').value = draft.source_template_id || input.template_id || '';
  qs('#exec-method').value = input.method || '';
  qs('#exec-path').value = input.path || '';
  qs('#exec-query').value = pretty(input.query || {});
  qs('#exec-path-params').value = pretty(input.path_params || {});
  qs('#exec-body').value = pretty(input.body || {});
  qs('#draft-transform').value = pretty(draft.transform_config || {});
  qs('#draft-llm-api-id').innerHTML = buildDraftLLMAPIOptions(llmSelection.llm_api_id);
  qs('#draft-llm-api-id').value = llmSelection.llm_api_id;
  renderDraftLLMAPIHint(llmSelection.llm_api_id);
  qs('#draft-llm').value = pretty(llmSelection.llm_config);
  qs('#draft-output').value = pretty(draft.output_config || {});
  clearTaskDraftFieldErrors();
  renderAPITaskList();
  focusAPITaskDefinitionArea();
}

function collectTaskDraftPayload() {
  syncTaskDraftCycleFields();
  const id = qs('#draft-id').value.trim();
  const name = qs('#draft-name').value.trim();
  const mode = qs('#draft-mode').value;
  const cycleMode = normalizeTaskDraftCycleMode(qs('#draft-cycle-mode').value || 'once');
  const runCount = cycleMode === 'once' ? 1 : Math.max(1, Number(qs('#draft-run-count').value || 1) || 1);
  const runUntil = cycleMode === 'once' ? '' : normalizeTaskDraftRunUntilForPayload(qs('#draft-run-until').value);
  const inputConfig = {
    template_id: qs('#exec-template').value.trim(),
    method: qs('#exec-method').value.trim(),
    path: qs('#exec-path').value.trim(),
    query: parseJSONOrEmpty(qs('#exec-query').value),
    path_params: parseJSONOrEmpty(qs('#exec-path-params').value),
    body: parseJSONOrEmpty(qs('#exec-body').value)
  };
  return {
    id,
    name,
    mode,
    cycle_mode: cycleMode,
    run_count: runCount,
    run_until: runUntil,
    source_template_id: inputConfig.template_id || '',
    ...getWebhookReferencePayload('#draft-webhook-config-id', '#draft-webhook-enabled'),
    input_config: inputConfig,
    transform_config: parseJSONOrEmpty(qs('#draft-transform').value),
    llm_config: readTaskDraftLLMConfig(),
    output_config: parseJSONOrEmpty(qs('#draft-output').value)
  };
}

function slugifyDraftPart(v) {
  return String(v || '')
    .trim()
    .replace(/[^a-zA-Z0-9]+/g, '_')
    .replace(/^_+|_+$/g, '')
    .toLowerCase();
}

function setDraftFieldError(fieldId, message) {
  const input = qs(`#${fieldId}`);
  const error = qs(`#${fieldId}-error`);
  if (input) input.classList.toggle('input-error', !!message);
  if (error) error.textContent = message || '';
}

function clearTaskDraftFieldErrors() {
  setDraftFieldError('draft-id', '');
  setDraftFieldError('draft-name', '');
}

function suggestTaskDraftIdentity() {
  const templateID = qs('#exec-template')?.value.trim() || '';
  const method = qs('#exec-method')?.value.trim() || '';
  const path = qs('#exec-path')?.value.trim() || '';
  const base = slugifyDraftPart(templateID || path.split('/').filter(Boolean).pop() || method || 'task');
  const id = `draft_${base || 'task'}`;
  const nameBase = templateID || (path ? `${method || 'API'} ${path}` : '飞连任务');
  return {
    id,
    name: `${nameBase} 草稿`
  };
}

function ensureTaskDraftIdentity() {
  const draftID = qs('#draft-id');
  const draftName = qs('#draft-name');
  const suggested = suggestTaskDraftIdentity();
  if (draftID && !draftID.value.trim()) {
    qs('#draft-id').value = suggested.id;
  }
  if (draftName && !draftName.value.trim()) {
    qs('#draft-name').value = suggested.name;
  }
}

function validateTaskDraftBasics(payload) {
  clearTaskDraftFieldErrors();
  if (!payload.id) {
    setDraftFieldError('draft-id', '请填写草稿 ID；也可以先从模板带入后自动生成。');
  }
  if (!payload.name) {
    setDraftFieldError('draft-name', '请填写草稿名称；也可以先从模板带入后自动生成。');
  }
  const firstMissing = !payload.id ? qs('#draft-id') : (!payload.name ? qs('#draft-name') : null);
  if (firstMissing) {
    focusAPITaskDefinitionArea(firstMissing);
    return false;
  }
  return true;
}

async function saveTaskDraft() {
  try {
    ensureTaskDraftIdentity();
    const payload = collectTaskDraftPayload();
    if (!validateTaskDraftBasics(payload)) {
      toast('任务草稿', '请先补全草稿 ID 和草稿名称', '');
      return;
    }
    const exists = (window.__taskDraftCache || []).some(it => it.id === payload.id);
    if (exists) {
      await fetchJSON(`/api/v1/task-drafts/${encodeURIComponent(payload.id)}`, { method: 'PUT', body: JSON.stringify(payload) });
    } else {
      await fetchJSON('/api/v1/task-drafts', { method: 'POST', body: JSON.stringify(payload) });
    }
    window.__currentDraftID = payload.id;
    toast('任务草稿', exists ? '草稿已更新' : '草稿已创建', 'ok');
    await refreshTaskDrafts(true);
    await refreshJobs(true);
    const savedDraft = (window.__taskDraftCache || []).find(it => it.id === payload.id) || null;
    if (savedDraft) {
      loadTaskDraftIntoWorkbench(savedDraft);
    }
    qs('#api-out').textContent = pretty({
      ok: true,
      action: 'save_task_draft',
      id: payload.id,
      mode: payload.mode,
      cycle_mode: payload.cycle_mode,
      run_count: payload.run_count,
      run_until: payload.run_until,
      webhook_config_id: payload.webhook_config_id,
      webhook_enabled: payload.webhook_enabled,
      saved_webhook_enabled: savedDraft ? !!savedDraft.webhook_enabled : null
    });
    focusApiOut();
  } catch (e) {
    toast('任务草稿保存失败', String(e), '');
  }
}

window.__tplCache = [];
window.__currentTplId = '';
window.__templateEditorMode = 'form';
window.__templateEditorIsCreate = false;
window.__templateEditorOriginalID = '';
window.__templateEditorReturnView = 'templates';

function viewTemplatesQ(sel) {
  return qs(`#view-templates ${sel}`);
}

function focusTplOut() {
  const box = viewTemplatesQ('#tpl-out');
  if (!box) return;
  box.scrollIntoView({ behavior: 'smooth', block: 'center' });
  box.classList.add('flash');
  setTimeout(() => box.classList.remove('flash'), 1200);
}

function writeTplOut(v) {
  const box = viewTemplatesQ('#tpl-out');
  if (!box) return;
  box.textContent = pretty(v);
  focusTplOut();
}

function writeTplDetail(v) {
  const box = viewTemplatesQ('#tpl-detail');
  if (!box) return;
  box.textContent = pretty(v);
}

async function loadTplDetailToTemplatesView(id) {
  if (!id) return;
  try {
    const payload = await fetchJSON(`/api/v1/api/templates/${encodeURIComponent(id)}`);
    writeTplDetail(payload.template || payload);
  } catch (e) {
    writeTplDetail({ error: String(e), id });
  }
}

function focusAPITaskDefinitionArea() {
  const card = qs('#api-task-definition-card');
  const field = qs('#exec-template');
  if (card) {
    card.scrollIntoView({ behavior: 'smooth', block: 'start' });
    card.classList.add('flash');
    setTimeout(() => card.classList.remove('flash'), 1200);
  } else if (field) {
    field.scrollIntoView({ behavior: 'smooth', block: 'center' });
  }
  setTimeout(() => {
    if (!field) return;
    field.focus({ preventScroll: true });
    if (typeof field.select === 'function') field.select();
  }, 180);
}

function applyTemplateToAPITask(template) {
  const tpl = template || {};
  openTaskCenterSubpage('once-cycle');
  if (qs('#exec-template')) qs('#exec-template').value = tpl.id || '';
  if (qs('#exec-method')) qs('#exec-method').value = tpl.method || '';
  if (qs('#exec-path')) qs('#exec-path').value = tpl.path || '';
  ensureTaskDraftIdentity();
  clearTaskDraftFieldErrors();
  focusAPITaskDefinitionArea();
  if (tpl.id) toast('飞连任务列表', `已用模板 ${tpl.id} 预填单次/周期任务定义`, 'ok');
}

function renderTemplatesTable() {
  const table = viewTemplatesQ('#tpl-table');
  const tbody = table ? table.querySelector('tbody') : null;
  if (!tbody) return;
  tbody.innerHTML = '';
  const filter = (viewTemplatesQ('#tpl-filter-2')?.value || '').trim().toLowerCase();

  (window.__tplCache || [])
    .filter(t => {
      if (!filter) return true;
      const hay = `${t.id || ''} ${t.name || ''} ${t.category || ''}`.toLowerCase();
      return hay.includes(filter);
    })
    .slice(0, 200)
    .forEach(t => {
      const tr = document.createElement('tr');
      if (window.__currentTplId === t.id) tr.classList.add('tpl-table-row-active');
      tr.innerHTML = `
        <td><b>${escapeHtml(t.id || '')}</b><div class="subtitle">${escapeHtml(t.name || '')}</div></td>
        <td>${escapeHtml(t.category || '')}</td>
        <td><span class="pill">${escapeHtml((t.method || '').toUpperCase())}</span></td>
        <td><code>${escapeHtml(t.path || '')}</code></td>
        <td style="white-space:nowrap">
          <button class="btn" data-act="test">测试</button>
          <button class="btn" data-act="create-task">用此模板创建任务</button>
          <button class="btn" data-act="edit">编辑</button>
          <button class="btn danger" data-act="delete">删除</button>
        </td>
      `;
      tr.style.cursor = 'pointer';
      tr.addEventListener('click', async (evt) => {
        if (evt.target.closest('button')) return;
        window.__currentTplId = t.id || '';
        renderTemplatesTable();
        await loadTplDetailToTemplatesView(t.id);
      });
      tr.querySelector('[data-act="test"]')?.addEventListener('click', async () => {
        try {
          const data = await fetchJSON(`/api/v1/api/templates/${encodeURIComponent(t.id)}/test`, {
            method: 'POST',
            body: JSON.stringify({ path_params: {}, query: {}, body: {} })
          });
          writeTplOut(data);
          toast('模板测试', `${t.id} 测试完成`, 'ok');
        } catch (e) {
          writeTplOut({ error: String(e), id: t.id });
          toast('模板测试失败', String(e), '');
        }
      });
      tr.querySelector('[data-act="create-task"]')?.addEventListener('click', () => {
        applyTemplateToAPITask(t);
      });
      tr.querySelector('[data-act="edit"]')?.addEventListener('click', () => openTemplateEditor(t, { isCreate: false, returnView: 'templates' }));
      tr.querySelector('[data-act="delete"]')?.addEventListener('click', async () => {
        if (!confirm(`确认删除模板 ${t.id}？`)) return;
        try {
          await fetchJSON(`/api/v1/api/templates/${encodeURIComponent(t.id)}`, { method: 'DELETE' });
          toast('删除成功', t.id, 'ok');
          if (window.__currentTplId === t.id) {
            window.__currentTplId = '';
            writeTplDetail('请选择一个模板');
          }
          await refreshTemplates(true);
        } catch (e) {
          toast('删除失败', String(e), '');
        }
      });
      tbody.appendChild(tr);
    });
}

async function refreshTemplates(quiet) {
  try {
    const data = await fetchJSON('/api/v1/api/templates');
    window.__tplCache = data.templates || [];
    renderTemplatesTable();
    if (!quiet) toast('飞连API列表', '已刷新飞连API列表', 'ok');
  } catch (e) {
    toast('模板', String(e), '');
  }
}

qs('#btn-templates-refresh')?.addEventListener('click', () => refreshTemplates(false));
viewTemplatesQ('#tpl-filter-2')?.addEventListener('input', () => renderTemplatesTable());

viewTemplatesQ('#btn-tpl-out-copy')?.addEventListener('click', async () => {
  try {
    await navigator.clipboard.writeText(viewTemplatesQ('#tpl-out')?.textContent || '');
    toast('已复制', '输出已复制到剪贴板', 'ok');
  } catch (e) {
    toast('复制失败', String(e), '');
  }
});
viewTemplatesQ('#btn-tpl-out-clear')?.addEventListener('click', () => {
  writeTplOut('暂无输出');
});

qs('#btn-template-create-2')?.addEventListener('click', () => {
  const draft = {
    id: '',
    name: '',
    category: 'custom',
    method: 'GET',
    path: '/api/open/v1/example',
    query_schema: {},
    path_params_schema: {},
    body_schema: {},
    dry_run_query_param: ''
  };
  openTemplateEditor(draft, { isCreate: true, returnView: 'templates' });
});
qs('#btn-template-editor-form')?.addEventListener('click', () => switchTemplateEditorMode('form'));
qs('#btn-template-editor-json-btn')?.addEventListener('click', () => switchTemplateEditorMode('json'));
qs('#btn-template-editor-back')?.addEventListener('click', () => {
  const back = window.__templateEditorReturnView || 'templates';
  setView(back);
  if (back === 'templates' && window.__currentTplId) loadTplDetailToTemplatesView(window.__currentTplId);
});
qs('#btn-template-editor-save')?.addEventListener('click', saveTemplateEditor);

async function buildExecutePayload() {
  const template_id = qs('#exec-template').value.trim();
  const method = qs('#exec-method').value.trim();
  const path = qs('#exec-path').value.trim();
  const query = parseJSONOrEmpty(qs('#exec-query').value);
  const path_params = parseJSONOrEmpty(qs('#exec-path-params').value);
  const body = parseJSONOrEmpty(qs('#exec-body').value);
  const webhookRef = getWebhookReferencePayload('#draft-webhook-config-id', '#draft-webhook-enabled');
  return {
    template_id,
    method,
    path,
    query,
    path_params,
    body,
    draft_id: qs('#draft-id')?.value.trim() || '',
    name: qs('#draft-name')?.value.trim() || '',
    mode: qs('#draft-mode')?.value.trim() || 'api_only',
    transform_config: parseJSONOrEmpty(qs('#draft-transform')?.value || '{}'),
    llm_config: readTaskDraftLLMConfig(),
    output_config: parseJSONOrEmpty(qs('#draft-output')?.value || '{}'),
    webhook_config_id: webhookRef.webhook_config_id,
    webhook_enabled: webhookRef.webhook_enabled,
    webhook_push_once: qs('#exec-webhook-push-once')?.checked === true
  };
}

function pretty(v) {
  if (typeof v === 'string') return v;
  return JSON.stringify(v, null, 2);
}

function renderAPIError(title, err) {
  const payload = err && err.payload ? err.payload : { error: String(err) };
  const conn = payload.runtime_connection || {};
  qs('#api-out').textContent = pretty({
    title,
    error: payload.error || String(err),
    runtime_connection: conn,
    hint: '模板测试和 Execute 使用的是已保存并热更新后的运行时配置；如果刚修改连接，请先到连接设置点击“保存并热更新”。'
  });
}

qs('#btn-exec').addEventListener('click', async () => {
  try {
    const payload = await buildExecutePayload();
    const data = await fetchJSON('/api/v1/api/execute', { method: 'POST', body: JSON.stringify(payload) });
    qs('#api-out').textContent = pretty(data);
    toast('Execute', '请求已完成', 'ok');
    focusApiOut();
  } catch (e) {
    renderAPIError('Execute 失败', e);
    toast('Execute 失败', String(e), '');
  }
});

qs('#btn-preview').addEventListener('click', async () => {
  try {
    const payload = await buildExecutePayload();
    const preview = Object.assign({}, payload, { preview_mode: 'diff_only', read_before_write: true });
    const data = await fetchJSON('/api/v1/api/preview', { method: 'POST', body: JSON.stringify(preview) });
    qs('#api-out').textContent = pretty(data);
    toast('Preview', '已生成变更预览', 'ok');
    focusApiOut();
  } catch (e) {
    renderAPIError('Preview 失败', e);
    toast('Preview 失败', String(e), '');
  }
});

function focusApiOut() {
  const box = qs('#api-out');
  if (!box) return;
  // 滚动到输出区并高亮闪烁
  box.scrollIntoView({ behavior: 'smooth', block: 'center' });
  box.classList.add('flash');
  setTimeout(() => box.classList.remove('flash'), 1200);
}

qs('#btn-copy-out').addEventListener('click', async () => {
  try {
    await navigator.clipboard.writeText(qs('#api-out').textContent || '');
    toast('已复制', '输出已复制到剪贴板', 'ok');
  } catch (e) {
    toast('复制失败', String(e), '');
  }
});

qs('#btn-logs').addEventListener('click', async () => {
  const tail = qs('#log-tail').value.trim() || '2000';
  try {
    const [data, execs] = await Promise.all([
      fetchJSON(`/api/v1/logs?tail=${encodeURIComponent(tail)}`),
      fetchJSON('/api/v1/logs/executions')
    ]);
    qs('#logs').textContent = data.text || pretty(data);
    renderExecutionLogs(execs.items || []);
    toast('Logs', '已刷新日志', 'ok');
  } catch (e) {
    toast('Logs', String(e), '');
  }
});

function renderExecutionLogs(items) {
  const host = qs('#execution-logs');
  if (!host) return;
  if (!items.length) {
    host.textContent = '暂无执行日志';
    return;
  }
  host.innerHTML = items.map(item => `
    <div class="card status-card ${item.ok ? 'status-card-ok' : 'status-card-bad'}" style="box-shadow:none; margin-bottom:10px">
      <div class="row" style="justify-content:space-between">
        <div>
          <div style="font-weight:750">${escapeHtml(item.target_type || '')} · ${escapeHtml(item.target_id || '')}</div>
          <div class="subtitle"><code>${escapeHtml(item.run_id || '')}</code></div>
        </div>
        <span class="pill ${item.ok ? 'ok' : 'bad'}">${item.ok ? 'SUCCESS' : 'FAILED'}</span>
      </div>
      <div class="status-strip ${item.ok ? 'status-strip-ok' : 'status-strip-bad'}">
        <span>Last Run: ${escapeHtml(item.last_run || '')}</span>
        <span>Duration: ${item.duration_ms || 0} ms</span>
      </div>
      ${item.error ? `<div class="alert-block alert-error"><div class="alert-title">错误信息</div><div>${escapeHtml(item.error)}</div></div>` : `<div class="alert-block alert-success"><div class="alert-title">执行状态</div><div>本次执行未返回错误信息，链路状态正常。</div></div>`}
      <div class="kv" style="margin-bottom:0">
        <div class="k">Last Run</div><div class="v">${escapeHtml(item.last_run || '')}</div>
        <div class="k">Duration</div><div class="v">${item.duration_ms || 0} ms</div>
        <div class="k">Error</div><div class="v">${escapeHtml(item.error || '')}</div>
      </div>
    </div>
  `).join('');
}

function renderJobsTable() {
  const tbody = qs('#jobs-table tbody');
  tbody.innerHTML = '';
  const search = (qs('#jobs-search')?.value || '').trim().toLowerCase();
  const typeFilter = qs('#jobs-filter-type')?.value || '';
  const enabledFilter = qs('#jobs-filter-enabled')?.value || '';
  const jobs = (window.__jobsCache || []).filter(j => {
    const targetId = j.target_id || j.draft_id || '';
    const idText = `${j.id || ''} ${targetId}`.toLowerCase();
    if (search && !idText.includes(search)) return false;
    if (typeFilter && j.schedule_type !== typeFilter) return false;
    if (enabledFilter === 'enabled' && !j.enabled) return false;
    if (enabledFilter === 'disabled' && j.enabled) return false;
    return true;
  });
  jobs.forEach(j => {
    const run = (window.__jobRunCache || {})[j.id] || {};
    const targetId = j.target_id || j.draft_id || '';
    const tr = document.createElement('tr');
    if (window.__currentJobName === j.id) tr.classList.add('jobs-table-row-active');
    const scheduleSummary = buildJobScheduleSummary(j);
    tr.innerHTML = `
      <td><b>${escapeHtml(j.id || '')}</b></td>
      <td><code>${escapeHtml(targetId)}</code><div class="subtitle">${escapeHtml(j.target_type || (j.draft_id ? 'task_draft' : ''))}</div></td>
      <td>${escapeHtml(scheduleSummary.modeLabel)}</td>
      <td>${escapeHtml(scheduleSummary.short)}<div class="subtitle">${escapeHtml(j.start_at || '')}${j.end_at ? ` → ${escapeHtml(j.end_at)}` : ''}</div><div class="subtitle">${escapeHtml(run.last_run || '')}</div></td>
      <td>${j.enabled ? '<span class="pill ok">ENABLED</span>' : '<span class="pill">DISABLED</span>'}</td>
      <td style="white-space:nowrap">
        <button class="btn" data-act="run">运行</button>
        <button class="btn" data-act="edit">编辑</button>
        <button class="btn ${j.enabled ? 'danger' : 'primary'}" data-act="toggle">${j.enabled ? '停用' : '启用'}</button>
        <button class="btn danger" data-act="delete">删除</button>
      </td>
    `;
    tr.style.cursor = 'pointer';
    tr.addEventListener('click', (evt) => {
      if (evt.target.closest('button')) return;
      loadJobDetail(j.id);
    });
    tr.querySelector('[data-act="run"]')?.addEventListener('click', async () => {
      try {
        await fetchJSON(`/api/v1/job-schedules/${encodeURIComponent(j.id)}/run`, { method: 'POST', body: '{}' });
        toast('手动运行', `${j.id} 已触发`, 'ok');
        refreshJobs(true);
        if (window.__currentJobName === j.id) loadJobDetail(j.id);
      } catch (e) { toast('手动运行失败', String(e), ''); }
    });
    tr.querySelector('[data-act="edit"]')?.addEventListener('click', () => openJobDrawer(j, { isCreate: false }));
    tr.querySelector('[data-act="toggle"]')?.addEventListener('click', async () => {
      try {
        const updated = cloneValue(j);
        updated.enabled = !updated.enabled;
        await fetchJSON(`/api/v1/job-schedules/${encodeURIComponent(j.id)}`, { method: 'PUT', body: JSON.stringify(updated) });
        toast('调度更新', `${j.id} 已${updated.enabled ? '启用' : '停用'}`, 'ok');
        refreshJobs(true);
        if (window.__currentJobName === j.id) loadJobDetail(j.id);
      } catch (e) { toast('调度更新失败', String(e), ''); }
    });
    tr.querySelector('[data-act="delete"]')?.addEventListener('click', async () => {
      if (!confirm(`确认删除调度 ${j.id}？`)) return;
      try {
        await fetchJSON(`/api/v1/job-schedules/${encodeURIComponent(j.id)}`, { method: 'DELETE' });
        toast('删除成功', j.id, 'ok');
        if (window.__currentJobName === j.id) {
          window.__currentJobName = '';
          qs('#job-detail').innerHTML = '';
          qs('#job-detail-empty').style.display = '';
        }
        refreshJobs(true);
      } catch (e) { toast('删除失败', String(e), ''); }
    });
    tbody.appendChild(tr);
  });
}

async function loadJobDetail(name) {
  try {
    const payload = await fetchJSON(`/api/v1/job-schedules/${encodeURIComponent(name)}`);
    window.__currentJobName = name;
    renderJobsTable();
    renderJobDetail(payload.schedule || {}, payload.draft || {}, payload.run || {});
  } catch (e) {
    toast('任务详情', String(e), '');
  }
}

function renderJobDetail(job, payloadDraft, run) {
  const targetType = job.target_type || (job.draft_id ? 'task_draft' : '');
  const targetID = job.target_id || job.draft_id || '';
  const draft = payloadDraft && Object.keys(payloadDraft).length
    ? payloadDraft
    : ((window.__taskDraftCache || []).find(it => it.id === targetID) || {});
  const complexTask = (window.__complexTaskCache || []).find(it => it.id === targetID) || {};
  const scheduleSummary = buildJobScheduleSummary(job);
  const hasRun = !!(run && run.last_run && run.last_run !== '0001-01-01T00:00:00Z');
  const box = qs('#job-detail');
  qs('#job-detail-empty').style.display = 'none';
  box.innerHTML = `
    <div class="row">
      <span class="pill">${escapeHtml(scheduleSummary.modeLabel)}</span>
      <span class="pill ${job.enabled ? 'ok' : ''}">${job.enabled ? 'ENABLED' : 'DISABLED'}</span>
    </div>
    <div class="card" style="box-shadow:none">
      <div style="font-weight:800">${escapeHtml(job.id || '')}</div>
      <div class="subtitle">${escapeHtml(scheduleSummary.short)}</div>
      <div class="kv">
        <div class="k">Schedule ID</div><div class="v">${escapeHtml(job.id || '')}</div>
        <div class="k">Target Type</div><div class="v">${escapeHtml(targetType)}</div>
        <div class="k">Target ID</div><div class="v"><code>${escapeHtml(targetID)}</code></div>
        <div class="k">Target Name</div><div class="v">${escapeHtml(draft.name || complexTask.name || '未找到目标')}</div>
        <div class="k">Webhook</div><div class="v">${escapeHtml(job.webhook_config_id || '未绑定')}</div>
        <div class="k">Webhook Push</div><div class="v">${job.webhook_enabled ? 'Yes' : 'No'}</div>
        <div class="k">调度方式</div><div class="v">${escapeHtml(scheduleSummary.short)}</div>
        <div class="k">调度说明</div><div class="v">${escapeHtml(scheduleSummary.detail)}</div>
        <div class="k">Enabled</div><div class="v">${job.enabled ? 'Yes' : 'No'}</div>
        <div class="k">Start At</div><div class="v">${escapeHtml(job.start_at || '')}</div>
        <div class="k">End At</div><div class="v">${escapeHtml(job.end_at || '')}</div>
      </div>
    </div>
    <div class="card" style="box-shadow:none">
      <div style="font-weight:750">最近一次运行</div>
      <div class="status-strip ${!hasRun ? '' : (run.last_ok ? 'status-strip-ok' : 'status-strip-bad')}">
        <span>Status: ${!hasRun ? 'N/A' : (run.last_ok ? 'SUCCESS' : 'FAILED')}</span>
        <span>Duration: ${(run && run.duration_ms) || 0} ms</span>
        <span>Next Run: ${escapeHtml((run && run.next_run) || '')}</span>
      </div>
      ${!hasRun
        ? '<div class="alert-block"><div class="alert-title">运行状态</div><div>当前暂无运行记录，请先手动运行或等待调度触发。</div></div>'
        : (run.last_ok
          ? '<div class="alert-block alert-success"><div class="alert-title">最近执行成功</div><div>最近一次周期任务执行成功，可结合草稿摘要检查输出是否符合预期。</div></div>'
          : `<div class="alert-block alert-error"><div class="alert-title">最近执行失败</div><div>${escapeHtml((run && run.last_error) || '未返回错误详情')}</div></div>`)}
      <div class="kv">
        <div class="k">Last Run</div><div class="v">${escapeHtml((run && run.last_run) || '暂无记录')}</div>
        <div class="k">Result</div><div class="v">${!hasRun ? '<span class="pill">N/A</span>' : (run.last_ok ? '<span class="pill ok">SUCCESS</span>' : '<span class="pill bad">FAILED</span>')}</div>
        <div class="k">Duration</div><div class="v">${(run && run.duration_ms) || 0} ms</div>
        <div class="k">Next Run</div><div class="v">${escapeHtml((run && run.next_run) || '')}</div>
        <div class="k">Error</div><div class="v">${escapeHtml((run && run.last_error) || '')}</div>
      </div>
    </div>
    <div class="card" style="box-shadow:none">
      <div style="font-weight:750">草稿摘要</div>
      <div class="kv">
        <div class="k">Mode</div><div class="v">${escapeHtml(draft.mode || complexTask.execution_mode || '')}</div>
        <div class="k">Template</div><div class="v"><code>${escapeHtml(draft.source_template_id || ((draft.input_config || {}).template_id || ''))}</code></div>
        <div class="k">Transform</div><div class="v"><code>${escapeHtml(JSON.stringify(draft.transform_config || {}))}</code></div>
        <div class="k">LLM</div><div class="v"><code>${escapeHtml(JSON.stringify(draft.llm_config || {}))}</code></div>
        <div class="k">Steps</div><div class="v">${(complexTask.steps || []).length || 0}</div>
      </div>
    </div>
    <div class="row">
      <button class="btn" id="btn-job-run">手动运行</button>
      <button class="btn" id="btn-job-edit-quick">快速编辑</button>
      <button class="btn primary" id="btn-job-edit-advanced">高级编辑</button>
      <button class="btn ${job.enabled ? 'danger' : 'ghost'}" id="btn-job-toggle">${job.enabled ? '停用' : '启用'}</button>
      <button class="btn danger" id="btn-job-delete">删除</button>
    </div>
  `;
  qs('#btn-job-run')?.addEventListener('click', async () => {
    try {
      await fetchJSON(`/api/v1/job-schedules/${encodeURIComponent(job.id)}/run`, { method: 'POST', body: '{}' });
      toast('手动运行', `${job.id} 已触发`, 'ok');
      refreshJobs(true);
      loadJobDetail(job.id);
    } catch (e) { toast('手动运行失败', String(e), ''); }
  });
  qs('#btn-job-toggle')?.addEventListener('click', async () => {
    try {
      const updated = cloneValue(job);
      updated.enabled = !updated.enabled;
      await fetchJSON(`/api/v1/job-schedules/${encodeURIComponent(job.id)}`, { method: 'PUT', body: JSON.stringify(updated) });
      toast('调度更新', `${job.id} 已${updated.enabled ? '启用' : '停用'}`, 'ok');
      refreshJobs(true);
      loadJobDetail(job.id);
    } catch (e) { toast('调度更新失败', String(e), ''); }
  });
  qs('#btn-job-edit-quick')?.addEventListener('click', () => openJobDrawer(job, { isCreate: false }));
  qs('#btn-job-edit-advanced')?.addEventListener('click', () => openAdvancedJobEditor(job, { isCreate: false }));
  qs('#btn-job-delete')?.addEventListener('click', async () => {
    if (!confirm(`确认删除调度 ${job.id}？`)) return;
    try {
      await fetchJSON(`/api/v1/job-schedules/${encodeURIComponent(job.id)}`, { method: 'DELETE' });
      toast('删除成功', job.id, 'ok');
      window.__currentJobName = '';
      qs('#job-detail').innerHTML = '';
      qs('#job-detail-empty').style.display = '';
      refreshJobs(true);
    } catch (e) { toast('删除失败', String(e), ''); }
  });
}

function openJobDrawer(job, opts) {
  window.__jobDrawerIsCreate = !!(opts && opts.isCreate);
  window.__jobDrawerOriginalID = job.id || '';
  const webhookHint = getWebhookReferenceHintText(getWebhookCatalogItem(job.webhook_config_id || ''));
  const uiState = inferScheduleUIState(job);
  const box = qs('#job-drawer-body');
  box.innerHTML = `
    <div class="row">
      <div class="field"><label>schedule_id</label><input id="job-drawer-id" value="${escapeHtml(job.id || '')}" /></div>
      <div class="field"><label>执行目标</label>
        <select id="job-drawer-target">${buildScheduleTargetOptions(job.target_type || (job.draft_id ? 'task_draft' : ''), job.target_id || job.draft_id || '')}</select>
      </div>
    </div>
    <div class="row">
      <div class="field"><label>任务定时窗口</label>
        <select id="job-drawer-time-mode">
          <option value="daily" ${uiState.mode === 'daily' ? 'selected' : ''}>每日定时</option>
          <option value="interval" ${uiState.mode === 'interval' ? 'selected' : ''}>固定间隔</option>
        </select>
        <div class="subtitle">默认使用当前服务器时区</div>
      </div>
      <label class="field"><span>enabled</span><select id="job-drawer-enabled"><option value="true" ${job.enabled ? 'selected' : ''}>true</option><option value="false" ${!job.enabled ? 'selected' : ''}>false</option></select></label>
    </div>
    <div class="row" id="job-drawer-daily-row">
      <div class="field"><label>小时</label><select id="job-drawer-daily-hour">${buildHourOptions(uiState.hour || '09')}</select></div>
      <div class="field"><label>分钟</label><select id="job-drawer-daily-minute">${buildMinuteOptions(uiState.minute || '00')}</select></div>
    </div>
    <div class="row" id="job-drawer-interval-row">
      <div class="field">
        <label>间隔</label>
        <select id="job-drawer-interval-preset">${buildIntervalPresetOptions(uiState.intervalPreset || '1h')}</select>
      </div>
    </div>
    <div class="row">
      <div class="field"><label>start_at</label><input id="job-drawer-start-at" value="${escapeHtml(job.start_at || '')}" placeholder="2026-06-04T09:00:00+08:00" /></div>
      <div class="field"><label>end_at</label><input id="job-drawer-end-at" value="${escapeHtml(job.end_at || '')}" placeholder="可选" /></div>
    </div>
    <div class="row">
      <div class="field">
        <label>webhook 配置</label>
        <select id="job-drawer-webhook-config-id">${buildWebhookReferenceOptions(job.webhook_config_id || '')}</select>
        <div id="job-drawer-webhook-auto-hint" class="subtitle">${escapeHtml(webhookHint)}</div>
      </div>
      <label class="field" style="max-width:220px">
        <span>启用推送引用</span>
        <input id="job-drawer-webhook-enabled" type="checkbox" ${job.webhook_config_id && job.webhook_enabled ? 'checked' : ''} />
      </label>
    </div>
    <div class="row">
      <button class="btn primary" id="job-drawer-save">保存</button>
    </div>
  `;
  qs('#job-drawer').classList.remove('hidden');
  toggleJobDrawerScheduleFields();
  qs('#job-drawer-time-mode')?.addEventListener('change', () => toggleJobDrawerScheduleFields());
  qs('#job-drawer-webhook-config-id')?.addEventListener('change', () => {
    syncWebhookReferenceEnabledState('#job-drawer-webhook-config-id', '#job-drawer-webhook-enabled', { autoEnable: true });
    renderWebhookReferenceHint('#job-drawer-webhook-config-id', '#job-drawer-webhook-auto-hint');
  });
  qs('#job-drawer-save')?.addEventListener('click', async () => {
    const updated = cloneValue(job);
    updated.id = qs('#job-drawer-id').value.trim();
    const targetValue = qs('#job-drawer-target').value.trim().split(':');
    updated.target_type = targetValue[0] || 'task_draft';
    updated.target_id = targetValue[1] || '';
    updated.draft_id = updated.target_type === 'task_draft' ? updated.target_id : '';
    updated.enabled = qs('#job-drawer-enabled').value === 'true';
    updated.start_at = qs('#job-drawer-start-at').value.trim();
    updated.end_at = qs('#job-drawer-end-at').value.trim();
    Object.assign(updated, buildSchedulePayloadFromUI(updated, {
      mode: '#job-drawer-time-mode',
      hour: '#job-drawer-daily-hour',
      minute: '#job-drawer-daily-minute',
      interval: '#job-drawer-interval-preset',
    }));
    Object.assign(updated, getWebhookReferencePayload('#job-drawer-webhook-config-id', '#job-drawer-webhook-enabled'));
    try {
      if (window.__jobDrawerIsCreate) {
        await fetchJSON('/api/v1/job-schedules', { method: 'POST', body: JSON.stringify(updated) });
      } else {
        await fetchJSON(`/api/v1/job-schedules/${encodeURIComponent(window.__jobDrawerOriginalID || updated.id)}`, { method: 'PUT', body: JSON.stringify(updated) });
      }
      toast('保存成功', updated.id, 'ok');
      closeJobDrawer();
      refreshJobs(true);
      loadJobDetail(updated.id);
    } catch (e) { toast('保存失败', String(e), ''); }
  });
}

function toggleJobDrawerScheduleFields() {
  const mode = qs('#job-drawer-time-mode')?.value || 'daily';
  qs('#job-drawer-daily-row')?.classList.toggle('hidden', mode !== 'daily');
  qs('#job-drawer-interval-row')?.classList.toggle('hidden', mode !== 'interval');
}

function toggleAdvancedJobEditorScheduleFields() {
  const mode = qs('#job-form-time-mode')?.value || 'daily';
  qs('#job-form-daily-row')?.classList.toggle('hidden', mode !== 'daily');
  qs('#job-form-interval-row')?.classList.toggle('hidden', mode !== 'interval');
}

function closeJobDrawer() {
  qs('#job-drawer').classList.add('hidden');
}

function openAdvancedJobEditor(job, opts) {
  window.__jobEditorIsCreate = !!(opts && opts.isCreate);
  window.__jobEditorOriginalName = job.id || '';
  window.__jobEditorCurrent = cloneValue(job);
  switchJobEditorMode('form');
  renderAdvancedEditorForm(window.__jobEditorCurrent);
  qs('#job-editor-json').value = pretty(window.__jobEditorCurrent);
  setView('view-job-editor'.replace('view-','')); // -> job-editor
}

function switchJobEditorMode(mode) {
  window.__jobEditorMode = mode;
  const form = qs('#job-editor-form');
  const json = qs('#job-editor-json');
  if (mode === 'json') {
    form.classList.add('hidden');
    json.classList.remove('hidden');
  } else {
    form.classList.remove('hidden');
    json.classList.add('hidden');
  }
}

function renderAdvancedEditorForm(job) {
  const box = qs('#job-editor-form');
  const webhookHint = getWebhookReferenceHintText(getWebhookCatalogItem(job.webhook_config_id || ''));
  const uiState = inferScheduleUIState(job);
  box.innerHTML = `
    <div class="row">
      <div class="field"><label>schedule_id</label><input id="job-form-id" value="${escapeHtml(job.id || '')}" /></div>
      <div class="field"><label>执行目标</label>
        <select id="job-form-target">${buildScheduleTargetOptions(job.target_type || (job.draft_id ? 'task_draft' : ''), job.target_id || job.draft_id || '')}</select>
      </div>
    </div>
    <div class="row">
      <div class="field"><label>任务定时窗口</label>
        <select id="job-form-time-mode">
          <option value="daily" ${uiState.mode === 'daily' ? 'selected' : ''}>每日定时</option>
          <option value="interval" ${uiState.mode === 'interval' ? 'selected' : ''}>固定间隔</option>
        </select>
        <div class="subtitle">默认使用当前服务器时区（Asia/Shanghai）</div>
      </div>
      <div class="field"><label>enabled</label>
        <select id="job-form-enabled"><option value="true" ${job.enabled ? 'selected' : ''}>true</option><option value="false" ${!job.enabled ? 'selected' : ''}>false</option></select>
      </div>
    </div>
    <div class="row" id="job-form-daily-row">
      <div class="field"><label>小时</label><select id="job-form-daily-hour">${buildHourOptions(uiState.hour || '09')}</select></div>
      <div class="field"><label>分钟</label><select id="job-form-daily-minute">${buildMinuteOptions(uiState.minute || '00')}</select></div>
    </div>
    <div class="row" id="job-form-interval-row">
      <div class="field"><label>间隔</label><select id="job-form-interval-preset">${buildIntervalPresetOptions(uiState.intervalPreset || '1h')}</select></div>
    </div>
    <div class="row">
      <div class="field"><label>start_at</label><input id="job-form-start-at" value="${escapeHtml(job.start_at || '')}" placeholder="2026-06-04T09:00:00+08:00" /></div>
      <div class="field"><label>end_at</label><input id="job-form-end-at" value="${escapeHtml(job.end_at || '')}" placeholder="可选" /></div>
    </div>
    <div class="row">
      <div class="field">
        <label>webhook 配置</label>
        <select id="job-form-webhook-config-id">${buildWebhookReferenceOptions(job.webhook_config_id || '')}</select>
        <div id="job-form-webhook-auto-hint" class="subtitle">${escapeHtml(webhookHint)}</div>
      </div>
      <label class="field" style="max-width:220px">
        <span>启用推送引用</span>
        <input id="job-form-webhook-enabled" type="checkbox" ${job.webhook_config_id && job.webhook_enabled ? 'checked' : ''} />
      </label>
    </div>
  `;
  qs('#job-form-webhook-config-id')?.addEventListener('change', () => {
    syncWebhookReferenceEnabledState('#job-form-webhook-config-id', '#job-form-webhook-enabled', { autoEnable: true });
    renderWebhookReferenceHint('#job-form-webhook-config-id', '#job-form-webhook-auto-hint');
  });
  toggleAdvancedJobEditorScheduleFields();
  qs('#job-form-time-mode')?.addEventListener('change', () => toggleAdvancedJobEditorScheduleFields());
}

async function saveAdvancedJobEditor() {
  try {
    let payload;
    if (window.__jobEditorMode === 'json') {
      payload = JSON.parse(qs('#job-editor-json').value || '{}');
    } else {
      payload = cloneValue(window.__jobEditorCurrent || {});
      payload.id = qs('#job-form-id').value.trim();
      {
        const targetValue = qs('#job-form-target').value.trim().split(':');
        payload.target_type = targetValue[0] || 'task_draft';
        payload.target_id = targetValue[1] || '';
        payload.draft_id = payload.target_type === 'task_draft' ? payload.target_id : '';
      }
      payload.enabled = qs('#job-form-enabled').value === 'true';
      payload.start_at = qs('#job-form-start-at').value.trim();
      payload.end_at = qs('#job-form-end-at').value.trim();
      Object.assign(payload, buildSchedulePayloadFromUI(payload, {
        mode: '#job-form-time-mode',
        hour: '#job-form-daily-hour',
        minute: '#job-form-daily-minute',
        interval: '#job-form-interval-preset',
      }));
      Object.assign(payload, getWebhookReferencePayload('#job-form-webhook-config-id', '#job-form-webhook-enabled'));
    }
    if (window.__jobEditorIsCreate) {
      await fetchJSON('/api/v1/job-schedules', { method: 'POST', body: JSON.stringify(payload) });
    } else {
      await fetchJSON(`/api/v1/job-schedules/${encodeURIComponent(window.__jobEditorOriginalName || payload.id)}`, { method: 'PUT', body: JSON.stringify(payload) });
    }
    toast('保存成功', payload.id || '调度已保存', 'ok');
    setView('tasks');
    switchTaskSubpage('schedule');
    await refreshJobs(true);
    if (payload.id) loadJobDetail(payload.id);
  } catch (e) {
    toast('保存失败', String(e), '');
  }
}

function buildDraftOptions(selectedID) {
  return (window.__taskDraftCache || []).map(d =>
    `<option value="${escapeHtml(d.id || '')}" ${(d.id || '') === selectedID ? 'selected' : ''}>${escapeHtml(d.id || '')}${d.name ? ` · ${escapeHtml(d.name)}` : ''}</option>`
  ).join('');
}

function buildScheduleTargetOptions(selectedType, selectedID) {
  const parts = [];
  (window.__taskDraftCache || []).forEach(d => {
    const value = `task_draft:${d.id || ''}`;
    const selected = selectedType === 'task_draft' && (d.id || '') === selectedID ? 'selected' : '';
    parts.push(`<option value="${escapeHtml(value)}" ${selected}>任务草稿 · ${escapeHtml(d.id || '')}${d.name ? ` · ${escapeHtml(d.name)}` : ''}</option>`);
  });
  (window.__complexTaskCache || []).forEach(t => {
    const value = `complex_task:${t.id || ''}`;
    const selected = selectedType === 'complex_task' && (t.id || '') === selectedID ? 'selected' : '';
    parts.push(`<option value="${escapeHtml(value)}" ${selected}>运营agent · ${escapeHtml(t.id || '')}${t.name ? ` · ${escapeHtml(t.name)}` : ''}</option>`);
  });
  return parts.join('');
}

function cloneValue(v) {
  return JSON.parse(JSON.stringify(v || {}));
}

function createEmptyComplexTask() {
  return {
    id: '',
    name: '',
    goal: '',
    execution_mode: 'workflow',
    webhook_config_id: '',
    webhook_enabled: false,
    steps: []
  };
}

function createComplexTaskStep(type) {
  const suffix = Math.random().toString(36).slice(2, 8);
  const base = {
    id: `step_${Date.now()}_${suffix}`,
    type,
    name: type
  };
  if (type === 'api_call') {
    base.name = 'API 查询';
    base.config = { template_id: qs('#exec-template')?.value.trim() || '', target_type: '', target_id: '', query: {}, path_params: {}, body: {} };
  } else if (type === 'data_transform') {
    base.name = '数据处理';
    base.config = { type: 'json_map', rules: [] };
  } else if (type === 'llm_inference') {
    base.name = '大模型调用';
    base.config = {
      role: 'planner',
      provider: 'deepseek',
      prompt: '请从安全分析角度总结结果，并指出风险与建议',
      prompt_text: '',
      skill_text: '',
      skill_file: '',
      soul_text: '',
      soul_file: ''
    };
  } else if (type === 'output') {
    base.name = '输出';
    base.config = { format: 'markdown', title: '结果摘要' };
  } else {
    base.config = {};
  }
  return base;
}

function syncCurrentComplexTaskFromForm() {
  const task = window.__currentComplexTask || createEmptyComplexTask();
  task.id = qs('#complex-task-id')?.value.trim() || '';
  task.name = qs('#complex-task-name')?.value.trim() || '';
  task.goal = qs('#complex-task-goal')?.value.trim() || '';
  task.execution_mode = qs('#complex-task-mode')?.value || 'workflow';
  Object.assign(task, getWebhookReferencePayload('#complex-task-webhook-config-id', '#complex-task-webhook-enabled'));
  if (!Array.isArray(task.steps)) task.steps = [];
  window.__currentComplexTask = task;
  return task;
}

function renderComplexTaskEditor(task) {
  const current = cloneValue(task);
  if (!Array.isArray(current.steps)) current.steps = [];
  window.__currentComplexTask = current;
  window.__currentComplexTaskID = current.id || '';
  window.__currentComplexRunPayload = null;
  qs('#complex-task-id').value = current.id || '';
  qs('#complex-task-name').value = current.name || '';
  qs('#complex-task-goal').value = current.goal || '';
  qs('#complex-task-mode').value = current.execution_mode || 'workflow';
  fillWebhookReferenceFields('#complex-task-webhook-config-id', '#complex-task-webhook-enabled', current);
  if (qs('#complex-task-run-out')) qs('#complex-task-run-out').textContent = '暂无运行结果';
  renderComplexTaskSteps();
  renderComplexTaskConfig();
}

function renderComplexTaskSteps() {
  const box = qs('#complex-task-steps');
  const task = syncCurrentComplexTaskFromForm();
  if (!task.steps.length) {
    box.innerHTML = '[]';
    return;
  }
  box.innerHTML = task.steps.map((step, idx) => `
    <div class="step-node ${window.__currentComplexStepID === step.id ? 'step-node-active' : ''}" data-step-card="true" data-id="${escapeHtml(step.id || '')}" draggable="true">
      <div class="step-node-head">
        <div class="step-node-meta">
          <span class="step-index">${idx + 1}</span>
          <div>
            <div class="step-node-title">${escapeHtml(step.name || step.type || '')}</div>
            <div class="subtitle"><code>${escapeHtml(step.id || '')}</code> · <span class="step-type-badge">${escapeHtml(step.type || '')}</span></div>
          </div>
        </div>
        <div class="row">
        <button class="btn" data-act="up" data-id="${escapeHtml(step.id || '')}">上移</button>
        <button class="btn" data-act="down" data-id="${escapeHtml(step.id || '')}">下移</button>
        <button class="btn" data-act="copy" data-id="${escapeHtml(step.id || '')}">复制</button>
        <button class="btn ${window.__currentComplexStepID === step.id ? 'primary' : ''}" data-act="focus" data-id="${escapeHtml(step.id || '')}">查看</button>
        <button class="btn danger" data-act="remove" data-id="${escapeHtml(step.id || '')}">删除</button>
        </div>
      </div>
      <div class="step-node-foot">
        <span class="step-chip">${escapeHtml(getStepTypeLabel(step.type || ''))}</span>
        <span class="step-chip">拖拽排序</span>
      </div>
    </div>
  `).join('');
  qsa('#complex-task-steps [data-act="focus"]').forEach(btn => btn.addEventListener('click', () => {
    if (!persistCurrentComplexStepEditor()) return;
    window.__currentComplexStepID = btn.dataset.id || '';
    renderComplexTaskConfig();
    renderComplexTaskSteps();
  }));
  qsa('#complex-task-steps [data-act="up"]').forEach(btn => btn.addEventListener('click', () => {
    if (!persistCurrentComplexStepEditor()) return;
    moveComplexTaskStep(btn.dataset.id || '', -1);
  }));
  qsa('#complex-task-steps [data-act="down"]').forEach(btn => btn.addEventListener('click', () => {
    if (!persistCurrentComplexStepEditor()) return;
    moveComplexTaskStep(btn.dataset.id || '', 1);
  }));
  qsa('#complex-task-steps [data-act="copy"]').forEach(btn => btn.addEventListener('click', () => {
    if (!persistCurrentComplexStepEditor()) return;
    duplicateComplexTaskStep(btn.dataset.id || '');
  }));
  qsa('#complex-task-steps [data-act="remove"]').forEach(btn => btn.addEventListener('click', () => {
    if (!persistCurrentComplexStepEditor()) return;
    const id = btn.dataset.id || '';
    const currentTask = syncCurrentComplexTaskFromForm();
    currentTask.steps = (currentTask.steps || []).filter(step => step.id !== id);
    if (window.__currentComplexStepID === id) window.__currentComplexStepID = '';
    renderComplexTaskSteps();
    renderComplexTaskConfig();
  }));
  qsa('#complex-task-steps [data-step-card="true"]').forEach(card => {
    card.addEventListener('dragstart', (evt) => {
      evt.dataTransfer?.setData('text/plain', card.dataset.id || '');
      card.style.opacity = '.45';
    });
    card.addEventListener('dragend', () => {
      card.style.opacity = '1';
    });
    card.addEventListener('dragover', (evt) => {
      evt.preventDefault();
      card.classList.add('step-node-drop');
    });
    card.addEventListener('dragleave', () => {
      card.classList.remove('step-node-drop');
    });
    card.addEventListener('drop', (evt) => {
      evt.preventDefault();
      card.classList.remove('step-node-drop');
      if (!persistCurrentComplexStepEditor()) return;
      const fromID = evt.dataTransfer?.getData('text/plain') || '';
      const toID = card.dataset.id || '';
      moveComplexTaskStepTo(fromID, toID);
    });
  });
}

function renderComplexTaskConfig() {
  const box = qs('#complex-task-config');
  const task = syncCurrentComplexTaskFromForm();
  const selected = (task.steps || []).find(step => step.id === window.__currentComplexStepID);
  if (!selected) {
    box.innerHTML = `
      <div class="subtitle">当前未选中步骤，下面展示任务整体结构。</div>
      <div class="card" style="box-shadow:none; margin-top:10px">
        <div class="kv">
          <div class="k">Task ID</div><div class="v">${escapeHtml(task.id || '')}</div>
          <div class="k">Name</div><div class="v">${escapeHtml(task.name || '')}</div>
          <div class="k">Mode</div><div class="v">${escapeHtml(task.execution_mode || '')}</div>
          <div class="k">Steps</div><div class="v">${(task.steps || []).length}</div>
        </div>
      </div>
      <textarea id="complex-task-json-preview" rows="14" readonly>${escapeHtml(pretty(task || {}))}</textarea>
    `;
    return;
  }

  box.innerHTML = `
    <div class="subtitle">正在编辑步骤：<code>${escapeHtml(selected.id || '')}</code></div>
    <div class="field" style="margin-top:10px">
      <label>步骤名称</label>
      <input id="complex-step-name" value="${escapeHtml(selected.name || '')}" placeholder="例如 查询活跃用户" />
    </div>
    <div class="field">
      <label>步骤类型</label>
      <input id="complex-step-type" value="${escapeHtml(selected.type || '')}" readonly />
    </div>
    <div class="row">
      <button class="btn" id="btn-complex-step-fill">从当前工作台回填</button>
      <button class="btn" id="btn-complex-step-format">格式化 JSON</button>
    </div>
    <div class="field">
      <label>步骤配置(JSON)</label>
      <textarea id="complex-step-config-editor" rows="14">${escapeHtml(pretty(selected.config || {}))}</textarea>
    </div>
  `;
  qs('#complex-step-name')?.addEventListener('input', () => {
    const step = getCurrentComplexStep();
    if (!step) return;
    step.name = qs('#complex-step-name').value;
    renderComplexTaskSteps();
  });
  qs('#complex-step-config-editor')?.addEventListener('input', () => {
    syncComplexStepConfigPreview();
  });
  qs('#btn-complex-step-format')?.addEventListener('click', () => {
    try {
      const editor = qs('#complex-step-config-editor');
      editor.value = pretty(parseJSONOrEmpty(editor.value));
      persistCurrentComplexStepEditor();
      toast('运营agent', '步骤配置已格式化', 'ok');
    } catch (e) {
      toast('运营agent', `JSON 格式错误：${String(e)}`, '');
    }
  });
  qs('#btn-complex-step-fill')?.addEventListener('click', () => {
    fillSelectedComplexStepFromWorkbench();
  });
}

function getCurrentComplexStep() {
  const task = syncCurrentComplexTaskFromForm();
  return (task.steps || []).find(step => step.id === window.__currentComplexStepID);
}

function syncComplexStepConfigPreview() {
  const editor = qs('#complex-step-config-editor');
  if (!editor) return;
  const step = getCurrentComplexStep();
  if (!step) return;
  try {
    step.config = parseJSONOrEmpty(editor.value);
    editor.dataset.invalid = 'false';
  } catch {
    editor.dataset.invalid = 'true';
  }
}

function persistCurrentComplexStepEditor(showToast = true) {
  const step = getCurrentComplexStep();
  const nameInput = qs('#complex-step-name');
  const editor = qs('#complex-step-config-editor');
  if (!step || !editor) return true;
  if (nameInput) step.name = nameInput.value.trim();
  try {
    step.config = parseJSONOrEmpty(editor.value);
    editor.dataset.invalid = 'false';
    return true;
  } catch (e) {
    editor.dataset.invalid = 'true';
    if (showToast) toast('运营agent', `步骤配置 JSON 格式错误：${String(e)}`, '');
    return false;
  }
}

function fillSelectedComplexStepFromWorkbench() {
  const step = getCurrentComplexStep();
  if (!step) {
    toast('运营agent', '请先选择一个步骤', '');
    return;
  }
  if (step.type !== 'api_call') {
    toast('运营agent', '当前仅 API 查询步骤支持从工作台回填', '');
    return;
  }
  step.config = {
    template_id: qs('#exec-template')?.value.trim() || '',
    method: qs('#exec-method')?.value.trim() || '',
    path: qs('#exec-path')?.value.trim() || '',
    query: parseJSONOrEmpty(qs('#exec-query')?.value || ''),
    path_params: parseJSONOrEmpty(qs('#exec-path-params')?.value || ''),
    body: parseJSONOrEmpty(qs('#exec-body')?.value || '')
  };
  if (!step.name || step.name === 'API 查询' || step.name === 'api_call') {
    step.name = step.config.template_id ? `API 查询 · ${step.config.template_id}` : 'API 查询';
  }
  renderComplexTaskSteps();
  renderComplexTaskConfig();
  toast('运营agent', '已从当前工作台回填 API 步骤配置', 'ok');
}

function appendComplexTaskStep(type) {
  if (!persistCurrentComplexStepEditor()) return;
  const task = syncCurrentComplexTaskFromForm();
  task.steps = task.steps || [];
  const step = createComplexTaskStep(type);
  task.steps.push(step);
  window.__currentComplexStepID = step.id;
  renderComplexTaskSteps();
  renderComplexTaskConfig();
}

function moveComplexTaskStep(id, offset) {
  const task = syncCurrentComplexTaskFromForm();
  const steps = task.steps || [];
  const idx = steps.findIndex(step => step.id === id);
  if (idx < 0) return;
  const target = idx + offset;
  if (target < 0 || target >= steps.length) return;
  const tmp = steps[idx];
  steps[idx] = steps[target];
  steps[target] = tmp;
  window.__currentComplexTask.steps = steps;
  window.__currentComplexStepID = id;
  renderComplexTaskSteps();
  renderComplexTaskConfig();
}

function moveComplexTaskStepTo(fromID, toID) {
  if (!fromID || !toID || fromID === toID) return;
  const task = syncCurrentComplexTaskFromForm();
  const steps = task.steps || [];
  const fromIdx = steps.findIndex(step => step.id === fromID);
  const toIdx = steps.findIndex(step => step.id === toID);
  if (fromIdx < 0 || toIdx < 0) return;
  const [item] = steps.splice(fromIdx, 1);
  steps.splice(toIdx, 0, item);
  window.__currentComplexTask.steps = steps;
  window.__currentComplexStepID = fromID;
  renderComplexTaskSteps();
  renderComplexTaskConfig();
}

function duplicateComplexTaskStep(id) {
  const task = syncCurrentComplexTaskFromForm();
  const steps = task.steps || [];
  const idx = steps.findIndex(step => step.id === id);
  if (idx < 0) return;
  const source = cloneValue(steps[idx]);
  source.id = `step_${Date.now()}_${Math.random().toString(36).slice(2, 8)}`;
  source.name = source.name ? `${source.name} 副本` : `${source.type || 'step'} 副本`;
  steps.splice(idx + 1, 0, source);
  window.__currentComplexTask.steps = steps;
  window.__currentComplexStepID = source.id;
  renderComplexTaskSteps();
  renderComplexTaskConfig();
  toast('运营agent', '步骤已复制', 'ok');
}

async function refreshComplexTasks(quiet) {
  try {
    const data = await fetchJSON('/api/v1/complex-tasks');
    window.__complexTaskCache = data.items || [];
    renderComplexTaskList();
    if (!quiet) toast('运营agent', '已刷新列表', 'ok');
  } catch (e) {
    toast('运营agent', String(e), '');
  }
}

async function refreshOutputTemplates(quiet) {
  try {
    const data = await fetchJSON('/api/v1/output-templates');
    window.__outputTemplateCache = data.items || [];
    renderOutputTemplateList();
    if (!quiet) toast('输出模板', '已刷新输出模板列表', 'ok');
  } catch (e) {
    toast('输出模板', String(e), '');
  }
}

function renderOutputTemplateList() {
  const box = qs('#output-template-list');
  if (!box) return;
  const items = window.__outputTemplateCache || [];
  if (!items.length) {
    box.innerHTML = '暂无输出模板';
    return;
  }
  box.innerHTML = items.map(item => `
    <div class="card" style="box-shadow:none; margin-top:10px">
      <div class="row" style="justify-content:space-between; align-items:flex-start">
        <div>
          <div style="font-weight:750">${escapeHtml(item.name || item.id || '')}</div>
          <div class="subtitle"><code>${escapeHtml(item.id || '')}</code> · ${escapeHtml(item.format || '')}</div>
          <div class="subtitle">${escapeHtml(item.title || '')}</div>
        </div>
        <div class="row">
          <button class="btn" data-act="apply" data-id="${escapeHtml(item.id || '')}">应用到输出步骤</button>
          <button class="btn danger" data-act="delete" data-id="${escapeHtml(item.id || '')}">删除</button>
        </div>
      </div>
    </div>
  `).join('');
  qsa('#output-template-list [data-act="apply"]').forEach(btn => btn.addEventListener('click', () => {
    applyOutputTemplateToCurrentTask(btn.dataset.id || '');
  }));
  qsa('#output-template-list [data-act="delete"]').forEach(btn => btn.addEventListener('click', async () => {
    const id = btn.dataset.id || '';
    if (!confirm(`确认删除输出模板 ${id}？`)) return;
    try {
      await fetchJSON(`/api/v1/output-templates/${encodeURIComponent(id)}`, { method: 'DELETE' });
      toast('输出模板', '删除成功', 'ok');
      refreshOutputTemplates(true);
    } catch (e) {
      toast('输出模板删除失败', String(e), '');
    }
  }));
}

function applyOutputTemplateToCurrentTask(id) {
  const tpl = (window.__outputTemplateCache || []).find(it => it.id === id);
  if (!tpl) return;
  const task = syncCurrentComplexTaskFromForm();
  task.steps = task.steps || [];
  let step = (task.steps || []).find(it => it.type === 'output');
  if (!step) {
    step = createComplexTaskStep('output');
    task.steps.push(step);
  }
  step.name = tpl.name || step.name;
  step.config = cloneValue(tpl.output_config || {});
  if (tpl.format) step.config.format = tpl.format;
  if (tpl.title) step.config.title = tpl.title;
  if (tpl.content) step.config.content_template = tpl.content;
  window.__currentComplexStepID = step.id;
  renderComplexTaskSteps();
  renderComplexTaskConfig();
  toast('输出模板', `已应用 ${id}`, 'ok');
}

function renderComplexTaskList() {
  const box = qs('#complex-task-list');
  const items = window.__complexTaskCache || [];
  if (!items.length) {
    box.innerHTML = '暂无运营agent';
    return;
  }
  box.innerHTML = items.map(task => `
    <div class="card" style="box-shadow:none; margin-top:10px">
      <div class="row" style="justify-content:space-between; align-items:flex-start">
        <div>
          <div style="font-weight:750">${escapeHtml(task.name || task.id || '')}</div>
          <div class="subtitle"><code>${escapeHtml(task.id || '')}</code> · ${escapeHtml(task.execution_mode || '')}</div>
          <div class="subtitle">steps=${(task.steps || []).length}</div>
          <div class="subtitle">webhook=${escapeHtml(task.webhook_config_id || '未绑定')} · push=${task.webhook_enabled ? 'on' : 'off'}</div>
        </div>
        <div class="row">
          <button class="btn" data-act="load" data-id="${escapeHtml(task.id || '')}">加载</button>
          <button class="btn danger" data-act="delete" data-id="${escapeHtml(task.id || '')}">删除</button>
        </div>
      </div>
    </div>
  `).join('');
  qsa('#complex-task-list [data-act="load"]').forEach(btn => btn.addEventListener('click', () => loadComplexTask(btn.dataset.id || '')));
  qsa('#complex-task-list [data-act="delete"]').forEach(btn => btn.addEventListener('click', async () => {
    const id = btn.dataset.id || '';
    if (!confirm(`确认删除运营agent ${id}？`)) return;
    try {
      await fetchJSON(`/api/v1/complex-tasks/${encodeURIComponent(id)}`, { method: 'DELETE' });
      if (window.__currentComplexTaskID === id) {
        window.__currentComplexTaskID = '';
        window.__currentComplexStepID = '';
        renderComplexTaskEditor(createEmptyComplexTask());
      }
      toast('删除成功', id, 'ok');
      refreshComplexTasks(true);
    } catch (e) {
      toast('删除失败', String(e), '');
    }
  }));
}

async function loadComplexTask(id) {
  try {
    const payload = await fetchJSON(`/api/v1/complex-tasks/${encodeURIComponent(id)}`);
    renderComplexTaskEditor(payload.task || createEmptyComplexTask());
    toast('运营agent', `已加载 ${id}`, 'ok');
  } catch (e) {
    toast('运营agent', String(e), '');
  }
}

async function saveComplexTask() {
  try {
    if (!persistCurrentComplexStepEditor()) return;
    const payload = syncCurrentComplexTaskFromForm();
    const exists = (window.__complexTaskCache || []).some(it => it.id === payload.id);
    if (exists) {
      await fetchJSON(`/api/v1/complex-tasks/${encodeURIComponent(payload.id)}`, { method: 'PUT', body: JSON.stringify(payload) });
    } else {
      await fetchJSON('/api/v1/complex-tasks', { method: 'POST', body: JSON.stringify(payload) });
    }
    window.__currentComplexTaskID = payload.id;
    toast('运营agent', exists ? '运营agent 已更新' : '运营agent 已创建', 'ok');
    await refreshComplexTasks(true);
  } catch (e) {
    toast('运营agent保存失败', String(e), '');
  }
}

async function runComplexTask() {
  try {
    if (!persistCurrentComplexStepEditor()) return;
    const payload = syncCurrentComplexTaskFromForm();
    if (!payload.name) {
      toast('运营agent', '请先填写任务名称再试运行', '');
      return;
    }
    const data = await fetchJSON('/api/v1/complex-tasks/run', {
      method: 'POST',
      body: JSON.stringify(payload)
    });
    window.__currentComplexRunPayload = data;
    renderComplexTaskRunOutput(data);
    toast('运营agent', data.ok ? '试运行完成' : '试运行已返回错误结果', data.ok ? 'ok' : '');
  } catch (e) {
    window.__currentComplexRunPayload = null;
    qs('#complex-task-run-out').textContent = String(e);
    toast('运营agent试运行失败', String(e), '');
  }
}

function renderComplexTaskRunOutput(payload) {
  const steps = payload.steps || [];
  const summary = {
    ok: !!payload.ok,
    started_at: payload.started_at || '',
    finished_at: payload.finished_at || '',
    error: payload.error || '',
    final_output: payload.final_output
  };
  const host = qs('#complex-task-run-out');
  host.innerHTML = `
    <div class="card status-card ${summary.ok ? 'status-card-ok' : 'status-card-bad'}" style="box-shadow:none; margin-bottom:12px">
      <div class="row" style="justify-content:space-between">
        <div style="font-weight:800">运行摘要</div>
        <span class="pill ${summary.ok ? 'ok' : 'bad'}">${summary.ok ? 'SUCCESS' : 'FAILED'}</span>
      </div>
      <div class="status-strip ${summary.ok ? 'status-strip-ok' : 'status-strip-bad'}">
        <span>Started: ${escapeHtml(summary.started_at || '')}</span>
        <span>Finished: ${escapeHtml(summary.finished_at || '')}</span>
        <span>Steps: ${steps.length}</span>
      </div>
      ${summary.error
        ? `<div class="alert-block alert-error"><div class="alert-title">运行错误</div><div>${escapeHtml(summary.error || '')}</div></div>`
        : `<div class="alert-block alert-success"><div class="alert-title">运行状态</div><div>本次运营agent 试运行未返回整体错误，已完成主链路处理。</div></div>`}
      <div class="kv" style="margin-bottom:0">
        <div class="k">Started</div><div class="v">${escapeHtml(summary.started_at || '')}</div>
        <div class="k">Finished</div><div class="v">${escapeHtml(summary.finished_at || '')}</div>
        <div class="k">Steps</div><div class="v">${steps.length}</div>
        <div class="k">Error</div><div class="v">${escapeHtml(summary.error || '')}</div>
      </div>
    </div>
    <div style="font-weight:800; margin-bottom:8px">步骤结果</div>
    ${steps.length ? steps.map((step, idx) => `
      <div class="card status-card ${step.ok ? 'status-card-ok' : 'status-card-bad'}" style="box-shadow:none; margin-bottom:10px">
        <div class="row" style="justify-content:space-between">
          <div>
            <div style="font-weight:750">${idx + 1}. ${escapeHtml(step.name || step.type || '')}</div>
            <div class="subtitle"><code>${escapeHtml(step.step_id || '')}</code> · ${escapeHtml(step.type || '')}</div>
          </div>
          <span class="pill ${step.ok ? 'ok' : 'bad'}">${step.ok ? 'OK' : 'ERROR'}</span>
        </div>
        <div class="status-strip ${step.ok ? 'status-strip-ok' : 'status-strip-bad'}">
          <span>Step: ${escapeHtml(step.step_id || '')}</span>
          <span>Type: ${escapeHtml(step.type || '')}</span>
          <span>Status: ${step.ok ? 'OK' : 'ERROR'}</span>
        </div>
        ${step.error ? `<div class="alert-block alert-error"><div class="alert-title">步骤错误</div><div>${escapeHtml(step.error)}</div></div>` : `<div class="alert-block alert-success"><div class="alert-title">步骤状态</div><div>该步骤运行完成，输出已写入下方结果区。</div></div>`}
        <pre class="pre" style="max-height:220px; margin-top:10px">${escapeHtml(pretty(step.output ?? null))}</pre>
      </div>
    `).join('') : '<div class="subtitle">暂无步骤结果</div>'}
    <div style="font-weight:800; margin:10px 0 8px">最终输出</div>
    <pre class="pre" style="max-height:240px">${escapeHtml(pretty(summary.final_output))}</pre>
  `;
}

function getStepTypeLabel(type) {
  const map = {
    api_call: '数据接入',
    data_transform: '数据处理',
    llm_inference: '智能推理',
    output: '结果输出'
  };
  return map[type] || type || '步骤';
}

function buildDraftPayloadFromComplexTask(task, runPayload) {
  const apiStep = (task.steps || []).find(step => step.type === 'api_call') || {};
  const transformStep = (task.steps || []).find(step => step.type === 'data_transform') || {};
  const llmStep = (task.steps || []).find(step => step.type === 'llm_inference') || {};
  const outputStep = (task.steps || []).find(step => step.type === 'output') || {};
  const id = `draft_${(task.id || task.name || 'complex').replace(/[^a-zA-Z0-9_]+/g, '_').toLowerCase()}`;
  return {
    id,
    name: `${task.name || task.id || '运营agent'} 草稿`,
    mode: 'workflow',
    webhook_config_id: task.webhook_config_id || '',
    webhook_enabled: !!(task.webhook_config_id && task.webhook_enabled),
    source_template_id: (apiStep.config || {}).template_id || '',
    input_config: cloneValue(apiStep.config || {}),
    transform_config: cloneValue(transformStep.config || {}),
    llm_config: cloneValue(llmStep.config || {}),
    output_config: {
      ...(cloneValue(outputStep.config || {})),
      final_output_example: runPayload?.final_output || null
    }
  };
}

async function saveComplexTaskAsDraft() {
  try {
    if (!persistCurrentComplexStepEditor()) return;
    const task = syncCurrentComplexTaskFromForm();
    const payload = buildDraftPayloadFromComplexTask(task, window.__currentComplexRunPayload);
    const exists = (window.__taskDraftCache || []).some(it => it.id === payload.id);
    if (exists) {
      await fetchJSON(`/api/v1/task-drafts/${encodeURIComponent(payload.id)}`, { method: 'PUT', body: JSON.stringify(payload) });
    } else {
      await fetchJSON('/api/v1/task-drafts', { method: 'POST', body: JSON.stringify(payload) });
    }
    await refreshTaskDrafts(true);
    toast('任务草稿', exists ? '已更新任务草稿' : '已保存为任务草稿', 'ok');
  } catch (e) {
    toast('保存任务草稿失败', String(e), '');
  }
}

function buildOutputTemplatePayloadFromRun(task, runPayload) {
  if (!runPayload) return null;
  const outputStep = (task.steps || []).find(step => step.type === 'output') || {};
  const format = (outputStep.config || {}).format || (typeof runPayload.final_output === 'string' ? 'text' : 'json');
  const title = (outputStep.config || {}).title || `${task.name || task.id || '运营agent'} 输出`;
  const id = `out_${(task.id || task.name || 'complex').replace(/[^a-zA-Z0-9_]+/g, '_').toLowerCase()}`;
  return {
    id,
    name: `${task.name || task.id || '运营agent'} 输出模板`,
    format,
    title,
    content: typeof runPayload.final_output === 'string' ? runPayload.final_output : pretty(runPayload.final_output),
    source_complex_task_id: task.id || '',
    output_config: cloneValue(outputStep.config || {}),
    meta: {
      steps: (runPayload.steps || []).length,
      ok: !!runPayload.ok
    }
  };
}

async function saveComplexRunAsOutputTemplate() {
  try {
    if (!persistCurrentComplexStepEditor()) return;
    const task = syncCurrentComplexTaskFromForm();
    const payload = buildOutputTemplatePayloadFromRun(task, window.__currentComplexRunPayload);
    if (!payload) {
      toast('输出模板', '请先试运行一次，再保存输出模板', '');
      return;
    }
    await fetchJSON('/api/v1/output-templates', { method: 'POST', body: JSON.stringify(payload) });
    await refreshOutputTemplates(true);
    toast('输出模板', '已保存输出模板', 'ok');
  } catch (e) {
    toast('保存输出模板失败', String(e), '');
  }
}

async function deleteCurrentComplexTask() {
  if (!persistCurrentComplexStepEditor(false)) return;
  const task = syncCurrentComplexTaskFromForm();
  if (!task.id) {
    toast('运营agent', '当前没有可删除的运营agent', '');
    return;
  }
  if (!confirm(`确认删除运营agent ${task.id}？`)) return;
  try {
    await fetchJSON(`/api/v1/complex-tasks/${encodeURIComponent(task.id)}`, { method: 'DELETE' });
    window.__currentComplexTaskID = '';
    window.__currentComplexStepID = '';
    renderComplexTaskEditor(createEmptyComplexTask());
    toast('删除成功', task.id, 'ok');
    refreshComplexTasks(true);
  } catch (e) {
    toast('删除失败', String(e), '');
  }
}

function openTemplateEditor(tpl, opts) {
  window.__templateEditorIsCreate = !!(opts && opts.isCreate);
  window.__templateEditorOriginalID = tpl.id || '';
  window.__templateEditorReturnView = (opts && opts.returnView) ? opts.returnView : 'templates';
  window.__templateEditorCurrent = cloneTemplate(tpl);
  switchTemplateEditorMode('form');
  renderTemplateEditorForm(window.__templateEditorCurrent);
  qs('#template-editor-json').value = pretty(window.__templateEditorCurrent);
  setView('template-editor');
}

function switchTemplateEditorMode(mode) {
  window.__templateEditorMode = mode;
  const form = qs('#template-editor-form');
  const json = qs('#template-editor-json');
  if (mode === 'json') {
    form.classList.add('hidden');
    json.classList.remove('hidden');
  } else {
    form.classList.remove('hidden');
    json.classList.add('hidden');
  }
}

function renderTemplateEditorForm(tpl) {
  const box = qs('#template-editor-form');
  box.innerHTML = `
    <div class="row">
      <div class="field"><label>id</label><input id="tpl-form-id" value="${escapeHtml(tpl.id || '')}" /></div>
      <div class="field"><label>name</label><input id="tpl-form-name" value="${escapeHtml(tpl.name || '')}" /></div>
    </div>
    <div class="row">
      <div class="field"><label>category</label><input id="tpl-form-category" value="${escapeHtml(tpl.category || '')}" /></div>
      <div class="field"><label>method</label><select id="tpl-form-method"><option value="GET" ${(tpl.method || '').toUpperCase()==='GET'?'selected':''}>GET</option><option value="POST" ${(tpl.method || '').toUpperCase()==='POST'?'selected':''}>POST</option><option value="PUT" ${(tpl.method || '').toUpperCase()==='PUT'?'selected':''}>PUT</option><option value="PATCH" ${(tpl.method || '').toUpperCase()==='PATCH'?'selected':''}>PATCH</option><option value="DELETE" ${(tpl.method || '').toUpperCase()==='DELETE'?'selected':''}>DELETE</option></select></div>
    </div>
    <div class="row">
      <div class="field"><label>path</label><input id="tpl-form-path" value="${escapeHtml(tpl.path || '')}" /></div>
      <div class="field"><label>dry_run_query_param</label><input id="tpl-form-dry-run" value="${escapeHtml(tpl.dry_run_query_param || '')}" /></div>
    </div>
    <div class="row">
      <div class="field"><label>query_schema (JSON)</label><textarea id="tpl-form-query" rows="6">${escapeHtml(pretty(tpl.query_schema || {}))}</textarea></div>
      <div class="field"><label>path_params_schema (JSON)</label><textarea id="tpl-form-path-params" rows="6">${escapeHtml(pretty(tpl.path_params_schema || {}))}</textarea></div>
    </div>
    <div class="row">
      <div class="field"><label>body_schema (JSON)</label><textarea id="tpl-form-body" rows="8">${escapeHtml(pretty(tpl.body_schema || {}))}</textarea></div>
    </div>
  `;
}

async function saveTemplateEditor() {
  try {
    let payload;
    if (window.__templateEditorMode === 'json') {
      payload = JSON.parse(qs('#template-editor-json').value || '{}');
    } else {
      payload = {
        id: qs('#tpl-form-id').value.trim(),
        name: qs('#tpl-form-name').value.trim(),
        category: qs('#tpl-form-category').value.trim(),
        method: qs('#tpl-form-method').value.trim(),
        path: qs('#tpl-form-path').value.trim(),
        dry_run_query_param: qs('#tpl-form-dry-run').value.trim(),
        query_schema: JSON.parse(qs('#tpl-form-query').value || '{}'),
        path_params_schema: JSON.parse(qs('#tpl-form-path-params').value || '{}'),
        body_schema: JSON.parse(qs('#tpl-form-body').value || '{}')
      };
    }
    if (window.__templateEditorIsCreate) {
      await fetchJSON('/api/v1/api/templates', { method: 'POST', body: JSON.stringify(payload) });
    } else {
      await fetchJSON(`/api/v1/api/templates/${encodeURIComponent(window.__templateEditorOriginalID || payload.id)}`, { method: 'PUT', body: JSON.stringify(payload) });
    }
    toast('模板保存成功', payload.id || '模板已保存', 'ok');
    const back = window.__templateEditorReturnView || 'templates';
    setView(back);
    await refreshTemplates(true);
    if (payload.id) {
      window.__currentTplId = payload.id;
      renderTemplatesTable();
      await loadTplDetailToTemplatesView(payload.id);
    }
  } catch (e) {
    toast('模板保存失败', String(e), '');
  }
}

function cloneTemplate(tpl) {
  return JSON.parse(JSON.stringify(tpl || {}));
}

function escapeHtml(s) {
  return String(s)
    .replaceAll('&', '&amp;')
    .replaceAll('<', '&lt;')
    .replaceAll('>', '&gt;')
    .replaceAll('"', '&quot;')
    .replaceAll("'", '&#039;');
}

// initial
refreshHealth(true);
refreshConnections();
refreshLLMAPIs();
refreshLLMAPIItems();
refreshWebhooks(true);
loadCurrentConnectionConfig(true, true);
loadCurrentLLMConfig(true, true);
setConnectionSubpage(window.__connSubpage || 'feilian');
refreshTaskDrafts(true);
refreshJobs(true);
refreshTemplates(true);
refreshComplexTasks(true);
refreshOutputTemplates(true);
renderComplexTaskEditor(createEmptyComplexTask());
