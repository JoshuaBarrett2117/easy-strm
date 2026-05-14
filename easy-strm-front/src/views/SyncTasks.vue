<template>
  <div class="sync-page">
    <section class="page-hero">
      <div>
        <div class="page-kicker">Sync & Ingest</div>
        <h2>同步入库工作台</h2>
        <p>对本地和 115 媒体源执行同步索引，随后触发入库、STRM 与媒体服务器刷新。</p>
      </div>
      <div class="hero-actions">
        <el-button @click="goLibrary" :disabled="!selectedSource">查看资产台账</el-button>
        <el-button :icon="Refresh" @click="loadSources" :loading="loading">刷新媒体源</el-button>
      </div>
    </section>

    <section class="summary-grid">
      <article v-for="card in summaryCards" :key="card.label" class="summary-card">
        <span>{{ card.label }}</span>
        <strong>{{ card.value }}</strong>
        <small>{{ card.hint }}</small>
      </article>
    </section>

    <section class="sync-layout">
      <aside class="source-panel">
        <div class="panel-title">媒体源</div>
        <div v-if="sources.length" class="source-list">
          <button
            v-for="source in sources"
            :key="source.id"
            type="button"
            class="source-item"
            :class="{ active: selectedSource?.id === source.id }"
            @click="selectSource(source)"
          >
            <strong>{{ source.name }}</strong>
            <span>{{ source.source_type === 'cloud115' ? '115 云盘' : '本地目录' }}</span>
            <small>{{ source.path || '-' }}</small>
          </button>
        </div>
        <el-empty v-else description="暂无媒体源" />
      </aside>

      <main class="sync-panel">
        <div class="panel-header">
          <div>
            <div class="panel-title">{{ selectedSource?.name || '请选择媒体源' }}</div>
            <p v-if="selectedSource">路径：{{ selectedSource.path || '-' }}</p>
          </div>
          <div class="panel-actions">
            <el-button :disabled="!selectedSource" :loading="syncing" @click="runSync('full')">全量同步</el-button>
            <el-button :disabled="!selectedSource" :loading="syncing" type="primary" @click="runSync('incremental')">增量同步</el-button>
            <el-button :disabled="!selectedSource" :loading="syncing" type="success" plain @click="runPipeline">执行入库</el-button>
          </div>
        </div>

        <el-alert
          v-if="lastResult"
          class="sync-result"
          type="success"
          show-icon
          :closable="false"
          :title="formatLastResult(lastResult)"
        >
          <template #default>
            <div class="result-actions">
              <el-button v-if="lastResult.task_id" size="small" text @click="goTask(lastResult.task_id)">查看任务</el-button>
              <el-button size="small" text @click="goLibrary">查看资产台账</el-button>
              <el-button size="small" text @click="goPending">查看待处理</el-button>
            </div>
          </template>
        </el-alert>

        <el-table :data="indexRows" v-loading="indexLoading" border>
          <el-table-column prop="source_name" label="文件名" min-width="220" />
          <el-table-column prop="source_path" label="源路径" min-width="260" show-overflow-tooltip />
          <el-table-column prop="target_path" label="目标路径" min-width="240" show-overflow-tooltip />
          <el-table-column label="同步状态" width="110">
            <template #default="{ row }">
              <el-tag :type="syncTagType(row.sync_status)" size="small">{{ syncLabel(row.sync_status) }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column label="识别状态" width="110">
            <template #default="{ row }">
              <el-tag :type="identityTagType(row.identity_status)" size="small">{{ identityLabel(row.identity_status) }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column prop="last_change_type" label="变更" width="120" />
          <el-table-column label="最近任务" min-width="180" show-overflow-tooltip>
            <template #default="{ row }">
              <button
                v-if="row.last_task_id"
                type="button"
                class="task-link"
                @click="goTask(row.last_task_id)"
              >
                {{ row.last_task_id }}
              </button>
              <span v-else>-</span>
            </template>
          </el-table-column>
        </el-table>
      </main>
    </section>
  </div>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { Refresh } from '@element-plus/icons-vue'
import { getMediaSources, getMediaSyncIndex, runFullMediaSync, runIncrementalMediaSync, runMediaLibraryPipeline } from '../utils/api/media'

const route = useRoute()
const router = useRouter()

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
    { label: '索引条目', value: indexRows.value.length, hint: '当前媒体源已记录资源' },
    { label: '有效资源', value: active, hint: '源端仍存在的同步资产' },
    { label: '已识别', value: identified, hint: '可进入后续入库动作' },
    { label: '待修正', value: failed, hint: '会进入待处理页面承接' }
  ]
})

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
    ElMessage.success(mode === 'full' ? '全量同步完成' : '增量同步完成')
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
    ElMessage.success('入库流水线已完成')
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

const syncLabel = (value) => ({
  active: '有效',
  missing: '源端缺失',
  deleted: '已删除'
})[value] || value || '-'

const syncTagType = (value) => value === 'active' ? 'success' : value === 'missing' ? 'warning' : value === 'deleted' ? 'danger' : 'info'

const identityLabel = (value) => ({
  identified: '已识别',
  failed: '失败',
  pending: '待识别'
})[value] || value || '未知'

const identityTagType = (value) => value === 'identified' ? 'success' : value === 'failed' ? 'warning' : 'info'

onMounted(loadSources)
</script>

<style scoped>
.sync-page,
.sync-layout,
.source-list {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.page-hero,
.summary-card,
.source-panel,
.sync-panel {
  background: rgba(255, 252, 247, 0.86);
  border: 1px solid rgba(120, 101, 72, 0.12);
  border-radius: 18px;
  box-shadow: 0 18px 44px rgba(58, 42, 24, 0.08);
}

.page-hero,
.panel-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
}

.page-hero {
  padding: 24px;
}

.hero-actions,
.panel-actions,
.result-actions {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
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

.sync-layout {
  display: grid;
  grid-template-columns: 300px minmax(0, 1fr);
  align-items: start;
}

.source-panel,
.sync-panel {
  padding: 18px;
}

.panel-title {
  font-size: 16px;
  font-weight: 700;
}

.source-item {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 12px;
  border: 1px solid #e4e7ed;
  border-radius: 10px;
  background: #fff;
  text-align: left;
  cursor: pointer;
}

.source-item.active {
  border-color: #1f6f78;
  background: rgba(31, 111, 120, 0.08);
}

.source-item span,
.source-item small,
.panel-header p {
  margin: 0;
  color: #6f6457;
  font-size: 13px;
}

.sync-result {
  margin: 16px 0;
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
  .panel-header {
    align-items: stretch;
    flex-direction: column;
  }

  .summary-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .sync-layout {
    grid-template-columns: 1fr;
  }
}

@media (max-width: 560px) {
  .summary-grid {
    grid-template-columns: 1fr;
  }
}
</style>
