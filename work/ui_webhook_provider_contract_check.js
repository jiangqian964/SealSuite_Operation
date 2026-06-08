const fs = require('fs');

const html = fs.readFileSync('internal/web/assets/ui/index.html', 'utf8');
const js = fs.readFileSync('internal/web/assets/ui/app.js', 'utf8');
const failures = [];
const uiBundle = `${html}\n${js}`;

[
  'id="webhook-provider"',
  'id="webhook-provider-hint"',
].forEach((token) => {
  if (!html.includes(token)) failures.push(`missing html token: ${token}`);
});

[
  "provider: 'generic'",
  'function renderWebhookProviderState(',
  '系统将自动按飞书机器人格式生成 payload',
].forEach((token) => {
  if (!js.includes(token)) failures.push(`missing js token: ${token}`);
});

[
  'id="draft-webhook-auto-hint"',
  'id="complex-task-webhook-auto-hint"',
  'id="job-drawer-webhook-auto-hint"',
  'id="job-form-webhook-auto-hint"',
  '已按飞书机器人协议自动匹配 payload。',
  '当前 webhook 使用自定义 body_template。',
  'function renderWebhookReferenceHint(',
].forEach((token) => {
  if (!uiBundle.includes(token)) failures.push(`missing task hint token: ${token}`);
});

if (failures.length) {
  console.error(failures.join('\n'));
  process.exit(1);
}

console.log('ui webhook provider contract ok');
