<template>
  <div class="space-y-4">
    <!-- 页头 -->
    <div class="rounded-2xl border border-slate-200 bg-white p-5 shadow-sm dark:border-white/5 dark:bg-ink-900 lg:p-6">
      <div class="flex flex-wrap items-center justify-between gap-4">
        <div>
          <div class="text-xs font-bold uppercase tracking-[0.16em] text-cyan-600 dark:text-cyan-400">Pending Queue</div>
          <h2 class="mt-1.5 text-2xl font-extrabold text-slate-800 dark:text-white">待处理资源</h2>
          <p class="mt-1 text-xs text-slate-400 dark:text-slate-500">
            集中承接识别失败项，完成 TMDB 修正、重新入库、忽略和任务回溯。
          </p>
        </div>
        <div class="flex flex-wrap items-center gap-2">
          <n-button :disabled="!filters.source_id" @click="goLibrary">查看资产台账</n-button>
          <n-button :loading="loading" @click="loadItems">
            <template #icon>
              <n-icon :component="RefreshOutline" />
            </template>
            刷新
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
          v-model:value="filters.source_id"
          class="w-full sm:w-56"
          placeholder="媒体源"
          clearable
          filterable
          :options="sourceOptions"
          @update:value="handleSourceChange"
        />
        <n-select
          v-model:value="filters.status"
          class="w-full sm:w-44"
          placeholder="状态"
          clearable
          :options="statusOptions"
          @update:value="loadItems"
        />
        <n-select
          v-model:value="filters.media_type"
          class="w-full sm:w-44"
          placeholder="媒体类型"
          clearable
          :options="mediaTypeOptions"
          @update:value="loadItems"
        />
      </div>
    </PageCard>

    <!-- 最近操作提示 -->
    <n-alert v-if="lastAction.message" type="success" :title="lastAction.message" class="rounded-2xl">
      <div class="flex flex-wrap items-center gap-3">
        <n-button v-if="lastAction.taskId" size="small" text type="primary" @click="goTask(lastAction.taskId)">
          查看任务
        </n-button>
        <n-button size="small" text type="primary" @click="goLibrary">回到资产台账</n-button>
      </div>
    </n-alert>

    <!-- 列表 -->
    <PageCard>
      <div class="overflow-x-auto">
        <n-data-table
          :columns="columns"
          :data="items"
          :loading="loading"
          :row-key="(row) => row.id"
          :scroll-x="1320"
          size="small"
        >
          <template #empty>
            <EmptyState title="暂无待处理项" />
          </template>
        </n-data-table>
      </div>
    </PageCard>

    <!-- 修正识别对话框 -->
    <n-modal v-model:show="identifyVisible" preset="card" title="修正识别" class="w-[92vw] max-w-md">
      <n-form label-placement="left" label-width="90">
        <n-form-item label="标题">
          <n-input v-model:value="identifyForm.title" />
        </n-form-item>
        <n-form-item label="TMDB ID">
          <n-input-number v-model:value="identifyForm.tmdb_id" :min="0" class="w-full" />
        </n-form-item>
        <n-form-item label="媒体类型">
          <n-select v-model:value="identifyForm.media_type" :options="mediaTypeOptions" />
        </n-form-item>
        <n-form-item label="年份">
          <n-input-number v-model:value="identifyForm.year" :min="0" class="w-full" />
        </n-form-item>
      </n-form>
      <template #footer>
        <div class="flex flex-wrap justify-end gap-2">
          <n-button @click="identifyVisible = false">取消</n-button>
          <n-button type="primary" @click="saveIdentify">保存</n-button>
          <n-button type="success" :loading="rowLoadingId === activeItem?.id" @click="saveIdentifyAndRun">
            保存并入库
          </n-button>
        </div>
      </template>
    </n-modal>
  </div>
</template>

<script setup>
import { computed, h, onMounted, reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import {
  NAlert,
  NButton,
  NDataTable,
  NForm,
  NFormItem,
  NIcon,
  NInput,
  NInputNumber,
  NModal,
  NSelect,
  NTag,
  useMessage
} from 'naive-ui'
import {
  AlertCircleOutline,
  CheckmarkCircleOutline,
  CloseCircleOutline,
  FileTrayFullOutline,
  RefreshOutline
} from '@vicons/ionicons5'
import PageCard from '../components/common/PageCard.vue'
import StatCard from '../components/common/StatCard.vue'
import EmptyState from '../components/common/EmptyState.vue'
import { getMediaSources, getPendingMediaItems, identifyPendingMediaItem, ignorePendingMediaItem, runPendingMediaItem } from '../utils/api/media'

const route = useRoute()
const router = useRouter()
const message = useMessage()

const loading = ref(false)
const sources = ref([])
const items = ref([])
const identifyVisible = ref(false)
const activeItem = ref(null)
const rowLoadingId = ref(null)
const filters = reactive({
  source_id: null,
  status: null,
  media_type: null
})
const identifyForm = reactive({
  title: '',
  tmdb_id: 0,
  media_type: 'movie',
  year: 0,
  season: 0,
  episode: 0
})
const lastAction = reactive({
  message: '',
  taskId: ''
})

const sourceOptions = computed(() =>
  sources.value.map(source => ({ label: source.name, value: source.id }))
)

const statusOptions = [
  { label: '待处理', value: 'pending' },
  { label: '已识别', value: 'identified' },
  { label: '执行中', value: 'running' },
  { label: '已完成', value: 'completed' },
  { label: '已忽略', value: 'ignored' },
  { label: '失败', value: 'failed' }
]

const mediaTypeOptions = [
  { label: '电影', value: 'movie' },
  { label: '剧集', value: 'tv' },
  { label: '动画', value: 'anime' }
]

const summaryCards = computed(() => {
  const total = items.value.length
  const pending = items.value.filter(item => item.status === 'pending').length
  const identified = items.value.filter(item => item.status === 'identified').length
  const failed = items.value.filter(item => item.status === 'failed').length
  return [
    { label: '待处理总数', value: total, hint: '当前筛选范围内的失败资源', icon: FileTrayFullOutline, tone: 'cyan' },
    { label: '待修正', value: pending, hint: '需要补充识别信息', icon: AlertCircleOutline, tone: 'amber' },
    { label: '已识别', value: identified, hint: '可重新进入入库流水线', icon: CheckmarkCircleOutline, tone: 'green' },
    { label: '入库失败', value: failed, hint: '需查看任务原因后重试', icon: CloseCircleOutline, tone: 'red' }
  ]
})

const mediaTypeLabel = (value) => ({ movie: '电影', tv: '剧集', anime: '动画' })[value] || value || '-'

const statusLabel = (value) => ({
  pending: '待处理',
  identified: '已识别',
  running: '执行中',
  completed: '已完成',
  ignored: '已忽略',
  failed: '失败'
})[value] || value || '-'

const statusTagType = (value) => {
  if (value === 'completed') return 'success'
  if (value === 'failed') return 'error'
  if (value === 'identified' || value === 'running') return 'warning'
  if (value === 'ignored') return 'info'
  return 'primary'
}

const columns = [
  {
    title: '标题',
    key: 'title',
    minWidth: 180
  },
  {
    title: '来源路径',
    key: 'source_path',
    minWidth: 260,
    ellipsis: { tooltip: true }
  },
  {
    title: '类型',
    key: 'media_type',
    width: 100,
    render: (row) => mediaTypeLabel(row.media_type)
  },
  {
    title: '年份',
    key: 'year',
    width: 90
  },
  {
    title: '状态',
    key: 'status',
    width: 110,
    render: (row) => h(NTag, { type: statusTagType(row.status), size: 'small' }, { default: () => statusLabel(row.status) })
  },
  {
    title: '原因',
    key: 'reason',
    minWidth: 180,
    ellipsis: { tooltip: true }
  },
  {
    title: '关联任务',
    key: 'related_task_id',
    minWidth: 180,
    render: (row) => {
      if (row.related_task_id) {
        return h(
          'button',
          {
            type: 'button',
            class: 'max-w-full truncate text-left text-cyan-600 hover:underline dark:text-cyan-400',
            onClick: () => goTask(row.related_task_id)
          },
          row.related_task_id
        )
      }
      return '-'
    }
  },
  {
    title: '操作',
    key: 'actions',
    width: 200,
    fixed: 'right',
    render: (row) => h('div', { class: 'flex items-center gap-2' }, [
      h(NButton, { size: 'small', onClick: () => openIdentify(row) }, { default: () => '修正' }),
      h(NButton, {
        size: 'small',
        type: 'primary',
        loading: rowLoadingId.value === row.id,
        onClick: () => runItem(row)
      }, { default: () => '入库' }),
      h(NButton, { size: 'small', onClick: () => ignoreItem(row) }, { default: () => '忽略' })
    ])
  }
]

const loadSources = async () => {
  const response = await getMediaSources()
  const payload = response.data?.data?.data || response.data?.data || []
  sources.value = Array.isArray(payload) ? payload : []
  const querySourceId = Number(route.query.source_id || 0)
  if (querySourceId > 0) {
    filters.source_id = querySourceId
  }
}

const loadItems = async () => {
  loading.value = true
  try {
    const response = await getPendingMediaItems({
      status: filters.status || '',
      media_type: filters.media_type || '',
      source_id: filters.source_id
    })
    const payload = response.data?.data?.data || response.data?.data || []
    items.value = Array.isArray(payload) ? payload : []
  } finally {
    loading.value = false
  }
}

const handleSourceChange = async () => {
  router.replace({ path: route.path, query: filters.source_id ? { source_id: filters.source_id } : {} })
  await loadItems()
}

const openIdentify = (row) => {
  activeItem.value = row
  identifyForm.title = row.title || ''
  identifyForm.tmdb_id = row.tmdb_id || 0
  identifyForm.media_type = row.media_type || 'movie'
  identifyForm.year = row.year || 0
  identifyForm.season = row.season || 0
  identifyForm.episode = row.episode || 0
  identifyVisible.value = true
}

const saveIdentify = async () => {
  if (!activeItem.value) return
  await identifyPendingMediaItem(activeItem.value.id, { ...identifyForm })
  message.success('识别信息已保存')
  identifyVisible.value = false
  await loadItems()
}

const saveIdentifyAndRun = async () => {
  if (!activeItem.value) return
  await identifyPendingMediaItem(activeItem.value.id, { ...identifyForm })
  identifyVisible.value = false
  await runItem(activeItem.value)
}

const runItem = async (row) => {
  rowLoadingId.value = row.id
  try {
    const response = await runPendingMediaItem(row.id)
    const payload = response.data?.data || response.data || {}
    const taskId = payload.task?.task_id || payload.data?.related_task_id || ''
    lastAction.message = taskId ? `已重新入库，任务 ${taskId}` : '已重新入库'
    lastAction.taskId = taskId
    message.success('已重新入库')
    await loadItems()
  } finally {
    rowLoadingId.value = null
  }
}

const ignoreItem = async (row) => {
  await ignorePendingMediaItem(row.id)
  lastAction.message = '已忽略该待处理项'
  lastAction.taskId = ''
  message.success('已忽略')
  await loadItems()
}

const goLibrary = () => {
  router.push({ path: '/dashboard/media-library', query: filters.source_id ? { source_id: filters.source_id } : {} })
}

const goTask = (taskId) => {
  router.push({ path: '/dashboard/tasks', query: { task_id: taskId } })
}

onMounted(async () => {
  await loadSources()
  await loadItems()
})
</script>
