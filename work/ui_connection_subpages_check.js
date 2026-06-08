const fs = require('fs');

const failures = [];

function readIfExists(file) {
  if (!fs.existsSync(file)) {
    failures.push(`missing file ${file}`);
    return '';
  }
  return fs.readFileSync(file, 'utf8');
}

function assertIncludes(label, content, expected) {
  expected.forEach((item) => {
    if (!content.includes(item)) failures.push(`missing ${label} ${item}`);
  });
}

const server = readIfExists('internal/web/server.go');
assertIncludes('route', server, [
  '/settings/llm/apis',
  '/settings/llm/apis/{id}/activate',
  '/settings/llm/apis/{id}',
  '/settings/webhooks',
  '/settings/webhooks/{id}',
  '/settings/webhooks/test',
]);

if (server.includes('/settings/llm/config-sets')) {
  failures.push('legacy llm config-set routes should not remain the primary API');
}

const html = readIfExists('internal/web/assets/ui/index.html');
assertIncludes('dom', html, [
  'id="conn-subnav"',
  'id="btn-sub-feilian"',
  'id="btn-sub-llm"',
  'id="btn-sub-webhook"',
  'id="subpage-feilian"',
  'id="subpage-llm"',
  'id="subpage-webhook"',
]);

const css = readIfExists('internal/web/assets/ui/styles.css');
assertIncludes('css', css, [
  '.conn-subnav',
  '.config-subpage-layout',
  '.config-record-list',
  '.config-record-item',
  '.config-record-item-active',
]);

const js = readIfExists('internal/web/assets/ui/app.js');
assertIncludes('js', js, [
  'function renderFeilianSubpage',
  'function openFeilianCreateMode',
  "qs('#btn-sub-feilian')",
  "qs('#btn-sub-llm')",
  "qs('#btn-sub-webhook')",
  'function getLLMAPIPayload',
  'function getWebhookPayload',
  'function renderWebhookSubpage',
  '/api/v1/settings/llm/apis',
  '/api/v1/settings/webhooks',
  '/api/v1/settings/webhooks/test',
]);

[
  'id="llm-api-id"',
  'id="llm-api-name"',
  'id="llm-api-tags"',
].forEach((marker) => {
  if (!html.includes(marker)) failures.push(`missing dom ${marker}`);
});

[
  'id="webhook-list"',
  'id="webhook-id"',
  'id="webhook-name"',
  'id="webhook-url"',
  'id="webhook-method"',
  'id="webhook-headers"',
  'id="webhook-body-template"',
  'id="webhook-test-payload"',
  'id="btn-webhook-save"',
  'id="btn-webhook-delete"',
  'id="btn-webhook-test"',
  'id="webhook-out"',
].forEach((marker) => {
  if (!html.includes(marker)) failures.push(`missing dom ${marker}`);
});

[
  'llm-set-id',
  'llm-set-name',
  'btn-llm-set-refresh',
  'btn-llm-set-create',
  'conn-form-llm-set',
].forEach((legacy) => {
  if (html.includes(legacy)) failures.push(`legacy dom should be removed ${legacy}`);
  if (js.includes(legacy)) failures.push(`legacy js should be removed ${legacy}`);
});

if (js.includes('/settings/llm/config-sets')) {
  failures.push('legacy llm config-set ui endpoint should be removed');
}

if (fs.existsSync('web/ui/index.html')) {
  const mirrorHTML = fs.readFileSync('web/ui/index.html', 'utf8');
  assertIncludes('mirror dom', mirrorHTML, [
    'id="conn-subnav"',
    'id="btn-sub-feilian"',
    'id="btn-sub-llm"',
    'id="btn-sub-webhook"',
    'id="subpage-feilian"',
    'id="subpage-llm"',
    'id="subpage-webhook"',
  ]);
}

if (fs.existsSync('web/ui/styles.css')) {
  const mirrorCSS = fs.readFileSync('web/ui/styles.css', 'utf8');
  assertIncludes('mirror css', mirrorCSS, [
    '.conn-subnav',
    '.config-subpage-layout',
    '.config-record-list',
    '.config-record-item',
    '.config-record-item-active',
  ]);
}

if (failures.length) {
  console.error(failures.join('\n'));
  process.exit(1);
}

console.log('connection subpages backend/dom/css ok');
