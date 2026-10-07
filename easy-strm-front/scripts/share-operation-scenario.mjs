// 更新日期：2026-10-07；维护者：Codex。只模拟API，验证排队清理的真实页面交互。
import assert from 'node:assert/strict'
import { chromium } from 'playwright'

export async function runShareOperationScenario(kind) {
  const browser = await chromium.launch({ headless: true })
  try {
    const page = await browser.newPage({ viewport: { width: 1600, height: 1000 } })
    const errors = []
    page.on('pageerror', error => errors.push(error.message))
    let submits = 0, cancels = 0, fail = true, status = null, removed = false, count = 3
    const taskID = `test-${kind}`
    const taskType = kind === 'delete' ? 'share_delete' : 'share_clear'
    const task = () => ({
      task_id: taskID, task_type: taskType, task_name: kind === 'delete' ? '删除分享' : '清空分享文件记录',
      status, progress: status === 'completed' ? 100 : 0,
      metadata: { record_ids: [9], phase: status === 'pending' ? '等待目标资源' : '执行清理', cancellable: status === 'pending', blocking_task_ids: ['identify-A'], result: { deleted: 3 } }
    })
    await page.addInitScript(() => localStorage.setItem('token', 'test'))
    await page.route('**/api/**', async route => {
      const request = route.request(), path = new URL(request.url()).pathname
      if (!path.startsWith('/api/')) return route.continue()
      const endpoint = `/api/media/share-records/9${kind === 'delete' ? '' : '/media'}`
      if (path === endpoint && request.method() === 'DELETE') {
        submits++
        if (fail) return route.fulfill({ status: 409, json: { error: '队列保存失败' } })
        status = 'pending'
        return route.fulfill({ json: { data: { task_id: taskID } } })
      }
      if (path === `/api/tasks/${taskID}/cancel`) {
        assert.equal(status, 'pending')
        cancels++; status = 'cancelled'
        return route.fulfill({ json: { data: {} } })
      }
      if (path === `/api/tasks/${taskID}`) {
        if (status === 'completed') { if (kind === 'delete') removed = true; else count = 0 }
        return route.fulfill({ json: { data: task() } })
      }
      if (path === '/api/tasks/unified') return route.fulfill({ json: { data: status ? [task()] : [] } })
      if (path === '/api/media/share-records') {
        const rows = [{ id: 10, name: '无关分享B', file_count: 1, pending_count: 1, media_count: 1 }]
        if (!removed) rows.unshift({ id: 9, name: '排队分享A', file_count: count, pending_count: count, media_count: count })
        return route.fulfill({ json: { data: { data: rows, total: rows.length } } })
      }
      return route.fulfill({ json: { data: [] } })
    })
    const label = kind === 'delete' ? '删除' : '清空文件记录'
    const confirm = kind === 'delete' ? '确认删除' : '确认清空'
    const row = name => page.locator('tr').filter({ hasText: name })
    const submit = async () => { await row('排队分享A').getByRole('button', { name: label, exact: true }).click(); await page.getByRole('button', { name: confirm, exact: true }).click() }
    await page.goto(`${process.env.E2E_BASE_URL || 'http://127.0.0.1:3001'}/dashboard/share-records`)
    await row('排队分享A').getByRole('button', { name: label, exact: true }).click()
    await page.getByRole('button', { name: '取消', exact: true }).click()
    assert.equal(submits, 0)
    await submit()
    await page.getByText('队列保存失败', { exact: true }).waitFor()
    assert.equal(await page.getByRole('button', { name: confirm, exact: true }).isVisible(), true)
    fail = false
    await page.getByRole('button', { name: confirm, exact: true }).click()
    await page.getByRole('button', { name: confirm, exact: true }).waitFor({ state: 'detached' })
    await page.getByText('等待目标资源，其他分享可继续操作', { exact: true }).waitFor()
    assert.equal(await row('排队分享A').getByRole('button', { name: label, exact: true }).isDisabled(), true)
    assert.equal(await row('无关分享B').getByRole('button', { name: label, exact: true }).isEnabled(), true)
    assert.equal(removed, false); assert.equal(count, 3)
    await page.reload()
    await page.getByText('等待目标资源，其他分享可继续操作', { exact: true }).waitFor()
    assert.equal(submits, 2, '刷新页面不得重新提交删除')
    await page.getByRole('button', { name: '取消等待', exact: true }).click()
    await row('排队分享A').getByRole('button', { name: label, exact: true }).waitFor()
    await page.waitForFunction(label => {
      const target = [...document.querySelectorAll('tr')].find(row => row.textContent.includes('排队分享A'))
      return target && [...target.querySelectorAll('button')].some(button => button.textContent.trim() === label && !button.disabled)
    }, label)
    assert.equal(cancels, 1); assert.equal(removed, false); assert.equal(count, 3)
    await submit()
    await page.getByText('等待目标资源，其他分享可继续操作', { exact: true }).waitFor()
    status = 'running'
    await page.getByText('正在执行清理', { exact: true }).waitFor()
    await page.getByRole('button', { name: '取消等待', exact: true }).waitFor({ state: 'detached' })
    status = 'completed'
    await page.getByText(kind === 'delete' ? '删除已完成' : '已清空 3 条文件记录', { exact: true }).waitFor()
    if (kind === 'delete') await row('排队分享A').waitFor({ state: 'detached' })
    else await row('排队分享A').getByText('文件 0 · 媒体 0', { exact: true }).waitFor()
    assert.equal(submits, 3)
    assert.equal(await row('无关分享B').getByRole('button', { name: label, exact: true }).isEnabled(), true)
    assert.deepEqual(errors, [])
    console.log(`PASS ${kind}: 确认取消、提交失败恢复、排队接受、无关分享独立、刷新恢复、取消不删除、执行不可取消、完成后刷新（API mocked）`)
  } catch (error) { console.error(error); console.error(await browser.contexts()[0]?.pages()[0]?.locator('body').innerText()); throw error } finally { await browser.close() }
}
