// 更新日期：2026-09-07；执行者：Codex。用可回读的接口夹具验证真实页面的权限选择与保存。
import assert from 'node:assert/strict'
import { mkdir } from 'node:fs/promises'
import { chromium } from 'playwright'

const url = process.env.E2E_FRONTEND_URL || 'http://127.0.0.1:3001'
const folders = [
  { id: 'eef44ceeaf834d4293445eef5872ba74', item_id: '127953', name: '电影' },
  { id: '0ba54b295beb487cb7617f5d066bd85c', item_id: '127954', name: '电视剧' }
]
let policy = { EnableAllFolders: false, EnabledFolders: [folders[0].id] }
let lastSave
let failSave = false
const ok = data => ({ state: true, code: 0, data })
const browser = await chromium.launch({ headless: true })
try {
  const page = await browser.newPage({ viewport: { width: 1440, height: 1000 } })
  await page.addInitScript(() => {
    localStorage.setItem('token', 'e2e-token')
    localStorage.setItem('user_name', 'admin')
  })
  await page.route('**/api/**', async route => {
    const path = new URL(route.request().url()).pathname
    if (!path.startsWith('/api/')) return route.continue()
    let data = { data: [], total: 0 }
    if (path === '/api/emby/servers') data = { data: [{ id: 1, name: '测试 Emby', enabled: true, is_default: true }] }
    if (path.endsWith('/user-libraries')) data = { data: folders, total: folders.length }
    if (path.endsWith('/libraries')) data = { data: folders.map(f => ({ ItemId: f.item_id, Name: f.name })) }
    if (path.endsWith('/users')) data = { data: [{ Id: 'u1', Name: '115tv', Policy: policy }] }
    if (path.endsWith('/users/u1') && route.request().method() === 'PUT') {
      lastSave = route.request().postDataJSON()
      if (failSave) return route.fulfill({ status: 502, json: { error: 'Emby 未应用所选媒体库权限，请重新加载后重试' } })
      policy = lastSave.policy
      data = { task_id: 'test-task' }
    }
    if (path.includes('/cover')) return route.fulfill({ status: 404, body: '' })
    await route.fulfill({ json: ok(data) })
  })
  await page.goto(`${url}/dashboard/emby-management`)
  const dialog = page.locator('.n-modal').filter({ hasText: '编辑 Emby 用户' })
  const open = async () => {
    await page.getByRole('button', { name: '编辑权限', exact: true }).click()
    await dialog.waitFor()
  }
  const save = async () => {
    await dialog.getByRole('button', { name: '保存', exact: true }).click()
    await dialog.waitFor({ state: 'hidden' })
  }
  await open()
  const selection = () => dialog.locator('.n-base-selection-tags')
  assert.match(await selection().innerText(), /电影/)
  assert.doesNotMatch(await selection().innerText(), /eef44|127953/)
  await dialog.locator('.n-select').click()
  await page.locator('.n-base-select-option').filter({ hasText: '电视剧' }).click()
  await dialog.getByText('用户名', { exact: true }).click()
  await save()
  assert.deepEqual(lastSave.policy.EnabledFolders, folders.map(f => f.id))
  assert.equal(lastSave.policy.EnableAllFolders, false)
  await open()
  assert.match(await selection().innerText(), /电影/)
  assert.match(await selection().innerText(), /电视剧/)
  await mkdir('../debug/emby-user-access', { recursive: true })
  await dialog.screenshot({ path: '../debug/emby-user-access/names-after-save.png', animations: 'disabled' })
  await dialog.getByRole('checkbox', { name: '全部媒体库', exact: true }).click()
  await save()
  assert.equal(lastSave.policy.EnableAllFolders, true)
  assert.deepEqual(lastSave.policy.EnabledFolders, [])
  await open()
  await dialog.getByRole('checkbox', { name: '全部媒体库', exact: true }).click()
  await save()
  assert.equal(lastSave.policy.EnableAllFolders, false)
  assert.deepEqual(lastSave.policy.EnabledFolders, [])
  // 模拟旧版本已保存数字 ItemId，编辑时仍显示名称并提交 Guid。
  policy = { EnableAllFolders: false, EnabledFolders: [folders[0].item_id] }
  await page.reload()
  await open()
  assert.match(await selection().innerText(), /电影/)
  await save()
  assert.deepEqual(lastSave.policy.EnabledFolders, [folders[0].id])
  failSave = true
  await open()
  await dialog.getByRole('button', { name: '保存', exact: true }).click()
  await page.getByText('Emby 未应用所选媒体库权限，请重新加载后重试', { exact: true }).waitFor()
  assert.equal(await dialog.isVisible(), true)
  console.log(JSON.stringify({ ok: true, cases: ['名称回显', 'Guid提交', '保存后重开', '全部媒体库', '空范围', '历史数字ID', '失败保留弹窗'], backend: 'mock' }))
} finally {
  await browser.close()
}
