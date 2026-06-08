const fs = require('fs');
try {
  const html = fs.readFileSync('internal/web/assets/ui/index.html', 'utf8');
  const js = fs.readFileSync('internal/web/assets/ui/app.js', 'utf8');
  function extractSection(id) {
    const re = new RegExp(`<section[^>]*\\bid="${id}"[\\s\\S]*?<\\/section>`, 'i');
    const m = html.match(re);
    return m ? m[0] : '';
  }

  const secTemplates = extractSection('view-templates');
  const secAPI = extractSection('view-api');

  const tplTableCount = (html.match(/id="tpl-table"/g) || []).length;
  if (tplTableCount !== 1) {
    console.error('tpl-table should appear exactly once, got:', tplTableCount);
    process.exit(1);
  }
  if (!secTemplates.includes('id="tpl-table"')) {
    console.error('tpl-table missing in view-templates');
    process.exit(1);
  }
  if (secAPI.includes('id="tpl-table"')) {
    console.error('tpl-table must not exist in view-api');
    process.exit(1);
  }

  const must = [
    'id="view-templates"',
    'id="btn-open-templates"',
    'id="tpl-out"',
    'id="tpl-detail"',
    'id="tpl-table"',
    'id="tpl-filter-2"',
    'id="btn-templates-refresh"',
    'id="btn-template-create-2"',
    'id="api-task-definition-card"',
  ];
  const mustRegex = [
    /\btemplates-split\b/,
    /\btpl-out-card\b/,
  ];

  const missingText = must.filter(x => !html.includes(x));
  const missingRegex = mustRegex.filter(re => !re.test(html)).map(re => re.toString());
  const missing = [...missingText, ...missingRegex];

  if (missing.length) {
    console.error('missing:', missing);
    process.exit(1);
  }

  const mustJS = [
    '用此模板创建任务',
    'function applyTemplateToAPITask',
    'function focusAPITaskDefinitionArea',
  ];
  const missingJS = mustJS.filter(x => !js.includes(x));
  if (missingJS.length) {
    console.error('missing js:', missingJS);
    process.exit(1);
  }
  console.log('templates layout ok');
} catch (e) {
  console.error('error:', e.message);
  process.exit(1);
}
