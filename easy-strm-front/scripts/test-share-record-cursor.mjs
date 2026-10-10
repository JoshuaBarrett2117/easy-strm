import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import test from 'node:test'

const source = (await readFile(new URL('../src/utils/api/media.js', import.meta.url), 'utf8'))
  .replace(/^import .*\n/, '')
  .replaceAll('export const ', 'const ')
const load = api => new Function('api', `${source}\nreturn getUnsyncedShareRecordIDs`)(api)

test('actual API cursor advances by record ID without page or total', async () => {
  const requests = []
  const pages = [[{ id: 99 }, { id: 23 }], [{ id: 7 }], []]
  const controller = new AbortController()
  const ids = await load({ get: async (url, options) => {
    assert.equal(url, '/media/share-records')
    assert.equal(options.signal, controller.signal)
    requests.push(options.params)
    return { data: { data: { data: pages.shift(), total: 0 } } }
  } })(controller.signal)
  assert.deepEqual(ids, [99, 23, 7])
  assert.deepEqual(requests.map(request => request.after_id), [0, 23, 7])
  assert.ok(requests.every(request => request.cursor && request.only_unsynced && !('page' in request)))
})

test('cancel and nonadvancing cursor stop rather than spin', async () => {
  const controller = new AbortController()
  controller.abort()
  await assert.rejects(load({ get: () => assert.fail('request after cancel') })(controller.signal), /取消/)
  await assert.rejects(load({ get: async () => ({ data: { data: { data: [{ id: 7 }] } } }) })(), /未推进/)
})
