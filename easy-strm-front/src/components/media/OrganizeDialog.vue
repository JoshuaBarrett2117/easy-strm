<template>
  <el-dialog
    v-model="visible"
    title="批量整理"
    :width="isMobile ? '100%' : '1000px'"
    :fullscreen="isMobile"
    :close-on-click-modal="false"
    destroy-on-close
    append-to-body
  >
    <div class="organize-container" v-loading="loading">
      <el-form :model="form" label-width="110px" class="organize-form">
        <el-form-item label="目标目录">
          <el-input v-model="form.target_path" placeholder="请输入整理后的目标目录" />
        </el-form-item>

        <el-form-item label="媒体类型">
          <el-select v-model="form.media_type" style="width: 100%">
            <el-option label="全部" value="all" />
            <el-option label="电影" value="movie" />
            <el-option label="剧集" value="tv" />
          </el-select>
        </el-form-item>

        <el-form-item label="冲突策略">
          <el-select v-model="form.conflict_policy" style="width: 100%">
            <el-option label="跳过" value="skip" />
            <el-option label="覆盖" value="overwrite" />
            <el-option label="追加序号" value="suffix" />
          </el-select>
        </el-form-item>

        <el-form-item label="重命名预设">
          <el-select
            v-model="form.preset_id"
            placeholder="选择预设模板（可选）"
            clearable
            style="width: 100%"
            @change="handlePresetChange"
          >
            <el-option
              v-for="preset in presets"
              :key="preset.id"
              :label="`${preset.name} (${preset.media_type === 'tv' ? '剧集' : '电影'})`"
              :value="preset.id"
            />
          </el-select>
          <div class="form-tip">选择预设后会自动填充重命名模板；留空则使用系统默认模板。</div>
        </el-form-item>

        <el-form-item label="整理方式">
          <el-select v-model="form.operation_mode" style="width: 100%">
            <el-option label="移动文件" value="move" />
            <el-option label="复制文件" value="copy" />
            <el-option label="硬链接" value="hardlink" :disabled="isCloud115" />
            <el-option label="软链接" value="symlink" :disabled="isCloud115" />
          </el-select>
          <div v-if="isCloud115" class="form-tip">115 云盘为远程存储，不支持硬链接和软链接。</div>
        </el-form-item>

        <el-form-item v-if="!isCloud115" label="刮削 NFO">
          <el-checkbox v-model="form.scrape_nfo">整理后同时刮削 NFO</el-checkbox>
          <div class="form-tip">整理完成后自动为成功的文件生成 NFO 信息。</div>
        </el-form-item>
      </el-form>

      <el-alert
        v-if="!hasPreview"
        :title="`已收集 ${candidateList.length} 个视频文件，请先点击“刷新预览”后再执行整理`"
        type="info"
        show-icon
        :closable="false"
        style="margin-bottom: 16px"
      />

      <el-alert
        v-if="hasPreview && summary"
        :title="`共 ${summary.total} 项，可处理 ${summary.processable || 0} 项，冲突 ${summary.conflicts || 0} 项，识别失败 ${summary.failed || 0} 项`"
        type="info"
        show-icon
        :closable="false"
        style="margin-bottom: 16px"
      />

      <el-alert
        v-if="hasPreview && manualCount > 0"
        :title="`已应用 ${manualCount} 项手动修正的识别结果，执行整理时将优先使用。`"
        type="warning"
        show-icon
        :closable="false"
        style="margin-bottom: 16px"
      />

      <el-table v-if="hasPreview && previewList.length > 0" :data="previewList" border stripe max-height="420">
        <el-table-column prop="file_name" label="原文件名" min-width="220" />
        <el-table-column prop="title" label="识别结果" min-width="180" />
        <el-table-column prop="new_name" label="新文件名" min-width="220">
          <template #default="scope">
            <div class="editable-cell">
              <el-input
                v-if="editingNewNameKey === getOverrideKey(scope.row)"
                v-model="scope.row.new_name"
                size="small"
                @blur="handleConfirmEditNewName(scope.row)"
                @keyup.enter="handleConfirmEditNewName(scope.row)"
              />
              <template v-else>
                <span>{{ scope.row.new_name }}</span>
                <el-button
                  size="small"
                  link
                  type="primary"
                  @click="handleStartEditNewName(scope.row)"
                  class="edit-name-btn"
                >
                  <el-icon><Edit /></el-icon>
                </el-button>
              </template>
            </div>
          </template>
        </el-table-column>
        <el-table-column prop="new_path" label="目标路径" min-width="260" show-overflow-tooltip />
        <el-table-column label="状态" width="140" align="center">
          <template #default="scope">
            <el-tag v-if="scope.row.identify_error" type="danger">识别失败</el-tag>
            <el-tag v-else-if="scope.row.conflict" type="warning">存在冲突</el-tag>
            <el-tag v-else type="success">可执行</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="120" align="center">
          <template #default="scope">
            <el-button size="small" @click="emit('preview-identify', scope.row)">手动识别</el-button>
          </template>
        </el-table-column>
      </el-table>

      <el-table v-else-if="candidateList.length > 0" :data="candidateList" border stripe max-height="420">
        <el-table-column prop="file_name" label="候选视频文件" min-width="240" />
        <el-table-column prop="file_path" label="源路径" min-width="320" show-overflow-tooltip />
      </el-table>

      <el-empty v-else description="暂无可显示内容" />
    </div>

    <template #footer>
      <span class="dialog-footer">
        <el-button @click="visible = false">取消</el-button>
        <el-button @click="handlePreview" :loading="previewLoading">刷新预览</el-button>
        <el-button type="primary" @click="handleExecute" :loading="executing">执行整理</el-button>
      </span>
    </template>
  </el-dialog>
</template>

<script setup>
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { Edit } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import {
  listOrganizeCandidates,
  executeOrganize,
  getOrganizePresets,
  scrapeFiles,
  startPreviewTaskAsync,
  getPreviewTaskStatus
} from '../../utils/api/media'
import { refreshEmbyLibrary } from '../../utils/api/emby'

const PREVIEW_POLL_INTERVAL_MS = 1000
const PREVIEW_POLL_TIMEOUT_MS = 120000

const props = defineProps({
  currentSource: {
    type: Object,
    default: null
  },
  currentPath: {
    type: String,
    default: '/'
  },
  isCloud115: {
    type: Boolean,
    default: false
  },
  currentDisplayPath: {
    type: String,
    default: '/'
  }
})

const emit = defineEmits(['execute-success', 'preview-identify', 'open-tmdb-search'])

const visible = defineModel('visible', { type: Boolean, default: false })
const isMobile = ref(window.innerWidth < 768)
let resizeTimer = null
let previewPollToken = 0

const loading = ref(false)
const previewLoading = ref(false)
const executing = ref(false)
const candidateList = ref([])
const previewList = ref([])
const hasPreview = ref(false)
const summary = ref(null)
const presets = ref([])
const manualOverrides = ref({})
const editingNewNameKey = ref('')
const currentTaskId = ref(null)
const selectedFileIds = ref([])

const form = ref({
  target_path: '',
  media_type: 'all',
  conflict_policy: 'skip',
  operation_mode: 'move',
  scrape_nfo: false,
  template: '',
  preset_id: null
})

const manualCount = computed(() => Object.keys(manualOverrides.value).length)

const getPayload = (response) => response?.data?.data || {}
const sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms))
const getTaskPayload = (response) => {
  const payload = getPayload(response)
  if (payload?.task_id || payload?.status) {
    return payload
  }
  return payload?.data || null
}

const stopPreviewPolling = () => {
  previewPollToken += 1
  currentTaskId.value = null
  previewLoading.value = false
}

const resetTransientState = () => {
  stopPreviewPolling()
  loading.value = false
  executing.value = false
  editingNewNameKey.value = ''
}

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
  stopPreviewPolling()
})

watch(visible, (value) => {
  if (!value) {
    resetTransientState()
  }
})

const getOverrideKey = (row) => {
  if (!row) return ''
  return row.cloud_id || row.file_id || row.cloudID || row.fileID || row.id || ''
}

const buildManualItems = () => Object.values(manualOverrides.value)

const buildPayload = () => ({
  source_id: props.currentSource.id,
  source_path: props.currentPath === '/' ? '' : props.currentPath,
  target_path: form.value.target_path,
  media_type: form.value.media_type,
  template: form.value.template || '',
  conflict_policy: form.value.conflict_policy,
  operation_mode: form.value.operation_mode,
  use_category: true,
  file_ids: selectedFileIds.value,
  manual_items: buildManualItems()
})

const handleLoadCandidates = async () => {
  loading.value = true
  try {
    const response = await listOrganizeCandidates({
      source_id: props.currentSource.id,
      source_path: props.currentPath === '/' ? '' : props.currentPath,
      media_type: form.value.media_type,
      file_ids: selectedFileIds.value
    })
    const payload = getPayload(response)
    candidateList.value = payload.data || []
    hasPreview.value = false
    if (candidateList.value.length === 0) {
      ElMessage.warning('当前选择范围内没有可整理的视频文件')
    }
  } catch (error) {
    console.error('[OrganizeDialog] 加载整理候选文件失败:', error)
    const errorMsg = error.response?.data?.error || error.message || '加载整理候选文件失败'
    ElMessage.error(errorMsg)
  } finally {
    loading.value = false
  }
}

const pollTaskStatus = async (taskId, activePollToken) => {
  const startedAt = Date.now()

  while (previewPollToken === activePollToken && visible.value && currentTaskId.value === taskId) {
    if (Date.now() - startedAt >= PREVIEW_POLL_TIMEOUT_MS) {
      ElMessage.warning('预览任务超时，请重新刷新预览')
      break
    }

    await sleep(PREVIEW_POLL_INTERVAL_MS)
    if (previewPollToken !== activePollToken || !visible.value || currentTaskId.value !== taskId) {
      return
    }

    try {
      const statusResponse = await getPreviewTaskStatus(taskId)
      const task = getTaskPayload(statusResponse)
      if (!task) {
        continue
      }

      const normalizedStatus = String(task.status || '').trim().toLowerCase()
      if (normalizedStatus === 'completed') {
        previewList.value = task.result?.previews || []
        summary.value = task.result?.summary || null
        hasPreview.value = true
        ElMessage.success('预览完成')
        break
      }

      if (normalizedStatus === 'failed') {
        ElMessage.error(task.error || '预览任务失败')
        break
      }
    } catch (error) {
      console.error('[OrganizeDialog] 获取预览任务状态失败:', error)
      if (error?.response?.status === 404) {
        ElMessage.error('预览任务不存在或已过期，请重新刷新预览')
        break
      }
    }
  }

  if (previewPollToken === activePollToken && currentTaskId.value === taskId) {
    currentTaskId.value = null
    previewLoading.value = false
  }
}

const handlePreview = async () => {
  if (!form.value.target_path.trim()) {
    ElMessage.warning('请输入目标目录')
    return
  }

  stopPreviewPolling()
  previewLoading.value = true
  hasPreview.value = false
  summary.value = null
  previewList.value = []

  try {
    const response = await startPreviewTaskAsync(buildPayload())
    const taskId = getPayload(response)?.task_id
    if (!taskId) {
      ElMessage.error('启动预览任务失败')
      previewLoading.value = false
      return
    }

    currentTaskId.value = taskId
    const activePollToken = previewPollToken
    await pollTaskStatus(taskId, activePollToken)
  } catch (error) {
    console.error('[OrganizeDialog] 预览整理失败:', error)
    const errorMsg = error.response?.data?.error || error.message || '预览整理失败'
    ElMessage.error(errorMsg)
    previewLoading.value = false
    currentTaskId.value = null
  }
}

const handleExecute = async () => {
  if (!form.value.target_path.trim()) {
    ElMessage.warning('请输入目标目录')
    return
  }
  if (!hasPreview.value) {
    ElMessage.warning('请先刷新预览，确认识别结果后再执行整理')
    return
  }

  executing.value = true
  try {
    const response = await executeOrganize(buildPayload())
    const payload = getPayload(response)
    const execSummary = payload.summary || {}
    ElMessage.success(`整理完成：成功 ${execSummary.success || 0}，跳过 ${execSummary.skipped || 0}，失败 ${execSummary.failed || 0}`)
    visible.value = false

    if (form.value.scrape_nfo) {
      const successItems = (payload.data || []).filter((item) => item.success && item.new_path)
      if (successItems.length > 0) {
        try {
          const scrapeResponse = await scrapeFiles({
            source_id: props.currentSource.id,
            file_paths: successItems.map((item) => item.new_path)
          })
          const scrapeData = scrapeResponse?.data?.data
          const scrapeSuccess = scrapeData?.success || 0
          const scrapeFailed = scrapeData?.failed || 0
          ElMessage.info(`NFO 刮削完成：成功 ${scrapeSuccess} 项，失败 ${scrapeFailed} 项`)
        } catch (scrapeError) {
          console.error('[OrganizeDialog] 整理后自动刮削失败:', scrapeError)
          const scrapeErrorMsg = scrapeError.response?.data?.error || scrapeError.message || '自动刮削失败'
          ElMessage.warning(`NFO 自动刮削失败：${scrapeErrorMsg}`)
        }
      }
    }

    if (props.currentSource?.emby_library_id && (execSummary.success || 0) > 0) {
      try {
        await refreshEmbyLibrary(props.currentSource.emby_library_id)
        ElMessage.info('已自动触发 Emby 媒体库刷新')
      } catch (embyError) {
        console.error('[OrganizeDialog] 自动刷新 Emby 媒体库失败:', embyError)
        ElMessage.warning('Emby 自动刷新失败')
      }
    }

    emit('execute-success', {
      summary: execSummary,
      resultList: payload.data || []
    })
  } catch (error) {
    console.error('[OrganizeDialog] 执行整理失败:', error)
    const errorMsg = error.response?.data?.error || error.message || '执行整理失败'
    ElMessage.error(errorMsg)
  } finally {
    executing.value = false
  }
}

const fetchPresets = async () => {
  try {
    const mediaType = form.value.media_type === 'all' ? '' : form.value.media_type
    const response = await getOrganizePresets({ media_type: mediaType })
    const apiData = getPayload(response)
    presets.value = apiData.data || []
  } catch (error) {
    console.error('[OrganizeDialog] 加载预设列表失败:', error)
    presets.value = []
  }
}

const handlePresetChange = (presetId) => {
  if (!presetId) {
    form.value.template = ''
    return
  }
  const preset = presets.value.find((item) => item.id === presetId)
  if (preset) {
    form.value.template = preset.template
  }
}

const handleStartEditNewName = (row) => {
  editingNewNameKey.value = getOverrideKey(row)
}

const handleConfirmEditNewName = (row) => {
  editingNewNameKey.value = ''
  if (!row.new_name || !row.new_name.trim()) {
    ElMessage.warning('新文件名不能为空')
    return
  }
  if (row.new_path) {
    const lastSep = Math.max(row.new_path.lastIndexOf('/'), row.new_path.lastIndexOf('\\'))
    row.new_path = lastSep >= 0 ? `${row.new_path.substring(0, lastSep + 1)}${row.new_name}` : row.new_name
  }
}

const applyManualOverride = async (overrideForm) => {
  const key = overrideForm.override_key || overrideForm.cloud_id || overrideForm.file_id
  manualOverrides.value = {
    ...manualOverrides.value,
    [key]: {
      file_id: overrideForm.file_id,
      cloud_id: overrideForm.cloud_id,
      media_type: overrideForm.media_type,
      tmdb_id: Number(overrideForm.tmdb_id || 0),
      title: overrideForm.title.trim(),
      year: Number(overrideForm.year || 0),
      season: Number(overrideForm.season || 0),
      episode: Number(overrideForm.episode || 0)
    }
  }
  await handlePreview()
}

const clearManualOverride = async (key) => {
  if (!key) return
  const next = { ...manualOverrides.value }
  delete next[key]
  manualOverrides.value = next
  await handlePreview()
}

const retryFailedItems = async (failedItems) => {
  executing.value = true
  try {
    const payload = {
      ...buildPayload(),
      file_ids: failedItems.map((item) => item.file_id || item.cloud_id).filter(Boolean)
    }
    const response = await executeOrganize(payload)
    const resultPayload = getPayload(response)
    const execSummary = resultPayload.summary || {}
    ElMessage.success(`重试完成：成功 ${execSummary.success || 0}，跳过 ${execSummary.skipped || 0}，失败 ${execSummary.failed || 0}`)
    return {
      summary: execSummary,
      resultList: resultPayload.data || []
    }
  } catch (error) {
    console.error('[OrganizeDialog] 重试失败项失败:', error)
    const errorMsg = error.response?.data?.error || error.message || '重试失败项失败'
    ElMessage.error(errorMsg)
    return null
  } finally {
    executing.value = false
  }
}

const open = async (fileIds, source) => {
  const defaultTargetPath = source?.organize_target_path || (props.isCloud115 ? props.currentDisplayPath : (source?.path || ''))
  const defaultOperationMode = source?.operation_mode || 'move'
  resetTransientState()
  form.value = {
    target_path: defaultTargetPath,
    media_type: source?.media_type || 'all',
    conflict_policy: source?.conflict_policy || 'skip',
    operation_mode: props.isCloud115 && !['move', 'copy'].includes(defaultOperationMode) ? 'move' : defaultOperationMode,
    scrape_nfo: false,
    template: '',
    preset_id: null
  }
  candidateList.value = []
  previewList.value = []
  hasPreview.value = false
  summary.value = null
  manualOverrides.value = {}
  selectedFileIds.value = fileIds
  visible.value = true
  await handleLoadCandidates()
  fetchPresets()
}

defineExpose({
  open,
  handlePreview,
  applyManualOverride,
  clearManualOverride,
  retryFailedItems,
  manualOverrides,
  form,
  previewList,
  hasPreview,
  summary,
  getOverrideKey
})
</script>

<style scoped>
.form-tip {
  font-size: 12px;
  color: #909399;
  line-height: 1.4;
  margin-top: 4px;
}

.organize-container {
  min-height: 320px;
}

.organize-form {
  margin-bottom: 16px;
}

.dialog-footer {
  display: flex;
  justify-content: flex-end;
  gap: 10px;
}

.editable-cell {
  display: flex;
  align-items: center;
  gap: 4px;
}

.edit-name-btn {
  flex-shrink: 0;
  opacity: 0;
  transition: opacity 0.2s;
}

.editable-cell:hover .edit-name-btn {
  opacity: 1;
}
</style>
