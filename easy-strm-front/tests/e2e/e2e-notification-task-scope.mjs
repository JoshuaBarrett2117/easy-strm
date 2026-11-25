import assert from 'node:assert/strict'
import { spawn } from 'node:child_process'
import { mkdir, writeFile } from 'node:fs/promises'
import { chromium } from 'playwright'

const baseURL = 'http://127.0.0.1:4317'
const evidenceDir = process.env.E2E_EVIDENCE_DIR || '../debug/notification-task-scope'
const ok = data => ({ state: true, code: 0, message: 'success', data, error: '', errno: 0 })
const tasks = [
  { task_key: 'key:a', task_name: '分享库增量导出', handler: 'shared_handler', status: 'enabled' },
  { task_key: 'key:b', task_name: '分享库全量对账', handler: 'shared_handler', status: 'disabled' },
  { task_key: `handler:cleanup_${'long_handler_'.repeat(8)}`, task_name: '旧版目录清理', handler: `cleanup_${'long_handler_'.repeat(8)}`, status: 'enabled' },
  { task_key: `handler:cleanup_${'long_handler_'.repeat(8)}`, task_name: '第二个旧版目录清理', handler: `cleanup_${'long_handler_'.repeat(8)}`, status: 'disabled' }
]
const server = spawn(process.execPath, ['node_modules/vite/bin/vite.js', 'preview', '--host', '127.0.0.1', '--port', '4317', '--strictPort'], { stdio: ['ignore', 'pipe', 'pipe'] })
let serverOutput = ''
server.stdout.on('data', chunk => { serverOutput += chunk })
server.stderr.on('data', chunk => { serverOutput += chunk })

async function waitForServer() {
  for (let attempt = 0; attempt < 100; attempt++) {
    if (server.exitCode !== null) throw new Error(`Preview failed: ${serverOutput}`)
    try {
      if ((await fetch(baseURL)).ok) return
    } catch {}
    await new Promise(resolve => setTimeout(resolve, 100))
  }
  throw new Error(`Preview timed out: ${serverOutput}`)
}

async function main() {
  await mkdir(evidenceDir, { recursive: true })
  await waitForServer()
  const browser = await chromium.launch({ headless: true })
  const context = await browser.newContext({ viewport: { width: 1440, height: 1000 } })
  const errors = []
  const requests = []
  let selected = [...new Set(tasks.map(task => task.task_key))]
  let failSave = false
  let failLoad = false
  await context.addInitScript(() => {
    localStorage.setItem('token', 'isolated-notify-test')
    localStorage.setItem('user_name', 'admin')
  })
  try {
    await context.route('**/*', async route => {
      const request = route.request()
      const url = new URL(request.url())
      if (url.origin !== baseURL) return route.abort()
      if (url.pathname.startsWith('/share-strm/')) return route.abort()
      if (!url.pathname.startsWith('/api/')) return route.continue()
      requests.push({ method: request.method(), path: url.pathname })
      let data = {}
      if (url.pathname === '/api/notify/task-scope') {
        if (request.method() === 'PUT') {
          if (failSave) return route.fulfill({ status: 500, json: { error: '保存范围失败（模拟）' } })
          const body = request.postDataJSON()
          assert(Array.isArray(body.selected))
          assert.equal(new Set(body.selected).size, body.selected.length)
          assert(body.selected.every(key => tasks.some(task => task.task_key === key)))
          selected = body.selected
          data = { selected }
        } else {
          if (failLoad) return route.fulfill({ status: 500, json: { error: '读取范围失败（模拟）' } })
          data = { tasks, selected }
        }
      }
      return route.fulfill({ status: 200, json: ok(data) })
    })
    const page = await context.newPage()
    page.on('pageerror', error => errors.push(error.message))
    const openScope = async () => {
      await page.goto(`${baseURL}/dashboard/settings`, { waitUntil: 'networkidle' })
      await page.getByText('通知', { exact: true }).click()
      await page.getByRole('region', { name: '任务通知范围', exact: true }).waitFor()
    }
    await openScope()
    const scope = page.getByRole('region', { name: '任务通知范围', exact: true })
    const checkboxes = scope.getByRole('checkbox')
    const assertSelected = async count => {
      await page.waitForFunction(expected => document.querySelectorAll('.notification-task-scope [role="checkbox"][aria-checked="true"]').length === expected, count)
    }
    await assertSelected(4)
    assert.equal(await checkboxes.count(), 4)
    await scope.getByText('已选 4 / 4', { exact: true }).waitFor()
    await scope.getByText('已停用', { exact: true }).first().waitFor()
    await scope.getByText('仅勾选的任务会推送通知；未勾选的任务静默执行', { exact: true }).waitFor()
    await scope.getByRole('button', { name: '全不选', exact: true }).click()
    await assertSelected(0)
    await scope.getByRole('button', { name: '反选', exact: true }).click()
    await assertSelected(4)
    await checkboxes.nth(1).click()
    await assertSelected(3)
    await scope.getByRole('button', { name: '反选', exact: true }).click()
    await assertSelected(1)
    await scope.getByText('已选 1 / 4', { exact: true }).waitFor()
    await scope.getByRole('button', { name: '保存任务范围', exact: true }).click()
    await page.getByText('任务通知范围已保存', { exact: true }).waitFor()
    assert.deepEqual(selected, ['key:b'])
    await scope.getByRole('button', { name: '全选', exact: true }).click()
    await assertSelected(4)
    await scope.getByRole('button', { name: '全不选', exact: true }).click()
    await Promise.all([
      page.waitForResponse(response => response.url().endsWith('/notify/task-scope') && response.request().method() === 'PUT' && response.ok()),
      scope.getByRole('button', { name: '保存任务范围', exact: true }).click()
    ])
    assert.deepEqual(selected, [])
    await openScope()
    await assertSelected(0)
    failSave = true
    await scope.getByRole('button', { name: '全选', exact: true }).click()
    await scope.getByRole('button', { name: '保存任务范围', exact: true }).click()
    await page.getByText('保存范围失败（模拟）', { exact: true }).waitFor()
    assert.deepEqual(selected, [])
    await assertSelected(4)
    await page.screenshot({ path: `${evidenceDir}/desktop.png`, fullPage: true })
    await page.setViewportSize({ width: 390, height: 844 })
    await scope.getByRole('button', { name: '保存任务范围', exact: true }).scrollIntoViewIfNeeded()
    assert.equal(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth), false, 'Mobile horizontal overflow')
    await page.screenshot({ path: `${evidenceDir}/mobile.png`, fullPage: true })
    failLoad = true
    await scope.getByRole('button', { name: '刷新任务', exact: true }).click()
    await scope.getByText('读取范围失败（模拟）', { exact: true }).waitFor()
    assert.equal(await scope.getByRole('button', { name: '保存任务范围', exact: true }).isDisabled(), true)
    failLoad = false
    await scope.getByRole('button', { name: '刷新任务', exact: true }).click()
    await assertSelected(0)
    assert.deepEqual(errors, [], 'Unexpected browser errors')
    const results = { ok: true, checks: ['all', 'none', 'invert', 'stable-key-save', 'grouped-keyless-count', 'reload-explicit-none', 'save-error', 'load-error-and-retry', 'mobile-no-overflow'], apiRequests: requests, browserErrors: errors }
    await writeFile(`${evidenceDir}/browser-results.json`, JSON.stringify(results, null, 2))
    console.log(JSON.stringify(results))
  } catch (error) {
    const page = context.pages()[0]
    if (page) {
      await page.screenshot({ path: `${evidenceDir}/browser-failure.png`, fullPage: true })
      await writeFile(`${evidenceDir}/browser-failure.json`, JSON.stringify({ error: error.message, browserErrors: errors, apiRequests: requests, body: await page.locator('body').innerText() }, null, 2))
    }
    throw error
  } finally {
    await browser.close()
  }
}

main().catch(error => { console.error(error); process.exitCode = 1 }).finally(() => { server.kill('SIGTERM') })
