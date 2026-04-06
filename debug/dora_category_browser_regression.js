const { chromium } = require('playwright');
const fs = require('fs');
const path = require('path');

(async () => {
  const step = (name) => console.log(`[dora-category] ${name}`);
  const workspace = 'C:/Users/a3875/Documents/code/easy-strm';
  const inputDir = path.join(workspace, 'debug', 'dora-category-input');
  const targetDir = path.join(workspace, 'debug', 'test');
  const fileName = '\u54c6\u5566A\u68a6\uff1a\u5927\u96c4\u7684\u6050\u9f99.mp4';
  const categoryName = '\u52a8\u753b\u7535\u5f71';
  const titleFolder = '\u54c6\u5566A\u68a6\uff1a\u5927\u96c4\u7684\u6050\u9f99 (1980)';
  const expectedFileName = '\u54c6\u5566A\u68a6\uff1a\u5927\u96c4\u7684\u6050\u9f99 - \u6620\u753b\u30c9\u30e9\u3048\u3082\u3093 \u306e\u3073\u592a\u306e\u6050\u7adc (1980).mp4';
  const expectedFile = path.join(targetDir, categoryName, titleFolder, expectedFileName);
  const missingCategoryFile = path.join(targetDir, titleFolder, expectedFileName);

  step('use prepared files');
  step('launch browser');
  const browser = await chromium.launch({ headless: true });
  const page = await browser.newPage({ viewport: { width: 1440, height: 1000 } });
  const requests = [];
  page.on('request', req => {
    if (req.url().includes('/api/media/organize/')) {
      requests.push({ url: req.url(), method: req.method(), postData: req.postData() });
    }
  });

  const api = async (url, options = {}) => page.evaluate(async ({ url, options }) => {
    const headers = Object.assign({ 'Content-Type': 'application/json' }, options.headers || {});
    const token = localStorage.getItem('token');
    if (token) {
      headers.Authorization = `Bearer ${token}`;
    }
    const resp = await fetch(url, Object.assign({}, options, { headers }));
    const text = await resp.text();
    let data;
    try {
      data = text ? JSON.parse(text) : null;
    } catch (err) {
      data = { raw: text };
    }
    if (!resp.ok) throw new Error(`${resp.status} ${url}: ${JSON.stringify(data)}`);
    return data;
  }, { url, options });

  step('goto login');
  await page.goto('http://127.0.0.1:3001/login', { waitUntil: 'networkidle' });
  step('login api');
  const loginResp = await api('/api/login', {
    method: 'POST',
    body: JSON.stringify({ name: 'admin', password: '21232f297a57a5a743894a0e4a801fc3' }),
  });
  await page.evaluate((loginResp) => {
    localStorage.setItem('token', loginResp.token);
    localStorage.setItem('user_id', String(loginResp.user_id || '1'));
    localStorage.setItem('user_name', loginResp.name || 'admin');
    document.cookie = `token=${encodeURIComponent(loginResp.token)};path=/`;
  }, loginResp);

  step('save settings');
  const movieTemplate = '{{ title }}{% if year %} ({{ year }}){% endif %}/{{ title }}{% if en_title and en_title != title %} - {{ en_title }}{% endif %}{% if year %} ({{ year }}){% endif %}{% if videoFormat %} [{{ videoFormat }}]{% endif %}{{ fileExt }}';
  await api('/api/settings/movie_naming_template', { method: 'PUT', body: JSON.stringify({ value: movieTemplate }) });

  step('upsert category');
  const catResp = await api('/api/media/categories');
  const categories = catResp?.data?.data || [];
  const animationCategory = categories.find(cat => cat.name === categoryName && cat.media_type === 'movie');
  const categoryPayload = {
    name: categoryName,
    media_type: 'movie',
    target_path: `/\u7535\u5f71/${categoryName}`,
    match_rules: { genre_ids: [16] },
    enabled: true,
  };
  if (animationCategory) {
    await api(`/api/media/categories/${animationCategory.id}`, { method: 'PUT', body: JSON.stringify(categoryPayload) });
  } else {
    await api('/api/media/categories', { method: 'POST', body: JSON.stringify(categoryPayload) });
  }

  step('create source');
  const sourceName = `browser-category-regression-${Date.now()}`;
  const sourceResp = await api('/api/media/sources', {
    method: 'POST',
    body: JSON.stringify({ name: sourceName, source_type: 'local', path: inputDir, priority: 1, enabled: true }),
  });
  const source = sourceResp?.data?.data;
  if (!source?.id) throw new Error(`create source failed: ${JSON.stringify(sourceResp)}`);

  step('goto media manager');
  await page.goto('http://127.0.0.1:3001/dashboard/media-manager', { waitUntil: 'networkidle' });
  await page.waitForSelector('.source-card .el-table__row', { timeout: 15000 });
  const sourceRow = page.locator('.source-card .el-table__row', { hasText: sourceName }).first();
  await sourceRow.waitFor({ timeout: 15000 });
  step('open source dialog');
  await sourceRow.locator('button').first().click();
  await page.waitForSelector('.el-dialog .el-table__row', { timeout: 15000 });
  const fileRow = page.locator('.el-dialog .el-table__row', { hasText: fileName }).first();
  await fileRow.waitFor({ timeout: 15000 });
  step('select file');
  await fileRow.locator('.el-checkbox__input').first().click();

  step('open organize dialog');
  const initialPreviewPromise = page.waitForResponse(resp => resp.url().includes('/api/media/organize/preview') && resp.request().method() === 'POST', { timeout: 60000 });
  await page.locator('.organize-primary-btn').click();
  await page.waitForSelector('.organize-container', { timeout: 15000 });
  await initialPreviewPromise.catch(() => null);
  await page.locator('.organize-form input').first().fill(targetDir);

  const organizeDialog = page.locator('.el-dialog').filter({ has: page.locator('.organize-container') }).last();
  step('refresh preview');
  const previewPromise = page.waitForResponse(resp => {
    if (!resp.url().includes('/api/media/organize/preview') || resp.request().method() !== 'POST') return false;
    try {
      return JSON.parse(resp.request().postData() || '{}').target_path === targetDir;
    } catch (err) {
      return false;
    }
  }, { timeout: 60000 });
  await organizeDialog.locator('button').filter({ hasText: '\u5237\u65b0\u9884\u89c8' }).click();
  const previewResp = await previewPromise;
  const previewJson = await previewResp.json();
  const previewItem = previewJson?.data?.data?.[0];
  if (!previewItem) throw new Error(`preview empty: ${JSON.stringify(previewJson)}`);

  step('execute organize');
  const executePromise = page.waitForResponse(resp => {
    if (!resp.url().includes('/api/media/organize/execute') || resp.request().method() !== 'POST') return false;
    try {
      return JSON.parse(resp.request().postData() || '{}').target_path === targetDir;
    } catch (err) {
      return false;
    }
  }, { timeout: 60000 });
  await organizeDialog.locator('button').filter({ hasText: '\u6267\u884c\u6574\u7406' }).click();
  const executeResp = await executePromise;
  const executeJson = await executeResp.json();
  await page.screenshot({ path: path.join(workspace, 'debug', 'dora-category-browser.png'), fullPage: true });

  step('write report');
  const previewRequest = requests.filter(r => r.url.includes('/api/media/organize/preview')).at(-1);
  const executeRequest = requests.filter(r => r.url.includes('/api/media/organize/execute')).at(-1);
  const report = {
    sourceName,
    sourceID: source.id,
    inputDir,
    targetDir,
    previewPath: previewItem.new_path,
    executePath: executeJson?.data?.data?.[0]?.new_path,
    expectedFile,
    missingCategoryFile,
    previewRequest: previewRequest ? JSON.parse(previewRequest.postData) : null,
    executeRequest: executeRequest ? JSON.parse(executeRequest.postData) : null,
    previewJson,
    executeJson,
  };
  fs.writeFileSync(path.join(workspace, 'debug', 'dora-category-browser-report.json'), JSON.stringify(report, null, 2));

  if (!report.previewRequest?.use_category || !report.executeRequest?.use_category) {
    throw new Error(`use_category missing: ${JSON.stringify(report, null, 2)}`);
  }
  if (!report.previewPath || !report.previewPath.includes(categoryName)) {
    throw new Error(`preview path missing category: ${JSON.stringify(report, null, 2)}`);
  }
  await api(`/api/media/sources/${source.id}`, { method: 'DELETE' }).catch(() => null);
  await browser.close();
  console.log(JSON.stringify({
    ok: true,
    previewPath: report.previewPath,
    executePath: report.executePath,
    expectedFile: report.expectedFile,
  }, null, 2));
})().catch((err) => {
  console.error(err && err.stack ? err.stack : err);
  process.exit(1);
});
