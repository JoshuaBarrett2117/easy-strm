import assert from 'node:assert/strict'
import test from 'node:test'
import { isShareStrmCronTask, shareStrmTaskSummary } from '../src/utils/ui/share-strm-task-summary.js'

test('two cron handlers show Chinese requested and effective modes', () => {
  for (const mode of ['incremental', 'reconciliation']) {
    const task = { task_id: 'cron_12_1', task_type: 'strm_generate', metadata: { share_export: true, cron_handler: mode === 'incremental' ? 'share_strm_incremental_export' : 'share_strm_full_reconciliation', requested_mode: mode, effective_mode: mode, processed_works: 0, affected_sources: 0 } }
    assert.equal(isShareStrmCronTask(task), true)
    const summary = shareStrmTaskSummary(task)
    assert.equal(summary[0].label, '请求模式')
    assert.equal(summary[1].label, '实际模式')
    assert.equal(summary[0].value, mode === 'incremental' ? '增量导出' : '全量对账')
    assert.ok(summary.some(item => item.label === '处理作品' && item.value === 0))
  }
})

test('fallback and resumed building are not presented as plain incremental', () => {
  const summary = shareStrmTaskSummary({ task_id: 'cron_fallback', metadata: { share_export: true, requested_mode: 'incremental', effective_mode: 'reconciliation', fallback_reason: '历史输出凭证不可信', recovery: '继续未完成作品' } })
  assert.equal(summary[1].value, '全量对账')
  assert.ok(summary.some(item => item.label === '升级原因' && item.value === '历史输出凭证不可信'))
  assert.ok(summary.some(item => item.label === '恢复说明' && item.value === '继续未完成作品'))
})

test('mutex skipped shows reason and blocking task, not successful export counters', () => {
  const summary = shareStrmTaskSummary({ task_id: 'cron_skipped', metadata: { share_export: true, requested_mode: 'reconciliation', effective_mode: 'skipped', outcome: 'skipped', skip_reason: '互斥跳过', blocked_by_task_id: 'cron_active' } })
  assert.equal(summary[1].value, '本次跳过')
  assert.ok(summary.some(item => item.label === '跳过原因' && item.value === '互斥跳过'))
  assert.ok(summary.some(item => item.label === '占用任务' && item.value === 'cron_active'))
  assert.equal(summary.some(item => item.label === '已生成 STRM'), false)
})

test('old share exports retain summary, unrelated cron does not gain one', () => {
  assert.deepEqual(shareStrmTaskSummary({ task_id: 'share_strm_old', processed_files: 3, total_files: 4, metadata: { exported_files: 2 } }), [{ label: '已处理记录', value: '3 / 4' }, { label: '已生成 STRM', value: 2 }])
  assert.equal(shareStrmTaskSummary({ task_id: 'cron_cleanup', task_type: 'cleanup' }), null)
  assert.equal(isShareStrmCronTask({ task_id: 'share_strm_old' }), false)
})
