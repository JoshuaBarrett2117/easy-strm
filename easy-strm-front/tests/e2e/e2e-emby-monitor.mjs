import { chromium } from 'playwright'

const FRONTEND_URL = process.env.E2E_FRONTEND_URL || 'http://127.0.0.1:3001'
const ok = data => ({ state: true, code: 0, message: 'success', data, error: '', errno: 0 })
const now = new Date().toISOString()

async function main() {
  const browser = await chromium.launch({ headless: true })
  try {
    const context = await browser.newContext({ viewport: { width: 1440, height: 960 } })
    await context.addInitScript(() => {
      localStorage.setItem('token', 'e2e-token')
      localStorage.setItem('user_name', 'admin')
    })
    const page = await context.newPage()
    await page.route('**/api/emby/servers', route => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(ok({ data: [{ id: 1, name: '测试 Emby', enabled: true, is_default: true }], total: 1 })) }))
    await page.route('**/api/emby/servers/1/monitor/overview', route => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(ok({ meta: { source: 'playback_reporting', generated_at: now, timezone: 'Asia/Shanghai', history_start_at: now }, summary: { peak_users: 2, today_seconds: 7200, week_seconds: 18000, month_seconds: 36000, total_seconds: 72000, today_active_users: 2, week_active_users: 4, month_active_users: 6 }, trend: [{ time: now, active_users: 2, watched_seconds: 3600 }], sessions: [{ session_id: 's1', user_id: 'u1', user_name: '测试用户', item_id: 'm1', item_name: '测试电影', client_name: 'SenPlayer', device_name: 'Apple TV', playback_method: 'DirectPlay', paused: false, progress: 45 }] })) }))
    await page.route('**/api/emby/servers/1/monitor/rankings/*', route => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(ok({ meta: { source: 'playback_reporting', generated_at: now }, data: [{ rank: 1, id: 'u1', name: '测试用户', watched_seconds: 7200, play_count: 3, percentage: 100 }], total: 1 })) }))
    await page.route('**/api/emby/servers/1/monitor/heatmap*', route => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(ok({ meta: { source: 'local_collector', generated_at: now, degraded_reason: '测试降级' }, users: [{ user_id: 'u1', user_name: '测试用户', total_seconds: 3600, cells: [{ date: '2026-08-30', hour: 10, seconds: 3600 }] }] })) }))
    await page.route('**/api/emby/servers/1/monitor/recent-items*', route => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(ok({ meta: { source: 'local_collector', generated_at: now }, data: [{ server_id: 1, item_id: 'm1', item_type: 'Movie', name: '测试电影', year: 2026, date_created: now, first_seen_at: now }], total: 1 })) }))
    await page.route('**/api/emby/servers/1/monitor/items/*/image', route => route.fulfill({ status: 404, contentType: 'application/json', body: JSON.stringify({ error: 'no image' }) }))
    await page.route('**/api/emby/servers/1/users/*/avatar', route => route.fulfill({ status: 404, contentType: 'application/json', body: JSON.stringify({ error: 'no avatar' }) }))

    await page.goto(`${FRONTEND_URL}/dashboard/emby-monitor`, { waitUntil: 'networkidle' })
    await page.getByText('Emby 观影监控').waitFor()
    await page.getByText('正在观看').waitFor()
    await page.getByText('测试电影').last().waitFor()

    for (const tab of ['用户排行', '媒体排行', '客户端排行']) {
      await page.getByText(tab, { exact: true }).click()
      await page.getByText('1. 测试用户', { exact: true }).last().waitFor()
    }
    await Promise.all([
      page.waitForResponse(response => response.url().includes('/monitor/heatmap') && response.ok()),
      page.getByText('活跃热力图', { exact: true }).click()
    ])
    await page.getByText('最近入库', { exact: true }).click()
    await page.getByText('暂无海报').waitFor()

    await page.setViewportSize({ width: 390, height: 844 })
    await page.getByText('测试电影').last().waitFor()
    console.log(JSON.stringify({ ok: true, tabs: 6, mobile: true }))
  } finally {
    await browser.close()
  }
}

main().catch(error => { console.error(error); process.exit(1) })
