<template>
  <el-dialog
    v-model="visible"
    title="批量整理"
    :width="isMobile ? '100%' : '1000px'"
    :fullscreen="isMobile"
    :close-on-click-modal="false"
    :before-close="handleDialogBeforeClose"
    destroy-on-close
    append-to-body
  >
    <div class="organize-container" v-loading="loading" :element-loading-text="candidateLoadingText">
      <section class="organize-overview">
        <div class="organize-overview__copy">
          <h3>批量整理工作流</h3>
          <p>先设定目标目录与整理策略；可先刷新预览确认识别结果，也可直接提交后台整理任务并在任务中心查看进度。</p>
        </div>
        <div class="organize-overview__grid">
          <article class="overview-chip">
            <span>候选文件</span>
            <strong>{{ candidateList.length }}</strong>
          </article>
          <article class="overview-chip">
            <span>手动修正</span>
            <strong>{{ manualCount }}</strong>
          </article>
          <article class="overview-chip">
            <span>预览状态</span>
            <strong>{{ hasPreview ? '已生成' : '待刷新' }}</strong>
          </article>
        </div>
      </section>

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
          <div class="form-tip">整理任务在后台执行，NFO 刮削按后端全局配置自动处理。</div>
        </el-form-item>
      </el-form>

      <el-alert
        v-if="!hasPreview"
        :title="`已收集 ${candidateList.length} 个视频文件。你可以先刷新预览确认识别结果，也可以直接执行整理并在任务中心查看进度。`"
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
        <el-table-column prop="title" label="识别结果" min-width="180">
          <template #default="scope">
            <div class="identify-result-cell">
              <span>{{ scope.row.title || '-' }}</span>
              <el-tag v-if="scope.row.manual_override" type="warning" size="small" effect="light">已手动修正</el-tag>
            </div>
          </template>
        </el-table-column>
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
            <el-tag v-if="updatingPreviewRowKey === getOverrideKey(scope.row)" type="info">更新中</el-tag>
            <el-tag v-else-if="scope.row.identify_error" type="danger">识别失败</el-tag>
            <el-tag v-else-if="scope.row.manual_override" type="warning">已修正</el-tag>
            <el-tag v-else-if="scope.row.conflict" type="warning">存在冲突</el-tag>
            <el-tag v-else type="success">可执行</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="120" align="center">
          <template #default="scope">
            <el-button
              size="small"
              :type="scope.row.manual_override ? 'warning' : 'default'"
              :loading="updatingPreviewRowKey === getOverrideKey(scope.row)"
              @click="emit('preview-identify', scope.row)"
            >
              {{ scope.row.manual_override ? '重新修正' : '手动识别' }}
            </el-button>
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
        <el-button @click="handleCloseDialog">取消</el-button>
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
  executeOrganizeAsync,
  getOrganizePresets,
  previewOrganize,
  startPreviewTaskAsync,
  getPreviewTaskStatus,
  startOrganizeCandidatesTaskAsync,
  getOrganizeCandidatesTaskStatus
} from '../../utils/api/media'

const CANDIDATE_POLL_INTERVAL_MS = 1000
const CANDIDATE_POLL_TIMEOUT_MS = 120000
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
let candidatePollToken = 0
let previewPollToken = 0

const loading = ref(false)
const candidateLoadingText = ref('正在加载候选文件...')
const previewLoading = ref(false)
const executing = ref(false)
const candidateList = ref([])
const previewList = ref([])
const previewBaseList = ref([])
const hasPreview = ref(false)
const summary = ref(null)
const presets = ref([])
const manualOverrides = ref({})
const renameOverrides = ref({})
const editingNewNameKey = ref('')
const updatingPreviewRowKey = ref('')
const currentTaskId = ref(null)
const currentCandidateTaskId = ref(null)
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

const stopCandidatePolling = () => {
  candidatePollToken += 1
  currentCandidateTaskId.value = null
  loading.value = false
  candidateLoadingText.value = '正在加载候选文件...'
}

const resetTransientState = () => {
  stopCandidatePolling()
  stopPreviewPolling()
  loading.value = false
  executing.value = false
  editingNewNameKey.value = ''
  updatingPreviewRowKey.value = ''
}

const handleCloseDialog = () => {
  resetTransientState()
  visible.value = false
}

const handleDialogBeforeClose = (done) => {
  resetTransientState()
  done()
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
  stopCandidatePolling()
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
const buildRenameItems = () => Object.values(renameOverrides.value)
const clonePreviewItems = (items) => items.map(item => ({ ...item }))

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
  manual_items: buildManualItems(),
  rename_items: buildRenameItems()
})

const findPreviewRowIndex = (key) => previewList.value.findIndex(item => getOverrideKey(item) === key)
const recalculatePreviewSummary = () => {
  const nextSummary = {
    total: previewList.value.length,
    conflicts: 0,
    failed: 0,
    processable: 0
  }

  previewList.value.forEach((item) => {
    if (item.identify_error) {
      nextSummary.failed += 1
      return
    }
    if (item.conflict) {
      nextSummary.conflicts += 1
    }
    nextSummary.processable += 1
  })

  summary.value = nextSummary
}

const applyLocalManualOverrideToPreview = (key) => {
  if (!key) return
  const index = findPreviewRowIndex(key)
  if (index < 0) return

  const override = manualOverrides.value[key]
  const baseRow = previewBaseList.value.find(item => getOverrideKey(item) === key) || previewList.value[index]
  const nextRow = {
    ...baseRow,
    ...(override || {})
  }

  if (override) {
    nextRow.identify_error = ''
    nextRow.manual_override = true
  } else {
    nextRow.manual_override = false
  }

  previewList.value.splice(index, 1, nextRow)
  recalculatePreviewSummary()
}

const applyLocalRenameOverrideToRow = (row) => {
  const key = getOverrideKey(row)
  const renameOverride = key ? renameOverrides.value[key] : null
  if (!renameOverride?.new_name) {
    return row
  }

  const nextRow = {
    ...row,
    new_name: renameOverride.new_name.trim()
  }

  if (nextRow.new_path) {
    const separators = ['/', '\\']
    let lastSep = -1
    for (const separator of separators) {
      lastSep = Math.max(lastSep, nextRow.new_path.lastIndexOf(separator))
    }
    nextRow.new_path = lastSep >= 0
      ? `${nextRow.new_path.substring(0, lastSep + 1)}${nextRow.new_name}`
      : nextRow.new_name
  }

  return nextRow
}

const refreshSinglePreviewRow = async (key) => {
  if (!key || !props.currentSource?.id) return false

  const row = previewList.value.find(item => getOverrideKey(item) === key)
  if (!row) return false

  updatingPreviewRowKey.value = key
  try {
    const response = await previewOrganize({
      ...buildPayload(),
      file_ids: [row.file_id || row.cloud_id].filter(Boolean)
    })
    const payload = getPayload(response)
    const nextRow = Array.isArray(payload?.data) ? payload.data[0] : null
    if (!nextRow) return false

    const hasManualOverride = Boolean(manualOverrides.value[key])
    const normalizedRow = {
      ...applyLocalRenameOverrideToRow(nextRow),
      manual_override: hasManualOverride,
      identify_error: hasManualOverride ? '' : nextRow.identify_error
    }
    const index = findPreviewRowIndex(key)
    if (index < 0) return false

    previewBaseList.value.splice(index, 1, { ...nextRow })
    previewList.value.splice(index, 1, normalizedRow)
    recalculatePreviewSummary()
    return true
  } finally {
    updatingPreviewRowKey.value = ''
  }
}

const handleLoadCandidates = async () => {
  stopCandidatePolling()
  candidateLoadingText.value = '正在启动候选扫描...'
  loading.value = true
  try {
    const response = await startOrganizeCandidatesTaskAsync({
      source_id: props.currentSource.id,
      source_path: props.currentPath === '/' ? '' : props.currentPath,
      media_type: form.value.media_type,
      file_ids: selectedFileIds.value
    })

    const taskId = getPayload(response)?.task_id
    if (!taskId) {
      ElMessage.error('启动候选扫描任务失败')
      return
    }

    currentCandidateTaskId.value = taskId
    const activePollToken = candidatePollToken
    const startedAt = Date.now()

    while (candidatePollToken === activePollToken && visible.value && currentCandidateTaskId.value === taskId) {
      if (Date.now() - startedAt >= CANDIDATE_POLL_TIMEOUT_MS) {
        ElMessage.warning('候选扫描超时，请重试')
        break
      }

      await sleep(CANDIDATE_POLL_INTERVAL_MS)
      if (candidatePollToken !== activePollToken || !visible.value || currentCandidateTaskId.value !== taskId) {
        return
      }

      try {
        const statusResponse = await getOrganizeCandidatesTaskStatus(taskId)
        const task = getTaskPayload(statusResponse)
        if (!task) {
          continue
        }

        const normalizedStatus = String(task.status || '').trim().toLowerCase()
        const progress = task.progress || {}
        if (progress.current_file) {
          const total = Number(progress.total || 0)
          const processed = Number(progress.processed || 0)
          candidateLoadingText.value = total > 0
            ? `${progress.current_file}（${processed}/${total}）`
            : progress.current_file
        }

        if (normalizedStatus === 'completed') {
          candidateList.value = Array.isArray(task.result) ? task.result : []
          selectedFileIds.value = candidateList.value
            .map(item => item.file_id || item.cloud_id)
            .filter(Boolean)
          hasPreview.value = false
          if (candidateList.value.length === 0) {
            ElMessage.warning('当前选择范围内没有可整理的视频文件')
          }
          break
        }

        if (normalizedStatus === 'failed') {
          ElMessage.error(task.error || '候选扫描任务失败')
          break
        }
      } catch (pollError) {
        console.error('[OrganizeDialog] 获取候选扫描任务状态失败:', pollError)
        if (pollError?.response?.status === 404) {
          ElMessage.error('候选扫描任务不存在或已过期，请重新加载')
          break
        }
      }
    }
  } catch (error) {
    console.error('[OrganizeDialog] 加载整理候选文件失败:', error)
    candidateLoadingText.value = '候选扫描失败'
    const errorMsg = error.response?.data?.error || error.message || '加载整理候选文件失败'
    ElMessage.error(errorMsg)
  } finally {
    currentCandidateTaskId.value = null
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
        previewBaseList.value = clonePreviewItems(task.result?.previews || [])
        previewList.value = clonePreviewItems(task.result?.previews || [])
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
  previewBaseList.value = []
  renameOverrides.value = {}

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

  executing.value = true
  try {
    const response = await executeOrganizeAsync(buildPayload())
    const payload = getPayload(response)
    const taskId = payload.task_id
    ElMessage.success(taskId ? `整理任务已提交，可在任务中心查看进度：${taskId}` : '整理任务已提交，可在任务中心查看进度')
    handleCloseDialog()
    emit('execute-success', {
      async: true,
      taskId
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
  const key = getOverrideKey(row)
  if (key) {
    renameOverrides.value = {
      ...renameOverrides.value,
      [key]: {
        file_id: row.file_id || '',
        cloud_id: row.cloud_id || '',
        new_name: row.new_name.trim()
      }
    }
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
      original_title: (overrideForm.original_title || '').trim(),
      year: Number(overrideForm.year || 0),
      season: Number(overrideForm.season || 0),
      episode: Number(overrideForm.episode || 0)
    }
  }
  try {
    const refreshed = await refreshSinglePreviewRow(key)
    if (!refreshed) {
      applyLocalManualOverrideToPreview(key)
    }
  } catch (error) {
    console.error('[OrganizeDialog] 手动识别后同步刷新单行预览失败:', error)
    applyLocalManualOverrideToPreview(key)
  }
  ElMessage.success('当前行已更新到预览，无需重新刷新整批预览')
}

const clearManualOverride = async (key) => {
  if (!key) return
  const next = { ...manualOverrides.value }
  delete next[key]
  manualOverrides.value = next
  try {
    const refreshed = await refreshSinglePreviewRow(key)
    if (!refreshed) {
      applyLocalManualOverrideToPreview(key)
    }
  } catch (error) {
    console.error('[OrganizeDialog] 清除手动识别后同步刷新单行预览失败:', error)
    applyLocalManualOverrideToPreview(key)
  }
  ElMessage.success('当前行已恢复自动识别结果，无需重新刷新整批预览')
}

const retryFailedItems = async (failedItems) => {
  executing.value = true
  try {
    const payload = {
      ...buildPayload(),
      file_ids: failedItems.map((item) => item.file_id || item.cloud_id).filter(Boolean)
    }
    const response = await executeOrganizeAsync(payload)
    const resultPayload = getPayload(response)
    const taskId = resultPayload.task_id
    ElMessage.success(taskId ? `重试任务已提交：${taskId}` : '重试任务已提交，可在任务中心查看进度')
    return {
      async: true,
      taskId
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
  previewBaseList.value = []
  hasPreview.value = false
  summary.value = null
  manualOverrides.value = {}
  renameOverrides.value = {}
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

.organize-overview {
  display: grid;
  grid-template-columns: minmax(260px, 1fr) minmax(0, 1.4fr);
  gap: 16px;
  margin-bottom: 16px;
}

.organize-overview__copy {
  padding: 18px;
  border-radius: 18px;
  background: linear-gradient(160deg, rgba(31, 111, 120, 0.12), rgba(242, 166, 90, 0.12));
  border: 1px solid rgba(31, 111, 120, 0.12);
}

.organize-overview__copy h3 {
  margin: 0;
  color: #17313a;
  font-size: 20px;
}

.organize-overview__copy p {
  margin: 10px 0 0;
  color: #6c6259;
  line-height: 1.7;
}

.organize-overview__grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 12px;
}

.overview-chip {
  padding: 16px;
  border-radius: 18px;
  background: rgba(244, 239, 231, 0.88);
}

.overview-chip span {
  display: block;
  font-size: 12px;
  color: #8a7b6d;
}

.overview-chip strong {
  display: block;
  margin-top: 8px;
  font-size: 22px;
  color: #17313a;
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

.identify-result-cell {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
}

.edit-name-btn {
  flex-shrink: 0;
  opacity: 0;
  transition: opacity 0.2s;
}

.editable-cell:hover .edit-name-btn {
  opacity: 1;
}

:global(.dark) .organize-overview__copy,
:global(.dark) .overview-chip {
  background: rgba(16, 26, 37, 0.88);
  border-color: rgba(139, 163, 185, 0.12);
}

:global(.dark) .organize-overview__copy h3,
:global(.dark) .overview-chip strong {
  color: #e8edf4;
}

:global(.dark) .organize-overview__copy p,
:global(.dark) .overview-chip span,
:global(.dark) .form-tip {
  color: #9faebb;
}

@media (max-width: 768px) {
  .organize-overview,
  .organize-overview__grid {
    grid-template-columns: 1fr;
  }
}
</style>
