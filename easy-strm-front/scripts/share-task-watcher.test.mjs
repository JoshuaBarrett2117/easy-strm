import test from 'node:test'
import assert from 'node:assert/strict'
import { createShareTaskWatcher } from '../src/utils/ui/shareTaskWatcher.js'

test('同步和识别独立返回，重复订阅不创建第二条轮询', async () => {
  const pending = new Map(), states = [], completed = []
  const watcher = createShareTaskWatcher(id => new Promise(resolve => pending.set(id, resolve)), task => states.push(task), task => completed.push(task))
  const sync = watcher.watch('sync'), identify = watcher.watch('identify')
  assert.equal(watcher.watch('sync'), sync)
  pending.get('sync')({ task_id: 'sync', status: 'completed', metadata: { synced_share_ids: [1] } })
  assert.deepEqual((await sync).metadata.synced_share_ids, [1])
  pending.get('identify')({ task_id: 'identify', status: 'failed' })
  assert.equal((await identify).status, 'failed')
  assert.equal(states.length, 2)
  assert.equal(completed.length, 2)
  watcher.dispose()
})

test('卸载时结束等待并忽略在途响应', async () => {
  let respond
  const watcher = createShareTaskWatcher(() => new Promise(resolve => { respond = resolve }), () => assert.fail('卸载后更新'), () => assert.fail('卸载后刷新'))
  const result = watcher.watch('sync')
  watcher.dispose()
  respond({ task_id: 'sync', status: 'completed' })
  assert.equal(await result, null)
  assert.equal(await watcher.watch('new'), null)
})
