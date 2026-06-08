function qs(sel) { return document.querySelector(sel); }
function qsa(sel) { return Array.from(document.querySelectorAll(sel)); }

async function fetchJSON(url, opts) {
  const res = await fetch(url, Object.assign({
    headers: { 'Content-Type': 'application/json' }
  }, opts || {}));
  const text = await res.text();
  let json;
  try { json = JSON.parse(text); } catch { json = { raw: text }; }
  if (!res.ok) {
    throw new Error(`HTTP ${res.status}: ${text}`);
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
  qs(`#view-${name}`).classList.remove('hidden');
}

qsa('.navbtn').forEach(b => b.addEventListener('click', () => setView(b.dataset.view)));
qs('#btn-open-templates')?.addEventListener('click', () => setView('templates'));
qs('#btn-api-go-templates')?.addEventListener('click', () => setView('templates'));

function pretty(v) {
  if (typeof v === 'string') return v;
  return JSON.stringify(v, null, 2);
}

function escapeHtml(s) {
  return String(s)
    .replaceAll('&', '&amp;')
    .replaceAll('<', '&lt;')
    .replaceAll('>', '&gt;')
    .replaceAll('"', '&quot;')
    .replaceAll("'", '&#039;');
}

window.__tplCache = [];
window.__currentTplId = '';
window.__taskDraftCache = [];
window.__currentDraftID = '';
window.__pendingJobDraft = null;

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

function focusApiOut() {
  const box = qs('#api-out');
  if (!box) return;
  box.scrollIntoView({ behavior: 'smooth', block: 'center' });
  box.classList.add('flash');
  setTimeout(() => box.classList.remove('flash'), 1200);
}

function applyTemplateToAPITask(template) {
  const tpl = template || {};
  setView('api');
  if (qs('#exec-template')) qs('#exec-template').value = tpl.id || '';
  if (qs('#exec-method')) qs('#exec-method').value = tpl.method || '';
  if (qs('#exec-path')) qs('#exec-path').value = tpl.path || '';
  if (qs('#draft-id') && !qs('#draft-id').value.trim()) qs('#draft-id').value = `draft_${String(tpl.id || 'task').replace(/[^a-zA-Z0-9]+/g, '_').replace(/^_+|_+$/g, '').toLowerCase() || 'task'}`;
  if (qs('#draft-name') && !qs('#draft-name').value.trim()) qs('#draft-name').value = `${tpl.id || '飞连API任务'} 草稿`;
  focusAPITaskDefinitionArea();
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
        } catch (e) {
          writeTplOut({ error: String(e), id: t.id });
        }
      });
      tr.querySelector('[data-act="create-task"]')?.addEventListener('click', () => {
        applyTemplateToAPITask(t);
      });
      tbody.appendChild(tr);
    });
}

async function refreshTemplates(quiet) {
  const data = await fetchJSON('/api/v1/api/templates');
  window.__tplCache = data.templates || [];
  renderTemplatesTable();
  if (!quiet && window.__tplCache.length) {
    // keep detail in sync with current selection
    if (window.__currentTplId) loadTplDetailToTemplatesView(window.__currentTplId);
  }
}

async function initVersion() {
  try {
    const v = await fetchJSON('/api/v1/version');
    qs('#version').textContent = v.version ? `v${v.version}` : '';
  } catch { /* ignore */ }
}
initVersion();

qs('#btn-health').addEventListener('click', async () => {
  const data = await fetchJSON('/api/v1/health');
  qs('#health').textContent = JSON.stringify(data, null, 2);
});

qs('#btn-jobs').addEventListener('click', async () => {
  const data = await fetchJSON('/api/v1/jobs');
  qs('#jobs').textContent = JSON.stringify(data, null, 2);
});
qs('#btn-reload').addEventListener('click', async () => {
  const data = await fetchJSON('/api/v1/reload', { method: 'POST', body: '{}' });
  qs('#jobs').textContent = JSON.stringify(data, null, 2);
});

qs('#btn-templates-refresh')?.addEventListener('click', async () => refreshTemplates(false));
viewTemplatesQ('#tpl-filter-2')?.addEventListener('input', () => renderTemplatesTable());
viewTemplatesQ('#btn-tpl-out-copy')?.addEventListener('click', async () => {
  await navigator.clipboard.writeText(viewTemplatesQ('#tpl-out')?.textContent || '');
});
viewTemplatesQ('#btn-tpl-out-clear')?.addEventListener('click', () => writeTplOut('暂无输出'));
qs('#btn-api-tasks-refresh')?.addEventListener('click', () => refreshTaskDrafts(false));
qs('#api-task-filter')?.addEventListener('input', () => renderAPITaskList());
qs('#btn-template-create-2')?.addEventListener('click', () => {
  // web/ui 简化版不包含模板编辑器；这里留空避免报错
  alert('当前版本未集成模板编辑器，请使用 internal 版本页面。');
});

async function refreshTaskDrafts(quiet) {
  try {
    const data = await fetchJSON('/api/v1/task-drafts');
    window.__taskDraftCache = data.items || [];
    renderAPITaskList();
  } catch (e) {
    if (!quiet) qs('#api-out').textContent = String(e);
  }
}

function clearTaskDraftWorkbench() {
  window.__currentDraftID = '';
  ['#draft-id', '#draft-name', '#exec-template', '#exec-method', '#exec-path', '#exec-query', '#exec-path-params', '#exec-body'].forEach(sel => {
    const node = qs(sel);
    if (node) node.value = '';
  });
  if (qs('#draft-mode')) qs('#draft-mode').value = 'api_only';
  renderAPITaskList();
}

function loadTaskDraftIntoWorkbench(draft) {
  window.__currentDraftID = draft.id || '';
  const input = draft.input_config || {};
  if (qs('#draft-id')) qs('#draft-id').value = draft.id || '';
  if (qs('#draft-name')) qs('#draft-name').value = draft.name || '';
  if (qs('#draft-mode')) qs('#draft-mode').value = draft.mode || 'api_only';
  if (qs('#exec-template')) qs('#exec-template').value = draft.source_template_id || input.template_id || '';
  if (qs('#exec-method')) qs('#exec-method').value = input.method || '';
  if (qs('#exec-path')) qs('#exec-path').value = input.path || '';
  if (qs('#exec-query')) qs('#exec-query').value = pretty(input.query || {});
  if (qs('#exec-path-params')) qs('#exec-path-params').value = pretty(input.path_params || {});
  if (qs('#exec-body')) qs('#exec-body').value = pretty(input.body || {});
  renderAPITaskList();
  focusAPITaskDefinitionArea();
}

async function deleteTaskDraftByID(id) {
  if (!id) return;
  if (!confirm(`确认删除任务草稿 ${id}？`)) return;
  await fetchJSON(`/api/v1/task-drafts/${encodeURIComponent(id)}`, { method: 'DELETE' });
  if (window.__currentDraftID === id) clearTaskDraftWorkbench();
  if ((window.__pendingJobDraft || {}).target_id === id) window.__pendingJobDraft = null;
  await refreshTaskDrafts(true);
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

function renderAPIError(title, err) {
  qs('#api-out').textContent = pretty({
    title,
    error: String(err)
  });
  focusApiOut();
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

function createScheduleFromTaskDraft(id) {
  const draft = (window.__taskDraftCache || []).find(it => it.id === id);
  if (!draft) {
    renderAPIError('创建周期任务失败', `未找到任务草稿 ${id}`);
    return;
  }
  loadTaskDraftIntoWorkbench(draft);
  window.__pendingJobDraft = {
    id: `job_${String(draft.id || draft.name || 'task').replace(/[^a-zA-Z0-9]+/g, '_').replace(/^_+|_+$/g, '').toLowerCase() || 'task'}`,
    target_type: 'task_draft',
    target_id: draft.id || '',
    draft_id: draft.id || '',
    enabled: false
  };
  setView('jobs');
  if (qs('#jobs')) {
    qs('#jobs').textContent = pretty({
      message: '当前页面未集成调度编辑器，已保存待创建调度预填数据。',
      pending_job_schedule: window.__pendingJobDraft
    });
  }
}

function renderAPITaskList() {
  const host = qs('#api-task-list');
  if (!host) return;
  const filter = (qs('#api-task-filter')?.value || '').trim().toLowerCase();
  const items = (window.__taskDraftCache || []).filter(d => {
    const hay = `${d.id || ''} ${d.name || ''} ${d.source_template_id || (d.input_config || {}).template_id || ''}`.toLowerCase();
    return !filter || hay.includes(filter);
  });
  if (!items.length) {
    host.className = 'subtitle';
    host.innerHTML = '<div class="subtitle">暂无任务草稿，请先在完整页面中保存第一条任务。</div>';
    return;
  }
  host.className = 'api-task-list';
  host.innerHTML = items.map(d => `
    <div class="api-task-item ${window.__currentDraftID === d.id ? 'api-task-item-active' : ''}" data-id="${escapeHtml(d.id || '')}">
      <div style="font-weight:750">${escapeHtml(d.name || d.id || '')}</div>
      <div class="subtitle"><code>${escapeHtml(d.id || '')}</code> · mode=${escapeHtml(d.mode || '')}</div>
      <div class="subtitle">template=${escapeHtml(d.source_template_id || (d.input_config || {}).template_id || '')}</div>
      <div class="api-task-actions">
        <button class="btn" data-act="test" data-id="${escapeHtml(d.id || '')}">测试</button>
        <button class="btn" data-act="schedule" data-id="${escapeHtml(d.id || '')}">创建周期任务</button>
        <button class="btn" data-act="edit" data-id="${escapeHtml(d.id || '')}">编辑</button>
        <button class="btn danger" data-act="delete" data-id="${escapeHtml(d.id || '')}">删除</button>
      </div>
    </div>
  `).join('');
  qsa('#api-task-list [data-act="edit"]').forEach(btn => btn.addEventListener('click', () => {
    const id = btn.dataset.id || '';
    const draft = (window.__taskDraftCache || []).find(it => it.id === id);
    if (!draft) return;
    loadTaskDraftIntoWorkbench(draft);
  }));
  qsa('#api-task-list [data-act="delete"]').forEach(btn => btn.addEventListener('click', async () => {
    try {
      await deleteTaskDraftByID(btn.dataset.id || '');
    } catch (e) {
      renderAPIError('删除任务草稿失败', e);
    }
  }));
  qsa('#api-task-list [data-act="test"]').forEach(btn => btn.addEventListener('click', async () => {
    try {
      await runTaskDraftFromList(btn.dataset.id || '');
    } catch (e) {
      renderAPIError('任务测试失败', e);
    }
  }));
  qsa('#api-task-list [data-act="schedule"]').forEach(btn => btn.addEventListener('click', () => {
    createScheduleFromTaskDraft(btn.dataset.id || '');
  }));
}

async function buildExecutePayload() {
  const template_id = qs('#exec-template').value.trim();
  const method = qs('#exec-method').value.trim();
  const path = qs('#exec-path').value.trim();
  const query = parseJSONOrEmpty(qs('#exec-query').value);
  const path_params = parseJSONOrEmpty(qs('#exec-path-params').value);
  const body = parseJSONOrEmpty(qs('#exec-body').value);
  const payload = { template_id, method, path, query, path_params, body };
  Object.keys(payload).forEach(k => {
    if (payload[k] && typeof payload[k] === 'string' && payload[k].length === 0) delete payload[k];
  });
  return payload;
}

qs('#btn-exec').addEventListener('click', async () => {
  try {
    const payload = await buildExecutePayload();
    const data = await fetchJSON('/api/v1/api/execute', { method: 'POST', body: JSON.stringify(payload) });
    qs('#api-out').textContent = pretty(data);
    focusApiOut();
  } catch (e) {
    renderAPIError('Execute 失败', e);
  }
});

qs('#btn-preview').addEventListener('click', async () => {
  try {
    const payload = await buildExecutePayload();
    const preview = Object.assign({}, payload, { preview_mode: 'diff_only', read_before_write: true });
    const data = await fetchJSON('/api/v1/api/preview', { method: 'POST', body: JSON.stringify(preview) });
    qs('#api-out').textContent = pretty(data);
    focusApiOut();
  } catch (e) {
    renderAPIError('Preview 失败', e);
  }
});

qs('#btn-logs').addEventListener('click', async () => {
  const tail = qs('#log-tail').value.trim() || '2000';
  const data = await fetchJSON(`/api/v1/logs?tail=${encodeURIComponent(tail)}`);
  qs('#logs').textContent = data.text || JSON.stringify(data, null, 2);
});

refreshTaskDrafts(true);
