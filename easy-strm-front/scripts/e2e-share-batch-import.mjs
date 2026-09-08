// 更新日期：2026-09-08；执行者：Codex。模拟API验证预览、修改、冲突确认及部分失败恢复。
import assert from 'node:assert/strict'
import { chromium } from 'playwright'

const browser = await chromium.launch({ headless: true })
const requests = []
let failURL = ''
let parseCount = 0
let reads = 0
const row = (code, name, warnings = []) => ({
  share_code: code, name, url: `https://115.com/s/${code}`, password: '0000',
  names: [name], passwords: ['0000'], lines: [1], warnings
})
const data = {
  records: [row('one', '电影一'), {
    ...row('two', '电影二', ['同一分享有多个名称，请选择或修改', '访问码冲突，请核对']),
    names: ['电影二', '电影别名'], passwords: ['0000', 'abcd'], lines: [3, 8]
  }, row('three', '', ['未确定名称，请补充'])],
  duplicates: 1, ignored: ['第9行：推广链接']
}
try {
  const page = await browser.newPage({ viewport: { width: 1280, height: 900 } })
  page.setDefaultTimeout(5000)
  const errors = []
  page.on('pageerror', error => errors.push(error.message))
  await page.addInitScript(() => localStorage.setItem('token', 'e2e-token'))
  await page.route('**/api/**', async route => {
    const request = route.request()
    const path = new URL(request.url()).pathname
    if (!path.startsWith('/api/')) return route.continue()
    if (path === '/api/media/share-records/parse') {
      parseCount++
      if (request.postDataJSON().text === '无链接') return route.fulfill({ status: 400, json: { error: '未识别到115分享链接' } })
      return route.fulfill({ json: { code: 0, data } })
    }
    if (path === '/api/media/share-records') {
      if (request.method() === 'POST') {
        const body = request.postDataJSON()
        requests.push(body)
        if (body.url === failURL) return route.fulfill({ status: 500, json: { error: '模拟导入失败' } })
      } else reads++
    }
    await route.fulfill({ json: { code: 0, state: true, data: { data: [], total: 0 } } })
  })
  await page.goto(`${process.env.E2E_FRONTEND_URL || 'http://127.0.0.1:3001'}/dashboard/share-records`)
  const dialog = page.locator('.n-modal').filter({ hasText: '批量导入分享' })
  const open = async () => {
    await page.getByRole('button', { name: '批量导入分享', exact: true }).click()
    await dialog.waitFor()
  }
  await open()
  await dialog.getByRole('button', { name: '取消', exact: true }).click()
  await dialog.waitFor({ state: 'hidden' })
  await open()
  await dialog.locator('textarea').fill('无链接')
  await dialog.getByRole('button', { name: '解析预览', exact: true }).click()
  await page.getByText('未识别到115分享链接', { exact: true }).waitFor()
  await dialog.locator('textarea').fill('电影 https://115.com/s/one')
  await dialog.getByRole('button', { name: '解析预览', exact: true }).click()
  const cards = dialog.getByTestId('import-entry')
  await cards.nth(2).waitFor()
  assert.equal(requests.length, 0)
  assert.match(await dialog.innerText(), /当前选中 1 条/)
  // 修改源文本立即清除旧预览，避免提交过期结果。
  await dialog.locator('textarea').fill('修改后的文案 https://115.com/s/one')
  assert.equal(await cards.count(), 0)
  await dialog.getByRole('button', { name: '解析预览', exact: true }).click()
  await cards.nth(2).waitFor()
  assert.equal(parseCount, 3)
  await cards.nth(1).getByRole('checkbox').click()
  // 名称、链接、访问码为三个文本输入，链接只读，访问码修改通过独立字段提交。
  await cards.nth(1).getByRole('textbox').nth(0).fill('确认后的名称')
  await cards.nth(1).getByRole('textbox').nth(2).fill('abcd')
  await cards.nth(2).getByRole('checkbox').click()
  await dialog.getByRole('button', { name: '导入选中（3）', exact: true }).click()
  await page.getByText('请为选中的分享填写名称', { exact: true }).waitFor()
  assert.equal(requests.length, 0)
  await cards.nth(2).getByRole('textbox').nth(0).fill('补充的名称')
  failURL = 'https://115.com/s/two'
  const oldReads = reads
  await dialog.getByRole('button', { name: '导入选中（3）', exact: true }).click()
  await cards.nth(1).getByText('模拟导入失败', { exact: true }).waitFor()
  assert.equal(requests.length, 2)
  assert.match(await cards.nth(0).getByRole('checkbox').getAttribute('class'), /n-checkbox--disabled/)
  failURL = ''
  await dialog.getByRole('button', { name: '导入选中（2）', exact: true }).click()
  await dialog.waitFor({ state: 'hidden' })
  assert.equal(requests.filter(r => r.url.endsWith('/one')).length, 1)
  assert.deepEqual(requests.at(-2), { name: '确认后的名称', url: 'https://115.com/s/two', password: 'abcd' })
  assert.equal(requests.at(-1).name, '补充的名称')
  assert.ok(reads > oldReads)
  await page.setViewportSize({ width: 390, height: 844 })
  await open()
  assert.equal(await dialog.locator('textarea').inputValue(), '')
  await dialog.locator('textarea').fill('https://115.com/s/one')
  await dialog.getByRole('button', { name: '解析预览', exact: true }).click()
  await cards.nth(2).waitFor()
  const bounds = await dialog.boundingBox()
  assert.ok(bounds.x >= 0 && bounds.x + bounds.width <= 390)
  const footerBounds = await dialog.getByRole('button', { name: '取消', exact: true }).boundingBox()
  assert.ok(footerBounds.y >= 0 && footerBounds.y + footerBounds.height <= 844)
  assert.deepEqual(errors, [])
  console.log('PASS: 解析不写入、无效输入、旧预览失效、冲突默认排除、名称密码修改、空名称校验、失败重试、列表刷新、窄屏；API mocked')
} finally {
  await browser.close()
}
