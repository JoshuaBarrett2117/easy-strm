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
        保留现有任务取消、恢复、详情与失败文件定位能力，并用正式页面承载。
      </p>
    </PageCard>

    <!-- 统计卡 -->
    <section class="grid grid-cols-2 gap-3 lg:grid-cols-4 lg:gap-4">
      <StatCard label="总任务" :value="taskList.length" :icon="ListOutline" tone="cyan" />
      <StatCard label="执行中" :value="taskCounts.running" :icon="PlayCircleOutline" tone="amber" />
      <StatCard label="失败" :value="taskCounts.failed" :icon="CloseCircleOutline" tone="red" />
      <StatCard label="已完成" :value="taskCounts.completed" :icon="CheckmarkCircleOutline" tone="green" />
    </section>

    <PageCard title="任务筛选" subtitle="按 Emby 实例、类型、状态、发起方式和时间筛选">
      <div class="grid gap-3 md:grid-cols-2 xl:grid-cols-5">
        <n-select v-model:value="filters.serverId" :options="embyServerOptions" clearable placeholder="Emby 实例" />
        <n-select v-model:value="filters.taskType" :options="embyTaskTypeOptions" clearable placeholder="任务类型" />
        <n-select v-model:value="filters.status" :options="statusOptions" clearable placeholder="执行状态" />
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
      <EmptyState v-else title="暂无任务记录" />
    </PageCard>

    <!-- 任务详情抽屉 -->
    <n-drawer v-model:show="taskDetailVisible" :width="drawerWidth" placement="right">
      <n-drawer-content title="任务详情" closable body-content-class="!p-4">
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
                <n-descriptions-item label="任务类型">{{ taskDetailData.task_type || '-' }}</n-descriptions-item>
                <n-descriptions-item label="状态">{{ taskDetailData.status || '-' }}</n-descriptions-item>
                <n-descriptions-item label="创建时间">{{ taskDetailData.create_time || '-' }}</n-descriptions-item>
                <n-descriptions-item label="更新时间">{{ taskDetailData.update_time || '-' }}</n-descriptions-item>
                <n-descriptions-item label="进度">
                  <n-progress type="line" :percentage="taskDetailProgress" :status="taskDetailData.status === 'failed' ? 'error' : taskDetailData.status === 'completed' ? 'success' : taskDetailData.status === 'cancelled' ? 'warning' : 'default'">{{ taskDetailProgress }}%</n-progress>
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
import { useRoute } from 'vue-router'
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
  CheckmarkCircleOutline
} from '@vicons/ionicons5'
import TaskCard from '../../components/TaskCard.vue'
import PageCard from '../../components/common/PageCard.vue'
import StatCard from '../../components/common/StatCard.vue'
import EmptyState from '../../components/common/EmptyState.vue'
import { cancelTask, getTaskDetail, getUnifiedTaskList, resumeTask } from '../../utils/api/task'

const route = useRoute()
const message = useMessage()

const taskLoading = ref(false)
const taskList = ref([])
const autoRefresh = ref(true)
const taskDetailVisible = ref(false)
const taskDetailLoading = ref(false)
const taskDetailData = ref(null)
const taskDetailProgress = computed(() => taskDetailData.value?.status === 'completed' ? 100 : Math.min(100, Math.max(0, Number(taskDetailData.value?.progress) || 0)))
const taskDetailError = ref('')
const activeTaskId = ref('')
const filters = ref({ serverId: null, taskType: null, status: null, origin: null, timeRange: null })
let timer = null
let taskDetailTimer = null

// 抽屉宽度：桌面 42%，移动端占满
const drawerWidth = computed(() => {
  if (typeof window !== 'undefined' && window.innerWidth < 768) {
    return '100%'
  }
  return '42%'
})

const taskCounts = computed(() => {
  return taskList.value.reduce((acc, item) => {
    const key = item.status || 'other'
    acc[key] = (acc[key] || 0) + 1
    if (key === 'success' || key === 'partial_success') acc.completed += 1
    return acc
  }, { running: 0, failed: 0, completed: 0 })
})

const embyServerOptions = computed(() => {
  const seen = new Map()
  taskList.value.forEach(task => {
    const id = task.metadata?.server_id
    if (id !== undefined && id !== null) seen.set(Number(id), task.metadata?.server_name || `实例 ${id}`)
  })
  return [...seen.entries()].map(([value, label]) => ({ value, label }))
})
const embyTaskTypeOptions = [
  { label: 'Emby 实例管理', value: 'emby_server' },
  { label: 'Emby 库刷新', value: 'emby_refresh' },
  { label: 'Emby 用户管理', value: 'emby_user' },
  { label: 'Emby 媒体库管理', value: 'emby_library' },
  { label: 'Emby 媒体库封面', value: 'emby_cover' },
  { label: '神医助手任务', value: 'emby_plugin' }
]
const statusOptions = [
  { label: '待执行/待确认', value: 'pending' }, { label: '执行中', value: 'running' },
  { label: '成功', value: 'success' }, { label: '部分成功', value: 'partial_success' },
  { label: '失败', value: 'failed' }, { label: '已取消', value: 'cancelled' }, { label: '结果未知', value: 'unknown' }
]
const originOptions = [{ label: '页面手动', value: 'manual' }, { label: '整理联动', value: 'organize_linkage' }, { label: '系统自动', value: 'automatic' }]
const filteredTaskList = computed(() => taskList.value.filter(task => {
  const filter = filters.value
  if (filter.serverId && Number(task.metadata?.server_id) !== Number(filter.serverId)) return false
  if (filter.taskType && task.task_type !== filter.taskType) return false
  if (filter.status && task.status !== filter.status) return false
  if (filter.origin && task.metadata?.origin !== filter.origin) return false
  if (Array.isArray(filter.timeRange) && filter.timeRange.length === 2) {
    const timestamp = Date.parse(String(task.create_time || '').replace(' ', 'T'))
    if (Number.isFinite(timestamp) && (timestamp < filter.timeRange[0] || timestamp > filter.timeRange[1])) return false
  }
  return true
}))

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
        formatted = ({ strm_scan_capture: '扫描 STRM 并生成视频封面', media_info: '媒体信息提取', subtitle_scan: '外挂字幕扫描', metadata_refresh: '元数据刷新' })[value] || value
      } else if (key === 'watch_interval' && Number.isFinite(Number(value))) {
        formatted = `${value} 秒`
      } else if (key === 'errors' && Array.isArray(value)) {
        formatted = value.join('\n') || '无'
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

const stopAutoRefresh = () => {
  if (timer) {
    clearInterval(timer)
    timer = null
  }
}

const stopTaskDetailRefresh = () => {
  if (taskDetailTimer) {
    clearInterval(taskDetailTimer)
    taskDetailTimer = null
  }
}

const startAutoRefresh = () => {
  stopAutoRefresh()
  timer = setInterval(() => {
    loadTasks()
  }, 3000)
}

const startTaskDetailRefresh = () => {
  stopTaskDetailRefresh()
  taskDetailTimer = setInterval(() => {
    if (!taskDetailVisible.value || !activeTaskId.value) return
    loadTaskDetail(activeTaskId.value, false)
  }, 2000)
}

const syncTaskInList = (taskDetail) => {
  if (!taskDetail?.task_id) return
  const index = taskList.value.findIndex(item => item.task_id === taskDetail.task_id)
  if (index === -1) return
  taskList.value[index] = {
    ...taskList.value[index],
    ...taskDetail
  }
}

const loadTasks = async () => {
  if (taskLoading.value) return
  taskLoading.value = true
  try {
    const response = await getUnifiedTaskList()
    const payload = response.data?.data?.data || response.data?.data || response.data || []
    taskList.value = Array.isArray(payload) ? payload : []
  } catch (error) {
    message.error('加载任务列表失败')
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
    taskDetailData.value = payload
    syncTaskInList(payload)
    const status = payload?.status
    if (taskDetailVisible.value && (status === 'pending' || status === 'running')) {
      startTaskDetailRefresh()
    } else {
      stopTaskDetailRefresh()
    }
  } catch (error) {
    taskDetailError.value = error.response?.data?.error || error.message || '加载任务详情失败'
    stopTaskDetailRefresh()
  } finally {
    if (showLoading) {
      taskDetailLoading.value = false
    }
  }
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

watch(autoRefresh, (value) => {
  if (value) startAutoRefresh()
  else stopAutoRefresh()
})

watch(taskDetailVisible, (value) => {
  if (!value) {
    stopTaskDetailRefresh()
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
  stopAutoRefresh()
  stopTaskDetailRefresh()
})

onMounted(async () => {
  if (autoRefresh.value) startAutoRefresh()
  if (route.query.emby_server_id) filters.value.serverId = Number(route.query.emby_server_id)
  await loadTasks()
  await openRouteTask()
})
</script>
