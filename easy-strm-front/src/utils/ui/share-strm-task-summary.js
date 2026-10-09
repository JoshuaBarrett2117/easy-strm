const shareCronHandlers = ['share_strm_incremental_export', 'share_strm_full_reconciliation']
const modeNames = { incremental: '增量导出', reconciliation: '全量对账', skipped: '本次跳过' }

export const isShareStrmCronTask = task => shareCronHandlers.includes(task?.metadata?.cron_handler)

export const shareStrmTaskSummary = task => {
  const metadata = task?.metadata || {}
  if (!String(task?.task_id || '').startsWith('share_strm_') && !metadata.share_export && !isShareStrmCronTask(task)) return null
  if (!metadata.requested_mode) {
    return [
      { label: '已处理记录', value: `${task.processed_files || 0} / ${task.total_files || 0}` },
      { label: '已生成 STRM', value: metadata.exported_files || 0 }
    ]
  }
  const items = [
    { label: '请求模式', value: modeNames[metadata.requested_mode] || metadata.requested_mode },
    { label: '实际模式', value: modeNames[metadata.effective_mode] || '等待检查点校验' }
  ]
  if (metadata.outcome === 'skipped') {
    items.push({ label: '跳过原因', value: metadata.skip_reason || metadata.message || '其他分享导出正在运行' })
    if (metadata.blocked_by_task_id) items.push({ label: '占用任务', value: metadata.blocked_by_task_id })
    return items
  }
  if (metadata.fallback_reason) items.push({ label: '升级原因', value: metadata.fallback_reason })
  if (metadata.recovery) items.push({ label: '恢复说明', value: metadata.recovery })
  if (metadata.phase) items.push({ label: '当前阶段', value: ({ preparing: '全量准备', consuming: '作品待办消费' })[metadata.phase] || metadata.phase })
  if (metadata.processed_works !== undefined) items.push({ label: '处理作品', value: metadata.processed_works })
  if (metadata.affected_sources !== undefined) items.push({ label: '受影响来源', value: metadata.affected_sources })
  items.push({ label: '新增 / 更新 / 跳过', value: `${metadata.added || 0} / ${metadata.updated || 0} / ${metadata.skipped || 0}` })
  items.push({ label: '归属冲突', value: metadata.conflicts || 0 })
  if (metadata.stale_marked !== undefined) items.push({ label: '标记失效（不删除）', value: metadata.stale_marked })
  if (metadata.reconciliation_unseen_outputs !== undefined) items.push({ label: '全量未见旧键', value: metadata.reconciliation_unseen_outputs })
  if (metadata.pending_works !== undefined) items.push({ label: '本轮失败待办', value: metadata.pending_works })
  if (metadata.output_path) items.push({ label: '输出目录', value: metadata.output_path })
  return items
}
