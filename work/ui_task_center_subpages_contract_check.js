const fs = require('fs');

function read(file) {
  return fs.readFileSync(file, 'utf8');
}

function expectIncludes(content, tokens, label, failures) {
  tokens.forEach((token) => {
    if (!content.includes(token)) failures.push(`${label} missing ${token}`);
  });
}

function expectNotIncludes(content, tokens, label, failures) {
  tokens.forEach((token) => {
    if (content.includes(token)) failures.push(`${label} should not include ${token}`);
  });
}

try {
  const html = read('internal/web/assets/ui/index.html');
  const js = read('internal/web/assets/ui/app.js');
  const css = read('internal/web/assets/ui/styles.css');
  const failures = [];

  expectIncludes(
    html,
    [
      'id="conn-workbench"',
      'id="conn-workbench-type"',
      'id="conn-workbench-mode"',
      'id="conn-subnav"',
      'id="btn-sub-feilian"',
      'id="btn-sub-llm"',
      'id="btn-sub-webhook"',
      'id="subpage-feilian"',
      'id="subpage-llm"',
      'id="subpage-webhook"',
      'config-subpage-layout',
      'config-subpage-side',
      'config-subpage-detail',
      '飞连API配置',
      'LLM_API配置',
      'webhook配置',
    ],
    'html',
    failures
  );

  expectNotIncludes(
    html,
    [
      'class="card connection-workbench"',
      'connection-workbench-head',
      'connection-workbench-note',
      '运行时配置提示',
      'id="btn-conn-create-inline"',
      'id="btn-llm-api-create-inline"',
    ],
    'html',
    failures
  );

  expectIncludes(
    html,
    [
      '单次/周期任务清单',
      '定时任务清单',
      'id="task-page-title"',
      'id="task-page-subtitle"',
      'id="btn-task-sub-once-cycle"',
      'id="btn-task-sub-schedule"',
      'id="task-subpage-once-cycle"',
      'id="task-subpage-schedule"',
    ],
    'task html',
    failures
  );

  expectIncludes(
    html,
    [
      'id="draft-cycle-mode"',
      'id="draft-run-count"',
      'id="draft-run-until"',
      '周期执行',
      '任务次数',
      '结束时间',
      '<option value="once"',
      '<option value="5min"',
      '<option value="30min"',
      '<option value="1h"',
      '<option value="6h"',
      '<option value="24h"',
      '<option value="7day"',
      '<option value="1month"',
      '<option value="1year"',
    ],
    'task draft cycle html',
    failures
  );

  expectNotIncludes(
    html,
    [
      'task-center-tabs',
      'task-center-tab-once',
      'task-center-tab-schedule',
      'task-center-pane-once',
      'task-center-pane-schedule',
    ],
    'task html',
    failures
  );

  expectIncludes(
    js,
    [
      'function switchTaskSubpage(',
      'function openTaskCenterSubpage(',
      'function renderTaskSubpageHeader(',
      'function getTaskSubpageMeta(',
      "switchTaskSubpage('once-cycle')",
      "switchTaskSubpage('schedule')",
      "openTaskCenterSubpage('once-cycle')",
      "openTaskCenterSubpage('schedule')",
    ],
    'task js',
    failures
  );

  expectIncludes(
    js,
    [
      'function syncTaskDraftCycleFields(',
      "qs('#draft-cycle-mode')?.addEventListener('change'",
      "qs('#draft-run-count').disabled = isOnce",
      "qs('#draft-run-until').disabled = isOnce",
      "qs('#draft-run-count').value = '1'",
      "qs('#draft-run-until').value = ''",
      'function normalizeTaskDraftCycleMode(',
      'function buildTaskDraftCycleModeOptions(',
      'cycle_mode:',
      'run_count:',
      'run_until:',
      "draft.cycle_mode || 'once'",
      "Number(draft.run_count || 1)",
    ],
    'task draft cycle js',
    failures
  );

  expectNotIncludes(
    js,
    [
      'function switchTaskCenterTab(',
      'function openTaskCenter(',
      "switchTaskCenterTab('schedule')",
      "openTaskCenter('once')",
      "openTaskCenter('schedule')",
    ],
    'task js',
    failures
  );

  expectIncludes(
    css,
    [
      '.seg',
      '.conn-subnav',
      '.config-subpage-layout',
      '.config-subpage-side',
      '.config-subpage-detail',
      '.config-record-list',
      '.connection-form-shell',
      '#task-subpage-switcher',
      '.task-subpage-screen',
      '.task-subnav',
    ],
    'css',
    failures
  );

  expectNotIncludes(
    css,
    [
      '.connection-workbench',
      '.connection-workbench-head',
      '.connection-workbench-note',
      '.task-center-tabs',
      '.task-center-pane-schedule-shell',
      '#task-center-pane-schedule .jobs-layout',
      '.seg-tabs',
    ],
    'css',
    failures
  );

  if (failures.length) {
    console.error(failures.join('\n'));
    process.exit(1);
  }

  console.log('ui task center subpages contract ok');
} catch (err) {
  console.error(err.message);
  process.exit(1);
}
