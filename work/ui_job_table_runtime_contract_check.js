const fs = require('fs');

const js = fs.readFileSync('internal/web/assets/ui/app.js', 'utf8');
const failures = [];

if (!js.includes('function renderJobsTable()')) {
  failures.push('missing renderJobsTable');
}

if (!js.includes("const targetId = j.target_id || j.draft_id || '';")) {
  failures.push('missing targetId declaration');
}

const forEachBlockRequired = [
  'jobs.forEach(j => {',
  "const targetId = j.target_id || j.draft_id || '';",
  '<td><code>${escapeHtml(targetId)}</code>',
];

let cursor = -1;
for (const token of forEachBlockRequired) {
  const next = js.indexOf(token, cursor + 1);
  if (next === -1) {
    failures.push(`missing token in order: ${token}`);
    break;
  }
  cursor = next;
}

if (failures.length) {
  console.error(failures.join('\n'));
  process.exit(1);
}

console.log('ui job table runtime contract ok');
