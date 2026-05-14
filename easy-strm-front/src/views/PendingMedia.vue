<template>
  <div class="pending-page">
    <section class="page-hero">
      <div>
        <div class="page-kicker">Pending Queue</div>
        <h2>待处理资源</h2>
        <p>集中承接识别失败项，完成 TMDB 修正、重新入库、忽略和任务回溯。</p>
      </div>
      <div class="hero-actions">
        <el-button @click="goLibrary" :disabled="!filters.source_id">查看资产台账</el-button>
        <el-button :icon="Refresh" @click="loadItems" :loading="loading">刷新</el-button>
      </div>
    </section>

    <section class="summary-grid">
      <article v-for="card in summaryCards" :key="card.label" class="summary-card">
        <span>{{ card.label }}</span>
        <strong>{{ card.value }}</strong>
        <small>{{ card.hint }}</small>
      </article>
    </section>

    <section class="filter-bar">
      <el-select v-model="filters.source_id" placeholder="媒体源" clearable filterable @change="handleSourceChange">
        <el-option v-for="source in sources" :key="source.id" :label="source.name" :value="source.id" />
      </el-select>
      <el-select v-model="filters.status" placeholder="状态" clearable @change="loadItems">
        <el-option label="待处理" value="pending" />
        <el-option label="已识别" value="identified" />
        <el-option label="执行中" value="running" />
        <el-option label="已完成" value="completed" />
        <el-option label="已忽略" value="ignored" />
        <el-option label="失败" value="failed" />
      </el-select>
      <el-select v-model="filters.media_type" placeholder="媒体类型" clearable @change="loadItems">
        <el-option label="电影" value="movie" />
        <el-option label="剧集" value="tv" />
        <el-option label="动画" value="anime" />
      </el-select>
    </section>

    <el-alert
      v-if="lastAction.message"
      class="action-alert"
      type="success"
      show-icon
      :closable="false"
      :title="lastAction.message"
    >
      <template #default>
        <div class="alert-actions">
          <el-button v-if="lastAction.taskId" size="small" text @click="goTask(lastAction.taskId)">查看任务</el-button>
          <el-button size="small" text @click="goLibrary">回到资产台账</el-button>
        </div>
      </template>
    </el-alert>

    <section class="table-panel">
      <el-table :data="items" v-loading="loading" border>
        <el-table-column prop="title" label="标题" min-width="180" />
        <el-table-column prop="source_path" label="来源路径" min-width="260" show-overflow-tooltip />
        <el-table-column label="类型" width="100">
          <template #default="{ row }">{{ mediaTypeLabel(row.media_type) }}</template>
        </el-table-column>
        <el-table-column prop="year" label="年份" width="90" />
        <el-table-column label="状态" width="110">
          <template #default="{ row }">
            <el-tag :type="statusTagType(row.status)" size="small">{{ statusLabel(row.status) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="reason" label="原因" min-width="180" show-overflow-tooltip />
        <el-table-column label="关联任务" min-width="180" show-overflow-tooltip>
          <template #default="{ row }">
            <button
              v-if="row.related_task_id"
              type="button"
              class="task-link"
              @click="goTask(row.related_task_id)"
            >
              {{ row.related_task_id }}
            </button>
            <span v-else>-</span>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="220" fixed="right">
          <template #default="{ row }">
            <el-button size="small" @click="openIdentify(row)">修正</el-button>
            <el-button size="small" type="primary" @click="runItem(row)" :loading="rowLoadingId === row.id">入库</el-button>
            <el-button size="small" @click="ignoreItem(row)">忽略</el-button>
          </template>
        </el-table-column>
      </el-table>
      <el-empty v-if="!loading && items.length === 0" description="暂无待处理项" />
    </section>

    <el-dialog v-model="identifyVisible" title="修正识别" width="420px">
      <el-form label-width="90px">
        <el-form-item label="标题">
          <el-input v-model="identifyForm.title" />
        </el-form-item>
        <el-form-item label="TMDB ID">
          <el-input-number v-model="identifyForm.tmdb_id" :min="0" />
        </el-form-item>
        <el-form-item label="媒体类型">
          <el-select v-model="identifyForm.media_type">
            <el-option label="电影" value="movie" />
            <el-option label="剧集" value="tv" />
            <el-option label="动画" value="anime" />
          </el-select>
        </el-form-item>
        <el-form-item label="年份">
          <el-input-number v-model="identifyForm.year" :min="0" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="identifyVisible = false">取消</el-button>
        <el-button type="primary" @click="saveIdentify">保存</el-button>
        <el-button type="success" @click="saveIdentifyAndRun" :loading="rowLoadingId === activeItem?.id">保存并入库</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { Refresh } from '@element-plus/icons-vue'
import { getMediaSources, getPendingMediaItems, identifyPendingMediaItem, ignorePendingMediaItem, runPendingMediaItem } from '../utils/api/media'

const route = useRoute()
const router = useRouter()

const loading = ref(false)
const sources = ref([])
const items = ref([])
const identifyVisible = ref(false)
const activeItem = ref(null)
const rowLoadingId = ref(null)
const filters = reactive({
  source_id: null,
  status: '',
  media_type: ''
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

const summaryCards = computed(() => {
  const total = items.value.length
  const pending = items.value.filter(item => item.status === 'pending').length
  const identified = items.value.filter(item => item.status === 'identified').length
  const failed = items.value.filter(item => item.status === 'failed').length
  return [
    { label: '待处理总数', value: total, hint: '当前筛选范围内的失败资源' },
    { label: '待修正', value: pending, hint: '需要补充识别信息' },
    { label: '已识别', value: identified, hint: '可重新进入入库流水线' },
    { label: '入库失败', value: failed, hint: '需查看任务原因后重试' }
  ]
})

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
      status: filters.status,
      media_type: filters.media_type,
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
  ElMessage.success('识别信息已保存')
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
    ElMessage.success('已重新入库')
    await loadItems()
  } finally {
    rowLoadingId.value = null
  }
}

const ignoreItem = async (row) => {
  await ignorePendingMediaItem(row.id)
  lastAction.message = '已忽略该待处理项'
  lastAction.taskId = ''
  ElMessage.success('已忽略')
  await loadItems()
}

const goLibrary = () => {
  router.push({ path: '/dashboard/media-library', query: filters.source_id ? { source_id: filters.source_id } : {} })
}

const goTask = (taskId) => {
  router.push({ path: '/dashboard/tasks', query: { task_id: taskId } })
}

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
  if (value === 'failed') return 'danger'
  if (value === 'identified' || value === 'running') return 'warning'
  if (value === 'ignored') return 'info'
  return 'primary'
}

onMounted(async () => {
  await loadSources()
  await loadItems()
})
</script>

<style scoped>
.pending-page {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.page-hero,
.summary-card,
.filter-bar,
.table-panel {
  background: rgba(255, 252, 247, 0.86);
  border: 1px solid rgba(120, 101, 72, 0.12);
  border-radius: 18px;
  box-shadow: 0 18px 44px rgba(58, 42, 24, 0.08);
}

.page-hero,
.filter-bar {
  display: flex;
  align-items: center;
  gap: 14px;
}

.hero-actions,
.alert-actions {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}

.page-hero {
  justify-content: space-between;
  padding: 24px;
}

.summary-grid {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 14px;
}

.summary-card {
  display: grid;
  gap: 8px;
  padding: 18px;
}

.summary-card span,
.summary-card small {
  color: #6f6457;
}

.summary-card strong {
  font-size: 32px;
}

.filter-bar,
.table-panel {
  padding: 18px;
}

.page-kicker {
  color: #1f6f78;
  font-size: 12px;
  font-weight: 700;
  letter-spacing: 0.16em;
  text-transform: uppercase;
}

.page-hero h2 {
  margin: 8px 0 6px;
  font-size: 30px;
}

.action-alert {
  border-radius: 14px;
}

.task-link {
  border: 0;
  padding: 0;
  background: transparent;
  color: #1f6f78;
  cursor: pointer;
  font: inherit;
  text-align: left;
}

@media (max-width: 900px) {
  .page-hero,
  .filter-bar {
    align-items: stretch;
    flex-direction: column;
  }

  .summary-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

@media (max-width: 560px) {
  .summary-grid {
    grid-template-columns: 1fr;
  }
}
</style>
