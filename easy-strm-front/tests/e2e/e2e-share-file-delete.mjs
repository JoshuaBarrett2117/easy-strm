// 更新日期：2026-10-07；维护者：Codex。验证单文件排队不会禁用同分享其他文件。
import assert from 'node:assert/strict'
import { chromium } from 'playwright'

const browser = await chromium.launch({ headless: true })
try {
  const page = await browser.newPage({ viewport: { width: 1600, height: 1000 } })
  const errors = []
  page.on('pageerror', error => errors.push(error.message))
  let status = null, submissions = 0
  const task = () => ({ task_id: 'delete-file-11', task_type: 'share_media_delete', status, metadata: { record_ids: [1], file_id: 11, cancellable: status === 'pending', result: { deleted: 1 } } })
  await page.addInitScript(() => localStorage.setItem('token', 'test'))
  await page.route('**/api/**', async route => {
    const path = new URL(route.request().url()).pathname
    if (!path.startsWith('/api/')) return route.continue()
    if (path === '/api/media/share-records/1/media/11') {
      submissions++; status = 'pending'
      return route.fulfill({ json: { data: { task_id: 'delete-file-11' } } })
    }
    if (path === '/api/tasks/delete-file-11/cancel') { status = 'cancelled'; return route.fulfill({ json: { data: {} } }) }
    if (path === '/api/tasks/delete-file-11') return route.fulfill({ json: { data: task() } })
    if (path === '/api/tasks/unified') return route.fulfill({ json: { data: status ? [task()] : [] } })
    if (path === '/api/media/share-records') return route.fulfill({ json: { data: { data: [{ id: 1, name: '文件粒度分享', file_count: 2 }], total: 1 } } })
    if (path === '/api/media/share-records/1/media') return route.fulfill({ json: { data: { data: [], total: 0 } } })
    if (path === '/api/media/share-records/1/files') return route.fulfill({ json: { data: { data: [
      { id: 11, file_name: '目标A.mkv', status: 'pending', available: true },
      { id: 12, file_name: '独立B.mkv', status: 'pending', available: true }
    ], total: 2 } } })
    return route.fulfill({ json: { data: [] } })
  })
  const openFiles = async () => { await page.locator('.n-data-table-expand-trigger').click(); await page.getByRole('button', { name: '查看全部文件', exact: true }).click() }
  const row = name => page.locator('tr').filter({ hasText: name })
  await page.goto(`${process.env.E2E_BASE_URL || 'http://127.0.0.1:3001'}/dashboard/share-records`)
  await openFiles()
  await row('目标A.mkv').getByRole('button', { name: '删除候选', exact: true }).click()
  await page.getByRole('button', { name: '确认删除', exact: true }).click()
  await page.getByRole('button', { name: '确认删除', exact: true }).waitFor({ state: 'detached' })
  assert.equal(await row('目标A.mkv').getByRole('button', { name: '删除候选', exact: true }).isDisabled(), true)
  assert.equal(await row('独立B.mkv').getByRole('button', { name: '自动识别', exact: true }).isEnabled(), true)
  assert.equal(await row('独立B.mkv').getByRole('button', { name: '删除候选', exact: true }).isEnabled(), true)
  await page.reload()
  await page.getByText('等待目标资源，其他分享可继续操作', { exact: true }).waitFor()
  await openFiles()
  assert.equal(await row('目标A.mkv').getByRole('button', { name: '删除候选', exact: true }).isDisabled(), true)
  assert.equal(await row('独立B.mkv').getByRole('button', { name: '手动识别', exact: true }).isEnabled(), true)
  assert.equal(submissions, 1)
  await page.keyboard.press('Escape')
  await page.getByRole('button', { name: '取消等待', exact: true }).click()
  await page.getByText('已取消等待中的操作', { exact: true }).waitFor()
  assert.deepEqual(errors, [])
  console.log('PASS file-delete: 文件候选异步提交、仅目标文件受限、同分享其他文件可操作、刷新恢复、等待取消（API mocked）')
} finally { await browser.close() }
