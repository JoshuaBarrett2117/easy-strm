import assert from 'node:assert/strict'
import { mkdir, mkdtemp, rm, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { createServer } from 'vite'
import vue from '@vitejs/plugin-vue'
import { chromium } from 'playwright'

const root = resolve(dirname(fileURLToPath(import.meta.url)), '..')
const envDir = await mkdtemp(resolve(tmpdir(), 'estrm-selection-env-'))
const output = process.env.E2E_OUTPUT_DIR || '/opt/data/output/estrm-round2/browser'
const selection = { media_item_key: 'fixture:tv:1:2', work_key: 'fixture:tv', title: '测试剧集', season_number: 1, episode_number: 2, selected_candidate_id: 11, selection_mode: 'auto', selection_revision: 1, exported_revision: 1, stable_relative_path: 'tv/测试剧集/Season 01/测试剧集 - S01E02.strm', valid: true }
const candidates = [
  { candidate_id: 11, share_name: '最先发现来源', file_name: '剧集/first.mkv', file_size: 1024 ** 3, first_seen_seq: 20, available: true, revoked: false, state: 'active' },
  { candidate_id: 12, share_name: '另一个分享', file_name: '剧集/remux.mkv', file_size: 8 * 1024 ** 3, first_seen_seq: 21, available: true, revoked: false, state: 'active' },
  { candidate_id: 13, share_name: '已撤销映射', file_name: 'wrong.mkv', file_size: 1024 ** 3, first_seen_seq: 22, available: false, revoked: true, state: 'stale' }
]
let changes = 0, refreshes = 0
let listMode = 'normal', detailMode = 'normal', conflictNext = false
const requests = []
let releaseInitialList
const initialListGate = new Promise(resolve => { releaseInitialList = resolve })
let initialList = true
const server = await createServer({ root, envDir, configFile: false, plugins: [vue(), {
  name: 'selection-isolated-fixture',
  resolveId(id) { if (id === '/__selection-entry.js') return id },
  load(id) { if (id === '/__selection-entry.js') return `import {createApp,h} from '/node_modules/.vite/deps/vue.js';import {NConfigProvider,NMessageProvider,NDialogProvider} from '/node_modules/.vite/deps/naive-ui.js';import Selection from '/src/views/ShareSelection.vue';createApp({render:()=>h(NConfigProvider,null,{default:()=>h(NMessageProvider,null,{default:()=>h(NDialogProvider,null,{default:()=>h(Selection)})})})}).mount('#app')` },
  configureServer(vite) { vite.middlewares.use((request, response, next) => { if (request.url !== '/__selection') return next(); response.setHeader('Content-Type', 'text/html'); response.end('<html><head><meta name="viewport" content="width=device-width, initial-scale=1"></head><body><div id="app"></div><script type="module" src="/__selection-entry.js"></script></body></html>') }) }
}], optimizeDeps: { include: ['vue', 'naive-ui'] }, server: { host: '127.0.0.1', port: 0 } })
let browser
try {
  await mkdir(output, { recursive: true }); await server.listen()
  browser = await chromium.launch({ headless: true, executablePath: process.env.E2E_BROWSER_EXECUTABLE || undefined, args: ['--no-sandbox'] })
  const page = await browser.newPage({ viewport: { width: 1280, height: 900 } })
  const pageErrors = []
  await page.addInitScript(() => localStorage.setItem('token', 'isolated-fixture-token'))
  page.on('pageerror', error => pageErrors.push(error.message))
  await page.route(url => url.pathname.startsWith('/api/'), async route => {
    const request = route.request(), url = new URL(request.url())
    assert.equal(url.hostname, '127.0.0.1')
    const headers = request.headers()
    assert.equal(headers.authorization, 'Bearer isolated-fixture-token')
    assert.match(headers['x-trace-id'], /^[a-f0-9-]{14}7[a-f0-9-]+$/)
    requests.push({ method: request.method(), path: url.pathname, page: url.searchParams.get('page'), keyword: url.searchParams.get('keyword'), trace: headers['x-trace-id'] })
    const fail = (status, error) => route.fulfill({ status, contentType: 'application/json', body: JSON.stringify({ state: false, error }) })
    let data
    if (url.pathname.endsWith('/detail')) {
      if (detailMode === 'error') return fail(404, '详情夹具不存在')
      data = { selection, candidates }
    }
    else if (request.method() === 'PUT') {
      const change = request.postDataJSON()
      assert.equal(change.expected_revision, selection.selection_revision)
      assert.equal(change.media_item_key, selection.media_item_key)
      assert.equal(change.candidate_id, 12)
      if (conflictNext) { conflictNext = false; return fail(409, '来源版本已变化，请刷新后重试') }
      changes++; selection.selected_candidate_id = 12; selection.selection_revision++; selection.selection_mode = 'manual'; data = { next_export: true }
    } else if (url.pathname.endsWith('/refresh')) { refreshes++; data = null }
    else {
      if (initialList) { initialList = false; await initialListGate }
      assert.ok(url.searchParams.has('page'))
      if (listMode === 'error') return fail(500, '列表夹具故障')
      data = listMode === 'empty' ? { data: [], total: 0 } : { data: [selection], total: 21 }
    }
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ state: true, data }) })
  })
  await page.goto(`http://127.0.0.1:${server.httpServer.address().port}/__selection`, { waitUntil: 'domcontentloaded' })
  await page.locator('.n-data-table-loading-wrapper').waitFor()
  await page.screenshot({ path: `${output}/loading.png`, fullPage: true })
  releaseInitialList()
  await page.getByRole('button', { name: '查看候选', exact: true }).waitFor()
  await Promise.all([page.waitForResponse(response => response.url().includes('page=2')), page.locator('.n-pagination-item').filter({ hasText: /^2$/ }).click()])
  await page.getByPlaceholder('搜索标题或媒体键').fill('测试剧集')
  await Promise.all([page.waitForResponse(response => response.url().includes('keyword=') && response.url().includes('page=1')), page.getByRole('button', { name: '搜索', exact: true }).click()])
  await page.getByRole('button', { name: '查看候选', exact: true }).click()
  await page.getByText('最先发现来源', { exact: true }).waitFor()
  const modal = page.locator('.n-modal').first()
  assert.match(await modal.innerText(), /当前来源 #11/)
  assert.match(await modal.innerText(), /识别映射已撤销/)
  const buttons = modal.getByRole('button', { name: '选择此来源', exact: true })
  assert.equal(await buttons.last().isDisabled(), true)
  conflictNext = true
  await buttons.first().click()
  await page.getByRole('button', { name: '确认改选', exact: true }).click()
  await page.getByText('来源版本已变化，请刷新后重试', { exact: true }).waitFor()
  await page.getByText('当前来源 #11', { exact: false }).waitFor()
  await page.locator('.n-dialog').waitFor({ state: 'hidden' })
  await page.screenshot({ path: `${output}/cas-conflict.png`, fullPage: true })
  const actionStart = requests.length
  await buttons.first().click()
  const actionListResponse = page.waitForResponse(response => response.url().includes('/selections?'))
  await page.getByRole('button', { name: '确认改选', exact: true }).click()
  await page.getByText('当前来源 #12', { exact: false }).waitFor()
  await page.locator('.n-dialog').waitFor({ state: 'hidden' })
  await actionListResponse
  const actionRequests = requests.slice(actionStart)
  assert.equal(actionRequests.length, 3)
  assert.deepEqual(actionRequests.map(request => request.method), ['PUT', 'GET', 'GET'])
  assert.equal(new Set(actionRequests.map(request => request.trace)).size, 1)
  assert.match(await modal.innerText(), /待下次导出应用/)
  assert.ok((await modal.innerText()).includes(selection.stable_relative_path))
  await page.screenshot({ path: `${output}/desktop.png`, fullPage: true })
  await page.setViewportSize({ width: 390, height: 844 })
  assert.equal(await modal.isVisible(), true)
  assert.ok(await page.evaluate(() => document.documentElement.scrollWidth <= 390))
  const mobileAction = await modal.getByRole('button', { name: '选定 · 设为手动', exact: true }).boundingBox()
  assert.ok(mobileAction && mobileAction.x >= 0 && mobileAction.x + mobileAction.width <= 390)
  await page.screenshot({ path: `${output}/mobile.png`, fullPage: true })
  await modal.locator('.n-base-close').click()
  await page.setViewportSize({ width: 1280, height: 900 })
  detailMode = 'error'
  await page.getByRole('button', { name: '查看候选', exact: true }).click()
  await page.getByText('详情夹具不存在', { exact: false }).waitFor()
  await page.screenshot({ path: `${output}/detail-error.png`, fullPage: true })
  detailMode = 'normal'
  await page.getByRole('button', { name: '刷新详情', exact: true }).click()
  await page.getByText('当前来源 #12', { exact: false }).waitFor()
  await modal.locator('.n-base-close').click()
  listMode = 'error'
  await page.getByRole('button', { name: '搜索', exact: true }).click()
  await page.getByText('列表夹具故障', { exact: false }).waitFor()
  await page.screenshot({ path: `${output}/list-error.png`, fullPage: true })
  listMode = 'empty'
  await page.getByRole('button', { name: '重试', exact: true }).click()
  await page.getByText('暂无可选来源，请先扫描识别分享文件，再刷新候选。', { exact: true }).waitFor()
  await page.screenshot({ path: `${output}/empty.png`, fullPage: true })
  listMode = 'normal'
  const refreshStart = requests.length
  await page.getByRole('button', { name: '刷新应用库候选', exact: true }).click()
  await page.getByRole('button', { name: '查看候选', exact: true }).waitFor()
  const refreshRequests = requests.slice(refreshStart)
  assert.equal(refreshRequests.length, 2)
  assert.equal(new Set(refreshRequests.map(request => request.trace)).size, 1)
  assert.notEqual(refreshRequests[0].trace, actionRequests[0].trace)
  assert.ok(requests.some(request => request.page === '2'))
  assert.ok(requests.some(request => request.keyword === '测试剧集' && request.page === '1'))
  assert.equal(changes, 1); assert.equal(refreshes, 1); assert.deepEqual(pageErrors, [])
  const evidence = { ok: true, api: 'all mocked, synthetic fixtures; no real DB', envDir: 'isolated empty fixture', changes, refreshes, actionRequests, refreshRequests, assertions: ['auth wrapper', 'list', 'search', 'pagination', 'loading', 'detail', 'current', 'disabled revoked', 'CAS error', 'confirm', 'pending rebuild', 'stable path', 'empty', 'list error', 'detail error', 'mobile', 'one real confirmation action three requests same X-Trace-Id'], screenshots: ['loading.png', 'cas-conflict.png', 'desktop.png', 'mobile.png', 'detail-error.png', 'list-error.png', 'empty.png'] }
  await writeFile(`${output}/assertions.json`, JSON.stringify(evidence, null, 2))
  console.log(JSON.stringify(evidence))
} finally { if (browser) await browser.close(); await server.close(); await rm(envDir, { recursive: true, force: true }) }
