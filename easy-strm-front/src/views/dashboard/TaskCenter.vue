<template>
  <div class="space-y-4">
    <!-- 页头 -->
    <PageCard title="统一任务队列" subtitle="Task Center">
      <template #action>
        <n-button :loading="taskLoading" @click="loadTasks">
          <template #icon>
            <n-icon :component="RefreshOutline" />
          </template>
          刷新
        </n-button>
        <div class="flex items-center gap-2">
          <n-switch v-model:value="autoRefresh" />
          <span class="text-xs text-slate-500 dark:text-slate-400">自动刷新</span>
        </div>
      </template>
      <p class="text-sm text-slate-400 dark:text-slate-500">
        {{ autoRefreshHint }}
      </p>
    </PageCard>

    <!-- 统计卡 -->
    <section class="grid grid-cols-2 gap-3 lg:grid-cols-4 lg:gap-4">
      <StatCard
        v-for="card in statCards"
        :key="card.key"
        :data-testid="`stat-${card.key}`"
        :label="card.label"
        :value="card.value"
        :icon="card.icon"
        :tone="card.tone"
        :hint="card.hint"
        clickable
        :active="activeStatCard === card.key"
        @click="selectStatCard(card.key)"
      />
    </section>

    <PageCard title="任务筛选" subtitle="按类型、状态、Emby 实例、发起方式和时间筛选；统计卡随筛选变化，点击统计卡按状态筛选">
      <div class="grid gap-3 md:grid-cols-2 xl:grid-cols-5">
        <n-select v-model:value="filters.taskType" :options="taskTypeOptions" clearable filterable placeholder="任务类型" data-testid="task-type-filter" />
        <n-select v-model:value="filters.status" :options="statusOptions" clearable placeholder="执行状态" @update:value="onStatusSelect" />
        <n-select v-model:value="filters.serverId" :options="embyServerOptions" clearable placeholder="Emby 实例" />
        <n-select v-model:value="filters.origin" :options="originOptions" clearable placeholder="发起方式" />
        <n-date-picker v-model:value="filters.timeRange" type="datetimerange" clearable />
      </div>
    </PageCard>

    <!-- 任务列表 -->
    <PageCard>
      <div v-if="filteredTaskList.length > 0" class="flex flex-col gap-3">
        <TaskCard
          v-for="task in filteredTaskList"
          :key="task.task_id"
          :task="task"
          @cancel="handleCancelTask"
          @resume="handleResumeTask"
          @detail="handleTaskDetail"
        />
      </div>
      <EmptyState v-else :title="taskList.length ? '没有符合筛选条件的任务' : '暂无任务记录'" />
    </PageCard>

    <!-- 任务详情抽屉 -->
    <n-drawer v-model:show="taskDetailVisible" :width="drawerWidth" placement="right">
      <n-drawer-content title="任务详情" closable body-content-class="!p-4">
        <template #header>
          <div class="flex items-center gap-3">
            <span>任务详情</span>
            <n-button
              v-if="activeTaskId"
              size="tiny"
              secondary
              data-testid="task-view-logs"
              @click="openTaskLogs(activeTaskId)"
            >
              <template #icon><n-icon :component="ReaderOutline" /></template>
              查看日志
            </n-button>
          </div>
        </template>
        <n-spin :show="taskDetailLoading">
          <div class="min-h-40 space-y-4">
            <n-alert
              v-if="taskDetailError"
              type="error"
              :title="taskDetailError"
              :closable="false"
            />

            <template v-if="taskDetailData">
              <n-descriptions :column="1" bordered label-placement="left" size="small">
                <n-descriptions-item label="任务ID">{{ taskDetailData.task_id || '-' }}</n-descriptions-item>
                <n-descriptions-item label="任务名称">{{ taskDetailData.task_name || '-' }}</n-descriptions-item>
                <n-descriptions-item label="任务类型">{{ taskDetailTypeText }}</n-descriptions-item>
                <n-descriptions-item label="状态">
                  <n-tag size="small" :type="taskStatusTagType(taskDetailStatus)" data-testid="task-detail-status">{{ taskStatusLabel(taskDetailStatus) }}</n-tag>
                </n-descriptions-item>
                <n-descriptions-item label="创建时间">{{ taskDetailData.create_time || '-' }}</n-descriptions-item>
                <n-descriptions-item label="更新时间">{{ taskDetailData.update_time || '-' }}</n-descriptions-item>
                <n-descriptions-item label="耗时"><span data-testid="task-detail-duration">{{ taskDetailDuration }}</span></n-descriptions-item>
                <n-descriptions-item label="进度">
                  <n-progress type="line" :percentage="taskDetailProgress" :status="['partial_success', 'unknown'].includes(taskDetailStatus) ? 'warning' : taskDetailStatus === 'failed' ? 'error' : ['completed', 'success'].includes(taskDetailStatus) ? 'success' : taskDetailStatus === 'cancelled' ? 'warning' : 'default'">{{ taskDetailProgress }}%</n-progress>
                  <span>已处理 {{ taskDetailData.processed_files || 0 }} / {{ taskDetailData.total_files || 0 }}</span>
                </n-descriptions-item>
                <n-descriptions-item label="文件统计">
                  成功 {{ taskDetailData.success_files || 0 }} / 总数 {{ taskDetailData.total_files || 0 }} / 失败 {{ taskDetailData.failed_files || 0 }}
                </n-descriptions-item>
                <n-descriptions-item label="错误信息">{{ taskDetailData.error_message || '-' }}</n-descriptions-item>
              </n-descriptions>

              <div v-if="taskDetailSteps.length > 0">
                <div class="mb-3 text-sm font-bold text-slate-800 dark:text-white">执行步骤</div>
                <div class="space-y-2">
                  <div v-for="(step, index) in taskDetailSteps" :key="`${index}-${step.name}`" class="flex gap-3 rounded-xl border border-slate-100 px-3 py-2.5 dark:border-white/5">
                    <n-tag size="small" :type="stepStatusType(step.status)">{{ stepStatusText(step.status) }}</n-tag>
                    <div><div class="text-sm font-medium text-slate-700 dark:text-slate-200">{{ step.name }}</div><div v-if="step.message" class="mt-0.5 text-xs text-slate-500">{{ step.message }}</div></div>
                  </div>
                </div>
              </div>

              <!-- 任务元数据 -->
              <div>
                <div class="mb-3 text-sm font-bold text-slate-800 dark:text-white">任务元数据</div>
                <div v-if="taskDetailMetadataRows.length > 0" class="grid grid-cols-1 gap-2.5 sm:grid-cols-2">
                  <div
                    v-for="item in taskDetailMetadataRows"
                    :key="item.key"
                    class="rounded-xl border border-slate-100 bg-slate-50 px-3 py-2.5 dark:border-white/5 dark:bg-white/5"
                  >
                    <div class="mb-1 text-xs text-slate-400 dark:text-slate-500">{{ item.label }}</div>
                    <div class="whitespace-pre-line break-words text-sm leading-relaxed text-slate-700 dark:text-slate-200">{{ item.value }}</div>
                  </div>
                </div>
                <EmptyState v-else title="暂无任务元数据" />
              </div>

              <!-- 失败文件 -->
              <div v-if="taskDetailFailedItems.length > 0">
                <div class="mb-3 text-sm font-bold text-slate-800 dark:text-white">失败文件</div>
                <div v-if="taskDetailFailureGroups.length > 0" class="mb-3 grid grid-cols-1 gap-2.5 sm:grid-cols-2">
                  <div
                    v-for="group in taskDetailFailureGroups"
                    :key="group.key"
                    class="rounded-xl border border-slate-100 bg-slate-50 px-3 py-2.5 dark:border-white/5 dark:bg-white/5"
                  >
                    <div class="mb-1 flex flex-wrap items-center justify-between gap-2">
                      <div class="text-sm font-semibold text-slate-700 dark:text-slate-200">{{ group.label }}</div>
                      <n-tag :type="group.tagType" size="small">{{ group.items.length }}</n-tag>
                    </div>
                    <div class="break-words text-xs leading-relaxed text-slate-500 dark:text-slate-400">
                      {{ group.reason || '未提供失败原因' }}
                    </div>
                  </div>
                </div>
                <div class="flex flex-col gap-2.5">
                  <div
                    v-for="item in taskDetailFailedItems"
                    :key="`${item.file_id || item.file_name}-${item.reason}`"
                    class="rounded-xl border border-slate-100 bg-slate-50 px-3 py-2.5 dark:border-white/5 dark:bg-white/5"
                  >
                    <div class="mb-1 flex flex-wrap items-center gap-2">
                      <span class="break-all text-sm font-bold text-slate-800 dark:text-white">{{ item.file_name }}</span>
                      <span v-if="item.file_id" class="break-all text-xs text-slate-400 dark:text-slate-500">{{ item.file_id }}</span>
                      <n-tag v-if="item.category" :type="taskDetailFailureTagType(item.category)" size="small">
                        {{ taskDetailFailureTagLabel(item.category) }}
                      </n-tag>
					  <n-tag v-if="item.ai_used" type="info" size="small">AI辅助 · {{ item.ai_scene || '未知场景' }}</n-tag>
					  <n-tag v-if="item.metadata_source" size="small">{{ item.metadata_source }}</n-tag>
                    </div>
                    <div class="break-words text-xs leading-relaxed text-red-500 dark:text-red-400">
					  {{ item.failure_reason || item.reason || '未提供失败原因' }}
                    </div>
                  </div>
                </div>
              </div>
            </template>
          </div>
        </n-spin>
      </n-drawer-content>
    </n-drawer>
  </div>
</template>

<script setup>
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import {
  NAlert,
  NButton,
  NDescriptions,
  NDescriptionsItem,
  NDrawer,
  NDrawerContent,
  NDatePicker,
  NIcon,
  NProgress,
  NSpin,
  NSelect,
  NSwitch,
  NTag,
  useMessage
} from 'naive-ui'
import {
  RefreshOutline,
  ListOutline,
  PlayCircleOutline,
  CloseCircleOutline,
  CheckmarkCircleOutline,
  ReaderOutline
} from '@vicons/ionicons5'
import TaskCard from '../../components/TaskCard.vue'
import PageCard from '../../components/common/PageCard.vue'
import StatCard from '../../components/common/StatCard.vue'
import EmptyState from '../../components/common/EmptyState.vue'
import { cancelTask, getTaskDetail, getUnifiedTaskList, resumeTask } from '../../utils/api/task'
import { usePolling } from '../../composables/usePolling'
import {
  TASK_STATUS_GROUPS,
  effectiveTaskStatus,
  isTaskActive,
  loadTaskTypes,
  taskDuration,
  taskLogsRoute,
  taskStatusLabel,
  taskStatusTagType,
  taskTypeLabel,
  taskTypeSelectOptions
} from '../../utils/tasks'

const route = useRoute()
const router = useRouter()
const message = useMessage()

const LIST_REFRESH_MS = 3000
const DETAIL_REFRESH_MS = 2000

const taskLoading = ref(false)
const taskList = ref([])
const autoRefresh = ref(true)
const taskDetailVisible = ref(false)
const taskDetailLoading = ref(false)
const taskDetailData = ref(null)
const taskDetailStatus = computed(() => effectiveTaskStatus(taskDetailData.value))
const taskDetailProgress = computed(() => ['completed', 'success'].includes(taskDetailData.value?.status) ? 100 : Math.min(100, Math.max(0, Number(taskDetailData.value?.progress) || 0)))
const taskDetailError = ref('')
const activeTaskId = ref('')
const filters = ref({ serverId: null, taskType: null, status: null, statusGroup: null, origin: null, timeRange: null })
const now = ref(Date.now())

// 抽屉宽度：桌面 42%，移动端占满；随窗口尺寸变化
const viewportWidth = ref(typeof window !== 'undefined' ? window.innerWidth : 1280)
const onResize = () => { viewportWidth.value = window.innerWidth }
const drawerWidth = computed(() => (viewportWidth.value < 768 ? '100%' : '42%'))

const taskDetailTypeText = computed(() => {
  const type = taskDetailData.value?.task_type
  if (!type) return '-'
  const label = taskTypeLabel(type)
  return label === type ? type : `${label}（${type}）`
})
const taskDetailDuration = computed(() => taskDuration(taskDetailData.value, now.value))

const matchesStatus = (task) => {
  const status = effectiveTaskStatus(task)
  const filter = filters.value
  if (filter.status && status !== filter.status) return false
  if (filter.statusGroup && !TASK_STATUS_GROUPS[filter.statusGroup].includes(status)) return false
  return true
}

// 除状态外的筛选条件；统计卡在此基础上计数
const baseFilteredList = computed(() => taskList.value.filter(task => {
  const filter = filters.value
  if (filter.serverId && Number(task.metadata?.server_id) !== Number(filter.serverId)) return false
  if (filter.taskType && task.task_type !== filter.taskType) return false
  if (filter.origin && task.metadata?.origin !== filter.origin) return false
  if (Array.isArray(filter.timeRange) && filter.timeRange.length === 2) {
    const timestamp = Date.parse(String(task.create_time || '').replace(' ', 'T'))
    if (Number.isFinite(timestamp) && (timestamp < filter.timeRange[0] || timestamp > filter.timeRange[1])) return false
  }
  return true
}))
const filteredTaskList = computed(() => baseFilteredList.value.filter(matchesStatus))

const taskCounts = computed(() => {
  const counts = { all: baseFilteredList.value.length, active: 0, failed: 0, done: 0 }
  for (const task of baseFilteredList.value) {
    const status = effectiveTaskStatus(task)
    for (const [group, statuses] of Object.entries(TASK_STATUS_GROUPS)) {
      if (statuses.includes(status)) counts[group] += 1
    }
  }
  return counts
})

const statCards = computed(() => [
  { key: 'all', label: '总任务', value: taskCounts.value.all, icon: ListOutline, tone: 'cyan', hint: '显示全部状态' },
  { key: 'active', label: '排队 / 执行中', value: taskCounts.value.active, icon: PlayCircleOutline, tone: 'amber', hint: '等待执行与执行中' },
  { key: 'failed', label: '失败', value: taskCounts.value.failed, icon: CloseCircleOutline, tone: 'red', hint: '失败与结果未知' },
  { key: 'done', label: '已完成', value: taskCounts.value.done, icon: CheckmarkCircleOutline, tone: 'green', hint: '成功、部分成功与跳过' }
])
const activeStatCard = computed(() => {
  if (filters.value.statusGroup) return filters.value.statusGroup
  return filters.value.status ? '' : 'all'
})
const selectStatCard = (key) => {
  filters.value.status = null
  filters.value.statusGroup = key === 'all' || filters.value.statusGroup === key ? null : key
}
const onStatusSelect = () => { filters.value.statusGroup = null }

const embyServerOptions = computed(() => {
  const seen = new Map()
  taskList.value.forEach(task => {
    const id = task.metadata?.server_id
    if (id !== undefined && id !== null) seen.set(Number(id), task.metadata?.server_name || `实例 ${id}`)
  })
  return [...seen.entries()].map(([value, label]) => ({ value, label }))
})
const taskTypeOptions = computed(() => taskTypeSelectOptions(taskList.value.map(task => task.task_type)))
const statusOptions = ['pending', 'running', 'success', 'completed', 'partial_success', 'failed', 'cancelled', 'skipped', 'unknown']
  .map(value => ({ label: taskStatusLabel(value), value }))
const originOptions = [{ label: '页面手动', value: 'manual' }, { label: '整理联动', value: 'organize_linkage' }, { label: '系统自动', value: 'automatic' }]

const hasActiveTasks = computed(() => taskList.value.some(isTaskActive))
const autoRefreshHint = computed(() => {
  if (!autoRefresh.value) return '自动刷新已关闭，可手动刷新。'
  return hasActiveTasks.value
    ? `有排队或执行中的任务，每 ${LIST_REFRESH_MS / 1000} 秒自动刷新（页面不可见时暂停）。`
    : '当前没有排队或执行中的任务，自动刷新已暂停；新提交的任务请点击刷新。'
})

const taskDetailMetadataRows = computed(() => {
  const metadata = taskDetailData.value?.metadata
  if (!metadata || typeof metadata !== 'object' || Array.isArray(metadata)) {
    return []
  }

  const labelMap = {
    exported_files: '已生成 STRM 文件数',
    output_path: '服务器导出目录',
    errors: '失败详情',
    source_id: '来源ID',
    source_name: '来源媒体源',
    source_type: '来源类型',
    source_path: '来源路径',
    organize_target_path: '整理目标',
    media_type: '媒体类型',
    conflict_policy: '冲突策略',
    operation_mode: '整理方式',
    watch_interval: '监控间隔',
    trigger_mode: '触发方式',
    detected_files: '检测文件数',
    success_files: '成功数',
    failed_files: '失败数',
    skipped_files: '跳过数',
    result_summary: '结果概览',
    failure_category: '失败分类',
    failure_reason: '失败原因',
    failed_item_count: '失败文件数',
    server_id: 'Emby 实例 ID',
    server_name: 'Emby 实例',
    operation: '操作类型',
 record_ids: '目标分享', file_id: '目标文件', blocking_task_ids: '等待任务', cancellable: '允许取消等待', result: '执行结果', recoverable_operation: '重启自动恢复',
    target: '操作目标',
    origin: '发起方式',
    current_step: '当前步骤',
    remote_task_id: 'Emby 任务 ID',
    success_count: '成功数量',
    failed_count: '失败数量',
    submitted_count: '已提交数量',
    library_id: 'Emby 媒体库 ID',
    library_name: 'Emby 媒体库',
    strm_count: 'STRM 视频数量',
    covered_before: '执行前已有主图',
    covered_after: '执行后已有主图',
    generated_cover_count: '本次新增主图',
    missing_cover_count: '仍缺少主图',
    conclusion: '执行结论'
  }

  return Object.entries(metadata)
    .filter(([key, value]) => key !== 'failed_items' && key !== 'steps' && key !== 'preview_path' && value !== null && value !== undefined && value !== '')
    .map(([key, value]) => {
      let formatted = value
      if (key === 'trigger_mode') {
        formatted = value === 'polling' ? '115 轮询监控' : value === 'fsnotify' ? '本地实时监控' : value
      } else if (key === 'media_type') {
        formatted = ({ all: '全部', movie: '电影', tv: '剧集' })[value] || value
      } else if (key === 'conflict_policy') {
        formatted = ({ skip: '跳过', overwrite: '覆盖', suffix: '追加序号' })[value] || value
      } else if (key === 'operation_mode') {
        formatted = ({ move: '移动', copy: '复制', hardlink: '硬链接', symlink: '软链接' })[value] || value
      } else if (key === 'operation') {
        formatted = ({ share_delete: '删除分享', share_clear: '清空分享文件记录', share_media_delete: '删除分享文件记录', strm_scan_capture: '扫描 STRM 并生成视频封面', media_info: '媒体信息提取', subtitle_scan: '外挂字幕扫描', metadata_refresh: '元数据刷新' })[value] || value
      } else if (key === 'watch_interval' && Number.isFinite(Number(value))) {
        formatted = `${value} 秒`
      } else if (key === 'errors' && Array.isArray(value)) {
        formatted = value.join('\n') || '无'
      } else if (['record_ids','blocking_task_ids'].includes(key) && Array.isArray(value)) {
 formatted=value.join('、') || '无'
 } else if (Array.isArray(value)) {
        formatted = `${value.length} 项`
      } else if (typeof value === 'object') {
        formatted = JSON.stringify(value)
      }

      return {
        key,
        label: labelMap[key] || key,
        value: String(formatted)
      }
    })
})

const taskDetailSteps = computed(() => Array.isArray(taskDetailData.value?.metadata?.steps) ? taskDetailData.value.metadata.steps : [])
const stepStatusText = status => ({ pending: '待执行', running: '执行中', success: '成功', skipped: '已跳过', failed: '失败', unknown: '未知', cancelled: '已取消' })[status] || status
const stepStatusType = status => ({ running: 'info', success: 'success', failed: 'error', unknown: 'warning', cancelled: 'default' })[status] || 'default'

const taskDetailFailedItems = computed(() => {
  const items = taskDetailData.value?.metadata?.failed_items
  if (!Array.isArray(items)) {
    return []
  }

  return items
    .map(item => ({
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

const taskDetailFailureGroups = computed(() => {
  const items = taskDetailFailedItems.value
  if (items.length === 0) {
    return []
  }

  const groupOrder = ['emby_library_refresh_failed', 'identify_failed', 'organize_failed', 'cloud115_auth_failed', 'cloud115_failed', 'scan_failed', 'target_path', 'partial_failed', 'conflict_skipped', 'panic', 'other']
  const groupMap = new Map()

  const getGroupKey = (category, reason) => {
    const normalizedCategory = String(category || '').trim()
    if (normalizedCategory) return normalizedCategory
    const text = String(reason || '').toLowerCase()
    if (text.includes('identify') || text.includes('tmdb') || text.includes('recogniz')) return 'identify_failed'
    if (text.includes('organize') || text.includes('move') || text.includes('copy')) return 'organize_failed'
    if (text.includes('cookie') || text.includes('auth') || text.includes('unauthorized')) return 'cloud115_auth_failed'
    if (text.includes('cloud115') || text.includes('115')) return 'cloud115_failed'
    if (text.includes('scan') || text.includes('list')) return 'scan_failed'
    if (text.includes('target') && text.includes('path')) return 'target_path'
    if (text.includes('partial') || text.includes('部分')) return 'partial_failed'
    if (text.includes('conflict') || text.includes('冲突')) return 'conflict_skipped'
    if (text.includes('panic') || text.includes('exception')) return 'panic'
    return 'other'
  }

  const getGroupMeta = (key) => {
    const meta = {
      emby_library_refresh_failed: { label: 'Emby 媒体库刷新失败', tagType: 'error' },
      identify_failed: { label: '识别失败', tagType: 'warning' },
      organize_failed: { label: '整理失败', tagType: 'error' },
      cloud115_auth_failed: { label: '115 账号失效', tagType: 'error' },
      cloud115_failed: { label: '115 操作失败', tagType: 'error' },
      scan_failed: { label: '扫描失败', tagType: 'warning' },
      target_path: { label: '目标路径异常', tagType: 'warning' },
      partial_failed: { label: '部分失败', tagType: 'warning' },
      conflict_skipped: { label: '冲突跳过', tagType: 'warning' },
      panic: { label: '异常中断', tagType: 'error' },
      other: { label: '其他失败', tagType: 'info' }
    }
    return meta[key] || meta.other
  }

  for (const item of items) {
    const key = getGroupKey(item.category, item.reason)
    if (!groupMap.has(key)) {
      const meta = getGroupMeta(key)
      groupMap.set(key, {
        key,
        label: meta.label,
        tagType: meta.tagType,
        reason: item.reason || '',
        items: []
      })
    }
    groupMap.get(key).items.push(item)
  }

  return groupOrder.filter(key => groupMap.has(key)).map(key => groupMap.get(key))
})

const taskDetailFailureTagLabel = (category) => {
  const map = {
    emby_library_refresh_failed: 'Emby 媒体库刷新失败',
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

const taskDetailFailureTagType = (category) => {
  const key = String(category || '').trim()
  if (key === 'identify_failed' || key === 'scan_failed' || key === 'target_path') return 'warning'
  if (key === 'emby_library_refresh_failed' || key === 'organize_failed' || key === 'cloud115_auth_failed' || key === 'cloud115_failed' || key === 'panic') return 'error'
  if (key === 'partial_failed') return 'warning'
  return 'info'
}

// 列表只在存在排队/执行中任务时自动刷新；详情仅在任务未结束时刷新。页面不可见时均暂停。
usePolling(() => loadTasks({ silent: true }), LIST_REFRESH_MS, { enabled: () => autoRefresh.value && hasActiveTasks.value })
const detailActive = computed(() => taskDetailVisible.value && !!activeTaskId.value && isTaskActive(taskDetailData.value))
usePolling(() => loadTaskDetail(activeTaskId.value, false), DETAIL_REFRESH_MS, { enabled: detailActive })

const syncTaskInList = (taskDetail) => {
  if (!taskDetail?.task_id) return
  const index = taskList.value.findIndex(item => item.task_id === taskDetail.task_id)
  if (index === -1) return
  taskList.value[index] = {
    ...taskList.value[index],
    ...taskDetail
  }
}

const loadTasks = async ({ silent = false } = {}) => {
  if (taskLoading.value) return
  taskLoading.value = true
  try {
    const response = await getUnifiedTaskList()
    const payload = response.data?.data?.data || response.data?.data || response.data || []
    taskList.value = Array.isArray(payload) ? payload : []
    now.value = Date.now()
  } catch (error) {
    if (!silent) message.error('加载任务列表失败')
  } finally {
    taskLoading.value = false
  }
}

const loadTaskDetail = async (taskId, showLoading = true) => {
  activeTaskId.value = taskId
  if (showLoading) {
    taskDetailLoading.value = true
  }
  try {
    const response = await getTaskDetail(taskId)
    const payload = response.data.data || response.data || null
    if (taskId !== activeTaskId.value) return
    taskDetailData.value = payload
    now.value = Date.now()
    syncTaskInList(payload)
  } catch (error) {
    if (taskId !== activeTaskId.value) return
    taskDetailError.value = error.response?.data?.error || error.message || '加载任务详情失败'
  } finally {
    if (showLoading) {
      taskDetailLoading.value = false
    }
  }
}

const openTaskLogs = (taskId) => {
  taskDetailVisible.value = false
  router.push(taskLogsRoute(taskId))
}

const handleTaskDetail = async (taskId) => {
  taskDetailVisible.value = true
  taskDetailError.value = ''
  taskDetailData.value = null
  await loadTaskDetail(taskId, true)
}

const handleCancelTask = async (taskId) => {
  try {
    await cancelTask(taskId)
    message.success('任务已取消')
    await loadTasks()
  } catch (error) {
    message.error('取消任务失败')
  }
}

const handleResumeTask = async (taskId) => {
  try {
    await resumeTask(taskId)
    message.success('任务已重新执行')
    await loadTasks()
  } catch (error) {
    message.error('恢复任务失败')
  }
}

const openRouteTask = async () => {
  const taskId = route.query.task_id
  if (taskId) {
    await handleTaskDetail(String(taskId))
  }
}

watch(taskDetailVisible, (value) => {
  if (!value) {
    activeTaskId.value = ''
    taskDetailError.value = ''
    taskDetailData.value = null
  }
})

watch(() => route.query.task_id, async (taskId) => {
  if (taskId) {
    await handleTaskDetail(String(taskId))
  }
})

onBeforeUnmount(() => {
  window.removeEventListener('resize', onResize)
})

onMounted(async () => {
  window.addEventListener('resize', onResize)
  void loadTaskTypes()
  if (route.query.emby_server_id) filters.value.serverId = Number(route.query.emby_server_id)
  if (route.query.task_type) filters.value.taskType = String(route.query.task_type)
  await loadTasks()
  await openRouteTask()
})
</script>
