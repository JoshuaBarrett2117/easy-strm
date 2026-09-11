// 更新日期：2026-09-10；执行者：Codex。资料库STRM配置、筛选导出、错误恢复及窄屏验证，API使用本地模拟。
import assert from 'node:assert/strict'
import { mkdir } from 'node:fs/promises'
import { chromium } from 'playwright'
import { createServer } from 'vite'

const server = await createServer({ server: { host: '127.0.0.1', port: 3107, strictPort: true } })
await server.listen()
let browser
let page
const pageErrors = []
try {
  browser = await chromium.launch({ headless: true })
  page = await browser.newPage()
  page.on('pageerror', error => pageErrors.push(error.message))
  await page.addInitScript(() => localStorage.setItem('token', 'fixture'))
  let config = { output_path: '/media/library', base_url: 'http://media.example', cloud115_id: 7, transfer_path: '/播放' }
  let exports = 0, rejectSave = false, taskReads = 0
  let task = { task_id: 'share_strm_fixture', task_name: '分享资料库STRM导出', task_type: 'strm_generate', status: 'running', progress: 25, total_files: 100, processed_files: 25, success_files: 25, failed_files: 0, metadata: { exported_files: 20 } }
  await page.route('**/api/**', async route => {
    const req = route.request(), url = new URL(req.url()), path = url.pathname
    if (!path.startsWith('/api/')) return route.continue()
    let data = { data: [], total: 0 }
    if (path === '/api/tasks/share_strm_fixture') { taskReads++; data = task }
    if (path === '/api/tasks/unified') data = [task]
    if (path === '/api/cloud115') data = [{ id: 7, name: '播放账号' }]
    if (path === '/api/media/share-library/options') data = { genres: [], countries: [], years: [] }
    if (path === '/api/media/share-library') data = { data: [{ work_key: 'tmdb:tv:10', title: '测试剧', media_type: 'tv', year: 2024, source_count: 1, available: true }], total: 1 }
    if (path === '/api/media/share-library/strm/settings') {
      if (req.method() === 'PUT') {
        if (rejectSave) { await route.fulfill({ status: 400, json: { error: '配置保存失败' } }); return }
        config = req.postDataJSON()
      }
      data = config
    }
    if (path === '/api/media/share-library/strm/export') {
      assert.equal(req.method(), 'POST')
      assert.equal(url.searchParams.get('keyword'), '测试')
      assert.equal(url.searchParams.has('page'), false)
      exports++
      data = { task_id: 'share_strm_fixture' }
    }
    await route.fulfill({ json: { data } })
  })
  await page.goto('http://127.0.0.1:3107/dashboard/share-library')
  await page.getByText('测试剧', { exact: true }).waitFor()
  await page.getByPlaceholder('名称搜索').fill('测试')
  await page.getByRole('button', { name: '筛选', exact: true }).click()
  await page.getByRole('button', { name: '导出 STRM', exact: true }).click()
  const modal = page.locator('.n-modal')
  await modal.getByText('播放账号', { exact: true }).waitFor()
  const output = modal.getByPlaceholder('例如 /media/share-library（Docker 内路径）')
  await output.fill('/media/exported')
  rejectSave = true
  await modal.getByRole('button', { name: '保存并导出', exact: true }).click()
  await page.getByText('配置保存失败', { exact: true }).first().waitFor()
  assert.equal(exports, 0)
  rejectSave = false
  await modal.getByRole('button', { name: '保存并导出', exact: true }).click()
  await modal.getByText(/导出任务已创建：share_strm_fixture/).waitFor()
  assert.equal(exports, 1)
  assert.equal(config.output_path, '/media/exported')
  await modal.locator('.n-progress').getByText('25%', { exact: true }).waitFor()
  await modal.getByText('25 / 100', { exact: true }).waitFor()
  await modal.getByRole('button', { name: '关闭', exact: true }).click()
  await modal.waitFor({ state: 'hidden' })
  const stoppedReads = taskReads
  await page.waitForTimeout(1300)
  assert.equal(taskReads, stoppedReads, '关闭弹窗必须停止进度轮询')
  task = { ...task, progress: 75, processed_files: 75, success_files: 75 }
  await page.getByRole('button', { name: '导出 STRM', exact: true }).click()
  await modal.locator('.n-progress').getByText('75%', { exact: true }).waitFor()
  task = { ...task, status: 'completed', progress: 100, processed_files: 100, success_files: 100 }
  await modal.locator('.n-progress').getByText('完成', { exact: true }).waitFor()
  const completedReads = taskReads
  await page.waitForTimeout(1300)
  assert.equal(taskReads, completedReads, '完成后必须停止进度轮询')
  await modal.getByRole('button', { name: '关闭', exact: true }).click()
  await modal.waitFor({ state: 'hidden' })
  await page.reload()
  await page.getByRole('button', { name: '导出 STRM', exact: true }).click()
  await modal.getByText('播放账号', { exact: true }).waitFor()
  assert.equal(await output.inputValue(), '/media/exported')
  await page.setViewportSize({ width: 390, height: 844 })
  await modal.getByRole('button', { name: '保存并导出', exact: true }).scrollIntoViewIfNeeded()
  const box = await modal.getByRole('button', { name: '保存并导出', exact: true }).boundingBox()
  assert.ok(box && box.x >= 0 && box.x + box.width <= 390)
  await mkdir('../debug/share-strm', { recursive: true })
  await page.screenshot({ path: '../debug/share-strm/export-mobile.png', fullPage: true })
  await page.goto('http://127.0.0.1:3107/dashboard/tasks?task_id=share_strm_fixture')
  await page.locator('.n-drawer .n-progress').getByText('100%', { exact: true }).waitFor()
  await page.getByText('已处理 100 / 100', { exact: true }).waitFor()
  assert.deepEqual(pageErrors, [])
  console.log('PASS: 配置回显/保存/刷新持久化、已应用筛选全量导出、失败阻止导出并可重试、窄屏按钮可达、进度25→75→完成、关闭/终态停止轮询、详情进度条；API mocked')
} catch (error) {
  console.error('页面错误：', pageErrors, 'URL：', page?.url(), '页面：', await page?.locator('body').innerText())
  throw error
} finally {
  await browser?.close()
  await server.close()
}
