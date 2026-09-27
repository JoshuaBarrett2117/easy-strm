// 更新日期：2026-09-27；维护者：Codex。模拟接口验证删除确认和失败重试，不操作真实分享。
import assert from 'node:assert/strict'
import { chromium } from 'playwright'

const browser = await chromium.launch({ headless: true })
try {
  const page = await browser.newPage()
  let deletes = 0, fail = true, removed = false
  await page.addInitScript(() => localStorage.setItem('token', 'test'))
  await page.route('**/api/**', async route => {
    const request = route.request(), path = new URL(request.url()).pathname
    if (!path.startsWith('/api/')) return route.continue()
    if (path === '/api/media/share-records/9' && request.method() === 'DELETE') {
      deletes++
      if (fail) return route.fulfill({ status: 500, json: { error: '分享STRM导出正在运行' } })
      removed = true
      return route.fulfill({ json: { data: null } })
    }
    return route.fulfill({ json: { data: { data: path === '/api/media/share-records' && !removed ? [{ id: 9, name: '删除回归分享', file_count: 3, media_count: 2 }] : [], total: removed ? 0 : 1 } } })
  })
  await page.goto(`${process.env.E2E_BASE_URL || 'http://127.0.0.1:3001'}/dashboard/share-records`)
  const open = () => page.getByRole('button', { name: '删除', exact: true }).click()
  await open()
  await page.getByText(/关联的 STRM 数据库记录和已生成的 STRM 文件/).waitFor()
  assert.equal(deletes, 0)
  await page.getByRole('button', { name: '取消', exact: true }).click()
  assert.equal(deletes, 0)
  await open()
  await page.getByRole('button', { name: '确认删除', exact: true }).click()
  await page.getByText('分享STRM导出正在运行', { exact: true }).waitFor()
  assert.equal(deletes, 1)
  assert.equal(await page.getByRole('button', { name: '确认删除', exact: true }).isVisible(), true)
  fail = false
  await page.getByRole('button', { name: '确认删除', exact: true }).click()
  await page.getByText('分享及关联的文件记录和 STRM 已删除', { exact: true }).waitFor()
  await page.getByRole('button', { name: '删除', exact: true }).waitFor({ state: 'detached' })
  assert.equal(deletes, 2)
  console.log('PASS: 取消不请求、清理范围提示、失败保留弹窗、重试成功刷新列表（API mocked）')
} finally { await browser.close() }
