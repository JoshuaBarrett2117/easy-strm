<template>
  <el-card v-if="task" class="task-card" shadow="hover" :class="['task-type-' + (task.task_type || 'unknown')]">
    <template #header>
      <div class="task-header">
        <div class="task-title">
          <el-icon class="task-icon" :class="{ spin: task.status === 'running' }">
            <component :is="taskStatusIcon" />
          </el-icon>
          <span class="task-name">{{ taskDisplayName }}</span>
          <el-tag :type="taskStatusType" size="small" class="status-tag">
            {{ taskStatusText }}
          </el-tag>
          <el-tag v-if="task.priority && task.priority < 5" type="danger" size="small" effect="dark" class="priority-tag">
            P{{ task.priority }}
          </el-tag>
        </div>
        <div class="task-actions">
          <el-button type="primary" size="small" plain @click="handleDetail">详情</el-button>
          <el-button v-if="canCancel" type="danger" size="small" plain @click="handleCancel">取消任务</el-button>
          <el-button v-if="canResume" type="warning" size="small" plain @click="handleResume">恢复任务</el-button>
          <span class="task-time">{{ task.create_time || '' }}</span>
        </div>
      </div>
    </template>

    <div class="task-body">
      <div class="task-type-info">
        <el-tag :type="taskTypeTagType" size="default" effect="plain">
          {{ taskTypeName }}
        </el-tag>
        <span v-if="task.config_name" class="config-name">{{ task.config_name }}</span>
        <el-tag
          v-if="watchFailureCategoryTag"
          :type="watchFailureCategoryTag.type"
          size="small"
          effect="light"
          class="failure-tag"
        >
          {{ watchFailureCategoryTag.label }}
        </el-tag>
      </div>

      <div v-if="taskSummaryItems.length > 0" class="task-summary">
        <div v-for="item in taskSummaryItems" :key="item.label" class="task-summary-item">
          <span class="task-summary-label">{{ item.label }}</span>
          <span class="task-summary-value">{{ item.value }}</span>
        </div>
      </div>

      <div v-if="task.task_type === 'watch_auto_organize'" class="watch-result-strip">
        <div class="watch-result-stat">
          <span class="watch-result-stat-label">检测</span>
          <span class="watch-result-stat-value">{{ watchDetectedCount }}</span>
        </div>
        <div class="watch-result-stat">
          <span class="watch-result-stat-label">成功</span>
          <span class="watch-result-stat-value success">{{ watchSuccessCount }}</span>
        </div>
        <div class="watch-result-stat">
          <span class="watch-result-stat-label">失败</span>
          <span class="watch-result-stat-value failed">{{ watchFailedCount }}</span>
        </div>
        <div class="watch-result-text">
          <span v-if="watchResultSummaryText">{{ watchResultSummaryText }}</span>
          <span v-if="watchFailureReasonText">{{ watchFailureReasonText }}</span>
        </div>
      </div>

      <div v-if="watchFailedItems.length > 0" class="watch-failed-panel">
        <div class="watch-failed-header">
          <div class="watch-failed-title">
            <span>失败文件</span>
            <el-tag size="small" type="danger">{{ watchFailedItemCount }}</el-tag>
          </div>
          <el-button type="primary" link size="small" @click="toggleWatchFailedItems">
            {{ watchFailedItemsExpanded ? '收起' : '展开全部' }}
          </el-button>
        </div>
        <div class="watch-failed-list">
          <div
            v-for="item in watchFailedItemsVisible"
            :key="`${item.file_id || item.file_name}-${item.reason}`"
            class="watch-failed-item"
          >
            <div class="watch-failed-item-main">
              <span class="watch-failed-item-name">{{ item.file_name }}</span>
              <span v-if="item.file_id" class="watch-failed-item-id">{{ item.file_id }}</span>
              <el-tag v-if="item.category" :type="watchFailureTagType(item.category)" size="small">
                {{ watchFailureTagLabel(item.category) }}
              </el-tag>
            </div>
            <div class="watch-failed-item-reason">
              {{ item.reason || '未提供失败原因' }}
            </div>
          </div>
        </div>
        <div v-if="watchFailedItemsHiddenCount > 0" class="watch-failed-more">
          还有 {{ watchFailedItemsHiddenCount }} 项失败文件未展开
        </div>
      </div>

      <el-progress
        v-if="showProgress"
        :percentage="task.progress || 0"
        :status="progressStatus"
        :stroke-width="14"
        class="task-progress"
        :format="progressFormat"
      />

      <div v-if="showFileStats" class="task-stats">
        <div class="stat-item">
          <div class="stat-value total">{{ task.total_files || 0 }}</div>
          <div class="stat-label">总文件</div>
        </div>
        <div class="stat-item">
          <div class="stat-value success">{{ task.success_files || 0 }}</div>
          <div class="stat-label">成功</div>
        </div>
        <div class="stat-item">
          <div class="stat-value failed">{{ task.failed_files || 0 }}</div>
          <div class="stat-label">失败</div>
        </div>
        <div class="stat-item">
          <div class="stat-value pending">{{ pendingCount }}</div>
          <div class="stat-label">处理中</div>
        </div>
      </div>

      <div v-if="task.scheduled_time" class="scheduled-info">
        <el-icon><Clock /></el-icon>
        <span>计划执行时间: {{ task.scheduled_time }}</span>
      </div>

      <el-alert
        v-if="task.error_message"
        type="error"
        :title="task.error_message"
        show-icon
        :closable="false"
        class="task-error"
      />
    </div>
  </el-card>
</template>

<script setup>
import { computed, ref, toRef } from 'vue'
import {
  Clock,
  Loading,
  CircleCheck,
  CircleClose,
  Timer,
  Document,
  Refresh,
  VideoCamera,
  Film,
  Search,
  Connection
} from '@element-plus/icons-vue'

const props = defineProps({
  task: {
    type: Object,
    default: () => ({})
  }
})

const emit = defineEmits(['cancel', 'resume', 'detail'])

const task = toRef(props, 'task')

const taskTypeNames = {
  strm_generate: 'STRM 文件生成',
  incremental_sync: '增量同步',
  sync_full: '全量同步',
  sync_transfer: '秒传同步',
  cleanup: '空间清理',
  proxy_refresh: '直链刷新',
  organize: '媒体整理',
  watch_auto_organize: '115 自动整理',
  scrape: 'NFO 刮削',
  emby_refresh: 'Emby 库刷新',
  log_clean: '日志清理',
  sync_files: '文件同步'
}

const taskTypeIcons = {
  strm_generate: Document,
  incremental_sync: Timer,
  sync_full: Timer,
  sync_transfer: Timer,
  cleanup: Refresh,
  proxy_refresh: Refresh,
  organize: VideoCamera,
  watch_auto_organize: Film,
  scrape: Search,
  emby_refresh: Connection,
  log_clean: Refresh,
  sync_files: Timer
}

const taskTypeTagTypes = {
  strm_generate: 'primary',
  incremental_sync: 'success',
  sync_full: 'success',
  sync_transfer: '',
  cleanup: 'warning',
  proxy_refresh: 'info',
  organize: 'primary',
  watch_auto_organize: 'success',
  scrape: 'warning',
  emby_refresh: 'danger',
  log_clean: 'warning',
  sync_files: 'info'
}

const taskTypeName = computed(() => {
  const t = task.value
  if (!t) return '未知任务'
  if (t.task_type === 'watch_auto_organize') {
    const sourceType = t.metadata?.source_type
    if (sourceType === 'local') return '本地自动整理'
    if (sourceType === 'cloud115') return '115 自动整理'
    return '自动整理'
  }
  return taskTypeNames[t.task_type] || '未知任务'
})
const taskDisplayName = computed(() => task.value ? (task.value.task_name || taskTypeName.value) : '任务')
const taskTypeTagType = computed(() => task.value ? (taskTypeTagTypes[task.value.task_type] || 'info') : 'info')

const taskStatusType = computed(() => {
  const statusMap = {
    pending: 'info',
    running: 'warning',
    completed: 'success',
    failed: 'danger',
    cancelled: 'info',
    scheduled: ''
  }
  return task.value ? (statusMap[task.value.status] || 'info') : 'info'
})

const taskStatusText = computed(() => {
  const textMap = {
    pending: '待执行',
    running: '执行中',
    completed: '已完成',
    failed: '失败',
    cancelled: '已取消',
    scheduled: '已调度'
  }
  return task.value ? (textMap[task.value.status] || '未知') : '未知'
})

const taskStatusIcon = computed(() => {
  const t = task.value
  if (!t) return Timer
  if (t.status === 'running') return Loading
  if (t.status === 'completed') return CircleCheck
  if (t.status === 'failed' || t.status === 'cancelled') return CircleClose
  if (t.status === 'scheduled') return Clock
  return taskTypeIcons[t.task_type] || Timer
})

const canCancel = computed(() => {
  const t = task.value
  return !!t && (t.status === 'pending' || t.status === 'running')
})

const canResume = computed(() => {
  const t = task.value
  return !!t && t.task_type === 'watch_auto_organize' && (t.status === 'cancelled' || t.status === 'failed')
})

const showProgress = computed(() => {
  const t = task.value
  return !!t && ['pending', 'running', 'completed', 'failed', 'cancelled'].includes(t.status)
})

const showFileStats = computed(() => {
  const t = task.value
  return !!t && ((t.total_files > 0) || (t.success_files > 0) || (t.failed_files > 0))
})

const progressStatus = computed(() => {
  const t = task.value
  if (!t) return ''
  if (t.status === 'completed') return 'success'
  if (t.status === 'failed') return 'exception'
  if (t.status === 'cancelled') return 'warning'
  return ''
})

const progressFormat = (percentage) => {
  const t = task.value
  if (!t) return ''
  if (t.status === 'completed') return '完成'
  if (t.status === 'cancelled') return '已取消'
  if (t.status === 'running' && t.total_files === 0) return '准备中...'
  return `${percentage}%`
}

const watchTaskMetadata = computed(() => {
  const t = task.value
  if (!t || t.task_type !== 'watch_auto_organize') return {}
  return t.metadata || {}
})

const watchDetectedCount = computed(() => {
  const total = Number(task.value?.total_files || watchTaskMetadata.value.detected_files || 0)
  return Number.isFinite(total) && total >= 0 ? total : 0
})

const watchSuccessCount = computed(() => {
  const success = Number(task.value?.success_files || watchTaskMetadata.value.success_files || 0)
  return Number.isFinite(success) && success >= 0 ? success : 0
})

const watchFailedCount = computed(() => {
  const failed = Number(task.value?.failed_files || watchTaskMetadata.value.failed_files || 0)
  return Number.isFinite(failed) && failed >= 0 ? failed : 0
})

const watchResultSummaryText = computed(() => watchTaskMetadata.value.result_summary || '')
const watchFailureReasonText = computed(() => watchTaskMetadata.value.failure_reason || task.value?.error_message || '')

const watchFailedItems = computed(() => {
  const items = watchTaskMetadata.value.failed_items
  if (!Array.isArray(items)) return []
  return items
    .map((item) => ({
      file_id: item?.file_id || item?.fileID || '',
      file_name: item?.file_name || item?.fileName || item?.file_id || '',
      category: item?.category || '',
      reason: item?.reason || ''
    }))
    .filter(item => item.file_id || item.file_name)
})

const watchFailedItemsExpanded = ref(false)

const watchFailureTagLabel = (category) => {
  const map = {
    identify_failed: '识别失败',
    organize_failed: '整理失败',
    cloud115_auth_failed: '115 账号失效',
    cloud115_failed: '115 操作失败',
    scan_failed: '扫描失败',
    target_path: '目标路径异常',
    panic: '异常中断',
    partial_failed: '部分失败',
    conflict_skipped: '冲突跳过',
    other: '其他失败'
  }
  return map[String(category || '').trim()] || '其他失败'
}

const watchFailureTagType = (category) => {
  const key = String(category || '').trim()
  if (key === 'identify_failed' || key === 'scan_failed' || key === 'target_path') return 'warning'
  if (key === 'organize_failed' || key === 'cloud115_auth_failed' || key === 'cloud115_failed' || key === 'panic') return 'danger'
  if (key === 'partial_failed') return 'warning'
  return 'info'
}

const watchFailedItemCount = computed(() => {
  const count = Number(watchTaskMetadata.value.failed_item_count)
  return Number.isFinite(count) && count >= 0 ? count : watchFailedItems.value.length
})

const watchFailedItemsVisible = computed(() => {
  return watchFailedItemsExpanded.value ? watchFailedItems.value : watchFailedItems.value.slice(0, 3)
})

const watchFailedItemsHiddenCount = computed(() => {
  return Math.max(0, watchFailedItems.value.length - watchFailedItemsVisible.value.length)
})

const watchSourceName = computed(() => {
  const t = task.value
  if (!t || t.task_type !== 'watch_auto_organize') return ''
  if (watchTaskMetadata.value.source_name) {
    return watchTaskMetadata.value.source_name
  }
  if (!t.task_name) return ''
  const parts = String(t.task_name).split('-')
  return parts.length > 1 ? parts.slice(1).join('-').trim() : ''
})

const watchFailureCategoryTag = computed(() => {
  const t = task.value
  if (!t || t.task_type !== 'watch_auto_organize') return null

  const category = watchTaskMetadata.value.failure_category
  if (!category) return null

  const categoryMap = {
    target_path: { label: '目标目录异常', type: 'warning' },
    scan_failed: { label: '扫描失败', type: 'danger' },
    identify_failed: { label: '识别失败', type: 'warning' },
    cloud115_auth_failed: { label: '115 账号失效', type: 'danger' },
    cloud115_failed: { label: '115 操作失败', type: 'danger' },
    conflict_skipped: { label: '目标冲突', type: 'warning' },
    organize_failed: { label: '整理失败', type: 'danger' },
    partial_failed: { label: '部分失败', type: 'warning' },
    panic: { label: '异常中断', type: 'danger' }
  }

  return categoryMap[category] || { label: '处理异常', type: 'info' }
})

const taskStageText = computed(() => {
  const t = task.value
  if (!t) return ''

  const stageMap = {
    pending: '等待执行',
    running: '扫描到新增文件，正在整理',
    completed: '新增文件已整理完成',
    failed: '整理失败，请查看错误信息',
    cancelled: '任务已取消',
    scheduled: '等待调度执行'
  }

  if (t.task_type === 'watch_auto_organize') {
    return stageMap[t.status] || '等待处理'
  }

  return ''
})

const taskSummaryItems = computed(() => {
  const t = task.value
  if (!t) return []

  const items = []

  if (t.task_type === 'watch_auto_organize') {
    if (watchSourceName.value) {
      items.push({ label: '来源媒体源', value: watchSourceName.value })
    }

    items.push({ label: '执行阶段', value: taskStageText.value })

    if (watchTaskMetadata.value.organize_target_path) {
      items.push({ label: '整理目标', value: watchTaskMetadata.value.organize_target_path })
    }

    if (watchTaskMetadata.value.watch_path) {
      items.push({ label: '监控目录', value: watchTaskMetadata.value.watch_path })
    }

    if (watchTaskMetadata.value.trigger_mode) {
      items.push({
        label: '触发方式',
        value: watchTaskMetadata.value.trigger_mode === 'polling' ? '115 轮询监控' : '本地实时监控'
      })
    }

    if (t.total_files > 0) {
      items.push({ label: '处理文件', value: `${t.success_files || 0}/${t.total_files} 成功` })
    }

    if (watchTaskMetadata.value.watch_interval > 0) {
      items.push({ label: '监控间隔', value: `${watchTaskMetadata.value.watch_interval} 秒` })
    }

    if (watchTaskMetadata.value.result_summary) {
      items.push({ label: '结果概览', value: watchTaskMetadata.value.result_summary })
    }

    if (watchTaskMetadata.value.failure_reason) {
      items.push({ label: '失败原因', value: watchTaskMetadata.value.failure_reason })
    }

    if (watchFailedItems.value.length > 0) {
      const preview = watchFailedItems.value
        .slice(0, 3)
        .map(item => `${item.file_name}${item.reason ? ` - ${item.reason}` : ''}`)
        .join('，')
      items.push({
        label: '失败文件',
        value: watchFailedItems.value.length > 3 ? `${preview} 等 ${watchFailedItems.value.length} 项` : preview
      })
    }
  }

  return items
})

const pendingCount = computed(() => {
  const t = task.value
  if (!t) return 0
  const total = t.total_files || 0
  const success = t.success_files || 0
  const failed = t.failed_files || 0
  return Math.max(0, total - success - failed)
})

const handleCancel = () => emit('cancel', task.value.task_id)
const handleResume = () => emit('resume', task.value.task_id)
const handleDetail = () => emit('detail', task.value.task_id)
const toggleWatchFailedItems = () => {
  watchFailedItemsExpanded.value = !watchFailedItemsExpanded.value
}
</script>

<style scoped>
.task-card {
  border-radius: 8px;
  margin-bottom: 12px;
  transition: all 0.3s ease;
}

.task-card:hover {
  transform: translateY(-2px);
}

.task-type-strm_generate,
.task-type-organize,
.task-type-watch_auto_organize,
.task-type-scrape,
.task-type-emby_refresh,
.task-type-log_clean,
.task-type-sync_files,
.task-type-incremental_sync,
.task-type-sync_full {
  border-left: 4px solid #409eff;
}

.task-type-organize { border-left-color: #9b59b6; }
.task-type-watch_auto_organize { border-left-color: #2f9e44; }
.task-type-scrape { border-left-color: #e67e22; }
.task-type-emby_refresh { border-left-color: #52b788; }
.task-type-log_clean { border-left-color: #e6a23c; }
.task-type-sync_files { border-left-color: #909399; }
.task-type-incremental_sync, .task-type-sync_full { border-left-color: #67c23a; }

.task-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.task-title,
.task-actions,
.task-type-info,
.watch-result-stat {
  display: flex;
  align-items: center;
}

.task-title { gap: 10px; }
.task-actions { gap: 10px; }
.task-type-info { gap: 10px; margin-bottom: 15px; }
.watch-result-stat { flex-direction: column; gap: 4px; }

.task-icon { font-size: 18px; }
.task-icon.spin { animation: spin 1s linear infinite; }

@keyframes spin {
  from { transform: rotate(0deg); }
  to { transform: rotate(360deg); }
}

.task-name {
  font-weight: 600;
  font-size: 15px;
  color: #303133;
}

.status-tag { margin-left: 8px; }
.priority-tag { margin-left: 4px; }
.task-time { font-size: 12px; color: #909399; }
.task-body { padding: 10px 0; }
.config-name { color: #606266; font-size: 14px; }
.failure-tag { margin-left: auto; }

.task-summary {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
  gap: 10px;
  margin-bottom: 15px;
}

.task-summary-item,
.watch-failed-item {
  padding: 10px 12px;
  border-radius: 8px;
  background: #f7f8fa;
}

.task-summary-label,
.watch-result-stat-label {
  display: block;
  margin-bottom: 4px;
  font-size: 12px;
  color: #909399;
}

.task-summary-value,
.watch-result-stat-value {
  color: #303133;
  font-size: 14px;
  font-weight: 500;
}

.watch-result-strip {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(110px, 1fr));
  gap: 10px;
  margin-bottom: 15px;
  padding: 12px 14px;
  border-radius: 10px;
  background: linear-gradient(135deg, rgba(47, 158, 68, 0.08), rgba(47, 158, 68, 0.02));
  border: 1px solid rgba(47, 158, 68, 0.16);
}

.watch-result-text,
.watch-failed-list,
.watch-failed-more {
  font-size: 12px;
  line-height: 1.6;
}

.watch-result-text {
  grid-column: 1 / -1;
  display: flex;
  flex-direction: column;
  gap: 6px;
  color: #606266;
}

.watch-result-stat-value.success { color: #67c23a; }
.watch-result-stat-value.failed { color: #f56c6c; }

.watch-failed-panel {
  margin-bottom: 15px;
  padding: 12px 14px;
  border: 1px solid #fde2e2;
  border-radius: 10px;
  background: linear-gradient(180deg, #fff9f9 0%, #fff 100%);
}

.watch-failed-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 10px;
}

.watch-failed-title {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 13px;
  font-weight: 600;
  color: #303133;
}

.watch-failed-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.watch-failed-item {
  background: #fff;
  border: 1px solid #f2f2f2;
}

.watch-failed-item-main {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  margin-bottom: 4px;
}

.watch-failed-item-name { font-weight: 600; color: #303133; word-break: break-all; }
.watch-failed-item-id { font-size: 12px; color: #909399; word-break: break-all; }
.watch-failed-item-reason { font-size: 12px; line-height: 1.5; color: #f56c6c; word-break: break-word; }
.watch-failed-more { margin-top: 8px; color: #909399; }

.task-progress { margin-bottom: 15px; }

.task-stats {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(100px, 1fr));
  gap: 10px;
  margin-bottom: 15px;
}

.stat-item {
  padding: 12px;
  border-radius: 8px;
  background: #f8f9fb;
  text-align: center;
}

.stat-value {
  font-size: 20px;
  font-weight: 700;
  margin-bottom: 4px;
}

.stat-value.total { color: #409eff; }
.stat-value.success { color: #67c23a; }
.stat-value.failed { color: #f56c6c; }
.stat-value.pending { color: #e6a23c; }

.stat-label { font-size: 12px; color: #909399; }
.scheduled-info {
  display: flex;
  align-items: center;
  gap: 6px;
  color: #909399;
  font-size: 13px;
  margin-top: 8px;
}
.task-error { margin-top: 12px; }
</style>
