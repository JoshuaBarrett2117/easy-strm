<template>
  <div class="sync-page">
    <section class="page-hero">
      <div>
        <div class="page-kicker">Library Sync</div>
        <h2>同步任务</h2>
        <p>对 115 与本地媒体源执行全量同步、增量同步，并查看同步索引。</p>
      </div>
      <el-button :icon="Refresh" @click="loadSources" :loading="loading">刷新</el-button>
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
            <el-button :disabled="!selectedSource" :loading="syncing" @click="runPipeline">执行入库</el-button>
          </div>
        </div>

        <el-alert
          v-if="lastResult"
          class="sync-result"
          type="success"
          show-icon
          :closable="false"
          :title="formatLastResult(lastResult)"
        />

        <el-table :data="indexRows" v-loading="indexLoading" border>
          <el-table-column prop="source_name" label="文件名" min-width="220" />
          <el-table-column prop="source_path" label="源路径" min-width="260" show-overflow-tooltip />
          <el-table-column prop="target_path" label="目标路径" min-width="260" show-overflow-tooltip />
          <el-table-column prop="sync_status" label="同步状态" width="110" />
          <el-table-column prop="last_change_type" label="变更" width="110" />
          <el-table-column prop="last_task_id" label="最近任务" min-width="180" show-overflow-tooltip />
          <el-table-column label="状态" width="150">
            <template #default="{ row }">
              <el-tag size="small" :type="row.identity_status === 'identified' ? 'success' : row.identity_status === 'failed' ? 'warning' : 'info'">
                {{ row.identity_status || 'unknown' }}
              </el-tag>
            </template>
          </el-table-column>
        </el-table>
      </main>
    </section>
  </div>
</template>

<script setup>
import { onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { Refresh } from '@element-plus/icons-vue'
import { getMediaSources, getMediaSyncIndex, runFullMediaSync, runIncrementalMediaSync, runMediaLibraryPipeline } from '../utils/api/media'

const loading = ref(false)
const syncing = ref(false)
const indexLoading = ref(false)
const sources = ref([])
const selectedSource = ref(null)
const indexRows = ref([])
const lastResult = ref(null)

const loadSources = async () => {
  loading.value = true
  try {
    const response = await getMediaSources()
    const payload = response.data?.data?.data || response.data?.data || []
    sources.value = Array.isArray(payload) ? payload : []
    if (!selectedSource.value && sources.value.length > 0) {
      await selectSource(sources.value[0])
    }
  } finally {
    loading.value = false
  }
}

const selectSource = async (source) => {
  selectedSource.value = source
  lastResult.value = null
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
  grid-template-columns: 280px minmax(0, 1fr);
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
.panel-header p {
  margin: 0;
  color: #6f6457;
  font-size: 13px;
}

.panel-actions {
  display: flex;
  gap: 10px;
}

.sync-result {
  margin: 16px 0;
}

@media (max-width: 900px) {
  .sync-layout {
    grid-template-columns: 1fr;
  }
}
</style>
