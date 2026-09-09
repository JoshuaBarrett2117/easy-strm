// 更新日期：2026-09-09；执行者：Codex。接口夹具验证新页面及原STRM/分享入口。
import assert from 'node:assert/strict'
import { chromium } from 'playwright'
import fs from 'node:fs/promises'
const browser = await chromium.launch({ headless: true })
const origin = process.env.E2E_BASE_URL || 'http://127.0.0.1:3001'
try {
  const page = await browser.newPage({ viewport: { width: 1440, height: 1000 } })
  const errors = []
  page.on('pageerror', (e) => errors.push(e.message))
  await page.addInitScript(() => localStorage.setItem('token', 'fixture'))
  const handlers = [
    {
      key: 'full_generate',
      name: 'STRM全量生成',
      parameters: [
        { key: 'cloud115_id', label: '115账号ID', default: 0 },
        { key: 'strm_config_id', label: 'STRM配置ID', default: 0 }
      ]
    },
    {
      key: 'identify_cache_cleanup',
      name: '识别缓存清理',
      parameters: [{ key: 'keep_days', label: '保留天数', default: 30 }]
    }
  ]
  let task = {
    id: 1,
    task_name: '已有全量任务',
    handler: 'full_generate',
    task_type: 'full_generate',
    strm_config_id: 2,
    cloud115_id: 1,
    params: { cloud115_id: 1, strm_config_id: 2 },
    timezone: 'Local',
    cron_expr: '0 0 3 * * *',
    status: 'enabled',
    builtin: false,
    last_run_status: 'success'
  }
  let enriched = false,
    saved = false,
    ran = false,
    filtered = false,
    sawSecondPage = false
  await page.route('**/api/**', async (route) => {
    const u = new URL(route.request().url()),
      path = u.pathname,
      method = route.request().method()
    if (!path.startsWith('/api/')) return route.continue()
    const ok = (data) => route.fulfill({ json: { data } })
    if (path === '/api/media/share-library/options')
      return ok({ genres: [18, 35], countries: ['CN', 'US'], years: [2024] })
    if (path === '/api/media/share-library') {
      const p = Number(u.searchParams.get('page'))
      if (p === 2) sawSecondPage = true
      if (u.searchParams.get('tmdb_id') === '100') {
        filtered = true
        assert.equal(u.searchParams.get('keyword'), '测试')
        return ok({
          data: [
            {
              work_key: 'tmdb:tv:100',
              tmdb_id: 100,
              title: '测试作品',
              year: 2024,
              media_type: 'tv',
              rating: 8.5,
              source_count: 2,
              available: true
            }
          ],
          total: 1
        })
      }
      return ok({
        data: Array.from({ length: p === 2 ? 1 : 24 }, (_, i) => ({
          work_key: 'tmdb:movie:' + (i + p * 24),
          title: p === 2 ? '第二页作品' : '作品' + i,
          year: 2024,
          media_type: 'movie',
          rating: i === 0 ? null : 8,
          source_count: 2,
          available: i !== 0
        })),
        total: 25
      })
    }
    if (path === '/api/media/share-library/sources')
      return ok({
        data: [
          {
            id: 1,
            share_id: 9,
            name: '测试分享来源',
            file_name: '作品/第一集.mkv',
            url: 'https://example.com/share',
            password: 'abcd',
            share_cancelled: true
          }
        ],
        total: 1
      })
    if (path === '/api/media/share-library/enrich') {
      enriched = true
      return ok({ task_id: 'metadata-1' })
    }
    if (path === '/api/media/share-records') {
      assert.equal(u.searchParams.get('share_id'), '9')
      return ok({ data: [{ id: 9, name: '测试分享来源', identified_count: 1 }], total: 1 })
    }
    if (path === '/api/cron/handlers') return ok(handlers)
    if (path === '/api/cron/tasks')
      return ok(
        u.searchParams.has('page')
          ? { data: [task], total: 1 }
          : [{ ...task, id: 8, handler: 'incremental_sync', task_type: 'incremental_sync', status: 'enabled' }, task]
      )
    if (path === '/api/strm/config')
      return ok([
        {
          id: 2,
          cloud115_id: 1,
          net_disk_path: '/测试媒体',
          local_path: '/strm',
          cron: '0 0 3 * * *',
          extension: 'mkv'
        }
      ])
    if (path === '/api/cloud115') return ok([{ id: 1, name: '测试账号' }])
    if (path === '/api/cron/task/1' && method === 'PUT') {
      task = { ...task, ...route.request().postDataJSON() }
      return ok(task)
    }
    if (path === '/api/cron/task' && method === 'POST') {
      const body = route.request().postDataJSON()
      assert.equal(body.task_name, '缓存清理测试')
      assert.equal(body.handler, 'identify_cache_cleanup')
      assert.equal(body.params.keep_days, 45)
      saved = true
      return ok({ ...body, id: 2 })
    }
    if (path === '/api/cron/task/1/run') {
      ran = true
      return ok({ task_id: 'cron-fixture-1' })
    }
    if (path === '/api/cron/task/1/runs')
      return ok({
        data: [
          {
            id: 1,
            task_id: 'cron-fixture-1',
            trigger_type: 'manual',
            status: 'skipped',
            message: '同一配置正在执行',
            started_at: '2026-09-09T03:00:00Z'
          }
        ],
        total: 1
      })
    return ok([])
  })
  await page.goto(origin + '/dashboard/share-library')
  await page.locator('.poster-card').nth(23).waitFor()
  assert.equal(await page.locator('.poster-card').count(), 24)
  await page.getByText('暂无评分', { exact: false }).first().waitFor()
  await page.locator('.library-page > .n-pagination .n-pagination-item').filter({ hasText: /^2$/ }).click()
  await page.getByText('第二页作品', { exact: true }).waitFor()
  assert.ok(sawSecondPage)
  await page.getByPlaceholder('名称搜索').fill('测试')
  await page.getByPlaceholder('TMDB ID').fill('100')
  await page.getByRole('button', { name: '筛选', exact: true }).click()
  await page.getByText('测试作品', { exact: true }).waitFor()
  assert.ok(filtered)
  await page.locator('.poster-card').click()
  await page.getByText('测试分享来源', { exact: true }).waitFor()
  await page.getByText('分享已取消', { exact: true }).waitFor()
  await page.getByRole('button', { name: '进入分享管理', exact: true }).click()
  await page.waitForURL('**/share-records?share_id=9')
  await page.getByText('测试分享来源', { exact: true }).waitFor()
  await page.goto(origin + '/dashboard/share-library')
  await page.getByRole('button', { name: '补全历史元数据' }).click()
  await page.getByText(/补全任务已创建/).waitFor()
  assert.ok(enriched)
  await fs.mkdir(new URL('../../debug/library-schedules/', import.meta.url), { recursive: true })
  await page.screenshot({
    path: new URL('../../debug/library-schedules/library-desktop.png', import.meta.url).pathname.replace(
      /^\/([A-Z]:)/,
      '$1'
    ),
    fullPage: true
  })
  await page.setViewportSize({ width: 390, height: 844 })
  await page.locator('.poster-card').first().waitFor()
  assert.ok(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth + 1))
  await page.screenshot({
    path: new URL('../../debug/library-schedules/library-mobile.png', import.meta.url).pathname.replace(
      /^\/([A-Z]:)/,
      '$1'
    ),
    fullPage: true
  })
  await page.setViewportSize({ width: 1440, height: 1000 })
  await page.goto(origin + '/dashboard/scheduled-tasks')
  await page.getByText('已有全量任务', { exact: true }).waitFor()
  await page.getByRole('button', { name: '停用', exact: true }).click()
  await page.getByRole('button', { name: '启用', exact: true }).waitFor()
  assert.equal(task.status, 'disabled')
  await page.getByRole('button', { name: '立即执行', exact: true }).click()
  await page.getByText(/触发记录：cron-fixture-1/).waitFor()
  assert.ok(ran)
  await page.getByRole('button', { name: '记录', exact: true }).click()
  await page.getByText('同一配置正在执行', { exact: true }).waitFor()
  await page.keyboard.press('Escape')
  await page.getByRole('button', { name: '新建任务', exact: true }).click()
  const modal = page.locator('.n-modal')
  await modal.locator('.n-form-item').filter({ hasText: '任务名称' }).locator('input').fill('缓存清理测试')
  await modal.locator('.n-form-item').filter({ hasText: '处理器' }).locator('.n-base-selection').click()
  await page.getByText('识别缓存清理', { exact: true }).last().click()
  await modal.locator('.n-form-item').filter({ hasText: '保留天数' }).locator('input').fill('45')
  await modal.getByRole('button', { name: '保存', exact: true }).click()
  await page.getByText('任务已保存，后续调度已更新', { exact: true }).waitFor()
  assert.ok(saved)
  await page.screenshot({
    path: new URL('../../debug/library-schedules/schedules-desktop.png', import.meta.url).pathname.replace(
      /^\/([A-Z]:)/,
      '$1'
    ),
    fullPage: true
  })
  await page.goto(origin + '/dashboard/strm-config')
  await page.getByText('/测试媒体', { exact: true }).waitFor()
  await page.getByText('已禁用', { exact: true }).waitFor()
  await page.getByRole('button', { name: '定时任务', exact: true }).click()
  await page.getByText('立即执行', { exact: true }).last().click()
  await page.getByText(/定时任务已触发：cron-fixture-1/).waitFor()
  assert.deepEqual(errors, [])
  console.log(
    'PASS: 资源库分页/搜索/失效来源/分享跳转/历史补全/窄屏，定时任务启停/执行/历史/参数表单，原STRM页面选择全量任务并返回执行ID；API mocked，无页面异常'
  )
} finally {
  await browser.close()
}
