const fs = require('fs');

function read(path) {
  return fs.readFileSync(path, 'utf8');
}

function extractIDs(js) {
  const ids = new Set();

  // qs('#id').addEventListener(...)
  const reQS = /qs\(\s*['"]#([^'"]+)['"]\s*\)\s*(?:\?\.)?\.addEventListener\b/g;
  let m;
  while ((m = reQS.exec(js))) ids.add(m[1]);

  // viewTemplatesQ('#id')?.addEventListener(...)
  const reTpl = /viewTemplatesQ\(\s*['"]#([^'"]+)['"]\s*\)\s*(?:\?\.)?\.addEventListener\b/g;
  while ((m = reTpl.exec(js))) ids.add(m[1]);

  return Array.from(ids).sort();
}

try {
  const js = read('internal/web/assets/ui/app.js');
  const html = read('internal/web/assets/ui/index.html');

  const ids = extractIDs(js);
  const missing = ids.filter(id => !html.includes(`id="${id}"`));
  const legacyRefs = ['#conn-table'].filter(sel => js.includes(sel));
  const legacyHTML = ['id="conn-table"'].filter(marker => html.includes(marker));
  const requiredHTML = [
    'id="draft-webhook-config-id"',
    'id="draft-webhook-enabled"',
    'id="complex-task-webhook-config-id"',
    'id="complex-task-webhook-enabled"'
  ].filter(marker => !html.includes(marker));
  const requiredJS = [
    '/api/v1/settings/webhooks',
    'webhook_config_id',
    'webhook_enabled',
    'job-form-webhook-config-id',
    'job-drawer-webhook-config-id'
  ].filter(marker => !js.includes(marker));

  if (missing.length) {
    console.error('missing ids in internal/web/assets/ui/index.html:', missing);
    process.exit(1);
  }

  if (requiredHTML.length || requiredJS.length) {
    console.error('missing webhook reference bindings:', { requiredHTML, requiredJS });
    process.exit(1);
  }

  if (legacyRefs.length || legacyHTML.length) {
    console.error('legacy connection-center bindings still present:', { legacyRefs, legacyHTML });
    process.exit(1);
  }

  console.log(`ui bindings ok (${ids.length} ids)`);
} catch (e) {
  console.error('error:', e.message);
  process.exit(1);
}
