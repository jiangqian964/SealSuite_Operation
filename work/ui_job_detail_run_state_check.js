const fs = require('fs');

const js = fs.readFileSync('internal/web/assets/ui/app.js', 'utf8');
const failures = [];

if (!js.includes('function renderJobDetail(job, payloadDraft, run) {')) {
  failures.push('missing renderJobDetail');
}

if (!js.includes("const hasRun = !!(run && run.last_run && run.last_run !== '0001-01-01T00:00:00Z');")) {
  failures.push('missing zero-time guard for hasRun');
}

if (!js.includes("!hasRun ? 'N/A' : (run.last_ok ? 'SUCCESS' : 'FAILED')")) {
  failures.push('missing N/A fallback status rendering');
}

if (failures.length) {
  console.error(failures.join('\n'));
  process.exit(1);
}

console.log('ui job detail run state contract ok');
