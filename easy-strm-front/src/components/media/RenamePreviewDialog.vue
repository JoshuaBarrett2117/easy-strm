<template>
  <el-dialog
    v-model="visible"
    title="重命名预览"
    :width="isMobile ? '100%' : '760px'"
    append-to-body
  >
    <div class="rename-container" v-loading="loading">
      <div class="table-shell">
        <el-table :data="previewList" border style="width: 100%" stripe>
          <el-table-column prop="original_name" label="原文件名" min-width="300" />
          <el-table-column width="50" align="center">
            <template #default>
              <el-icon><Right /></el-icon>
            </template>
          </el-table-column>
          <el-table-column prop="new_name" label="新文件名" min-width="300" />
        </el-table>
      </div>
    </div>
    <template #footer>
      <span class="dialog-footer">
        <el-button @click="visible = false">取消</el-button>
        <el-button type="primary" @click="handleExecute" :loading="loading">执行重命名</el-button>
      </span>
    </template>
  </el-dialog>
</template>

<script setup>
import { ref, onMounted, onUnmounted } from 'vue'
import { Right } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import { batchExecuteRename } from '../../utils/api/media'

const props = defineProps({
  previewList: {
    type: Array,
    default: () => []
  },
  sourceId: {
    type: Number,
    default: null
  }
})

const emit = defineEmits(['execute-success'])

const visible = defineModel('visible', { type: Boolean, default: false })
const loading = defineModel('loading', { type: Boolean, default: false })
const isMobile = ref(window.innerWidth < 768)
let resizeTimer = null

const handleResize = () => {
  clearTimeout(resizeTimer)
  resizeTimer = setTimeout(() => {
    isMobile.value = window.innerWidth < 768
  }, 150)
}

onMounted(() => {
  window.addEventListener('resize', handleResize)
})

onUnmounted(() => {
  window.removeEventListener('resize', handleResize)
  if (resizeTimer) {
    clearTimeout(resizeTimer)
  }
})

const handleExecute = async () => {
  loading.value = true
  try {
    const items = props.previewList.map(item => ({
      source_id: props.sourceId,
      file_id: item.file_id,
      new_name: item.new_name
    }))

    const response = await batchExecuteRename({ items })
    const data = response.data.data || response.data || {}
    const successCount = data.success || 0
    const failedCount = data.failed || 0
    ElMessage.success(`批量重命名完成：成功 ${successCount} 项，失败 ${failedCount} 项`)
    visible.value = false
    emit('execute-success')
  } catch (error) {
    console.error('[RenamePreviewDialog] 批量重命名失败:', error)
    const errorMsg = error.response?.data?.error || error.message || '批量重命名失败'
    ElMessage.error(errorMsg)
  } finally {
    loading.value = false
  }
}
</script>

<style scoped>
.rename-container {
  max-height: 500px;
  overflow-y: auto;
}

.table-shell {
  overflow-x: auto;
}

.dialog-footer {
  display: flex;
  justify-content: flex-end;
  gap: 10px;
}

@media (max-width: 768px) {
  .rename-container {
    max-height: none;
  }
}
</style>
