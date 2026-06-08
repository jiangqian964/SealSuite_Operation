const fs = require('fs');

const server = fs.readFileSync('internal/web/server.go', 'utf8');
const html = fs.readFileSync('internal/web/assets/ui/index.html', 'utf8');
const js = fs.readFileSync('internal/web/assets/ui/app.js', 'utf8');
const failures = [];

[
  '/settings/llm/config-sets',
  '/settings/llm/config-sets/{id}/activate',
  '/settings/llm/config-sets/{id}',
].forEach((k) => {
  if (!server.includes(k)) failures.push(`missing route ${k}`);
});

if (!server.includes('apiR.Delete("/connections/{id}"')) {
  failures.push('missing DELETE /connections/{id}');
}

[
  'id="conn-center-layout"',
  'id="conn-list-feilian"',
  'id="conn-list-llm"',
  'id="btn-conn-create"',
  'id="btn-llm-set-create"',
  'id="conn-workbench"',
  'id="conn-workbench-type"',
  'id="conn-workbench-mode"',
  'id="conn-form-feilian"',
  'id="conn-form-llm-set"',
  'id="btn-conn-create-inline"',
  'id="btn-llm-set-create-inline"',
  'id="btn-conn-edit"',
  'id="btn-llm-edit"',
  'id="llm-set-id"',
  'id="llm-set-name"',
  'id="llm-set-activate"',
].forEach((k) => {
  if (!html.includes(k)) failures.push(`missing ${k}`);
});

[
  'id="conn-table"',
  '最近启用（最多 3 条）',
].forEach((k) => {
  if (html.includes(k)) failures.push(`legacy html still present: ${k}`);
});

[
  'window.__connCenterState',
  'function setConnectionWorkbenchMode',
  'function renderConnectionCenter',
  'function renderFeilianConfigList',
  'function openConnectionRecord',
  'function openConnectionCreateMode',
  'function openConnectionEditMode',
  'function refreshConnections',
  'function refreshLLMConfigSets',
  'function renderLLMConfigSetList',
  'function openLLMConfigSetRecord',
  'function openLLMConfigSetCreateMode',
  'function openLLMConfigSetEditMode',
  'function getLLMConfigSetPayload',
  '/api/v1/connections/',
  '/api/v1/settings/llm/config-sets',
].forEach((k) => {
  if (!js.includes(k)) failures.push(`missing js ${k}`);
});

[
  '#conn-table',
].forEach((k) => {
  if (js.includes(k)) failures.push(`legacy js reference still present: ${k}`);
});

if (failures.length) {
  console.error(failures.join('\n'));
  process.exit(1);
}

console.log('connection center routes, DOM, and JS helpers ok');
