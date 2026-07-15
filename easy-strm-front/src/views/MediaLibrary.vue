<template>
  <div class="space-y-4">
    <!-- 页头 -->
    <div class="rounded-2xl border border-slate-200 bg-white p-5 shadow-sm dark:border-white/5 dark:bg-ink-900 lg:p-6">
      <div class="flex flex-wrap items-center justify-between gap-4">
        <div>
          <div class="text-xs font-bold uppercase tracking-[0.16em] text-cyan-600 dark:text-cyan-400">Asset Ledger</div>
          <h2 class="mt-1.5 text-2xl font-extrabold text-slate-800 dark:text-white">媒体资产台账</h2>
          <p class="mt-1 text-xs text-slate-400 dark:text-slate-500">
            从同步索引追踪源文件、识别状态、STRM、元数据、最近任务和资源健康度。
          </p>
        </div>
        <div class="flex flex-wrap items-center gap-2">
          <n-button :disabled="!selectedSourceId" @click="goSync">同步入库</n-button>
          <n-button @click="goPending">待处理</n-button>
          <n-button type="primary" :loading="loading" @click="loadItems">
            <template #icon>
              <n-icon :component="RefreshOutline" />
            </template>
            刷新台账
          </n-button>
        </div>
      </div>
    </div>

    <!-- 统计卡 -->
    <div class="grid grid-cols-2 gap-3 lg:grid-cols-4 lg:gap-4">
      <StatCard
        v-for="card in summaryCards"
        :key="card.label"
        :label="card.label"
        :value="card.value"
        :hint="card.hint"
        :icon="card.icon"
        :tone="card.tone"
      />
    </div>

    <!-- 筛选区 -->
    <PageCard>
      <div class="flex flex-wrap items-center gap-3">
        <n-select
          v-model:value="selectedSourceId"
          class="w-full sm:w-56"
          placeholder="选择媒体源"
          filterable
          :options="sourceOptions"
          @update:value="handleSourceChange"
        />
        <n-select
          v-model:value="status"
          class="w-full sm:w-44"
          placeholder="同步状态"
          clearable
          :options="statusOptions"
          @update:value="loadItems"
        />
        <n-select
          v-model:value="healthFilter"
          class="w-full sm:w-44"
          placeholder="健康状态"
          clearable
          :options="healthOptions"
        />
      </div>
    </PageCard>

    <!-- 最近操作提示 -->
    <n-alert v-if="lastAction.message" type="success" :title="lastAction.message" class="rounded-2xl">
      <div class="flex flex-wrap items-center gap-3">
        <span v-if="lastAction.taskId" class="text-xs">最近任务：{{ lastAction.taskId }}</span>
        <n-button v-if="canOpenTask(lastAction.taskId)" size="small" text type="primary" @click="goTask(lastAction.taskId)">
          查看任务
        </n-button>
      </div>
    </n-alert>

    <!-- 台账表格 -->
    <PageCard>
      <div class="overflow-x-auto">
        <n-data-table
          :columns="columns"
          :data="filteredItems"
          :loading="loading"
          :row-key="(row) => row.id"
          :scroll-x="1660"
          size="small"
        >
          <template #empty>
            <EmptyState title="暂无媒体资产" description="请先执行同步入库" />
          </template>
        </n-data-table>
      </div>
    </PageCard>
  </div>
</template>

<script setup>
import { computed, h, onMounted, reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { NAlert, NButton, NDataTable, NIcon, NSelect, NTag, useMessage } from 'naive-ui'
import {
  AlbumsOutline,
  AlertCircleOutline,
  CheckmarkCircleOutline,
  DocumentTextOutline,
  RefreshOutline
} from '@vicons/ionicons5'
import PageCard from '../components/common/PageCard.vue'
import StatCard from '../components/common/StatCard.vue'
import EmptyState from '../components/common/EmptyState.vue'
import {
  generateMediaLibraryItemStrm,
  getMediaLibraryItems,
  getMediaSources,
  refreshMediaLibraryItemServer,
  runMediaLibraryItemPipeline
} from '../utils/api/media'

const route = useRoute()
const router = useRouter()
const message = useMessage()

const loading = ref(false)
const sources = ref([])
const selectedSourceId = ref(null)
const status = ref(null)
const healthFilter = ref(null)
const items = ref([])
const rowLoadingId = ref(null)
const lastAction = reactive({
  message: '',
  taskId: ''
})

const sourceOptions = computed(() =>
  sources.value.map(source => ({ label: source.name, value: source.id }))
)

const statusOptions = [
  { label: '有效', value: 'active' },
  { label: '源端缺失', value: 'missing' },
  { label: '已删除', value: 'deleted' }
]

const healthOptions = [
  { label: '正常', value: 'ok' },
  { label: '待识别', value: 'identify_failed' },
  { label: '缺少 STRM', value: 'strm_missing' },
  { label: '源端异常', value: 'missing' }
]

const filteredItems = computed(() => {
  if (!healthFilter.value) return items.value
  return items.value.filter(item => item.health_status === healthFilter.value)
})

const summaryCards = computed(() => {
  const total = items.value.length
  const ok = items.value.filter(item => item.health_status === 'ok').length
  const pending = items.value.filter(item => item.health_status === 'identify_failed').length
  const strmMissing = items.value.filter(item => item.health_status === 'strm_missing').length
  return [
    { label: '资产总数', value: total, hint: '当前媒体源同步索引条目', icon: AlbumsOutline, tone: 'cyan' },
    { label: '健康资源', value: ok, hint: '识别和关键资产状态正常', icon: CheckmarkCircleOutline, tone: 'green' },
    { label: '待处理', value: pending, hint: '识别失败或需要人工修正', icon: AlertCircleOutline, tone: 'amber' },
    { label: '缺少 STRM', value: strmMissing, hint: '云盘资源需补齐播放入口', icon: DocumentTextOutline, tone: 'violet' }
  ]
})

const healthLabel = (value) => ({
  ok: '正常',
  identify_failed: '待识别',
  strm_missing: '缺 STRM',
  missing: '源端异常'
})[value] || value || '-'

const healthTagType = (value) => ({
  ok: 'success',
  identify_failed: 'warning',
  strm_missing: 'warning',
  missing: 'error'
})[value] || 'info'

const identityLabel = (value) => ({
  identified: '已识别',
  failed: '失败',
  pending: '待识别',
  unknown: '未知'
})[value] || value || '未知'

const identityTagType = (value) => value === 'identified' ? 'success' : value === 'failed' ? 'warning' : 'info'

const syncLabel = (value) => ({
  active: '有效',
  missing: '源端缺失',
  deleted: '已删除'
})[value] || value || '-'

const syncTagType = (value) => value === 'active' ? 'success' : value === 'missing' ? 'warning' : value === 'deleted' ? 'error' : 'info'

const renderTag = (type, label) => h(NTag, { type, size: 'small' }, { default: () => label })

const ellipsisCell = (text) => h('span', { class: 'block truncate', title: text || '' }, text || '-')

const columns = [
  {
    title: '资源条目',
    key: 'source_name',
    minWidth: 220
  },
  {
    title: '健康状态',
    key: 'health_status',
    width: 120,
    render: (row) => renderTag(healthTagType(row.health_status), healthLabel(row.health_status))
  },
  {
    title: '识别',
    key: 'identity_status',
    width: 110,
    render: (row) => renderTag(identityTagType(row.identity_status), identityLabel(row.identity_status))
  },
  {
    title: '同步',
    key: 'sync_status',
    width: 110,
    render: (row) => renderTag(syncTagType(row.sync_status), syncLabel(row.sync_status))
  },
  {
    title: 'STRM',
    key: 'has_strm',
    width: 90,
    render: (row) => renderTag(row.has_strm ? 'success' : 'info', row.has_strm ? '有' : '无')
  },
  {
    title: '元数据',
    key: 'has_metadata',
    width: 100,
    render: (row) => renderTag(row.has_metadata ? 'success' : 'info', row.has_metadata ? '有' : '缺失')
  },
  {
    title: '源路径',
    key: 'source_path',
    minWidth: 260,
    ellipsis: { tooltip: true }
  },
  {
    title: '目标路径',
    key: 'target_path',
    minWidth: 240,
    ellipsis: { tooltip: true }
  },
  {
    title: '最近任务',
    key: 'latest_task_id',
    minWidth: 180,
    render: (row) => {
      if (canOpenTask(row.latest_task_id)) {
        return h(
          'button',
          {
            type: 'button',
            class: 'max-w-full truncate text-left text-cyan-600 hover:underline dark:text-cyan-400',
            onClick: () => goTask(row.latest_task_id)
          },
          row.latest_task_id
        )
      }
      return ellipsisCell(row.latest_task_id)
    }
  },
  {
    title: '操作',
    key: 'actions',
    width: 240,
    fixed: 'right',
    render: (row) => h('div', { class: 'flex items-center gap-2' }, [
      h(NButton, {
        size: 'small',
        loading: rowLoadingId.value === row.id,
        onClick: () => runPipeline(row)
      }, { default: () => '入库' }),
      h(NButton, {
        size: 'small',
        loading: rowLoadingId.value === row.id,
        onClick: () => generateStrm(row)
      }, { default: () => 'STRM' }),
      h(NButton, {
        size: 'small',
        type: 'primary',
        loading: rowLoadingId.value === row.id,
        onClick: () => refreshServer(row)
      }, { default: () => '刷新库' })
    ])
  }
]

const readPayload = (response) => response?.data?.data || response?.data || {}

const loadSources = async () => {
  const response = await getMediaSources()
  const payload = response.data?.data?.data || response.data?.data || []
  sources.value = Array.isArray(payload) ? payload : []
  const querySourceId = Number(route.query.source_id || 0)
  const queryMatched = sources.value.find(source => source.id === querySourceId)
  if (queryMatched) {
    selectedSourceId.value = queryMatched.id
  } else if (!selectedSourceId.value && sources.value.length > 0) {
    selectedSourceId.value = sources.value[0].id
  }
}

const loadItems = async () => {
  if (!selectedSourceId.value) {
    items.value = []
    return
  }
  loading.value = true
  try {
    const response = await getMediaLibraryItems({
      source_id: selectedSourceId.value,
      status: status.value || ''
    })
    const payload = response.data?.data?.data || response.data?.data || []
    items.value = Array.isArray(payload) ? payload : []
  } finally {
    loading.value = false
  }
}

const handleSourceChange = async () => {
  if (selectedSourceId.value) {
    router.replace({ path: route.path, query: { source_id: selectedSourceId.value } })
  }
  await loadItems()
}

const runRowAction = async (row, action, successMessage) => {
  rowLoadingId.value = row.id
  try {
    const response = await action(row.id)
    const payload = readPayload(response)
    const taskId = payload.task_id || payload.task?.task_id || payload.last_task_id || payload.latest_task_id || ''
    lastAction.message = taskId ? `${successMessage}，任务 ${taskId}` : successMessage
    lastAction.taskId = taskId
    message.success(successMessage)
    await loadItems()
  } finally {
    rowLoadingId.value = null
  }
}

const runPipeline = (row) => runRowAction(row, runMediaLibraryItemPipeline, '已执行入库流水线')

const generateStrm = (row) => runRowAction(row, generateMediaLibraryItemStrm, 'STRM 已生成并更新台账')

const refreshServer = (row) => runRowAction(row, refreshMediaLibraryItemServer, '媒体服务器刷新请求已发送')

const goSync = () => {
  router.push({ path: '/dashboard/sync-tasks', query: selectedSourceId.value ? { source_id: selectedSourceId.value } : {} })
}

const goPending = () => {
  router.push({ path: '/dashboard/pending-media', query: selectedSourceId.value ? { source_id: selectedSourceId.value } : {} })
}

const goTask = (taskId) => {
  router.push({ path: '/dashboard/tasks', query: { task_id: taskId } })
}

const canOpenTask = (taskId) => {
  return Boolean(taskId && !String(taskId).startsWith('manual_'))
}

onMounted(async () => {
  await loadSources()
  await loadItems()
})
</script>
