const fs = require('fs');

function read(path) {
  return fs.readFileSync(path, 'utf8');
}

function assertIncludes(source, markers, label, failures) {
  markers.forEach(marker => {
    if (!source.includes(marker)) failures.push(`${label} missing ${marker}`);
  });
}

try {
  const failures = [];

  const internalHTML = read('internal/web/assets/ui/index.html');
  const internalJS = read('internal/web/assets/ui/app.js');
  const webHTML = read('web/ui/index.html');
  const webJS = read('web/ui/app.js');

  if (internalHTML.includes('模板入口')) failures.push('internal view-api should not show 模板入口 card');
  ['已保存任务草稿', 'id="btn-drafts-refresh"', 'id="task-draft-list"'].forEach(marker => {
    if (internalHTML.includes(marker)) failures.push(`internal view-api should remove legacy draft block marker: ${marker}`);
  });
  assertIncludes(
    internalHTML,
    ['id="api-task-list-card"', 'id="api-task-filter"', 'id="btn-api-tasks-refresh"', 'id="api-task-list"'],
    'internal html',
    failures
  );

  assertIncludes(
    internalJS,
    [
      'function renderAPITaskList',
      "qs('#btn-api-tasks-refresh')",
      "qs('#api-task-filter')",
      "qs('#api-task-list')",
      'data-act="edit"',
      'data-act="delete"',
      'data-act="test"',
      'data-act="schedule"',
      'loadTaskDraftIntoWorkbench',
      'function runTaskDraftFromList',
      'function createScheduleFromTaskDraft',
      'target_type: \'task_draft\''
    ],
    'internal js',
    failures
  );
  ['btn-drafts-refresh', '#task-draft-list', 'function renderTaskDraftList'].forEach(marker => {
    if (internalJS.includes(marker)) failures.push(`internal js should not depend on legacy draft UI: ${marker}`);
  });
  if (!/async function refreshTaskDrafts\(quiet\)[\s\S]*window\.__taskDraftCache = data\.items \|\| \[\];[\s\S]*renderAPITaskList\(\);/.test(internalJS)) {
    failures.push('internal refreshTaskDrafts should refresh cache and renderAPITaskList()');
  }

  if (webHTML.includes('id="api-task-list"')) {
    assertIncludes(
      webJS,
      [
        'function renderAPITaskList',
        "qs('#btn-api-tasks-refresh')",
        "qs('#api-task-filter')",
        "qs('#api-task-list')",
        'function runTaskDraftFromList',
        'function createScheduleFromTaskDraft'
      ],
      'web js',
      failures
    );
  }

  if (failures.length) {
    console.error(failures.join('\n'));
    process.exit(1);
  }

  console.log('api task list bindings ok');
} catch (e) {
  console.error(e.message);
  process.exit(1);
}
