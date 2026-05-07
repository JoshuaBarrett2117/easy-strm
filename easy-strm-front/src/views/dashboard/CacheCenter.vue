<template>
  <div class="cache-center">
    <section class="cache-hero">
      <div>
        <div class="page-kicker">Cache Center</div>
        <h2>缓存管理</h2>
        <p>集中查看识别、刮削和 Redis 热点缓存，并支持按类型手动清理。</p>
      </div>
      <div class="cache-hero-actions">
        <el-button :icon="Refresh" @click="loadOverview" :loading="loading">刷新概览</el-button>
        <el-button type="danger" plain @click="handleClear('all', '全部缓存')" :loading="clearingScope === 'all'">
          一键清理
        </el-button>
      </div>
    </section>

    <section class="cache-summary-grid">
      <div class="summary-card">
        <span>Redis 状态</span>
        <strong>{{ overview.redis_connected ? '已连接' : '未连接' }}</strong>
      </div>
      <div class="summary-card">
        <span>Redis Key 总数</span>
        <strong>{{ formatCount(overview.redis_total_keys) }}</strong>
      </div>
      <div class="summary-card">
        <span>Redis 内存</span>
        <strong>{{ overview.redis_memory || '-' }}</strong>
      </div>
      <div class="summary-card">
        <span>已纳管缓存</span>
        <strong>{{ overview.groups.length }}</strong>
      </div>
    </section>

    <section class="cache-panel">
      <el-table :data="overview.groups" v-loading="loading" stripe border>
        <el-table-column prop="name" label="缓存名称" min-width="160" />
        <el-table-column prop="description" label="说明" min-width="260" />
        <el-table-column label="存储位置" width="120">
          <template #default="{ row }">
            <el-tag :type="storageTagType(row.storage)">{{ storageLabel(row.storage) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="条目数" width="120">
          <template #default="{ row }">
            <strong>{{ formatCount(row.count) }}</strong>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="150">
          <template #default="{ row }">
            <el-button
              type="danger"
              link
              :disabled="row.count <= 0"
              :loading="clearingScope === row.key"
              @click="handleClear(row.key, row.name)"
            >
              清理
            </el-button>
          </template>
        </el-table-column>
      </el-table>
    </section>
  </div>
</template>

<script setup>
import { reactive, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { Refresh } from '@element-plus/icons-vue'
import { getCacheOverview, clearCacheGroup } from '../../utils/api/cache'
import { showConfirmDialog } from '../../utils/ui/messageBox'

const loading = ref(false)
const clearingScope = ref('')
const overview = reactive({
  redis_connected: false,
  redis_total_keys: 0,
  redis_memory: '',
  groups: []
})

const formatCount = (value) => {
  const parsed = Number(value || 0)
  return Number.isFinite(parsed) ? parsed.toLocaleString('zh-CN') : '0'
}

const storageLabel = (storage) => {
  return {
    redis: 'Redis',
    postgres: 'PostgreSQL',
    mixed: '混合'
  }[storage] || storage
}

const storageTagType = (storage) => {
  return {
    redis: 'danger',
    postgres: 'success',
    mixed: 'warning'
  }[storage] || 'info'
}

const loadOverview = async () => {
  loading.value = true
  try {
    const response = await getCacheOverview()
    const payload = response?.data?.data || {}
    overview.redis_connected = Boolean(payload.redis_connected)
    overview.redis_total_keys = Number(payload.redis_total_keys || 0)
    overview.redis_memory = payload.redis_memory || ''
    overview.groups = Array.isArray(payload.groups) ? payload.groups : []
  } catch (error) {
    ElMessage.error('加载缓存概览失败')
  } finally {
    loading.value = false
  }
}

const handleClear = async (scope, label) => {
  try {
    await showConfirmDialog(`确认清理${label}吗？此操作会直接删除对应缓存内容。`, '清理缓存', {
      confirmButtonText: '立即清理',
      cancelButtonText: '取消',
      type: 'warning'
    })
  } catch (error) {
    return
  }

  clearingScope.value = scope
  try {
    const response = await clearCacheGroup(scope)
    const deletedCount = response?.data?.data?.deleted_count ?? 0
    ElMessage.success(`缓存清理完成，共删除 ${formatCount(deletedCount)} 项`)
    await loadOverview()
  } catch (error) {
    ElMessage.error(error?.response?.data?.error || '清理缓存失败')
  } finally {
    clearingScope.value = ''
  }
}

loadOverview()
</script>

<style scoped>
.cache-center {
  display: flex;
  flex-direction: column;
  gap: 18px;
}

.cache-hero,
.cache-panel,
.summary-card {
  border-radius: 24px;
  background: rgba(255, 252, 247, 0.84);
  border: 1px solid rgba(120, 101, 72, 0.12);
  box-shadow: 0 24px 60px rgba(58, 42, 24, 0.08);
}

:global(.dark) .cache-hero,
:global(.dark) .cache-panel,
:global(.dark) .summary-card {
  background: rgba(14, 21, 32, 0.86);
  border-color: rgba(139, 163, 185, 0.12);
  box-shadow: 0 24px 60px rgba(0, 0, 0, 0.24);
}

.cache-hero,
.cache-panel {
  padding: 24px;
}

.cache-hero {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
}

.cache-hero-actions {
  display: flex;
  gap: 12px;
}

.page-kicker {
  color: #1f6f78;
  font-size: 12px;
  font-weight: 700;
  letter-spacing: 0.16em;
  text-transform: uppercase;
}

.cache-hero h2 {
  margin: 10px 0 6px;
  font-size: 30px;
}

.cache-hero p {
  color: #6f6457;
}

.cache-summary-grid {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 16px;
}

.summary-card {
  padding: 20px 22px;
}

.summary-card span {
  display: block;
  color: #6f6457;
  font-size: 13px;
}

.summary-card strong {
  display: block;
  margin-top: 8px;
  font-size: 28px;
  color: #1f2933;
}

:global(.dark) .summary-card span,
:global(.dark) .cache-hero p {
  color: #8fa1b5;
}

:global(.dark) .summary-card strong {
  color: #ebf2fa;
}

@media (max-width: 980px) {
  .cache-summary-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

@media (max-width: 860px) {
  .cache-hero {
    flex-direction: column;
    align-items: stretch;
  }

  .cache-hero-actions {
    width: 100%;
  }

  .cache-hero-actions :deep(.el-button) {
    flex: 1;
  }
}

@media (max-width: 640px) {
  .cache-summary-grid {
    grid-template-columns: 1fr;
  }
}
</style>
