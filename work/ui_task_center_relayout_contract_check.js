const fs = require('fs');

function read(path) {
  return fs.readFileSync(path, 'utf8');
}

function expectIncludes(source, tokens, label, failures) {
  tokens.forEach((token) => {
    if (!source.includes(token)) failures.push(`${label} missing ${token}`);
  });
}

function expectNotIncludes(source, tokens, label, failures) {
  tokens.forEach((token) => {
    if (source.includes(token)) failures.push(`${label} should not include ${token}`);
  });
}

function expectPattern(source, pattern, label, failures, description) {
  if (!pattern.test(source)) failures.push(`${label} missing pattern ${description}`);
}

try {
  const html = read('internal/web/assets/ui/index.html');
  const js = read('internal/web/assets/ui/app.js');
  const css = read('internal/web/assets/ui/styles.css');
  const failures = [];

  expectIncludes(
    html,
    [
      '飞连API列表',
      '飞连任务列表',
      '单次任务清单',
      '定时任务清单',
      'id="task-center-tabs"',
      'id="task-center-tab-once"',
      'id="task-center-tab-schedule"',
      'id="view-tasks"',
      'id="task-center-pane-schedule"',
      'id="jobs-table"',
      'id="job-detail"',
      'id="btn-jobs"',
      'id="btn-job-create"',
      'task-center-pane-schedule-shell',
    ],
    'html',
    failures
  );

  expectNotIncludes(html, ['id="view-jobs"'], 'html', failures);

  expectIncludes(
    js,
    [
      'function switchTaskCenterTab(',
      "setView('tasks')",
      'window.__taskCenterTab',
      'function renderAPITaskList(',
      'window.__taskDraftCache',
      'drafts.forEach(',
      'Array.isArray(data.items) ? data.items : []',
      "switchTaskCenterTab('schedule');",
    ],
    'js',
    failures
  );

  expectIncludes(
    `${html}\n${js}`,
    [
      '任务定时窗口',
      'job-drawer-time-mode',
      'job-drawer-daily-hour',
      'job-drawer-daily-minute',
      'job-drawer-interval-preset',
      '默认使用当前服务器时区',
      '每日定时',
      '固定间隔',
      '5min',
      '30min',
      '1h',
      '6h',
      '24h',
      '7day',
      '1month',
      '1year',
    ],
    'drawer schedule',
    failures
  );

  expectIncludes(
    js,
    [
      'function buildHourOptions(',
      'function buildMinuteOptions(',
      'function normalizeIntervalPreset(',
      'function inferScheduleUIState(',
      'function buildSchedulePayloadFromUI(',
      'job-drawer-daily-row',
      'job-drawer-interval-row',
      "payload.schedule_type = 'interval'",
      "payload.schedule_type = 'cron'",
      "payload.timezone = 'Asia/Shanghai'",
      "Object.assign(updated, buildSchedulePayloadFromUI(updated, {",
    ],
    'js schedule mapping',
    failures
  );

  expectNotIncludes(
    js,
    [
      'id="job-drawer-timezone"',
      'id="job-drawer-cron"',
      'id="job-drawer-interval"',
      "updated.schedule_type = qs('#job-drawer-type').value;",
      "updated.cron = qs('#job-drawer-cron').value.trim();",
      "updated.interval = qs('#job-drawer-interval').value.trim();",
      "updated.timezone = qs('#job-drawer-timezone').value.trim();",
    ],
    'js quick drawer legacy schedule fields',
    failures
  );

  expectPattern(
    js,
    /qs\('#job-drawer-time-mode'\)\?\.addEventListener\('change',\s*\(\)\s*=>\s*toggleJobDrawerScheduleFields\(\)\s*\);/,
    'js',
    failures,
    'job drawer schedule mode toggle binding'
  );

  expectPattern(
    js,
    /qs\('#btn-job-editor-back'\)\?\.addEventListener\('click',\s*\(\)\s*=>\s*\{\s*setView\('tasks'\);\s*switchTaskCenterTab\('schedule'\);\s*if\s*\(window\.__currentJobName\)\s*loadJobDetail\(window\.__currentJobName\);\s*\}\);/,
    'js',
    failures,
    "job-editor back handler with setView('tasks') + switchTaskCenterTab('schedule')"
  );

  expectIncludes(
    js,
    [
      'function renderAdvancedEditorForm(',
      '任务定时窗口',
      'job-form-time-mode',
      'job-form-daily-hour',
      'job-form-daily-minute',
      'job-form-interval-preset',
      '默认使用当前服务器时区',
      'function toggleAdvancedJobEditorScheduleFields(',
      'buildSchedulePayloadFromUI(payload, {',
      "mode: '#job-form-time-mode'",
      "hour: '#job-form-daily-hour'",
      "minute: '#job-form-daily-minute'",
      "interval: '#job-form-interval-preset'",
    ],
    'js advanced editor schedule window',
    failures
  );

  expectNotIncludes(
    js,
    [
      'id="job-form-type"',
      'id="job-form-cron"',
      'id="job-form-interval"',
      'id="job-form-timezone"',
      "payload.schedule_type = qs('#job-form-type').value;",
      "payload.cron = qs('#job-form-cron').value.trim();",
      "payload.interval = qs('#job-form-interval').value.trim();",
      "payload.timezone = qs('#job-form-timezone').value.trim();",
    ],
    'js advanced editor legacy schedule fields',
    failures
  );

  expectPattern(
    js,
    /qs\('#job-form-time-mode'\)\?\.addEventListener\('change',\s*\(\)\s*=>\s*toggleAdvancedJobEditorScheduleFields\(\)\s*\);/,
    'js',
    failures,
    'advanced editor schedule mode toggle binding'
  );

  expectIncludes(
    js,
    [
      'function buildJobScheduleSummary(',
      'function buildIntervalPresetLabel(',
      '<div class="k">调度方式</div>',
      '<div class="k">调度说明</div>',
      '服务器时区',
    ],
    'js schedule summary detail',
    failures
  );

  expectNotIncludes(
    js,
    [
      '<div class="k">Cron</div>',
      '<div class="k">Interval</div>',
      '<div class="k">Timezone</div>',
    ],
    'js legacy detail schedule kv',
    failures
  );

  expectIncludes(
    css,
    [
      '.task-center-tabs',
      '.template-layout',
      '.template-detail-stack',
      '.task-center-pane-schedule-shell',
      '#task-center-pane-schedule .jobs-layout',
    ],
    'css',
    failures
  );

  if (failures.length) {
    console.error(failures.join('\n'));
    process.exit(1);
  }

  console.log('ui task center relayout contract ok');
} catch (err) {
  console.error(err.message);
  process.exit(1);
}
