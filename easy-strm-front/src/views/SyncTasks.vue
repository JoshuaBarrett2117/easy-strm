<template>
  <div class="space-y-4">
    <!-- 页头 -->
    <div class="rounded-2xl border border-slate-200 bg-white p-5 shadow-sm dark:border-white/5 dark:bg-ink-900 lg:p-6">
      <div class="flex flex-wrap items-center justify-between gap-4">
        <div>
          <div class="text-xs font-bold uppercase tracking-[0.16em] text-cyan-600 dark:text-cyan-400">Sync &amp; Ingest</div>
          <h2 class="mt-1.5 text-2xl font-extrabold text-slate-800 dark:text-white">同步入库工作台</h2>
          <p class="mt-1 text-xs text-slate-400 dark:text-slate-500">
            对本地和 115 媒体源执行同步索引，随后触发入库、STRM 与媒体服务器刷新。
          </p>
        </div>
        <div class="flex flex-wrap items-center gap-2">
          <n-button :disabled="!selectedSource" @click="goLibrary">查看资产台账</n-button>
          <n-button :loading="loading" @click="loadSources">
            <template #icon>
              <n-icon :component="RefreshOutline" />
            </template>
            刷新媒体源
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

    <!-- 主体:媒体源列表 + 同步面板 -->
    <div class="grid grid-cols-1 items-start gap-4 lg:grid-cols-[300px_minmax(0,1fr)]">
      <!-- 媒体源列表 -->
      <PageCard title="媒体源">
        <div v-if="sources.length" class="flex flex-col gap-3">
          <button
            v-for="source in sources"
            :key="source.id"
            type="button"
            class="flex flex-col gap-1 rounded-xl border p-3 text-left transition-colors"
            :class="selectedSource?.id === source.id
              ? 'border-cyan-500/60 bg-cyan-500/10'
              : 'border-slate-200 bg-white hover:border-cyan-500/40 dark:border-white/10 dark:bg-ink-800'"
            @click="selectSource(source)"
          >
            <strong class="text-sm font-bold text-slate-800 dark:text-white">{{ source.name }}</strong>
            <span class="text-xs text-slate-500 dark:text-slate-400">
              {{ source.source_type === 'cloud115' ? '115 云盘' : '本地目录' }}
            </span>
            <small class="truncate text-xs text-slate-400 dark:text-slate-500">{{ source.path || '-' }}</small>
          </button>
        </div>
        <EmptyState v-else title="暂无媒体源" />
      </PageCard>

      <!-- 同步面板 -->
      <PageCard>
        <div class="mb-4 flex flex-wrap items-center justify-between gap-4">
          <div>
            <div class="text-base font-bold text-slate-800 dark:text-white">
              {{ selectedSource?.name || '请选择媒体源' }}
            </div>
            <p v-if="selectedSource" class="mt-0.5 text-xs text-slate-400 dark:text-slate-500">
              路径：{{ selectedSource.path || '-' }}
            </p>
          </div>
          <div class="flex flex-wrap items-center gap-2">
            <n-button :disabled="!selectedSource" :loading="syncing" @click="runSync('full')">全量同步</n-button>
            <n-button :disabled="!selectedSource" :loading="syncing" type="primary" @click="runSync('incremental')">增量同步</n-button>
            <n-button :disabled="!selectedSource" :loading="syncing" type="success" ghost @click="runPipeline">执行入库</n-button>
          </div>
        </div>

        <n-alert v-if="lastResult" type="success" :title="formatLastResult(lastResult)" class="mb-4 rounded-2xl">
          <div class="flex flex-wrap items-center gap-3">
            <n-button v-if="lastResult.task_id" size="small" text type="primary" @click="goTask(lastResult.task_id)">
              查看任务
            </n-button>
            <n-button size="small" text type="primary" @click="goLibrary">查看资产台账</n-button>
            <n-button size="small" text type="primary" @click="goPending">查看待处理</n-button>
          </div>
        </n-alert>

        <div class="overflow-x-auto">
          <n-data-table
            :columns="columns"
            :data="indexRows"
            :loading="indexLoading"
            :row-key="(row) => row.id ?? row.source_path"
            :scroll-x="1240"
            size="small"
          >
            <template #empty>
              <EmptyState title="暂无索引条目" description="执行全量或增量同步后将在此展示" />
            </template>
          </n-data-table>
        </div>
      </PageCard>
    </div>
  </div>
</template>

<script setup>
import { computed, h, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { NAlert, NButton, NDataTable, NIcon, NTag, useMessage } from 'naive-ui'
import {
  AlertCircleOutline,
  CheckmarkCircleOutline,
  LayersOutline,
  PulseOutline,
  RefreshOutline
} from '@vicons/ionicons5'
import PageCard from '../components/common/PageCard.vue'
import StatCard from '../components/common/StatCard.vue'
import EmptyState from '../components/common/EmptyState.vue'
import { getMediaSources, getMediaSyncIndex, runFullMediaSync, runIncrementalMediaSync, runMediaLibraryPipeline } from '../utils/api/media'

const route = useRoute()
const router = useRouter()
const message = useMessage()

const loading = ref(false)
const syncing = ref(false)
const indexLoading = ref(false)
const sources = ref([])
const selectedSource = ref(null)
const indexRows = ref([])
const lastResult = ref(null)

const summaryCards = computed(() => {
  const active = indexRows.value.filter(item => item.sync_status === 'active').length
  const failed = indexRows.value.filter(item => item.identity_status === 'failed').length
  const identified = indexRows.value.filter(item => item.identity_status === 'identified').length
  return [
    { label: '索引条目', value: indexRows.value.length, hint: '当前媒体源已记录资源', icon: LayersOutline, tone: 'cyan' },
    { label: '有效资源', value: active, hint: '源端仍存在的同步资产', icon: PulseOutline, tone: 'green' },
    { label: '已识别', value: identified, hint: '可进入后续入库动作', icon: CheckmarkCircleOutline, tone: 'violet' },
    { label: '待修正', value: failed, hint: '会进入待处理页面承接', icon: AlertCircleOutline, tone: 'amber' }
  ]
})

const syncLabel = (value) => ({
  active: '有效',
  missing: '源端缺失',
  deleted: '已删除'
})[value] || value || '-'

const syncTagType = (value) => value === 'active' ? 'success' : value === 'missing' ? 'warning' : value === 'deleted' ? 'error' : 'info'

const identityLabel = (value) => ({
  identified: '已识别',
  failed: '失败',
  pending: '待识别'
})[value] || value || '未知'

const identityTagType = (value) => value === 'identified' ? 'success' : value === 'failed' ? 'warning' : 'info'

const columns = [
  {
    title: '文件名',
    key: 'source_name',
    minWidth: 220
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
    title: '同步状态',
    key: 'sync_status',
    width: 110,
    render: (row) => h(NTag, { type: syncTagType(row.sync_status), size: 'small' }, { default: () => syncLabel(row.sync_status) })
  },
  {
    title: '识别状态',
    key: 'identity_status',
    width: 110,
    render: (row) => h(NTag, { type: identityTagType(row.identity_status), size: 'small' }, { default: () => identityLabel(row.identity_status) })
  },
  {
    title: '变更',
    key: 'last_change_type',
    width: 120
  },
  {
    title: '最近任务',
    key: 'last_task_id',
    minWidth: 180,
    render: (row) => {
      if (row.last_task_id) {
        return h(
          'button',
          {
            type: 'button',
            class: 'max-w-full truncate text-left text-cyan-600 hover:underline dark:text-cyan-400',
            onClick: () => goTask(row.last_task_id)
          },
          row.last_task_id
        )
      }
      return '-'
    }
  }
]

const loadSources = async () => {
  loading.value = true
  try {
    const response = await getMediaSources()
    const payload = response.data?.data?.data || response.data?.data || []
    sources.value = Array.isArray(payload) ? payload : []
    const querySourceId = Number(route.query.source_id || 0)
    const queryMatched = sources.value.find(source => source.id === querySourceId)
    if (queryMatched) {
      await selectSource(queryMatched)
    } else if (!selectedSource.value && sources.value.length > 0) {
      await selectSource(sources.value[0])
    }
  } finally {
    loading.value = false
  }
}

const selectSource = async (source) => {
  selectedSource.value = source
  lastResult.value = null
  router.replace({ path: route.path, query: { source_id: source.id } })
  await loadIndex()
}

const loadIndex = async () => {
  if (!selectedSource.value) return
  indexLoading.value = true
  try {
    const response = await getMediaSyncIndex(selectedSource.value.id)
    const payload = response.data?.data?.data || response.data?.data || []
    indexRows.value = Array.isArray(payload) ? payload : []
  } finally {
    indexLoading.value = false
  }
}

const runSync = async (mode) => {
  if (!selectedSource.value) return
  syncing.value = true
  try {
    const response = mode === 'full'
      ? await runFullMediaSync(selectedSource.value.id)
      : await runIncrementalMediaSync(selectedSource.value.id)
    lastResult.value = response.data?.data || response.data
    message.success(mode === 'full' ? '全量同步完成' : '增量同步完成')
    await loadIndex()
  } finally {
    syncing.value = false
  }
}

const formatLastResult = (result) => {
  if (!result) return ''
  if (result.scanned !== undefined) {
    return `任务 ${result.task_id} 已完成：扫描 ${result.scanned}，写入 ${result.changed}，失效 ${result.missing}`
  }
  return `任务 ${result.task_id} 已完成：总数 ${result.total}，识别 ${result.identified}，待处理 ${result.pending}，STRM ${result.strm}`
}

const runPipeline = async () => {
  if (!selectedSource.value) return
  syncing.value = true
  try {
    const response = await runMediaLibraryPipeline(selectedSource.value.id)
    lastResult.value = response.data?.data || response.data
    message.success('入库流水线已完成')
    await loadIndex()
  } finally {
    syncing.value = false
  }
}

const goLibrary = () => {
  router.push({ path: '/dashboard/media-library', query: selectedSource.value ? { source_id: selectedSource.value.id } : {} })
}

const goPending = () => {
  router.push({ path: '/dashboard/pending-media', query: selectedSource.value ? { source_id: selectedSource.value.id } : {} })
}

const goTask = (taskId) => {
  router.push({ path: '/dashboard/tasks', query: { task_id: taskId } })
}

onMounted(loadSources)
</script>
