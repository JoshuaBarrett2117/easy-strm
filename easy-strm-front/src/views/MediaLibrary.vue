<template>
  <div class="library-page">
    <section class="page-hero">
      <div>
        <div class="page-kicker">Library</div>
        <h2>媒体库</h2>
        <p>从同步索引查看媒体条目、STRM 状态、元数据完整度与最近任务。</p>
      </div>
      <div class="hero-actions">
        <el-button :icon="Refresh" @click="loadItems" :loading="loading">刷新</el-button>
      </div>
    </section>

    <section class="filter-bar">
      <el-select v-model="selectedSourceId" placeholder="选择媒体源" filterable @change="loadItems">
        <el-option v-for="source in sources" :key="source.id" :label="source.name" :value="source.id" />
      </el-select>
      <el-select v-model="status" placeholder="同步状态" clearable @change="loadItems">
        <el-option label="有效" value="active" />
        <el-option label="源端缺失" value="missing" />
        <el-option label="已删除" value="deleted" />
      </el-select>
    </section>

    <section class="table-panel">
      <el-table :data="items" v-loading="loading" border>
        <el-table-column prop="source_name" label="条目" min-width="220" />
        <el-table-column prop="health_status" label="健康状态" width="120" />
        <el-table-column prop="sync_status" label="同步状态" width="110" />
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
        <el-table-column prop="target_path" label="目标路径" min-width="260" show-overflow-tooltip />
        <el-table-column prop="latest_task_id" label="最近任务" min-width="180" show-overflow-tooltip />
        <el-table-column label="操作" width="270" fixed="right">
          <template #default="{ row }">
            <el-button size="small" @click="runPipeline(row)" :loading="rowLoadingId === row.id">入库</el-button>
            <el-button size="small" @click="generateStrm(row)" :loading="rowLoadingId === row.id">STRM</el-button>
            <el-button size="small" type="primary" @click="refreshServer(row)" :loading="rowLoadingId === row.id">刷新库</el-button>
          </template>
        </el-table-column>
      </el-table>
      <el-empty v-if="!loading && items.length === 0" description="暂无媒体库条目，请先执行同步" />
    </section>
  </div>
</template>

<script setup>
import { onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { Refresh } from '@element-plus/icons-vue'
import {
  generateMediaLibraryItemStrm,
  getMediaLibraryItems,
  getMediaSources,
  refreshMediaLibraryItemServer,
  runMediaLibraryItemPipeline
} from '../utils/api/media'

const loading = ref(false)
const sources = ref([])
const selectedSourceId = ref(null)
const status = ref('')
const items = ref([])
const rowLoadingId = ref(null)

const loadSources = async () => {
  const response = await getMediaSources()
  const payload = response.data?.data?.data || response.data?.data || []
  sources.value = Array.isArray(payload) ? payload : []
  if (!selectedSourceId.value && sources.value.length > 0) {
    selectedSourceId.value = sources.value[0].id
  }
}

const loadItems = async () => {
  if (!selectedSourceId.value) return
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

const runRowAction = async (row, action, successMessage) => {
  rowLoadingId.value = row.id
  try {
    await action(row.id)
    ElMessage.success(successMessage)
    await loadItems()
  } finally {
    rowLoadingId.value = null
  }
}

const runPipeline = (row) => runRowAction(row, runMediaLibraryItemPipeline, '已重新执行入库流水线')

const generateStrm = (row) => runRowAction(row, generateMediaLibraryItemStrm, 'STRM 已生成或确认')

const refreshServer = (row) => runRowAction(row, refreshMediaLibraryItemServer, '媒体服务器刷新请求已发送')

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

.hero-actions {
  display: flex;
  gap: 10px;
}

.page-hero {
  justify-content: space-between;
  padding: 24px;
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
</style>
