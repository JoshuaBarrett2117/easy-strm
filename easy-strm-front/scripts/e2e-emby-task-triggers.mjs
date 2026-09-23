// 更新日期：2026-09-23；维护者：Codex。使用接口夹具，不操作真实 Emby。
import assert from 'node:assert/strict'
import { createServer } from 'vite'
import { chromium } from 'playwright'

const entry = `
    import { createApp, h } from 'vue'
    import { NMessageProvider } from 'naive-ui'
    import Tasks from '/src/components/EmbyScheduledTasks.vue'
    import '/src/style.css'
    createApp({render: () => h(NMessageProvider, null, {default: () => h(Tasks, {serverId: 1, active: true})})}).mount('#app')
  `
const server = await createServer({ server: { port: 0, open: false }, plugins: [{
  name: 'trigger-test-harness',
  resolveId(id) { if (id === '/__trigger_entry.js') return id },
  load(id) { if (id === '/__trigger_entry.js') return entry },
  configureServer(vite) {
    vite.middlewares.use('/__trigger_test', (_req, res) => {
      res.setHeader('Content-Type', 'text/html')
      res.end('<html><body><div id="app"></div><script type="module" src="/__trigger_entry.js"></script></body></html>')
    })
  }
}] })
let browser
try {
  await server.listen()
  browser = await chromium.launch({ headless: true })
  const page = await browser.newPage({ viewport: { width: 390, height: 844 } })
  const errors = []
  page.on('console', msg => { if (msg.type() === 'error') console.error(msg.text()) })
  page.on('pageerror', err => { errors.push(err.message); console.error(err.message) })
  let triggers = [{ Type: 'DailyTrigger', TimeOfDayTicks: 108000000000, MaxRuntimeTicks: 600000000 }]
  const writes = []
  await page.route('**/api/**', route => {
    if (!new URL(route.request().url()).pathname.startsWith('/api/')) return route.continue()
    if (route.request().method() === 'PUT') {
      triggers = route.request().postDataJSON().triggers
      writes.push(triggers)
      return route.fulfill({ json: { data: { message: '触发规则已保存' } } })
    }
    return route.fulfill({ json: { data: { data: [{ Id: 'task1', Name: '测试任务', State: 'Idle', Triggers: triggers }], total: 1 } } })
  })
  await page.goto(`http://localhost:${server.httpServer.address().port}/__trigger_test`)
  await page.waitForFunction(() => document.querySelector('#app')?.children.length > 0, null, { timeout: 10000 })
  await page.getByRole('button', { name: '编辑触发规则' }).click()
  const time = page.locator('.n-time-picker input')
  await time.fill('06:00:00')
  await time.press('Enter')
  await page.getByRole('button', { name: '保存规则', exact: true }).click()
  await page.getByText('触发规则：每天 06:00', { exact: true }).waitFor()
  assert.equal(writes[0][0].TimeOfDayTicks, 216000000000)
  assert.equal(writes[0][0].MaxRuntimeTicks, 600000000)
  await page.getByRole('button', { name: '编辑触发规则' }).click()
  await page.getByRole('button', { name: '删除规则', exact: true }).click()
  await page.getByText('保存后此任务将不再自动触发。').waitFor()
  await page.getByRole('button', { name: '保存规则', exact: true }).click()
  await page.getByText('触发规则：未配置自动触发', { exact: true }).waitFor()
  assert.deepEqual(writes[1], [])
  await page.getByRole('button', { name: '编辑触发规则' }).click()
  await page.getByRole('button', { name: '添加规则', exact: true }).click()
  await page.getByRole('button', { name: '取消', exact: true }).click()
  assert.equal(writes.length, 2)
  assert.deepEqual(errors, [])
  console.log('PASS: 修改时间、保留附加字段、删除全部规则、取消不保存及窄屏操作')
} finally {
  await browser?.close()
  await server.close()
}
