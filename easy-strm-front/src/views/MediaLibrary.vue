<template>
  <div class="library-page">
    <section class="page-hero">
      <div>
        <div class="page-kicker">Asset Ledger</div>
        <h2>媒体资产台账</h2>
        <p>从同步索引追踪源文件、识别状态、STRM、元数据、最近任务和资源健康度。</p>
      </div>
      <div class="hero-actions">
        <el-button @click="goSync" :disabled="!selectedSourceId">同步入库</el-button>
        <el-button @click="goPending">待处理</el-button>
        <el-button type="primary" :icon="Refresh" @click="loadItems" :loading="loading">刷新台账</el-button>
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
      <el-select v-model="selectedSourceId" placeholder="选择媒体源" filterable @change="handleSourceChange">
        <el-option v-for="source in sources" :key="source.id" :label="source.name" :value="source.id" />
      </el-select>
      <el-select v-model="status" placeholder="同步状态" clearable @change="loadItems">
        <el-option label="有效" value="active" />
        <el-option label="源端缺失" value="missing" />
        <el-option label="已删除" value="deleted" />
      </el-select>
      <el-select v-model="healthFilter" placeholder="健康状态" clearable>
        <el-option label="正常" value="ok" />
        <el-option label="待识别" value="identify_failed" />
        <el-option label="缺少 STRM" value="strm_missing" />
        <el-option label="源端异常" value="missing" />
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
          <span v-if="lastAction.taskId">最近任务：{{ lastAction.taskId }}</span>
          <el-button v-if="canOpenTask(lastAction.taskId)" size="small" text @click="goTask(lastAction.taskId)">查看任务</el-button>
        </div>
      </template>
    </el-alert>

    <section class="table-panel">
      <el-table :data="filteredItems" v-loading="loading" border>
        <el-table-column prop="source_name" label="资源条目" min-width="220" />
        <el-table-column label="健康状态" width="120">
          <template #default="{ row }">
            <el-tag :type="healthTagType(row.health_status)" size="small">{{ healthLabel(row.health_status) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="识别" width="110">
          <template #default="{ row }">
            <el-tag :type="identityTagType(row.identity_status)" size="small">{{ identityLabel(row.identity_status) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="同步" width="110">
          <template #default="{ row }">
            <el-tag :type="syncTagType(row.sync_status)" size="small">{{ syncLabel(row.sync_status) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="STRM" width="90">
          <template #default="{ row }">
            <el-tag :type="row.has_strm ? 'success' : 'info'" size="small">{{ row.has_strm ? '有' : '无' }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="元数据" width="100">
          <template #default="{ row }">
            <el-tag :type="row.has_metadata ? 'success' : 'info'" size="small">{{ row.has_metadata ? '有' : '缺失' }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="source_path" label="源路径" min-width="260" show-overflow-tooltip />
        <el-table-column prop="target_path" label="目标路径" min-width="240" show-overflow-tooltip />
        <el-table-column label="最近任务" min-width="180" show-overflow-tooltip>
          <template #default="{ row }">
            <button
              v-if="canOpenTask(row.latest_task_id)"
              type="button"
              class="task-link"
              @click="goTask(row.latest_task_id)"
            >
              {{ row.latest_task_id }}
            </button>
            <span v-else>{{ row.latest_task_id || '-' }}</span>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="320" fixed="right">
          <template #default="{ row }">
            <el-button size="small" @click="runPipeline(row)" :loading="rowLoadingId === row.id">入库</el-button>
            <el-button size="small" @click="generateStrm(row)" :loading="rowLoadingId === row.id">STRM</el-button>
            <el-button size="small" type="primary" @click="refreshServer(row)" :loading="rowLoadingId === row.id">刷新库</el-button>
          </template>
        </el-table-column>
      </el-table>
      <el-empty v-if="!loading && filteredItems.length === 0" description="暂无媒体资产，请先执行同步入库" />
    </section>
  </div>
</template>

<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { Refresh } from '@element-plus/icons-vue'
import {
  generateMediaLibraryItemStrm,
  getMediaLibraryItems,
  getMediaSources,
  refreshMediaLibraryItemServer,
  runMediaLibraryItemPipeline
} from '../utils/api/media'

const route = useRoute()
const router = useRouter()

const loading = ref(false)
const sources = ref([])
const selectedSourceId = ref(null)
const status = ref('')
const healthFilter = ref('')
const items = ref([])
const rowLoadingId = ref(null)
const lastAction = reactive({
  message: '',
  taskId: ''
})

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
    { label: '资产总数', value: total, hint: '当前媒体源同步索引条目' },
    { label: '健康资源', value: ok, hint: '识别和关键资产状态正常' },
    { label: '待处理', value: pending, hint: '识别失败或需要人工修正' },
    { label: '缺少 STRM', value: strmMissing, hint: '云盘资源需补齐播放入口' }
  ]
})

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
      status: status.value
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
    ElMessage.success(successMessage)
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
  missing: 'danger'
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

const syncTagType = (value) => value === 'active' ? 'success' : value === 'missing' ? 'warning' : value === 'deleted' ? 'danger' : 'info'

onMounted(async () => {
  await loadSources()
  await loadItems()
})
</script>

<style scoped>
.library-page {
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
