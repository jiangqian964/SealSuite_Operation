const fs = require('fs');
const failures = [];
const delivery = fs.existsSync('internal/webhook/delivery.go')
  ? fs.readFileSync('internal/webhook/delivery.go', 'utf8')
  : '';
const store = fs.existsSync('internal/storage/webhook_store.go')
  ? fs.readFileSync('internal/storage/webhook_store.go', 'utf8')
  : '';
const html = fs.existsSync('internal/web/assets/ui/index.html')
  ? fs.readFileSync('internal/web/assets/ui/index.html', 'utf8')
  : '';
const app = fs.existsSync('internal/web/assets/ui/app.js')
  ? fs.readFileSync('internal/web/assets/ui/app.js', 'utf8')
  : '';
const server = fs.existsSync('internal/web/server.go')
  ? fs.readFileSync('internal/web/server.go', 'utf8')
  : '';
const runner = fs.existsSync('internal/runner/runner.go')
  ? fs.readFileSync('internal/runner/runner.go', 'utf8')
  : '';
[
  'type DeliveryResult struct',
  'func DeliverWebhookForSuccess',
  'event',
  'source_type',
  'webhook_delivery',
].forEach(k => { if (!delivery.includes(k)) failures.push(`missing ${k}`); });
if (!/func \(s \*?WebhookStore\) Get\(/.test(store)) failures.push('missing WebhookStore.Get');
[
  'id="exec-webhook-push-once"',
  'webhook_push_once',
  'webhook_delivery',
].forEach(k => {
  if (!html.includes(k) && !app.includes(k) && !server.includes(k)) failures.push(`missing ${k}`);
});
if (!runner.includes('runTaskDraft(d storage.TaskDraft) (interface{}, webhook.DeliveryResult, error)')) {
  failures.push('runner should return webhook delivery from runTaskDraft');
}
if (!runner.includes('deliverTaskDraftWebhook')) {
  failures.push('runner should deliver saved task draft webhook');
}
[
  'RunScheduleOnce(ctx context.Context, id string)',
  'deliverScheduleWebhook',
  'executeTargetOutputForSchedule',
  'SourceType: "job_schedule"',
].forEach(k => {
  if (!runner.includes(k)) failures.push(`missing ${k}`);
});
if (!server.includes('/job-schedules/{id}/run') || !server.includes('"webhook_delivery"')) {
  failures.push('server should return webhook_delivery for relevant saved task draft callers');
}
[
  '/complex-tasks/run',
  '/complex-tasks/{id}/run',
  'deliverWebhookFromConfig',
  'SourceType: "complex_task"',
  'complexTaskWebhookData',
].forEach(k => {
  if (!server.includes(k)) failures.push(`missing complex task webhook runtime contract: ${k}`);
});
if (failures.length) { console.error(failures.join('\n')); process.exit(1); }
console.log('webhook runtime helper ok');
