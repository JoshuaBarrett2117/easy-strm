import assert from 'node:assert/strict'
import { mkdir } from 'node:fs/promises'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { createServer } from 'vite'
import vue from '@vitejs/plugin-vue'
import tailwindcss from '@tailwindcss/vite'
import { chromium } from 'playwright'

const root = resolve(dirname(fileURLToPath(import.meta.url)), '..')
const output = '/opt/data/output/easy-strm-phase03'
const handlers = [
  { key: 'share_strm_incremental_export', name: '分享库 STRM 增量导出（每日多次）', parameters: [], default_cron: '0 */6 * * *' },
  { key: 'share_strm_full_reconciliation', name: '分享库 STRM 全量对账（每周）', parameters: [], default_cron: '0 3 * * 0' }
]
const task = (id, metadata, status = 'completed') => ({ task_id: id, task_name: id, task_type: 'strm_generate', status, metadata: { share_export: true, ...metadata } })
const tasks = [
  task('cron_incremental', { cron_handler: handlers[0].key, requested_mode: 'incremental', effective_mode: 'incremental', processed_works: 0, affected_sources: 0 }),
  task('cron_reconciliation', { cron_handler: handlers[1].key, requested_mode: 'reconciliation', effective_mode: 'reconciliation', processed_works: 5, affected_sources: 8 }),
  task('cron_fallback', { cron_handler: handlers[0].key, requested_mode: 'incremental', effective_mode: 'reconciliation', fallback_reason: '历史输出凭证不可信', recovery: '继续已准备版本的未完成作品' }, 'failed'),
  task('cron_skipped', { cron_handler: handlers[1].key, requested_mode: 'reconciliation', effective_mode: 'skipped', outcome: 'skipped', skip_reason: '分享导出正在运行，互斥跳过', blocked_by_task_id: 'cron_incremental' }),
  { task_id: 'share_strm_legacy', task_name: '原导出', task_type: 'strm_generate', status: 'completed', processed_files: 2, total_files: 2, metadata: { exported_files: 2 } }
]
const entry = `
import { createApp, h } from 'vue'
import { NConfigProvider, NMessageProvider, NDialogProvider } from 'naive-ui'
import TaskCard from '/src/components/TaskCard.vue'
import ScheduledTasks from '/src/views/ScheduledTasks.vue'
import '/src/style.css'
const tasks = ${JSON.stringify(tasks)}
createApp({ render: () => h(NConfigProvider, {}, { default: () => h(NMessageProvider, {}, { default: () => h(NDialogProvider, {}, { default: () => h('main', { style: 'padding: 16px' }, [...tasks.map(task => h(TaskCard, { task })), h(ScheduledTasks)]) }) }) }) }).mount('#app')
`
const server = await createServer({ root, configFile: false, envFile: false, plugins: [vue(), tailwindcss(), {
  name: 'isolated-stage-a-fixture',
  resolveId(id) { if (id === '/__phase03-entry.js') return '\0phase03-entry' },
  load(id) { if (id === '\0phase03-entry') return entry },
  configureServer(vite) {
    vite.middlewares.use((request, response, next) => {
      if (request.url !== '/__phase03') return next()
      response.setHeader('Content-Type', 'text/html')
      response.end('<html><head><meta name="viewport" content="width=device-width, initial-scale=1"></head><body><div id="app"></div><script type="module" src="/__phase03-entry.js"></script></body></html>')
    })
  }
}], server: { host: '127.0.0.1', port: 0 } })
let browser
try {
  await mkdir(output, { recursive: true })
  await server.listen()
  browser = await chromium.launch({ headless: true, executablePath: process.env.E2E_BROWSER_EXECUTABLE || undefined, args: ['--no-sandbox'] })
  const page = await browser.newPage({ viewport: { width: 1280, height: 1000 } })
  const pageErrors = []
  page.on('pageerror', error => { pageErrors.push(error.message); console.error('fixture pageerror:', error.message) })
  page.on('console', message => { if (message.type() === 'error') console.error('fixture console:', message.text()) })
  await page.route(url => url.pathname.startsWith('/api/'), route => {
    const path = new URL(route.request().url()).pathname
    const data = path === '/api/cron/handlers' ? handlers : { data: [], total: 0 }
    return route.fulfill({ contentType: 'application/json', body: JSON.stringify({ state: true, code: 0, data }) })
  })
  const port = server.httpServer.address().port
  await page.goto(`http://127.0.0.1:${port}/__phase03`, { waitUntil: 'networkidle' })
  const cards = page.locator('[data-testid="task-card"]')
  await cards.nth(4).waitFor()
  assert.equal(await cards.count(), 5)
  assert.match(await cards.nth(0).innerText(), /增量导出/)
  assert.match(await cards.nth(1).innerText(), /全量对账/)
  assert.match(await cards.nth(2).innerText(), /历史输出凭证不可信/)
  assert.match(await cards.nth(2).innerText(), /继续已准备版本/)
  assert.equal(await cards.nth(2).getByRole('button', { name: /继续|重试|恢复/ }).count(), 0)
  assert.match(await cards.nth(3).innerText(), /已跳过/)
  assert.match(await cards.nth(3).innerText(), /互斥跳过/)
  assert.match(await cards.nth(3).innerText(), /cron_incremental/)
  assert.equal(await cards.nth(3).locator('.n-progress').count(), 0)
  assert.match(await cards.nth(4).innerText(), /已生成 STRM/)
  await page.getByRole('button', { name: '新建任务', exact: true }).click()
  const dialog = page.locator('.n-modal')
  await dialog.waitFor()
  assert.ok(await dialog.locator('input').count() > 0)
  assert.match(await dialog.innerText(), /不自动补跑/)
  assert.ok(await dialog.locator('input').evaluateAll(inputs => inputs.some(input => input.value === '0 */6 * * *')))
  await dialog.locator('.n-base-selection').first().click()
  await page.getByText(handlers[1].name, { exact: true }).last().click()
  assert.ok(await dialog.locator('input').evaluateAll(inputs => inputs.some(input => input.value === '0 3 * * 0')))
  await page.screenshot({ path: `${output}/frontend-desktop.png`, fullPage: true })
  await page.setViewportSize({ width: 390, height: 844 })
  assert.equal(await dialog.getByRole('button', { name: '保存', exact: true }).isVisible(), true)
  await page.screenshot({ path: `${output}/frontend-mobile.png`, fullPage: true })
  assert.deepEqual(pageErrors, [])
  console.log(JSON.stringify({ ok: true, isolated: true, backendRequests: 'all intercepted', cards: 5, handlers: 2, cronDefaults: handlers.map(handler => handler.default_cron), mobile: true }))
} finally {
  if (browser) await browser.close()
  await server.close()
}
