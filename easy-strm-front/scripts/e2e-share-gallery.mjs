// 更新日期：2026-09-12；执行者：Codex。验证分享文件同步、媒体聚合和文件级季集展示。
import assert from 'node:assert/strict'
import { chromium } from 'playwright'

const browser = await chromium.launch({ headless: true })
try {
  let syncCalled = false
  const page = await browser.newPage()
  await page.addInitScript(() => localStorage.setItem('token', 'test'))
  await page.route('**/api/**', async route => {
    const url = new URL(route.request().url())
    const path = url.pathname
    if (!path.startsWith('/api/')) return route.continue()
    if (path === '/api/media/share-records/1/sync') {
      syncCalled = true
      return route.fulfill({ json: { data: { task_id: 'sync-test' } } })
    }
    if (path === '/api/tasks/sync-test') {
      return route.fulfill({ json: { data: { task_id: 'sync-test', status: 'completed', progress: 100, metadata: { file_count: 2 } } } })
    }
    if (path === '/api/media/share-records/1/media') {
      return route.fulfill({ json: { data: { data: [{ id: 11, media_id: 101, share_id: 1, file_name: '剧集/Show.S01E01E02.mkv', file_count: 2, available: true, metadata_source: 'tmdb', status: 'identified', version: 2, result: { success: true, tmdb_id: 1, media_type: 'tv', title: '剧集' } }], total: 1, duplicate_count: 1 } } })
    }
    if (path === '/api/media/share-records/1/files') {
      return route.fulfill({ json: { data: { data: [
        { id: 11, file_name: '剧集/Show.S01E01E02.mkv', status: 'identified', available: true, episodes: [{ season_number: 1, episode_number: 1 }, { season_number: 1, episode_number: 2 }] },
        { id: 12, file_name: '剧集/unknown.mkv', status: 'failed', available: true, error: '缺少季集信息', episodes: [] }
      ], total: 2 } } })
    }
    if (path === '/api/media/share-records') {
      return route.fulfill({ json: { data: { data: [{ id: 1, name: '测试分享', file_count: 2, media_count: 1, identified_count: 1, pending_count: 0, failed_count: 1, unavailable_count: 0 }], total: 1 } } })
    }
    return route.fulfill({ json: { data: [] } })
  })

  await page.goto('http://127.0.0.1:3001/dashboard/share-records')
  await page.getByText('测试分享', { exact: true }).waitFor()
  const syncResponse = page.waitForResponse(response => response.url().endsWith('/share-records/1/sync'))
  await page.getByRole('button', { name: '同步分享文件', exact: true }).click()
  await syncResponse
  assert.equal(syncCalled, true)

  await page.locator('.n-data-table-expand-trigger').click()
  await page.locator('.media-card').waitFor()
  assert.equal(await page.locator('.media-card').count(), 1)
  await page.getByRole('button', { name: '查看全部文件', exact: true }).click()
  await page.getByText('S01E01、S01E02', { exact: true }).waitFor()
  await page.getByText('缺少季集信息', { exact: true }).waitFor()
  console.log('PASS: 分享同步、媒体主数据聚合、文件状态和多集映射展示')
} finally {
  await browser.close()
}
