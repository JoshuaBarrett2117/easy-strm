<template>
  <div class="space-y-4">
    <!-- Hero -->
    <section class="rounded-2xl border border-slate-200 bg-white p-5 shadow-sm dark:border-white/5 dark:bg-ink-900 lg:p-7">
      <div class="grid gap-6 lg:grid-cols-[1.5fr_1fr]">
        <div>
          <div class="text-xs font-bold uppercase tracking-[0.16em] text-cyan-600 dark:text-cyan-400">Resource Hub</div>
          <h2 class="mt-3 text-2xl font-extrabold leading-snug text-slate-800 dark:text-white lg:text-3xl">
            以媒体资产台账为中心，串起同步、入库、STRM 与任务追踪。
          </h2>
          <p class="mt-3 text-sm leading-relaxed text-slate-400 dark:text-slate-500">
            首版主链路固定为媒体源同步索引、媒体库台账、入库流水线、任务中心和待处理修正。
          </p>
        </div>

        <div class="flex flex-col justify-center gap-3">
          <router-link
            to="/dashboard/media-library"
            class="inline-flex min-h-12 items-center justify-center rounded-xl bg-gradient-to-r from-cyan-500 to-cyan-400 px-4 font-bold text-white shadow-sm transition-opacity hover:opacity-90"
          >
            打开资产台账
          </router-link>
          <router-link
            to="/dashboard/sync-tasks"
            class="inline-flex min-h-12 items-center justify-center rounded-xl bg-slate-100 px-4 font-bold text-slate-700 transition-colors hover:bg-slate-200 dark:bg-white/5 dark:text-slate-200 dark:hover:bg-white/10"
          >
            同步入库
          </router-link>
          <router-link
            to="/dashboard/pending-media"
            class="inline-flex min-h-12 items-center justify-center rounded-xl bg-slate-100 px-4 font-bold text-slate-700 transition-colors hover:bg-slate-200 dark:bg-white/5 dark:text-slate-200 dark:hover:bg-white/10"
          >
            处理失败项
          </router-link>
          <router-link
            to="/dashboard/tasks"
            class="inline-flex min-h-12 items-center justify-center rounded-xl bg-slate-100 px-4 font-bold text-slate-700 transition-colors hover:bg-slate-200 dark:bg-white/5 dark:text-slate-200 dark:hover:bg-white/10"
          >
            查看任务中心
          </router-link>
        </div>
      </div>
    </section>

    <!-- 工作流 -->
    <section class="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-4 lg:gap-4">
      <article
        v-for="step in workflowSteps"
        :key="step.title"
        class="rounded-2xl border border-slate-200 bg-white p-4 shadow-sm dark:border-white/5 dark:bg-ink-900 lg:p-5"
      >
        <div class="text-xs font-extrabold text-cyan-600 dark:text-cyan-400">{{ step.index }}</div>
        <div class="mt-2 text-lg font-bold text-slate-800 dark:text-white">{{ step.title }}</div>
        <div class="mt-1 text-xs leading-relaxed text-slate-400 dark:text-slate-500">{{ step.desc }}</div>
      </article>
    </section>

    <!-- 统计卡 -->
    <section class="grid grid-cols-2 gap-3 lg:grid-cols-4 lg:gap-4">
      <StatCard
        v-for="card in metricCards"
        :key="card.label"
        :label="card.label"
        :value="card.value"
        :hint="card.foot"
        :icon="card.icon"
        :tone="card.tone"
      />
    </section>

    <!-- 主区域 -->
    <section class="grid grid-cols-1 gap-4 lg:grid-cols-3">
      <!-- 资源整理任务 -->
      <PageCard title="资源整理任务" subtitle="Task Flow" class="lg:col-span-2">
        <template #action>
          <router-link to="/dashboard/tasks" class="text-xs font-bold text-cyan-600 hover:text-cyan-500 dark:text-cyan-400">
            查看全部
          </router-link>
        </template>

        <div class="mb-4 grid grid-cols-1 gap-3 sm:grid-cols-3">
          <div class="rounded-xl bg-cyan-500/10 p-4">
            <div class="text-xs text-slate-500 dark:text-slate-400">运行中</div>
            <div class="mt-2 text-2xl font-extrabold tabular-nums text-slate-800 dark:text-white">{{ stats.tasks.running || 0 }}</div>
            <div class="mt-1 text-xs text-slate-400 dark:text-slate-500">当前仍在执行的任务</div>
          </div>
          <div class="rounded-xl bg-emerald-500/10 p-4">
            <div class="text-xs text-slate-500 dark:text-slate-400">今日完成</div>
            <div class="mt-2 text-2xl font-extrabold tabular-nums text-slate-800 dark:text-white">{{ stats.tasks.completed_today || 0 }}</div>
            <div class="mt-1 text-xs text-slate-400 dark:text-slate-500">当天成功落地的任务</div>
          </div>
          <div class="rounded-xl bg-red-500/10 p-4">
            <div class="text-xs text-slate-500 dark:text-slate-400">今日失败</div>
            <div class="mt-2 text-2xl font-extrabold tabular-nums text-slate-800 dark:text-white">{{ stats.tasks.failed_today || 0 }}</div>
            <div class="mt-1 text-xs text-slate-400 dark:text-slate-500">需要进入任务中心排查</div>
          </div>
        </div>

        <div class="flex flex-col gap-2">
          <p v-if="recentTasks.length === 0" class="text-sm text-slate-400 dark:text-slate-500">暂无任务记录</p>
          <button
            v-for="task in recentTasks"
            :key="task.task_id"
            type="button"
            class="flex items-center justify-between gap-3 rounded-xl bg-slate-50 px-4 py-3 text-left transition-colors hover:bg-slate-100 dark:bg-white/5 dark:hover:bg-white/10"
            @click="router.push('/dashboard/tasks')"
          >
            <span class="truncate text-sm font-semibold text-slate-700 dark:text-slate-200">
              {{ task.task_name || task.task_type || '任务' }}
            </span>
            <span
              class="shrink-0 rounded-full px-2.5 py-1 text-xs font-bold"
              :class="taskStatusPillClass(task.status)"
            >
              {{ formatTaskStatus(task.status) }}
            </span>
          </button>
        </div>
      </PageCard>

      <!-- 同步入口 -->
      <PageCard title="同步入口" subtitle="Source">
        <template #action>
          <router-link to="/dashboard/sync-tasks" class="text-xs font-bold text-cyan-600 hover:text-cyan-500 dark:text-cyan-400">
            进入同步
          </router-link>
        </template>

        <div class="flex flex-col gap-3">
          <div class="flex items-center justify-between border-b border-dashed border-slate-200 pb-2.5 dark:border-white/10">
            <span class="text-sm text-slate-400 dark:text-slate-500">总媒体源</span>
            <strong class="text-lg tabular-nums text-slate-800 dark:text-white">{{ stats.media_sources.total || 0 }}</strong>
          </div>
          <div class="flex items-center justify-between border-b border-dashed border-slate-200 pb-2.5 dark:border-white/10">
            <span class="text-sm text-slate-400 dark:text-slate-500">本地</span>
            <strong class="text-lg tabular-nums text-slate-800 dark:text-white">{{ stats.media_sources.local || 0 }}</strong>
          </div>
          <div class="flex items-center justify-between border-b border-dashed border-slate-200 pb-2.5 dark:border-white/10">
            <span class="text-sm text-slate-400 dark:text-slate-500">115</span>
            <strong class="text-lg tabular-nums text-slate-800 dark:text-white">{{ stats.media_sources.cloud115 || 0 }}</strong>
          </div>
          <div class="flex items-center justify-between">
            <span class="text-sm text-slate-400 dark:text-slate-500">启用中</span>
            <strong class="text-lg tabular-nums text-slate-800 dark:text-white">{{ stats.media_sources.enabled || 0 }}</strong>
          </div>
        </div>
      </PageCard>

      <!-- 账号配额 -->
      <PageCard title="账号配额" subtitle="Quota">
        <template #action>
          <router-link to="/dashboard/cloud115" class="text-xs font-bold text-cyan-600 hover:text-cyan-500 dark:text-cyan-400">
            查看账号
          </router-link>
        </template>

        <p v-if="storageAccounts.length === 0" class="text-sm text-slate-400 dark:text-slate-500">暂无账号配额数据</p>
        <div v-else class="flex flex-col gap-4">
          <div v-for="account in storageAccounts" :key="account.name">
            <div class="mb-2 flex items-center justify-between gap-3">
              <span class="truncate text-sm text-slate-500 dark:text-slate-400">{{ account.name }}</span>
              <strong class="shrink-0 text-sm tabular-nums text-slate-800 dark:text-white">{{ formatBytes(account.used) }}</strong>
            </div>
            <div class="h-2.5 overflow-hidden rounded-full bg-slate-100 dark:bg-white/5">
              <span
                class="block h-full rounded-full bg-gradient-to-r from-cyan-500 to-cyan-300"
                :style="{ width: `${account.percentage || 12}%` }"
              ></span>
            </div>
          </div>
        </div>
      </PageCard>

      <!-- 资源监控 -->
      <PageCard title="资源监控" subtitle="Runtime">
        <template #action>
          <span class="text-xs text-slate-400 dark:text-slate-500">{{ formatDateTime(monitor.sampled_at) }}</span>
        </template>

        <div class="flex flex-col gap-3">
          <div class="flex items-center justify-between border-b border-dashed border-slate-200 pb-2.5 dark:border-white/10">
            <span class="text-sm text-slate-400 dark:text-slate-500">运行内存</span>
            <strong class="text-lg tabular-nums text-slate-800 dark:text-white">{{ formatBytes(monitor.memory_bytes) }}</strong>
          </div>
          <div class="flex items-center justify-between border-b border-dashed border-slate-200 pb-2.5 dark:border-white/10">
            <span class="text-sm text-slate-400 dark:text-slate-500">堆分配</span>
            <strong class="text-lg tabular-nums text-slate-800 dark:text-white">{{ formatBytes(monitor.heap_alloc_bytes) }}</strong>
          </div>
          <div class="flex items-center justify-between border-b border-dashed border-slate-200 pb-2.5 dark:border-white/10">
            <span class="text-sm text-slate-400 dark:text-slate-500">Goroutines</span>
            <strong class="text-lg tabular-nums text-slate-800 dark:text-white">{{ monitor.goroutines || 0 }}</strong>
          </div>
          <div class="flex items-center justify-between">
            <span class="text-sm text-slate-400 dark:text-slate-500">CPU 核心</span>
            <strong class="text-lg tabular-nums text-slate-800 dark:text-white">{{ monitor.cpu_cores || 0 }}</strong>
          </div>
        </div>
      </PageCard>

      <!-- 近 7 天整理趋势 -->
      <PageCard title="近 7 天整理趋势" subtitle="Trend" class="lg:col-span-2">
        <template #action>
          <div class="flex items-center gap-4 text-xs text-slate-400 dark:text-slate-500">
            <span class="flex items-center gap-1.5">
              <i class="inline-block h-2 w-2 rounded-full bg-cyan-500"></i>STRM
            </span>
            <span class="flex items-center gap-1.5">
              <i class="inline-block h-2 w-2 rounded-full bg-amber-400"></i>入库/整理
            </span>
          </div>
        </template>

        <div class="relative h-64 pt-3">
          <div class="absolute inset-x-0 top-0 bottom-7 grid grid-rows-4">
            <div v-for="row in 4" :key="row" class="border-t border-dashed border-slate-200 dark:border-white/10"></div>
          </div>
          <div class="relative z-10 grid h-full grid-cols-7 items-end gap-2 sm:gap-3">
            <div v-for="point in trendPoints" :key="point.date" class="flex h-full flex-col items-center gap-2">
              <div class="flex w-full flex-1 items-end justify-center gap-1.5 sm:gap-2">
                <span
                  class="w-3 rounded-t-full bg-gradient-to-b from-cyan-500 to-cyan-600 sm:w-4"
                  :style="{ height: `${point.strmHeight}%` }"
                ></span>
                <span
                  class="w-3 rounded-t-full bg-gradient-to-b from-amber-400 to-amber-500 sm:w-4"
                  :style="{ height: `${point.organizeHeight}%` }"
                ></span>
              </div>
              <div class="text-xs text-slate-400 dark:text-slate-500">{{ point.label }}</div>
            </div>
          </div>
        </div>
      </PageCard>

      <!-- 网络探针 -->
      <PageCard title="网络探针" subtitle="Probe">
        <template #action>
          <router-link to="/dashboard/network" class="text-xs font-bold text-cyan-600 hover:text-cyan-500 dark:text-cyan-400">
            详细探测
          </router-link>
        </template>

        <p v-if="probeLoading" class="text-sm text-slate-400 dark:text-slate-500">探针检测中...</p>
        <div v-else class="flex flex-col gap-2.5">
          <div
            v-for="probe in probes"
            :key="probe.url"
            class="flex items-center justify-between gap-3 rounded-xl bg-slate-50 px-4 py-3 dark:bg-white/5"
          >
            <div class="min-w-0">
              <div class="truncate text-sm font-bold text-slate-700 dark:text-slate-200">{{ probe.name }}</div>
              <div class="mt-0.5 truncate text-xs text-slate-400 dark:text-slate-500">{{ probe.url }}</div>
            </div>
            <span
              class="shrink-0 rounded-full px-2.5 py-1 text-xs font-bold"
              :class="probeStatusPillClass(probe.ok)"
            >
              {{ probe.ok === true ? '正常' : probe.ok === false ? '失败' : '待测' }}
            </span>
          </div>
        </div>
      </PageCard>

      <!-- 最近入库 -->
      <PageCard title="最近入库" subtitle="Recent Ingest" class="lg:col-span-2">
        <template #action>
          <router-link to="/dashboard/media-library" class="text-xs font-bold text-cyan-600 hover:text-cyan-500 dark:text-cyan-400">
            查看台账
          </router-link>
        </template>

        <p v-if="recentIngest.length === 0" class="text-sm text-slate-400 dark:text-slate-500">暂无最近入库记录</p>
        <div v-else class="flex flex-col gap-2.5">
          <div
            v-for="item in recentIngest"
            :key="item.task_id"
            class="flex items-center justify-between gap-3 rounded-xl bg-slate-50 px-4 py-3 dark:bg-white/5"
          >
            <div class="min-w-0">
              <div class="truncate text-sm font-bold text-slate-700 dark:text-slate-200">{{ item.task_name || '任务' }}</div>
              <div class="mt-0.5 truncate text-xs text-slate-400 dark:text-slate-500">{{ item.source_name || item.task_type || '系统任务' }}</div>
            </div>
            <div class="flex shrink-0 flex-col items-end gap-0.5 text-right">
              <span class="text-xs text-slate-600 dark:text-slate-300">{{ item.result_brief }}</span>
              <small class="text-xs text-slate-400 dark:text-slate-500">{{ item.update_time || '-' }}</small>
            </div>
          </div>
        </div>
      </PageCard>
    </section>
  </div>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import {
  DocumentTextOutline,
  CloudOutline,
  PlayCircleOutline,
  ServerOutline
} from '@vicons/ionicons5'
import PageCard from '../../components/common/PageCard.vue'
import StatCard from '../../components/common/StatCard.vue'
import { getDashboardOverview, getDashboardResourceMonitor, getDashboardTrend } from '../../utils/api/dashboard'
import { getNetworkProbeSites, testNetworkConnectivity } from '../../utils/api/setting'

const router = useRouter()

const overview = ref({
  stats: {
    accounts: {},
    media_sources: {},
    strm_files: {},
    tasks: {},
    storage: { accounts: [] }
  },
  strm_task: {},
  archive_task: {},
  recent_tasks: [],
  recent_ingest: []
})
const monitorState = ref({})
const probes = ref([])
const probeLoading = ref(false)
const strmTrend = ref({ points: [] })
const archiveTrend = ref({ points: [] })

const workflowSteps = [
  { index: '01', title: '媒体源', desc: '本地与 115 统一接入' },
  { index: '02', title: '同步索引', desc: '全量或增量扫描资源' },
  { index: '03', title: '资产台账', desc: '追踪识别、STRM 与元数据' },
  { index: '04', title: '任务闭环', desc: '失败进入待处理修正' }
]

const stats = computed(() => ({
  accounts: {
    total: 0,
    active: 0,
    cooling: 0,
    disabled: 0,
    ...(overview.value?.stats?.accounts || {})
  },
  media_sources: {
    total: 0,
    local: 0,
    cloud115: 0,
    enabled: 0,
    ...(overview.value?.stats?.media_sources || {})
  },
  strm_files: {
    total: 0,
    last_generation_time: null,
    ...(overview.value?.stats?.strm_files || {})
  },
  tasks: {
    running: 0,
    completed_today: 0,
    failed_today: 0,
    ...(overview.value?.stats?.tasks || {})
  },
  storage: {
    accounts: [],
    ...(overview.value?.stats?.storage || {})
  }
}))

const monitor = computed(() => ({
  memory_bytes: 0,
  heap_alloc_bytes: 0,
  system_bytes: 0,
  goroutines: 0,
  cpu_cores: 0,
  running_tasks: 0,
  active_accounts: 0,
  enabled_sources: 0,
  sampled_at: '',
  ...(monitorState.value || {})
}))

const metricCards = computed(() => {
  return [
    {
      label: 'STRM 资产',
      value: stats.value.strm_files.total || 0,
      foot: stats.value.strm_files.last_generation_time ? `最近生成 ${formatDateTime(stats.value.strm_files.last_generation_time)}` : '暂无最近生成记录',
      icon: DocumentTextOutline,
      tone: 'cyan'
    },
    {
      label: '云盘账号',
      value: stats.value.accounts.total || 0,
      foot: `可用 ${stats.value.accounts.active || 0} / 冷却 ${stats.value.accounts.cooling || 0}`,
      icon: CloudOutline,
      tone: 'violet'
    },
    {
      label: '运行中任务',
      value: stats.value.tasks.running || 0,
      foot: `今日完成 ${stats.value.tasks.completed_today || 0} 项`,
      icon: PlayCircleOutline,
      tone: 'amber'
    },
    {
      label: '启用媒体源',
      value: stats.value.media_sources.enabled || 0,
      foot: `总数 ${stats.value.media_sources.total || 0}`,
      icon: ServerOutline,
      tone: 'green'
    }
  ]
})

const recentTasks = computed(() => (overview.value.recent_tasks || []).slice(0, 5))
const recentIngest = computed(() => (overview.value.recent_ingest || []).slice(0, 5))
const storageAccounts = computed(() => stats.value.storage.accounts.slice(0, 4))

const trendPoints = computed(() => {
  const strmPoints = strmTrend.value.points || []
  const archivePoints = archiveTrend.value.points || []
  const peak = Math.max(
    1,
    ...strmPoints.map(item => item.value || 0),
    ...archivePoints.map(item => item.value || 0)
  )

  return strmPoints.map((item, index) => {
    const archivePoint = archivePoints[index] || { value: 0 }
    return {
      date: item.date,
      label: String(item.date || '').slice(5),
      strmHeight: Math.max(8, Math.round(((item.value || 0) / peak) * 100)),
      organizeHeight: Math.max(8, Math.round(((archivePoint.value || 0) / peak) * 100))
    }
  })
})

const loadOverview = async () => {
  const response = await getDashboardOverview()
  overview.value = response.data.data || response.data || overview.value
}

const loadMonitor = async () => {
  const response = await getDashboardResourceMonitor()
  monitorState.value = response.data.data || response.data || {}
}

const loadTrends = async () => {
  const [strmResponse, archiveResponse] = await Promise.all([
    getDashboardTrend('strm', 7),
    getDashboardTrend('archive', 7)
  ])
  strmTrend.value = strmResponse.data.data || strmResponse.data || { points: [] }
  archiveTrend.value = archiveResponse.data.data || archiveResponse.data || { points: [] }
}

const loadProbes = async () => {
  probeLoading.value = true
  try {
    const siteResponse = await getNetworkProbeSites({ skipGlobalErrorMessage: true })
    const rawSiteList = siteResponse.data.data?.data || siteResponse.data.data || siteResponse.data || []
    const siteList = Array.isArray(rawSiteList) ? rawSiteList : []
    probes.value = siteList.map(site => ({ ...site, ok: null }))

    for (const site of siteList.slice(0, 3)) {
      try {
        const response = await testNetworkConnectivity({
          name: site.name,
          url: site.url,
          skipGlobalErrorMessage: true
        })
        const result = Array.isArray(response.data.data)
          ? response.data.data[0]
          : response.data.data?.data?.[0] || response.data.data || null
        const target = probes.value.find(item => item.url === site.url)
        if (target && result) {
          Object.assign(target, result)
        }
      } catch (error) {
        const target = probes.value.find(item => item.url === site.url)
        if (target) {
          target.ok = false
        }
      }
    }
  } finally {
    probeLoading.value = false
  }
}

const formatBytes = (value) => {
  const size = Number(value || 0)
  if (!size) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  const level = Math.min(Math.floor(Math.log(size) / Math.log(1024)), units.length - 1)
  return `${(size / (1024 ** level)).toFixed(level === 0 ? 0 : 1)} ${units[level]}`
}

const formatDateTime = (value) => {
  if (!value) return '-'
  return new Date(value).toLocaleString('zh-CN', { hour12: false })
}

const formatTaskStatus = (status) => {
  const map = {
    pending: '待执行',
    running: '执行中',
    completed: '已完成',
    failed: '失败',
    cancelled: '已取消',
    scheduled: '已调度'
  }
  return map[status] || '未知'
}

const taskStatusPillClass = (status) => {
  const map = {
    completed: 'bg-emerald-500/10 text-emerald-600 dark:text-emerald-400',
    running: 'bg-cyan-500/10 text-cyan-600 dark:text-cyan-400',
    failed: 'bg-red-500/10 text-red-600 dark:text-red-400',
    pending: 'bg-amber-500/10 text-amber-600 dark:text-amber-400',
    cancelled: 'bg-slate-500/10 text-slate-500 dark:text-slate-400',
    scheduled: 'bg-violet-500/10 text-violet-600 dark:text-violet-400'
  }
  return map[status] || 'bg-slate-500/10 text-slate-500 dark:text-slate-400'
}

const probeStatusPillClass = (ok) => {
  if (ok === true) return 'bg-emerald-500/10 text-emerald-600 dark:text-emerald-400'
  if (ok === false) return 'bg-red-500/10 text-red-600 dark:text-red-400'
  return 'bg-slate-500/10 text-slate-500 dark:text-slate-400'
}

onMounted(async () => {
  await Promise.all([loadOverview(), loadMonitor(), loadTrends(), loadProbes()])
})
</script>
