import { chromium } from 'playwright'
import assert from 'node:assert/strict'

// 用模拟接口验证核对流程，避免修改真实分享记录。
const browser = await chromium.launch({ headless: true })
const page = await browser.newPage({ viewport: { width: 1440, height: 1000 } })
const errors = []
page.on('pageerror', e => errors.push(e.message))
let items = [
  { id: 1, share_id: 8, share_name: '测试分享', file_name: '失败电影.mkv', media_type: 'movie', status: 'failed', error: '没有唯一匹配', version: 2, available: true },
  { id: 2, share_id: 8, share_name: '测试分享', file_name: '待识别电影.mkv', media_type: 'movie', status: 'pending', version: 1, available: true },
  { id: 3, share_id: 8, share_name: '测试分享', file_name: '剧集.S00E01E02.mkv', media_type: 'tv', status: 'failed', version: 3, available: true, episodes: [{ season_number: 0, episode_number: 1 }, { season_number: 0, episode_number: 2 }] }
]
const saves = []
await page.addInitScript(() => localStorage.setItem('token', 'fixture'))
await page.route('**/api/**', async route => {
  const url = new URL(route.request().url())
  let data = {}
  if (url.pathname.endsWith('/share-review')) {
    const rows = items.filter(i => !url.searchParams.get('media_id') || i.id === Number(url.searchParams.get('media_id')))
    data = { data: rows, total: rows.length }
  } else if (url.pathname.endsWith('/share-records')) data = { data: [{ id: 8, name: '测试分享' }], total: 1 }
  else if (url.pathname.endsWith('/tmdb/search')) data = { data: [{ title: '核对候选', original_title: 'Review', year: 2008, media_type: url.searchParams.get('type'), tmdb_id: 123, metadata_source: 'tmdb' }] }
  else if (url.pathname.endsWith('/manual-identify')) {
    const payload = route.request().postDataJSON()
    saves.push(payload)
    items = items.filter(i => i.id !== payload.id)
  }
  await route.fulfill({ json: { data } })
})
try {
  await page.goto((process.env.E2E_BASE_URL || 'http://localhost:3001') + '/dashboard/share-review?share_id=8')
  await page.getByRole('button', { name: '核对', exact: true }).first().click()
  await page.getByRole('button', { name: '搜索 TMDB', exact: true }).click()
  await page.getByRole('button', { name: '选择候选', exact: true }).click()
  assert.equal(saves.length, 0)
  await page.getByRole('button', { name: '保存核对结果', exact: true }).click()
  await page.getByText('共 2 条待核对媒体').waitFor()
  assert.equal(saves[0].version, 2)
  assert.equal(saves[0].result.media_type, 'movie')
  await page.goto((process.env.E2E_BASE_URL || 'http://localhost:3001') + '/dashboard/share-review?share_id=8&media_id=3')
  await page.getByRole('button', { name: '搜索 TMDB', exact: true }).click()
  await page.getByRole('button', { name: '选择候选', exact: true }).click()
  await page.getByRole('button', { name: '保存核对结果', exact: true }).click()
  await page.getByText('共 1 条待核对媒体').waitFor()
  assert.deepEqual(saves[1].episodes, [{ season_number: 0, episode_number: 1 }, { season_number: 0, episode_number: 2 }])
  await page.getByRole('button', { name: '核对', exact: true }).click()
  await page.getByRole('button', { name: '搜索 TMDB', exact: true }).click()
  await page.getByRole('button', { name: '选择候选', exact: true }).click()
  await page.getByRole('button', { name: '保存核对结果', exact: true }).click()
  await page.getByText('共 0 条待核对媒体').waitFor()
  assert.equal(errors.length, 0, errors.join('\n'))
  console.log('PASS: failed movie, pending movie, multi-episode TV, deep link, confirmation and queue refresh')
} catch(error) { console.error(errors, await page.locator('body').innerText()); throw error } finally { await browser.close() }
