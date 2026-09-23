<template>
  <div
    v-if="task"
    data-testid="task-card"
    class="rounded-2xl border border-slate-200 border-l-4 bg-white p-4 shadow-sm transition-shadow hover:shadow-md dark:border-white/5 dark:bg-ink-900 lg:p-5"
    :class="taskAccentClass"
  >
    <!-- 头部：任务名 / 状态 / 操作 -->
    <div class="flex flex-wrap items-center justify-between gap-3 border-b border-slate-100 pb-3 dark:border-white/5">
      <div class="flex min-w-0 flex-wrap items-center gap-2">
        <n-icon
          size="18"
          :component="taskStatusIcon"
          class="shrink-0 text-slate-500 dark:text-slate-400"
          :class="{ 'animate-spin': task.status === 'running' }"
        />
        <span class="truncate text-sm font-bold text-slate-800 dark:text-white">{{ taskDisplayName }}</span>
        <n-tag :type="taskStatusType" size="small" round>{{ taskStatusText }}</n-tag>
        <n-tag v-if="task.priority && task.priority < 5" type="error" size="small" round>
          P{{ task.priority }}
        </n-tag>
      </div>
      <div class="flex flex-wrap items-center gap-2">
        <n-button type="primary" size="small" secondary @click="handleDetail">详情</n-button>
        <n-button v-if="canCancel" type="error" size="small" secondary @click="handleCancel">取消任务</n-button>
        <n-button v-if="canResume" type="warning" size="small" secondary @click="handleResume">{{ resumeButtonText }}</n-button>
        <span class="text-xs text-slate-400 dark:text-slate-500">{{ task.create_time || '' }}</span>
      </div>
    </div>

    <div class="pt-3">
      <!-- 任务类型信息 -->
      <div class="mb-3 flex flex-wrap items-center gap-2">
        <n-tag :type="taskTypeTagType" size="small">{{ taskTypeName }}</n-tag>
        <span v-if="task.config_name" class="text-sm text-slate-500 dark:text-slate-400">{{ task.config_name }}</span>
        <n-tag
          v-if="watchFailureCategoryTag"
          :type="watchFailureCategoryTag.type"
          size="small"
          class="ml-auto"
        >
          {{ watchFailureCategoryTag.label }}
        </n-tag>
      </div>

      <!-- 摘要信息 -->
      <div v-if="taskSummaryItems.length > 0" class="mb-3 grid grid-cols-1 gap-2.5 sm:grid-cols-2 lg:grid-cols-3">
        <div
          v-for="item in taskSummaryItems"
          :key="item.label"
          class="rounded-xl bg-slate-50 px-3 py-2.5 dark:bg-white/5"
        >
          <span class="block text-xs text-slate-400 dark:text-slate-500">{{ item.label }}</span>
          <span class="mt-0.5 block break-words text-sm font-medium text-slate-700 dark:text-slate-200">{{ item.value }}</span>
        </div>
      </div>

      <!-- watch_auto_organize 结果条 -->
      <div
        v-if="task.task_type === 'watch_auto_organize'"
        class="mb-3 grid grid-cols-3 gap-2.5 rounded-xl border border-emerald-500/20 bg-emerald-500/5 p-3"
      >
        <div class="flex flex-col gap-1 text-center">
          <span class="text-xs text-slate-400 dark:text-slate-500">检测</span>
          <span class="text-sm font-bold tabular-nums text-slate-700 dark:text-slate-200">{{ watchDetectedCount }}</span>
        </div>
        <div class="flex flex-col gap-1 text-center">
          <span class="text-xs text-slate-400 dark:text-slate-500">成功</span>
          <span class="text-sm font-bold tabular-nums text-emerald-600 dark:text-emerald-400">{{ watchSuccessCount }}</span>
        </div>
        <div class="flex flex-col gap-1 text-center">
          <span class="text-xs text-slate-400 dark:text-slate-500">失败</span>
          <span class="text-sm font-bold tabular-nums text-red-500 dark:text-red-400">{{ watchFailedCount }}</span>
        </div>
        <div
          v-if="watchResultSummaryText || watchFailureReasonText"
          class="col-span-3 flex flex-col gap-1 text-xs leading-relaxed text-slate-500 dark:text-slate-400"
        >
          <span v-if="watchResultSummaryText">{{ watchResultSummaryText }}</span>
          <span v-if="watchFailureReasonText">{{ watchFailureReasonText }}</span>
        </div>
      </div>

      <!-- 失败文件面板 -->
      <div
        v-if="watchFailedItems.length > 0"
        class="mb-3 rounded-xl border border-red-200 bg-red-50/50 p-3 dark:border-red-500/20 dark:bg-red-500/5"
      >
        <div class="mb-2.5 flex items-center justify-between gap-3">
          <div class="flex items-center gap-2 text-xs font-bold text-slate-700 dark:text-slate-200">
            <span>失败文件</span>
            <n-tag size="small" type="error">{{ watchFailedItemCount }}</n-tag>
          </div>
          <n-button text type="primary" size="small" @click="toggleWatchFailedItems">
            {{ watchFailedItemsExpanded ? '收起' : '展开全部' }}
          </n-button>
        </div>
        <div class="flex flex-col gap-2">
          <div
            v-for="item in watchFailedItemsVisible"
            :key="`${item.file_id || item.file_name}-${item.reason}`"
            class="rounded-lg border border-slate-100 bg-white px-3 py-2.5 dark:border-white/5 dark:bg-white/5"
          >
            <div class="mb-1 flex flex-wrap items-center gap-2">
              <span class="break-all text-sm font-semibold text-slate-800 dark:text-white">{{ item.file_name }}</span>
              <span v-if="item.file_id" class="break-all text-xs text-slate-400 dark:text-slate-500">{{ item.file_id }}</span>
              <n-tag v-if="item.category" :type="watchFailureTagType(item.category)" size="small">
                {{ watchFailureTagLabel(item.category) }}
              </n-tag>
			  <n-tag v-if="item.ai_used" type="info" size="small">AI辅助 · {{ item.ai_scene || '未知场景' }}</n-tag>
			  <n-tag v-if="item.metadata_source" size="small">{{ item.metadata_source }}</n-tag>
            </div>
            <div class="break-words text-xs leading-relaxed text-red-500 dark:text-red-400">
			  {{ item.failure_reason || item.reason || '未提供失败原因' }}
            </div>
          </div>
        </div>
        <div v-if="watchFailedItemsHiddenCount > 0" class="mt-2 text-xs text-slate-400 dark:text-slate-500">
          还有 {{ watchFailedItemsHiddenCount }} 项失败文件未展开
        </div>
      </div>

      <!-- 进度条 -->
      <n-progress
        v-if="showProgress"
        type="line"
        class="mb-3"
        :percentage="task.status === 'completed' ? 100 : (task.progress || 0)"
        :status="progressStatus"
        :height="14"
        indicator-placement="outside"
      >
        {{ progressFormat(task.progress || 0) }}
      </n-progress>

      <!-- 跨账号云下载分步骤进度 -->
      <div v-if="crossAccountSteps.length > 0" class="mb-3 rounded-xl border border-indigo-100 bg-indigo-50/40 p-3 dark:border-indigo-400/20 dark:bg-indigo-400/5">
        <div class="mb-2 text-xs font-bold text-slate-600 dark:text-slate-300">执行步骤</div>
        <div class="flex flex-col gap-2 sm:flex-row sm:items-stretch">
          <div v-for="(step, index) in crossAccountSteps" :key="step.name" class="flex min-w-0 flex-1 items-center gap-2 rounded-lg bg-white/80 px-2.5 py-2 dark:bg-white/5">
            <n-tag size="small" round :type="stepTagType(step.status)">{{ stepStatusText(step.status) }}</n-tag>
            <div class="min-w-0"><div class="truncate text-xs font-semibold text-slate-700 dark:text-slate-200">{{ index + 1 }}. {{ step.name }}</div><div v-if="step.message" class="truncate text-[11px] text-slate-400">{{ step.message }}</div></div>
          </div>
        </div>
      </div>

      <!-- 文件统计 -->
      <div v-if="showFileStats" class="mb-3 grid grid-cols-2 gap-2.5 sm:grid-cols-4">
        <div class="rounded-xl bg-slate-50 p-3 text-center dark:bg-white/5">
          <div class="text-lg font-bold tabular-nums text-cyan-600 dark:text-cyan-400">{{ task.total_files || 0 }}</div>
          <div class="text-xs text-slate-400 dark:text-slate-500">总文件</div>
        </div>
        <div class="rounded-xl bg-slate-50 p-3 text-center dark:bg-white/5">
          <div class="text-lg font-bold tabular-nums text-emerald-600 dark:text-emerald-400">{{ task.success_files || 0 }}</div>
          <div class="text-xs text-slate-400 dark:text-slate-500">成功</div>
        </div>
        <div class="rounded-xl bg-slate-50 p-3 text-center dark:bg-white/5">
          <div class="text-lg font-bold tabular-nums text-red-500 dark:text-red-400">{{ task.failed_files || 0 }}</div>
          <div class="text-xs text-slate-400 dark:text-slate-500">失败</div>
        </div>
        <div class="rounded-xl bg-slate-50 p-3 text-center dark:bg-white/5">
          <div class="text-lg font-bold tabular-nums text-amber-600 dark:text-amber-400">{{ pendingCount }}</div>
          <div class="text-xs text-slate-400 dark:text-slate-500">处理中</div>
        </div>
      </div>

      <!-- 计划执行时间 -->
      <div v-if="task.scheduled_time" class="mt-2 flex items-center gap-1.5 text-xs text-slate-400 dark:text-slate-500">
        <n-icon size="14" :component="TimeOutline" />
        <span>计划执行时间: {{ task.scheduled_time }}</span>
      </div>

      <!-- 错误信息 -->
      <n-alert
        v-if="task.error_message"
        type="error"
        :title="task.error_message"
        :closable="false"
        class="mt-3"
      />
    </div>
  </div>
</template>

<script setup>
import { computed, ref, toRef } from 'vue'
import { NAlert, NButton, NIcon, NProgress, NTag } from 'naive-ui'
import {
  TimeOutline,
  SyncOutline,
  CheckmarkCircleOutline,
  CloseCircleOutline,
  AlertCircleOutline,
  TimerOutline,
  DocumentTextOutline,
  RefreshOutline,
  VideocamOutline,
  FilmOutline,
  SearchOutline,
  LinkOutline,
  CloudDownloadOutline,
  SwapHorizontalOutline
} from '@vicons/ionicons5'

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
  emby_server: 'Emby 实例管理',
  emby_user: 'Emby 用户管理',
  emby_library: 'Emby 媒体库管理',
  emby_cover: 'Emby 媒体库封面',
  emby_plugin: '神医助手任务',
  emby_scheduled: 'Emby定时任务触发',
  log_clean: '日志清理',
  sync_files: '文件同步',
  offline_download: '115 云下载',
  file_transfer: '文件传输'
}

const taskTypeIcons = {
  strm_generate: DocumentTextOutline,
  incremental_sync: TimerOutline,
  sync_full: TimerOutline,
  sync_transfer: TimerOutline,
  cleanup: RefreshOutline,
  proxy_refresh: RefreshOutline,
  organize: VideocamOutline,
  watch_auto_organize: FilmOutline,
  scrape: SearchOutline,
  emby_refresh: LinkOutline,
  emby_server: LinkOutline,
  emby_user: LinkOutline,
  emby_library: FilmOutline,
  emby_cover: FilmOutline,
  emby_plugin: LinkOutline,
  log_clean: RefreshOutline,
  sync_files: TimerOutline,
  offline_download: CloudDownloadOutline,
  file_transfer: SwapHorizontalOutline
}

const taskTypeTagTypes = {
  strm_generate: 'primary',
  incremental_sync: 'success',
  sync_full: 'success',
  sync_transfer: 'default',
  cleanup: 'warning',
  proxy_refresh: 'info',
  organize: 'primary',
  watch_auto_organize: 'success',
  scrape: 'warning',
  emby_refresh: 'error',
  emby_server: 'info',
  emby_user: 'info',
  emby_library: 'primary',
  emby_cover: 'success',
  emby_plugin: 'warning',
  log_clean: 'warning',
  sync_files: 'info',
  offline_download: 'info',
  file_transfer: 'primary'
}

// 任务类型左侧强调色
const taskAccentClasses = {
  strm_generate: 'border-l-cyan-500',
  incremental_sync: 'border-l-green-500',
  sync_full: 'border-l-green-500',
  sync_transfer: 'border-l-cyan-500',
  cleanup: 'border-l-amber-500',
  proxy_refresh: 'border-l-sky-500',
  organize: 'border-l-violet-500',
  watch_auto_organize: 'border-l-emerald-500',
  scrape: 'border-l-orange-500',
  emby_refresh: 'border-l-teal-500',
  emby_server: 'border-l-sky-500',
  emby_user: 'border-l-sky-500',
  emby_library: 'border-l-cyan-500',
  emby_cover: 'border-l-emerald-500',
  emby_plugin: 'border-l-violet-500',
  log_clean: 'border-l-amber-500',
  sync_files: 'border-l-slate-400',
  offline_download: 'border-l-blue-500',
  file_transfer: 'border-l-indigo-500'
}

const taskAccentClass = computed(() => {
  return taskAccentClasses[task.value?.task_type] || 'border-l-cyan-500'
})

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
    success: 'success',
    partial_success: 'warning',
    unknown: 'warning',
    failed: 'error',
    cancelled: 'info',
    scheduled: 'default'
  }
  return task.value ? (statusMap[task.value.status] || 'info') : 'info'
})

const taskStatusText = computed(() => {
  const textMap = {
    pending: '待执行',
    running: '执行中',
    completed: '已完成',
    success: '成功',
    partial_success: '部分成功',
    unknown: '结果未知',
    failed: '失败',
    cancelled: '已取消',
    scheduled: '已调度'
  }
  return task.value ? (textMap[task.value.status] || '未知') : '未知'
})

const taskStatusIcon = computed(() => {
  const t = task.value
  if (!t) return TimerOutline
  if (t.status === 'running') return SyncOutline
  if (t.status === 'completed' || t.status === 'success') return CheckmarkCircleOutline
  if (t.status === 'partial_success' || t.status === 'partial_failed') return AlertCircleOutline
  if (t.status === 'failed' || t.status === 'cancelled') return CloseCircleOutline
  if (t.status === 'scheduled') return TimeOutline
  return taskTypeIcons[t.task_type] || TimerOutline
})

const canCancel = computed(() => {
  const t = task.value
  return !!t && (t.status === 'pending' || t.status === 'running')
})

const canResume = computed(() => {
  const t = task.value
  return !!t && t.task_type === 'watch_auto_organize' && (t.status === 'cancelled' || t.status === 'failed' || t.status === 'partial_success')
})

const showProgress = computed(() => {
  const t = task.value
  return !!t && ['pending', 'running', 'completed', 'success', 'partial_success', 'unknown', 'failed', 'cancelled'].includes(t.status)
})

const showFileStats = computed(() => {
  const t = task.value
  return !!t && ((t.total_files > 0) || (t.success_files > 0) || (t.failed_files > 0))
})

const progressStatus = computed(() => {
  const t = task.value
  if (!t) return 'default'
  if (t.status === 'completed' || t.status === 'success') return 'success'
  if (t.status === 'partial_success' || t.status === 'unknown') return 'warning'
  if (t.status === 'failed') return 'error'
  if (t.status === 'cancelled') return 'warning'
  return 'default'
})

const progressFormat = (percentage) => {
  const t = task.value
  if (!t) return ''
  if (t.status === 'completed' || t.status === 'success') return '完成'
  if (t.status === 'partial_success') return '部分成功'
  if (t.status === 'unknown') return '结果未知'
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
	  reason: item?.reason || '',
	  ai_used: Boolean(item?.ai_used),
	  ai_scene: item?.ai_scene || '',
	  metadata_source: item?.metadata_source || '',
	  failure_reason: item?.failure_reason || ''
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
  if (key === 'organize_failed' || key === 'cloud115_auth_failed' || key === 'cloud115_failed' || key === 'panic') return 'error'
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
    scan_failed: { label: '扫描失败', type: 'error' },
    identify_failed: { label: '识别失败', type: 'warning' },
    cloud115_auth_failed: { label: '115 账号失效', type: 'error' },
    cloud115_failed: { label: '115 操作失败', type: 'error' },
    conflict_skipped: { label: '目标冲突', type: 'warning' },
    organize_failed: { label: '整理失败', type: 'error' },
    partial_failed: { label: '部分失败', type: 'warning' },
    panic: { label: '异常中断', type: 'error' }
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

  if (String(t.task_id || '').startsWith('share_strm_')) {
    return [
      { label: '已处理记录', value: `${t.processed_files || 0} / ${t.total_files || 0}` },
      { label: '已生成 STRM', value: t.metadata?.exported_files || 0 }
    ]
  }

  const items = []

  if (String(t.task_type || '').startsWith('emby_')) {
    if (t.metadata?.server_name) items.push({ label: 'Emby 实例', value: t.metadata.server_name })
    if (t.metadata?.target) items.push({ label: '操作目标', value: t.metadata.target })
    if (t.metadata?.origin) items.push({ label: '发起方式', value: ({ manual: '页面手动', organize_linkage: '整理联动', automatic: '系统自动' })[t.metadata.origin] || t.metadata.origin })
    if (t.metadata?.current_step) items.push({ label: '当前步骤', value: t.metadata.current_step })
    if (t.metadata?.conclusion) items.push({ label: '执行结论', value: t.metadata.conclusion })
  }

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

const resumeButtonText = computed(() => task.value?.task_type === 'watch_auto_organize' ? 'AI辅助重试' : '恢复任务')

const crossAccountSteps = computed(() => {
  const t = task.value
  if (!t || t.task_type !== 'offline_download' || !t.metadata?.cross_account) return []
  return Array.isArray(t.metadata.steps) ? t.metadata.steps : []
})
const stepStatusText = status => ({ pending: '待执行', running: '执行中', success: '成功', failed: '失败', skipped: '跳过' })[status] || '未知'
const stepTagType = status => ({ running: 'info', success: 'success', failed: 'error', skipped: 'default' })[status] || 'warning'

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
