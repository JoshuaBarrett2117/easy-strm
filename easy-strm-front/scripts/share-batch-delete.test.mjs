import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { runInNewContext } from 'node:vm'
import { parse } from '@vue/compiler-sfc'

const source = readFileSync(new URL('../src/views/ShareRecords.vue', import.meta.url), 'utf8')
const script = parse(source).descriptor.scriptSetup.content.replace(/^import .*$/gm, '')
const plain = value => JSON.parse(JSON.stringify(value))

function fixture(submit = async () => ({ data: { data: { task_id: 'batch-delete' } } })) {
  const calls = [], dialogs = [], messages = [], watches = []
  let complete, refreshes = 0
  const sandbox = {
    AbortController, Set,
    ref: value => ({ value }),
    computed: read => ({ get value() { return read() } }),
    h: () => null, onMounted: () => {}, onBeforeUnmount: () => {}, watch: () => {},
    useRoute: () => ({ query: {} }), useRouter: () => ({ push() {} }),
    useMessage: () => Object.fromEntries(['info', 'success', 'warning', 'error'].map(kind => [kind, text => messages.push([kind, text])])),
    dialog: { warning: options => dialogs.push(options) },
    batchDeleteShareRecords: async ids => { calls.push(plain(ids)); return submit(ids) },
    getShareRecords: async () => { refreshes++; return { data: { data: { data: [], total: 0 } } } },
    createShareTaskWatcher: (_fetch, _state, onComplete) => {
      complete = onComplete
      return { watch: async id => { watches.push(id); return null }, dispose() {} }
    },
    NButton: {}, NSpace: {}, NTag: {}, ShareMediaGallery: {}
  }
  runInNewContext(`${script}\nglobalThis.subject = { selectedShareIDs, taskStates, deletingSelected, deleteSelectedShares, shareBusy };`, sandbox)
  return { ...sandbox.subject, calls, dialogs, messages, watches, complete: task => complete(task), get refreshes() { return refreshes } }
}

test('API使用固定路由和share_ids请求体，无生产请求', async () => {
  const media = readFileSync(new URL('../src/utils/api/media.js', import.meta.url), 'utf8')
  const declaration = media.match(/^export const batchDeleteShareRecords = (.+)$/m)
  assert.ok(declaration)
  let request
  const submit = runInNewContext(declaration[1], { api: { post: (path, body) => { request = { path, body } } } })
  await submit([2, 1])
  assert.deepEqual(plain(request), { path: '/media/share-records/batch-delete', body: { share_ids: [2, 1] } })
})

test('按钮位于批量清空后，禁用条件包含提交态、空选择和目标忙碌', () => {
  assert.match(source, /批量清空识别内容<\/n-button>\s*<n-button type="error" :loading="deletingSelected" :disabled="deletingSelected \|\| !selectedShareIDs.length \|\| selectedShareIDs.some\(shareBusy\)" @click="deleteSelectedShares">批量删除分享/)
  const page = fixture()
  page.deleteSelectedShares()
  assert.equal(page.dialogs.length, 0)
  page.selectedShareIDs.value = [1]
  page.deletingSelected.value = true
  page.deleteSelectedShares()
  assert.equal(page.dialogs.length, 0)
})

test('确认内容完整保留单条风险，快照目标且防止重复提交', async () => {
  let resolve
  const page = fixture(() => new Promise(done => { resolve = done }))
  page.selectedShareIDs.value = [1, 2]
  page.deleteSelectedShares()
  const confirmation = page.dialogs[0]
  const singleRisk = source.match(/确定删除“\$\{row.name\}”吗？([^`]+)`/)[1]
  assert.equal(confirmation.content, `确定删除选中的 2 个分享吗？${singleRisk}`)
  page.selectedShareIDs.value = [9]
  const pending = confirmation.onPositiveClick()
  assert.equal(page.deletingSelected.value, true)
  assert.equal(await confirmation.onPositiveClick(), false)
  page.deleteSelectedShares()
  assert.equal(page.dialogs.length, 1)
  assert.deepEqual(page.calls, [[1, 2]])
  resolve({ data: { data: { task_id: 'batch-delete' } } })
  await pending
  assert.equal(page.deletingSelected.value, false)
  assert.deepEqual(page.watches, ['batch-delete'])
  assert.deepEqual(plain(page.taskStates.value['batch-delete'].metadata.record_ids), [1, 2])
})

test('活跃同步和识别仅阻塞目标分享，确认回调重新检查忙碌', async () => {
  const page = fixture()
  page.selectedShareIDs.value = [1]
  page.taskStates.value = {
    sync: { task_type: 'share_sync', status: 'running', metadata: { record_ids: [2, 3] } },
    identify: { task_type: 'share_identify', status: 'pending', metadata: { record_ids: [4] } },
    old: { task_type: 'share_delete', status: 'completed', metadata: { record_ids: [1] } },
    file: { task_type: 'share_media_delete', status: 'running', metadata: { record_ids: [1], file_id: 7 } }
  }
  assert.equal(page.shareBusy(1), false)
  for (const id of [2, 3, 4]) assert.equal(page.shareBusy(id), true)
  page.deleteSelectedShares()
  assert.equal(page.dialogs.length, 1)
  page.taskStates.value.identify = { task_type: 'share_identify', status: 'running', metadata: { current_share_id: 1 } }
  assert.equal(await page.dialogs[0].onPositiveClick(), false)
  assert.deepEqual(page.calls, [])
  page.deleteSelectedShares()
  assert.equal(page.dialogs.length, 1)
})

test('部分失败警告计数，移除成功和跳过项但保留失败及其他选择并刷新', async () => {
  const page = fixture()
  page.selectedShareIDs.value = [1, 2, 3, 9]
  page.complete({ task_type: 'share_delete', status: 'failed', error_message: '2: 磁盘清理失败', metadata: { record_ids: [1, 2, 3], result: { deleted: 1, deleted_ids: [1], failed_ids: [2], skipped_ids: [3] } } })
  await Promise.resolve()
  assert.deepEqual(plain(page.selectedShareIDs.value), [2, 9])
  assert.equal(page.messages[0][0], 'warning')
  assert.match(page.messages[0][1], /已删除 1 个分享，失败 1 个分享.*2: 磁盘清理失败/)
  assert.equal(page.refreshes, 1)
})

test('成功终态报告实际删除数并兼容旧版单条结果', () => {
  const page = fixture()
  page.selectedShareIDs.value = [1, 2, 3]
  page.complete({ task_type: 'share_delete', status: 'completed', metadata: { record_ids: [1, 2], result: { deleted: 1, deleted_ids: [1], failed_ids: [], skipped_ids: [2] } } })
  assert.deepEqual(plain(page.selectedShareIDs.value), [3])
  assert.deepEqual(page.messages[0], ['success', '已删除 1 个分享'])
  page.complete({ task_type: 'share_delete', status: 'completed', metadata: { record_ids: [3], result: { deleted: 1 } } })
  assert.deepEqual(plain(page.selectedShareIDs.value), [])
  assert.equal(page.refreshes, 2)
})

test('取消和全部失败保留选择，提交失败释放提交态', async () => {
  const page = fixture(async () => { throw new Error('模拟提交失败') })
  page.selectedShareIDs.value = [1, 2]
  page.deleteSelectedShares()
  assert.equal(await page.dialogs[0].onPositiveClick(), false)
  assert.equal(page.deletingSelected.value, false)
  page.complete({ task_type: 'share_delete', status: 'cancelled', metadata: { record_ids: [1, 2] } })
  page.complete({ task_type: 'share_delete', status: 'failed', metadata: { record_ids: [1, 2], result: { deleted: 0, deleted_ids: [], failed_ids: [1, 2], skipped_ids: [] } } })
  assert.deepEqual(plain(page.selectedShareIDs.value), [1, 2])
  assert.match(page.messages.at(-1)[1], /已删除 0 个分享，失败 2 个分享/)
})