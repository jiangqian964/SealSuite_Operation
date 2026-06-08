const fs = require('fs');

function read(file) {
  return fs.readFileSync(file, 'utf8');
}

function expectIncludes(content, tokens, label, failures) {
  tokens.forEach((token) => {
    if (!content.includes(token)) failures.push(`${label} missing ${token}`);
  });
}

try {
  const html = read('internal/web/assets/ui/index.html');
  const js = read('internal/web/assets/ui/app.js');
  const failures = [];

  expectIncludes(
    html,
    [
      'id="draft-llm-api-id"',
      'id="draft-llm-api-hint"',
      '使用 role 的默认模型',
      `placeholder='{"role":"formatter","prompt":"请总结结果"}'`
    ],
    'html',
    failures
  );

  expectIncludes(
    js,
    [
      'window.__llmAPIItems = [];',
      'function buildDraftLLMAPIOptions(',
      'function renderDraftLLMAPIHint(',
      'function refreshLLMAPIItems(',
      'function splitDraftLLMConfigSelection(',
      'function readTaskDraftLLMConfig() {',
      "fetchJSON('/api/v1/settings/llm/apis')",
      "qs('#draft-llm-api-id').innerHTML = buildDraftLLMAPIOptions(selected);",
      "renderDraftLLMAPIHint(selected);",
      "qs('#draft-llm-api-hint').textContent = `LLM_API 列表加载失败：${String(e)}`;",
      "qs('#draft-llm-api-id')?.addEventListener('change', () => {",
      "renderDraftLLMAPIHint(qs('#draft-llm-api-id').value || '');",
      'refreshLLMAPIItems();',
      'const llmSelection = splitDraftLLMConfigSelection(draft.llm_config || {});',
      "qs('#draft-llm-api-id').value = llmSelection.llm_api_id;",
      "renderDraftLLMAPIHint(llmSelection.llm_api_id);",
      "qs('#draft-llm').value = pretty(llmSelection.llm_config);",
      'llm_config: readTaskDraftLLMConfig(),',
      '已删除',
      '已禁用',
      "item.enabled === false"
    ],
    'js',
    failures
  );

  if (failures.length) {
    console.error(failures.join('\n'));
    process.exit(1);
  }

  console.log('ui task draft llm api selector contract ok');
} catch (err) {
  console.error(err.message);
  process.exit(1);
}
