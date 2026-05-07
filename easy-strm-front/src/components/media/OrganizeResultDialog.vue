<template>
  <el-dialog
    v-model="visible"
    title="整理结果"
    width="900px"
    :close-on-click-modal="false"
    destroy-on-close
    append-to-body
  >
    <section class="result-overview">
      <div class="result-overview__copy">
        <h3>整理执行回执</h3>
        <p>这里展示本轮整理的成功、跳过与失败明细；后续还能继续重试失败项、刷新 Emby 或生成 STRM。</p>
      </div>
    </section>

    <div v-if="normalizedSummary" class="result-summary-panel">
      <div class="summary-item summary-total">
        <span class="summary-count">{{ normalizedSummary.total || 0 }}</span>
        <span class="summary-label">总计</span>
      </div>
      <div class="summary-item summary-success">
        <span class="summary-count">{{ normalizedSummary.success || 0 }}</span>
        <span class="summary-label">成功</span>
      </div>
      <div class="summary-item summary-skipped">
        <span class="summary-count">{{ normalizedSummary.skipped || 0 }}</span>
        <span class="summary-label">跳过</span>
      </div>
      <div class="summary-item summary-failed">
        <span class="summary-count">{{ normalizedSummary.failed || 0 }}</span>
        <span class="summary-label">失败</span>
      </div>
    </div>

    <el-table v-if="displayResultList.length > 0" :data="displayResultList" border stripe max-height="420">
      <el-table-column prop="file_name" label="文件名" min-width="220" />
      <el-table-column prop="message" label="结果" min-width="220" />
      <el-table-column prop="new_path" label="目标路径" min-width="280" show-overflow-tooltip />
      <el-table-column label="状态" width="120" align="center">
        <template #default="{ row }">
          <el-tag v-if="row.skipped" type="warning">已跳过</el-tag>
          <el-tag v-else-if="row.success" type="success">成功</el-tag>
          <el-tag v-else type="danger">失败</el-tag>
        </template>
      </el-table-column>
    </el-table>
    <el-empty v-else description="暂无整理结果" />

    <template #footer>
      <span class="dialog-footer">
        <el-button
          v-if="failedItems.length > 0"
          type="warning"
          @click="handleRetryFailed"
          :loading="executing"
        >
          <el-icon><RefreshRight /></el-icon>
          重试失败项 ({{ failedItems.length }})
        </el-button>
        <el-button
          v-if="canRefreshEmby"
          type="warning"
          @click="handleRefreshEmby"
          :loading="embyRefreshing"
        >
          <el-icon><RefreshRight /></el-icon>
          刷新 Emby
        </el-button>
        <el-button
          v-if="canGenerateStrm"
          type="success"
          @click="handleGenerateStrm"
          :loading="strmGenerating"
        >
          <el-icon><VideoCamera /></el-icon>
          生成 STRM
        </el-button>
        <el-button type="primary" @click="visible = false">关闭</el-button>
      </span>
    </template>
  </el-dialog>
</template>

<script setup>
import { computed, ref } from 'vue'
import { RefreshRight, VideoCamera } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import { generateStrmFromOrganize } from '../../utils/api/strm'
import { refreshEmbyLibrary } from '../../utils/api/emby'

const props = defineProps({
  summary: {
    type: Object,
    default: null
  },
  resultList: {
    type: [Array, Object],
    default: () => []
  },
  currentSource: {
    type: Object,
    default: null
  },
  executing: {
    type: Boolean,
    default: false
  }
})

const emit = defineEmits(['retry-failed', 'refresh-file-list'])

const visible = defineModel('visible', { type: Boolean, default: false })

const normalizedResultList = computed(() => {
  if (Array.isArray(props.resultList)) {
    return props.resultList
  }
  if (Array.isArray(props.resultList?.data)) {
    return props.resultList.data
  }
  return []
})

const normalizedSummary = computed(() => {
  if (props.summary && typeof props.summary === 'object') {
    return props.summary
  }
  if (props.resultList?.summary && typeof props.resultList.summary === 'object') {
    return props.resultList.summary
  }
  if (normalizedResultList.value.length === 0) {
    return null
  }
  return {
    total: normalizedResultList.value.length,
    success: normalizedResultList.value.filter(item => item.success).length,
    skipped: normalizedResultList.value.filter(item => item.skipped).length,
    failed: normalizedResultList.value.filter(item => !item.success && !item.skipped).length
  }
})

const displayResultList = computed(() => normalizedResultList.value.map(item => ({
  ...item,
  file_name: item.file_name || item.new_name || item.old_path?.split(/[\\/]/).pop() || item.new_path?.split(/[\\/]/).pop() || '-'
})))

const failedItems = computed(() => normalizedResultList.value.filter(item => !item.success && !item.skipped))
const canRefreshEmby = computed(() => Boolean(props.currentSource?.emby_library_id))
const canGenerateStrm = computed(() => {
  if (!props.currentSource || props.currentSource.source_type !== 'cloud115') {
    return false
  }
  return normalizedResultList.value.some(item => item.success)
})

const embyRefreshing = ref(false)
const strmGenerating = ref(false)
const getNestedData = (response) => response?.data?.data || response?.data || {}

const handleRetryFailed = () => {
  emit('retry-failed', failedItems.value)
}

const handleRefreshEmby = async () => {
  const libraryId = props.currentSource?.emby_library_id
  if (!libraryId) {
    ElMessage.warning('当前媒体源未绑定 Emby 媒体库')
    return
  }

  embyRefreshing.value = true
  try {
    const response = await refreshEmbyLibrary(libraryId)
    const payload = getNestedData(response)
    if (payload?.success === false) {
      ElMessage.error(payload.message || '刷新 Emby 媒体库失败')
      return
    }
    ElMessage.success('Emby 媒体库刷新已触发')
  } catch (error) {
    console.error('[OrganizeResultDialog] 刷新 Emby 媒体库失败:', error)
    const errorMsg = error.response?.data?.error || error.message || '刷新 Emby 媒体库失败'
    ElMessage.error(errorMsg)
  } finally {
    embyRefreshing.value = false
  }
}

const handleGenerateStrm = async () => {
  if (!props.currentSource) return

  const successItems = normalizedResultList.value.filter(item => item.success && item.new_path)
  if (successItems.length === 0) {
    ElMessage.warning('没有成功整理的文件可生成 STRM')
    return
  }

  const targetPaths = [...new Set(
    successItems
      .map(item => {
        const currentPath = item.new_path || ''
        const parts = currentPath.split('/').filter(Boolean)
        if (parts.length >= 2) {
          return `/${parts.slice(0, 2).join('/')}`
        }
        return currentPath
      })
      .filter(Boolean)
  )]

  strmGenerating.value = true
  try {
    const response = await generateStrmFromOrganize({
      source_id: props.currentSource.id,
      target_paths: targetPaths,
      strm_config_id: 0
    })
    const data = response.data.data || response.data || {}
    ElMessage.success(`STRM 生成任务已创建，配置 ID: ${data.strm_config_id || '自动匹配'}`)
    visible.value = false
  } catch (error) {
    console.error('[OrganizeResultDialog] 生成 STRM 失败:', error)
    const errorMsg = error.response?.data?.error || error.message || '生成 STRM 失败'
    ElMessage.error(errorMsg)
  } finally {
    strmGenerating.value = false
  }
}
</script>

<style scoped>
.result-overview {
  margin-bottom: 16px;
}

.result-overview__copy {
  padding: 18px;
  border-radius: 18px;
  background: linear-gradient(160deg, rgba(31, 111, 120, 0.12), rgba(242, 166, 90, 0.12));
  border: 1px solid rgba(31, 111, 120, 0.12);
}

.result-overview__copy h3 {
  margin: 0;
  font-size: 20px;
  color: #17313a;
}

.result-overview__copy p {
  margin: 10px 0 0;
  color: #6c6259;
  line-height: 1.7;
}

.result-summary-panel {
  display: flex;
  gap: 16px;
  margin-bottom: 16px;
  padding: 16px 20px;
  background: #f5f7fa;
  border-radius: 8px;
}

.summary-item {
  display: flex;
  flex-direction: column;
  align-items: center;
  min-width: 80px;
  padding: 8px 16px;
  border-radius: 6px;
}

.summary-item .summary-count {
  font-size: 28px;
  font-weight: 700;
  line-height: 1.2;
}

.summary-item .summary-label {
  font-size: 12px;
  margin-top: 4px;
  color: #909399;
}

.summary-total {
  background: rgba(144, 147, 153, 0.1);
}

.summary-total .summary-count {
  color: #606266;
}

.summary-success {
  background: rgba(103, 194, 58, 0.1);
}

.summary-success .summary-count {
  color: #67c23a;
}

.summary-skipped {
  background: rgba(230, 162, 60, 0.1);
}

.summary-skipped .summary-count {
  color: #e6a23c;
}

.summary-failed {
  background: rgba(245, 108, 108, 0.1);
}

.summary-failed .summary-count {
  color: #f56c6c;
}

.dialog-footer {
  display: flex;
  justify-content: flex-end;
  gap: 10px;
}

:global(.dark) .result-overview__copy,
:global(.dark) .result-summary-panel {
  background: rgba(16, 26, 37, 0.88);
  border-color: rgba(139, 163, 185, 0.12);
}

:global(.dark) .result-overview__copy h3 {
  color: #e8edf4;
}

:global(.dark) .result-overview__copy p,
:global(.dark) .summary-item .summary-label {
  color: #9faebb;
}

@media (max-width: 768px) {
  .result-summary-panel {
    flex-wrap: wrap;
  }
}
</style>
