// 更新日期：2026-10-06；维护者：Codex。模拟 API 验证真实实例弹窗，不写入实际服务器。
import assert from 'node:assert/strict'
import { chromium } from 'playwright'

const url = process.env.E2E_FRONTEND_URL || 'http://127.0.0.1:3001'
let server = { id: 1, name: '测试 Emby', base_url: 'http://emby:8096', api_key_mask: '********', enabled: true, is_default: true, proxy_port: 8097 }
let lastSave
let failSave = false
const browser = await chromium.launch({ headless: true })
try {
  const page = await browser.newPage({ viewport: { width: 1280, height: 900 } })
  await page.addInitScript(() => {
    localStorage.setItem('token', 'e2e-token')
    localStorage.setItem('user_name', 'admin')
  })
  await page.route('**/api/**', async route => {
    const path = new URL(route.request().url()).pathname
    if (!path.startsWith('/api/')) return route.continue()
    const method = route.request().method()
    let data = { data: [], total: 0 }
    if (path === '/api/emby/servers' && method === 'GET') data = { data: [server], total: 1 }
    if (path === '/api/emby/servers/1' && method === 'PUT') {
      lastSave = route.request().postDataJSON()
      if (failSave) return route.fulfill({ status: 400, json: { state: false, code: 400, error: '反代端口已被占用' } })
      server = { ...server, ...lastSave }
      data = { server, task_id: 'mock-task' }
    }
    await route.fulfill({ json: { state: true, code: 0, data } })
  })
  await page.goto(`${url}/dashboard/emby-management`)
  const open = async () => {
    await page.getByRole('button', { name: '编辑', exact: true }).click()
    await page.getByText('编辑 Emby 实例', { exact: true }).waitFor()
  }
  const dialog = page.locator('.n-modal').filter({ hasText: '编辑 Emby 实例' })
  const port = () => dialog.locator('.n-input-number input')
  const save = async () => {
    await dialog.getByRole('button', { name: '保存', exact: true }).click()
    await dialog.waitFor({ state: 'hidden' })
  }
  await open()
  assert.equal(await port().inputValue(), '8097')
  assert.match(await dialog.innerText(), /Docker 部署还必须发布对应容器端口/)
  await port().fill('8098')
  await save()
  assert.equal(lastSave.proxy_port, 8098)
  await open()
  assert.equal(await port().inputValue(), '8098')
  await port().fill('')
  await save()
  assert.equal(lastSave.proxy_port, 0)
  await open()
  assert.equal(await port().inputValue(), '0')
  failSave = true
  await port().fill('8099')
  await dialog.getByRole('button', { name: '保存', exact: true }).click()
  await page.getByText('反代端口已被占用', { exact: true }).waitFor()
  assert.equal(await dialog.isVisible(), true)
  assert.equal(server.proxy_port, 0)
  console.log(JSON.stringify({ ok: true, backend: 'mock', cases: ['端口回显', '保存', '重开回显', '留空关闭', 'Docker提示', '保存失败保留弹窗'] }))
} finally {
  await browser.close()
}
