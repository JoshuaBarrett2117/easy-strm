<template>
  <div class="dashboard-home">
    <section class="hero-panel">
      <div class="hero-copy">
        <div class="hero-kicker">Resource Hub</div>
        <h2>以媒体资产台账为中心，串起同步、入库、STRM 与任务追踪。</h2>
        <p>
          首版主链路固定为媒体源同步索引、媒体库台账、入库流水线、任务中心和待处理修正。
        </p>
      </div>

      <div class="hero-actions">
        <router-link to="/dashboard/media-library" class="hero-action hero-action-primary">打开资产台账</router-link>
        <router-link to="/dashboard/sync-tasks" class="hero-action">同步入库</router-link>
        <router-link to="/dashboard/pending-media" class="hero-action">处理失败项</router-link>
        <router-link to="/dashboard/tasks" class="hero-action">查看任务中心</router-link>
      </div>
    </section>

    <section class="workflow-strip">
      <article v-for="step in workflowSteps" :key="step.title" class="workflow-step">
        <span>{{ step.index }}</span>
        <strong>{{ step.title }}</strong>
        <small>{{ step.desc }}</small>
      </article>
    </section>

    <section class="metric-grid">
      <article v-for="card in metricCards" :key="card.label" class="metric-card">
        <div class="metric-label">{{ card.label }}</div>
        <div class="metric-value">{{ card.value }}</div>
        <div class="metric-foot">{{ card.foot }}</div>
      </article>
    </section>

    <section class="main-grid">
      <article class="surface-card surface-span-2">
        <div class="surface-header">
          <div>
            <div class="surface-kicker">Task Flow</div>
            <h3>资源整理任务</h3>
          </div>
          <router-link to="/dashboard/tasks" class="surface-link">查看全部</router-link>
        </div>

        <div class="task-panels">
          <div class="task-panel">
            <div class="task-panel-label">运行中</div>
            <div class="task-panel-value">{{ stats.tasks.running || 0 }}</div>
            <div class="task-panel-foot">当前仍在执行的任务</div>
          </div>
          <div class="task-panel">
            <div class="task-panel-label">今日完成</div>
            <div class="task-panel-value">{{ stats.tasks.completed_today || 0 }}</div>
            <div class="task-panel-foot">当天成功落地的任务</div>
          </div>
          <div class="task-panel task-panel-alert">
            <div class="task-panel-label">今日失败</div>
            <div class="task-panel-value">{{ stats.tasks.failed_today || 0 }}</div>
            <div class="task-panel-foot">需要进入任务中心排查</div>
          </div>
        </div>

        <div class="recent-task-list">
          <div v-if="recentTasks.length === 0" class="empty-state">暂无任务记录</div>
          <button
            v-for="task in recentTasks"
            :key="task.task_id"
            type="button"
            class="recent-task-item"
            @click="router.push('/dashboard/tasks')"
          >
            <span class="recent-task-name">{{ task.task_name || task.task_type || '任务' }}</span>
            <span class="recent-task-status" :data-status="task.status">{{ formatTaskStatus(task.status) }}</span>
          </button>
        </div>
      </article>

      <article class="surface-card">
        <div class="surface-header">
          <div>
            <div class="surface-kicker">Source</div>
            <h3>同步入口</h3>
          </div>
          <router-link to="/dashboard/sync-tasks" class="surface-link">进入同步</router-link>
        </div>

        <div class="stat-stack">
          <div class="stat-row">
            <span>总媒体源</span>
            <strong>{{ stats.media_sources.total || 0 }}</strong>
          </div>
          <div class="stat-row">
            <span>本地</span>
            <strong>{{ stats.media_sources.local || 0 }}</strong>
          </div>
          <div class="stat-row">
            <span>115</span>
            <strong>{{ stats.media_sources.cloud115 || 0 }}</strong>
          </div>
          <div class="stat-row">
            <span>启用中</span>
            <strong>{{ stats.media_sources.enabled || 0 }}</strong>
          </div>
        </div>
      </article>

      <article class="surface-card">
        <div class="surface-header">
          <div>
            <div class="surface-kicker">Quota</div>
            <h3>账号配额</h3>
          </div>
          <router-link to="/dashboard/cloud115" class="surface-link">查看账号</router-link>
        </div>

        <div v-if="storageAccounts.length === 0" class="empty-state">暂无账号配额数据</div>
        <div v-else class="storage-list">
          <div v-for="account in storageAccounts" :key="account.name" class="storage-item">
            <div class="storage-head">
              <span>{{ account.name }}</span>
              <strong>{{ formatBytes(account.used) }}</strong>
            </div>
            <div class="storage-bar">
              <span class="storage-fill" :style="{ width: `${account.percentage || 12}%` }"></span>
            </div>
          </div>
        </div>
      </article>

      <article class="surface-card">
        <div class="surface-header">
          <div>
            <div class="surface-kicker">Runtime</div>
            <h3>资源监控</h3>
          </div>
          <span class="surface-link">{{ formatDateTime(monitor.sampled_at) }}</span>
        </div>

        <div class="stat-stack">
          <div class="stat-row">
            <span>运行内存</span>
            <strong>{{ formatBytes(monitor.memory_bytes) }}</strong>
          </div>
          <div class="stat-row">
            <span>堆分配</span>
            <strong>{{ formatBytes(monitor.heap_alloc_bytes) }}</strong>
          </div>
          <div class="stat-row">
            <span>Goroutines</span>
            <strong>{{ monitor.goroutines || 0 }}</strong>
          </div>
          <div class="stat-row">
            <span>CPU 核心</span>
            <strong>{{ monitor.cpu_cores || 0 }}</strong>
          </div>
        </div>
      </article>

      <article class="surface-card surface-span-2">
        <div class="surface-header">
          <div>
            <div class="surface-kicker">Trend</div>
            <h3>近 7 天整理趋势</h3>
          </div>
          <div class="trend-legend">
            <span><i class="legend-dot legend-dot-primary"></i>STRM</span>
            <span><i class="legend-dot legend-dot-secondary"></i>入库/整理</span>
          </div>
        </div>

        <div class="trend-chart">
          <div class="trend-grid">
            <div v-for="row in 4" :key="row" class="trend-grid-line"></div>
          </div>
          <div class="trend-series">
            <div v-for="point in trendPoints" :key="point.date" class="trend-column">
              <div class="trend-bars">
                <span class="trend-bar trend-bar-primary" :style="{ height: `${point.strmHeight}%` }"></span>
                <span class="trend-bar trend-bar-secondary" :style="{ height: `${point.organizeHeight}%` }"></span>
              </div>
              <div class="trend-label">{{ point.label }}</div>
            </div>
          </div>
        </div>
      </article>

      <article class="surface-card">
        <div class="surface-header">
          <div>
            <div class="surface-kicker">Probe</div>
            <h3>网络探针</h3>
          </div>
          <router-link to="/dashboard/network" class="surface-link">详细探测</router-link>
        </div>

        <div v-if="probeLoading" class="empty-state">探针检测中...</div>
        <div v-else class="probe-list">
          <div v-for="probe in probes" :key="probe.url" class="probe-item">
            <div>
              <div class="probe-name">{{ probe.name }}</div>
              <div class="probe-url">{{ probe.url }}</div>
            </div>
            <span class="probe-status" :data-status="probe.ok === true ? 'success' : probe.ok === false ? 'danger' : 'pending'">
              {{ probe.ok === true ? '正常' : probe.ok === false ? '失败' : '待测' }}
            </span>
          </div>
        </div>
      </article>

      <article class="surface-card surface-span-2">
        <div class="surface-header">
          <div>
            <div class="surface-kicker">Recent Ingest</div>
            <h3>最近入库</h3>
          </div>
          <router-link to="/dashboard/media-library" class="surface-link">查看台账</router-link>
        </div>

        <div v-if="recentIngest.length === 0" class="empty-state">暂无最近入库记录</div>
        <div v-else class="ingest-list">
          <div v-for="item in recentIngest" :key="item.task_id" class="ingest-item">
            <div>
              <div class="ingest-name">{{ item.task_name || '任务' }}</div>
              <div class="ingest-meta">{{ item.source_name || item.task_type || '系统任务' }}</div>
            </div>
            <div class="ingest-brief">
              <span>{{ item.result_brief }}</span>
              <small>{{ item.update_time || '-' }}</small>
            </div>
          </div>
        </div>
      </article>
    </section>
  </div>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
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
      foot: stats.value.strm_files.last_generation_time ? `最近生成 ${formatDateTime(stats.value.strm_files.last_generation_time)}` : '暂无最近生成记录'
    },
    {
      label: '云盘账号',
      value: stats.value.accounts.total || 0,
      foot: `可用 ${stats.value.accounts.active || 0} / 冷却 ${stats.value.accounts.cooling || 0}`
    },
    {
      label: '运行中任务',
      value: stats.value.tasks.running || 0,
      foot: `今日完成 ${stats.value.tasks.completed_today || 0} 项`
    },
    {
      label: '启用媒体源',
      value: stats.value.media_sources.enabled || 0,
      foot: `总数 ${stats.value.media_sources.total || 0}`
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

onMounted(async () => {
  await Promise.all([loadOverview(), loadMonitor(), loadTrends(), loadProbes()])
})
</script>

<style scoped>
.dashboard-home {
  display: flex;
  flex-direction: column;
  gap: 18px;
}

.hero-panel,
.workflow-step,
.surface-card,
.metric-card {
  background: rgba(255, 252, 247, 0.84);
  border: 1px solid rgba(120, 101, 72, 0.12);
  box-shadow: 0 24px 60px rgba(58, 42, 24, 0.08);
  backdrop-filter: blur(14px);
}

:global(.dark) .hero-panel,
:global(.dark) .workflow-step,
:global(.dark) .surface-card,
:global(.dark) .metric-card {
  background: rgba(14, 21, 32, 0.86);
  border-color: rgba(139, 163, 185, 0.12);
  box-shadow: 0 24px 60px rgba(0, 0, 0, 0.22);
}

.hero-panel {
  display: grid;
  grid-template-columns: 1.5fr 1fr;
  gap: 24px;
  padding: 28px;
  border-radius: 28px;
}

.hero-kicker,
.surface-kicker {
  color: #1f6f78;
  font-size: 12px;
  font-weight: 700;
  letter-spacing: 0.16em;
  text-transform: uppercase;
}

:global(.dark) .hero-kicker,
:global(.dark) .surface-kicker {
  color: #77c3d4;
}

.hero-copy h2 {
  margin: 12px 0;
  font-size: 34px;
  line-height: 1.15;
}

.hero-copy p {
  color: #695d51;
  line-height: 1.8;
}

:global(.dark) .hero-copy p,
:global(.dark) .surface-link,
:global(.dark) .metric-foot,
:global(.dark) .stat-row span,
:global(.dark) .probe-url,
:global(.dark) .trend-label,
:global(.dark) .empty-state,
:global(.dark) .recent-task-name {
  color: #8fa1b5;
}

.hero-actions {
  display: flex;
  flex-direction: column;
  gap: 12px;
  justify-content: center;
}

.hero-action {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-height: 54px;
  padding: 0 18px;
  border-radius: 18px;
  background: rgba(31, 111, 120, 0.08);
  color: #1f2933;
  text-decoration: none;
  font-weight: 700;
}

.hero-action-primary {
  background: linear-gradient(135deg, #1f6f78, #f2a65a);
  color: #fff;
}

.workflow-strip {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 14px;
}

.workflow-step {
  display: grid;
  gap: 8px;
  min-height: 128px;
  padding: 20px;
  border-radius: 18px;
}

.workflow-step span {
  color: #1f6f78;
  font-size: 13px;
  font-weight: 800;
}

.workflow-step strong {
  font-size: 20px;
}

.workflow-step small {
  color: #776c60;
  line-height: 1.6;
}

:global(.dark) .workflow-step span {
  color: #77c3d4;
}

:global(.dark) .workflow-step small {
  color: #8fa1b5;
}

.metric-grid {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 18px;
}

.metric-card {
  padding: 22px;
  border-radius: 24px;
}

.metric-label {
  color: #695d51;
  font-size: 13px;
  font-weight: 600;
}

.metric-value {
  margin: 10px 0 6px;
  font-size: 40px;
  font-weight: 800;
  line-height: 1;
}

.metric-foot {
  font-size: 12px;
  color: #776c60;
  line-height: 1.6;
}

.main-grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 18px;
}

.surface-card {
  padding: 22px;
  border-radius: 24px;
}

.surface-span-2 {
  grid-column: span 2;
}

.surface-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
  margin-bottom: 18px;
}

.surface-header h3 {
  margin-top: 8px;
  font-size: 24px;
}

.surface-link {
  color: #5b5147;
  text-decoration: none;
  font-size: 13px;
  font-weight: 700;
}

.task-panels {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 12px;
  margin-bottom: 18px;
}

.task-panel {
  padding: 18px;
  border-radius: 18px;
  background: rgba(31, 111, 120, 0.08);
}

.task-panel-alert {
  background: rgba(224, 95, 76, 0.08);
}

.task-panel-label,
.task-panel-foot {
  font-size: 13px;
  color: #6f6457;
}

.task-panel-value {
  margin: 10px 0 8px;
  font-size: 32px;
  font-weight: 800;
}

.recent-task-list {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.recent-task-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 14px 16px;
  border: 0;
  border-radius: 16px;
  background: rgba(255, 255, 255, 0.58);
  cursor: pointer;
  text-align: left;
}

:global(.dark) .recent-task-item {
  background: rgba(255, 255, 255, 0.04);
}

.recent-task-name {
  color: #1f2933;
  font-weight: 600;
}

:global(.dark) .recent-task-name {
  color: #ebf2fa;
}

.recent-task-status,
.probe-status {
  padding: 6px 10px;
  border-radius: 999px;
  font-size: 12px;
  font-weight: 700;
}

.recent-task-status[data-status='completed'],
.probe-status[data-status='success'] {
  background: rgba(79, 168, 100, 0.14);
  color: #2f7d32;
}

.recent-task-status[data-status='running'] {
  background: rgba(242, 166, 90, 0.18);
  color: #9a5a17;
}

.recent-task-status[data-status='failed'],
.probe-status[data-status='danger'] {
  background: rgba(220, 94, 74, 0.14);
  color: #9f3729;
}

.recent-task-status[data-status='pending'],
.probe-status[data-status='pending'] {
  background: rgba(76, 110, 245, 0.12);
  color: #3154a6;
}

.stat-stack,
.storage-list,
.probe-list {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.stat-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding-bottom: 10px;
  border-bottom: 1px dashed rgba(120, 101, 72, 0.16);
}

.stat-row strong,
.storage-head strong {
  font-size: 20px;
}

.storage-item {
  padding: 14px 0;
}

.storage-head {
  display: flex;
  justify-content: space-between;
  margin-bottom: 8px;
}

.storage-bar {
  height: 10px;
  border-radius: 999px;
  background: rgba(31, 111, 120, 0.08);
  overflow: hidden;
}

.storage-fill {
  display: block;
  height: 100%;
  border-radius: inherit;
  background: linear-gradient(90deg, #1f6f78, #f2a65a);
}

.trend-chart {
  position: relative;
  height: 280px;
  padding-top: 14px;
}

.trend-grid {
  position: absolute;
  inset: 0 0 28px;
  display: grid;
  grid-template-rows: repeat(4, 1fr);
}

.trend-grid-line {
  border-top: 1px dashed rgba(120, 101, 72, 0.12);
}

.trend-series {
  position: relative;
  z-index: 1;
  height: 100%;
  display: grid;
  grid-template-columns: repeat(7, minmax(0, 1fr));
  align-items: end;
  gap: 12px;
}

.trend-column {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 10px;
  height: 100%;
}

.trend-bars {
  flex: 1;
  width: 100%;
  display: flex;
  align-items: flex-end;
  justify-content: center;
  gap: 8px;
}

.trend-bar {
  width: 18px;
  min-height: 8%;
  border-radius: 999px 999px 6px 6px;
}

.trend-bar-primary {
  background: linear-gradient(180deg, #1f6f78, #3d96a1);
}

.trend-bar-secondary {
  background: linear-gradient(180deg, #f2a65a, #d58534);
}

.trend-label {
  font-size: 12px;
  color: #6f6457;
}

.trend-legend {
  display: flex;
  gap: 14px;
  color: #6f6457;
  font-size: 12px;
}

.legend-dot {
  display: inline-block;
  width: 8px;
  height: 8px;
  margin-right: 6px;
  border-radius: 999px;
}

.legend-dot-primary {
  background: #1f6f78;
}

.legend-dot-secondary {
  background: #f2a65a;
}

.probe-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 14px 16px;
  border-radius: 16px;
  background: rgba(255, 255, 255, 0.52);
}

:global(.dark) .probe-item {
  background: rgba(255, 255, 255, 0.04);
}

.probe-name {
  font-weight: 700;
}

.probe-url {
  margin-top: 4px;
  font-size: 12px;
  color: #776c60;
}

.empty-state {
  color: #776c60;
  font-size: 14px;
}

.ingest-list {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.ingest-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 14px 16px;
  border-radius: 16px;
  background: rgba(255, 255, 255, 0.52);
}

:global(.dark) .ingest-item {
  background: rgba(255, 255, 255, 0.04);
}

.ingest-name {
  font-weight: 700;
}

.ingest-meta,
.ingest-brief small {
  color: #776c60;
  font-size: 12px;
}

.ingest-meta {
  margin-top: 4px;
}

.ingest-brief {
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  gap: 4px;
  text-align: right;
}

@media (max-width: 1180px) {
  .metric-grid,
  .main-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

@media (max-width: 860px) {
  .hero-panel,
  .workflow-strip,
  .metric-grid,
  .main-grid,
  .task-panels {
    grid-template-columns: 1fr;
  }

  .surface-span-2 {
    grid-column: span 1;
  }
}
</style>
