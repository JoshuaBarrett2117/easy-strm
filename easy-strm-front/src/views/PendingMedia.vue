<template>
  <div class="pending-page">
    <section class="page-hero">
      <div>
        <div class="page-kicker">Pending</div>
        <h2>待处理</h2>
        <p>集中处理识别失败、人工修正、重新入库和忽略项。</p>
      </div>
      <el-button :icon="Refresh" @click="loadItems" :loading="loading">刷新</el-button>
    </section>

    <section class="filter-bar">
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

    <section class="table-panel">
      <el-table :data="items" v-loading="loading" border>
        <el-table-column prop="title" label="标题" min-width="180" />
        <el-table-column prop="source_path" label="来源路径" min-width="260" show-overflow-tooltip />
        <el-table-column prop="media_type" label="类型" width="100" />
        <el-table-column prop="year" label="年份" width="90" />
        <el-table-column prop="status" label="状态" width="110" />
        <el-table-column prop="reason" label="原因" min-width="180" show-overflow-tooltip />
        <el-table-column label="操作" width="190" fixed="right">
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
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { onMounted, reactive, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { Refresh } from '@element-plus/icons-vue'
import { getPendingMediaItems, identifyPendingMediaItem, ignorePendingMediaItem, runPendingMediaItem } from '../utils/api/media'

const loading = ref(false)
const items = ref([])
const identifyVisible = ref(false)
const activeItem = ref(null)
const rowLoadingId = ref(null)
const filters = reactive({
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

const loadItems = async () => {
  loading.value = true
  try {
    const response = await getPendingMediaItems(filters)
    const payload = response.data?.data?.data || response.data?.data || []
    items.value = Array.isArray(payload) ? payload : []
  } finally {
    loading.value = false
  }
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

const runItem = async (row) => {
  rowLoadingId.value = row.id
  try {
    await runPendingMediaItem(row.id)
    ElMessage.success('已重新入库')
    await loadItems()
  } finally {
    rowLoadingId.value = null
  }
}

const ignoreItem = async (row) => {
  await ignorePendingMediaItem(row.id)
  ElMessage.success('已忽略')
  await loadItems()
}

onMounted(loadItems)
</script>

<style scoped>
.pending-page {
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
