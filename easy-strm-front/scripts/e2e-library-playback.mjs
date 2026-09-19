// 更新日期：2026-09-19；维护者：Codex。验证分集滚动和 Emby 跳转。
import assert from 'node:assert/strict'
import { chromium } from 'playwright'

const browser = await chromium.launch({ headless: true })
try {
  const page = await browser.newPage({ viewport: { width: 1920, height: 911 } })
  await page.addInitScript(() => localStorage.setItem('token', 'fixture'))
  await page.route('**/api/**', route => {
    const url = new URL(route.request().url())
    const ok = data => route.fulfill({ json: { data } })
    if (url.pathname === '/api/media/share-library') return ok({ data: [{ work_key: 'tmdb:tv:105136', tmdb_id: 105136, media_type: 'tv', title: '测试剧集', source_count: 2 }], total: 1 })
    if (url.pathname.endsWith('/options')) return ok({ genres: [], countries: [] })
    if (url.pathname.endsWith('/tv-detail')) return ok({ title: '测试剧集', seasons: [{ season_number: 2, episode_count: 8, matched_episode_count: 8, file_count: 16 }] })
    if (url.pathname.endsWith('/tv-seasons')) return ok({ season_number: 2, episodes: Array.from({ length: 8 }, (_, i) => ({ episode_number: i + 1, name: `第 ${i + 1} 集`, files: [1, 2].map(id => ({ id, name: '测试分享', file_name: '欧美剧集/2021/测试剧集/Season 2/测试剧集 S02E01.mp4', file_size: 3000000000, available: true })) })) })
    if (url.pathname.endsWith('/emby/servers')) return ok({ data: [{ id: 1, name: '家庭 Emby', enabled: true, is_default: true }] })
    if (url.pathname.endsWith('/playback-links')) {
      assert.equal(url.searchParams.get('tmdb_id'), '105136')
      assert.equal(url.searchParams.get('season_number'), '2')
      assert.equal(url.searchParams.get('episode_number'), '8')
      return ok({ data: [{ item_id: 'ep8', name: '第 8 集', url: 'https://emby.example/web/index.html#!/item?id=ep8&serverId=server' }], total: 1 })
    }
    return ok([])
  })
  await page.goto(process.env.E2E_BASE_URL || 'http://127.0.0.1:3001/dashboard/share-library')
  await page.locator('.poster-card').click()
  await page.locator('.episode-row').last().waitFor()
  assert.equal(await page.locator('.episode-row').count(), 8)
  for (const viewport of [{ width: 1920, height: 911 }, { width: 1280, height: 720 }, { width: 390, height: 844 }]) {
    await page.setViewportSize(viewport)
    const last = page.locator('.episode-row').last()
    await last.scrollIntoViewIfNeeded()
    const reachable = await last.evaluate(el => {
      const box = el.getBoundingClientRect()
      const x = box.left + box.width / 2, y = Math.min(box.bottom - 5, innerHeight - 5)
      return y >= box.top && el.contains(document.elementFromPoint(x, y))
    })
    assert.ok(reachable, `第 8 集应能滚动到可见区域：${viewport.width}x${viewport.height}`)
  }
  if (!process.env.SCROLL_ONLY) {
    await page.locator('.episode-row').last().getByRole('button', { name: '去 Emby 播放' }).click()
    const link = page.getByRole('link', { name: '在 Emby 打开：第 8 集' })
    await link.waitFor()
    assert.equal(await link.getAttribute('href'), 'https://emby.example/web/index.html#!/item?id=ep8&serverId=server')
    assert.equal(await link.getAttribute('target'), '_blank')
  }
  console.log('PASS: 八集列表在桌面、矮屏和手机均可滚动到底；Emby 分集跳转正确')
} finally { await browser.close() }
