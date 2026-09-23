// 更新日期：2026-09-23；维护者：Codex。模拟 Emby 接口验证编辑与定时任务交互。
import assert from 'node:assert/strict'
import { chromium } from 'playwright'
const browser = await chromium.launch({ headless: true })
try {
 const page = await browser.newPage({ viewport: { width: 1280, height: 900 } })
 await page.addInitScript(() => { localStorage.setItem('token', 'test'); localStorage.setItem('user_name', 'admin') })
 let runs = 0, reads = 0, saved
 await page.route('**/api/**', async route => {
  const path = new URL(route.request().url()).pathname
  let data = { data: [], total: 0 }
  if (path === '/api/emby/servers') data = { data: [{ id: 1, name: '测试', enabled: true, is_default: true }] }
  if (path.endsWith('/libraries')) data = { data: [{ ItemId: '123', Name: '电影测试', CollectionType: 'movies', Locations: ['/media'], LibraryOptions: { PreferredMetadataLanguage: 'en', MetadataCountryCode: 'US', EnableRealtimeMonitor: false } }] }
  if (path.endsWith('/libraries/123') && route.request().method() === 'PUT') { saved = route.request().postDataJSON(); data = { task_id: 'edit' } }
  if (path.endsWith('/scheduled-tasks')) { reads++; data = { data: [{ Id: 'task1', Name: '媒体扫描测试', State: runs ? 'Running' : 'Idle', CurrentProgressPercentage: 20, Triggers: [{ Type: 'DailyTrigger', TimeOfDayTicks: 72000000000 }] }] } }
  if (path.endsWith('/task1/run')) { runs++; data = { task_id: 'trigger' } }
  if (path.includes('/cover')) return route.fulfill({ status: 404, body: '' })
  await route.fulfill({ json: { state: true, code: 0, data } })
 })
 await page.goto(`${process.env.E2E_FRONTEND_URL || 'http://127.0.0.1:3017'}/dashboard/emby-management`)
 await page.getByText('媒体库管理', { exact: true }).click()
 await page.getByRole('button', { name: '编辑', exact: true }).last().click()
 const modal = page.locator('.n-modal').filter({ hasText: '编辑媒体库' })
 await modal.getByRole('button', { name: '保存', exact: true }).click()
 await modal.waitFor({ state: 'hidden' })
 assert.equal(saved.metadata_language, 'en')
 assert.equal(saved.metadata_country, 'US')
 assert.equal(saved.enable_realtime_monitor, false)
 await page.getByText('定时任务管理', { exact: true }).click()
 await page.getByText('媒体扫描测试', { exact: true }).waitFor()
 assert.match(await page.locator('body').innerText(), /每天 02:00/)
 await page.getByRole('button', { name: '立即触发', exact: true }).click()
 await page.getByText('运行中', { exact: true }).waitFor()
 assert.equal(runs, 1)
 assert.equal(await page.getByRole('button', { name: '立即触发', exact: true }).isDisabled(), true)
 await page.getByText('用户管理', { exact: true }).click()
 const before = reads
 await page.waitForTimeout(5500)
 assert.equal(reads, before, '离开页签后应停止轮询')
 console.log('PASS: 编辑保留配置、任务查看、立即触发、防重复与轮询停止')
} finally { await browser.close() }
