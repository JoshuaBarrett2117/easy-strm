import assert from 'node:assert/strict'
import { createServer } from 'vite'
import { chromium } from 'playwright'

// 使用真实组件和模拟接口验证查看行为，不读取或修改用户的真实凭据。
const entry = `
import { createApp, h, reactive } from 'vue'
import SecretConfigInput from '/src/components/common/SecretConfigInput.vue'
const state = reactive({ value: '', secretKey: 'tmdb_api_key', hasSaved: true, serverId: 0, resetKey: 0 })
window.secretTest = state
createApp({ render: () => h(SecretConfigInput, { ...state, placeholder: '已配置，留空保持不变', 'onUpdate:value': value => state.value = value }) }).mount('#app')
`
const server = await createServer({
  server: { host: '127.0.0.1', port: 0, open: false },
  plugins: [{
    name: 'config-secret-test',
    resolveId(id) { if (id === '/__secret-test.js') return '\0secret-test' },
    load(id) { if (id === '\0secret-test') return entry },
    configureServer(server) {
      server.middlewares.use('/__secret-test.html', async (req, res) => {
        res.setHeader('Content-Type', 'text/html')
        res.end(await server.transformIndexHtml('/__secret-test.html', '<div id="app" style="max-width:600px;margin:40px"></div><script type="module" src="/__secret-test.js"></script>'))
      })
    }
  }]
})
let browser
try {
  await server.listen()
  browser = await chromium.launch({ headless: true })
  const page = await browser.newPage()
  const errors = []
  page.on('pageerror', error => errors.push(error.message))
  let requests = 0
  let fail = false
  let pending
  let delay = false
  await page.route('**/api/**/secrets/**', async route => {
    requests++
    if (delay) await new Promise(resolve => { pending = resolve })
    await route.fulfill({ status: fail ? 500 : 200, contentType: 'application/json', body: JSON.stringify(fail ? { error: '模拟读取失败' } : { data: { value: `saved-${new URL(route.request().url()).pathname.split('/').at(-1)}` } }) })
  })
  await page.goto(`${server.resolvedUrls.local[0]}__secret-test.html`)
  const input = page.locator('input')
  const show = () => page.getByRole('button', { name: '查看明文', exact: true }).click()
  const hide = () => page.getByRole('button', { name: '隐藏明文', exact: true }).click()
  const waitFor = expected => page.waitForFunction(expected => document.querySelector('input')?.value === expected, expected)
  await input.waitFor()
  assert.equal(requests, 0)
  assert.equal(await input.getAttribute('type'), 'password')
  for (const key of ['ai_recognition_api_key', 'tmdb_api_key', 'global_api_key', 'telegram_bot_token', 'wecom_secret', 'wecom_callback_token', 'wecom_encoding_aes_key', 'emby_server_api_key', 'emby_cover_ai_api_key']) {
    await page.evaluate(key => Object.assign(window.secretTest, { secretKey: key, serverId: key === 'emby_server_api_key' ? 7 : 0 }), key)
    await show()
    await waitFor(`saved-${key}`)
    assert.equal(await page.evaluate(() => window.secretTest.value), '')
    await hide()
    await waitFor('')
    assert.equal(await input.getAttribute('type'), 'password')
  }
  const beforeDraft = requests
  await input.fill('unsaved-edit')
  await show()
  assert.equal(await input.inputValue(), 'unsaved-edit')
  assert.equal(requests, beforeDraft)
  await hide()
  assert.equal(await input.inputValue(), 'unsaved-edit')
  await input.fill('')
  fail = true
  await show()
  await page.getByText('模拟读取失败', { exact: true }).waitFor()
  assert.equal(await input.inputValue(), '')
  assert.equal(await input.getAttribute('type'), 'password')
  fail = false
  await show()
  await waitFor('saved-emby_cover_ai_api_key')
  await page.evaluate(() => window.secretTest.resetKey++)
  await waitFor('')
  delay = true
  await show()
  while (!pending) await new Promise(resolve => setTimeout(resolve, 10))
  await page.evaluate(() => { window.secretTest.serverId = 99; window.secretTest.resetKey++ })
  pending()
  await page.waitForResponse(response => response.url().includes('/secrets/'))
  await page.evaluate(() => new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve))))
  assert.equal(await input.inputValue(), '')
  assert.equal(await input.getAttribute('type'), 'password')
  assert.deepEqual(errors, [])
  console.log('PASS: nine secret fields, hide/reveal, unchanged form, unsaved edits, retry, reset, stale response')
} finally {
  await browser?.close()
  await server.close()
}
