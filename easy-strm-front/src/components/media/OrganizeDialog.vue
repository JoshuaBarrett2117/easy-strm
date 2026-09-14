<template>
  <n-modal
    v-model:show="visible"
    preset="card"
    title="批量整理"
    class="w-[96vw] max-w-[1000px]"
    :mask-closable="false"
  >
    <n-spin :show="loading" :description="candidateLoadingText">
      <div class="min-h-[320px]">
        <section class="mb-4 grid gap-4 lg:grid-cols-[minmax(260px,1fr)_minmax(0,1.4fr)]">
          <div class="rounded-2xl border border-cyan-500/10 bg-gradient-to-br from-cyan-500/10 to-amber-400/10 p-4 lg:p-5">
            <h3 class="text-lg font-bold text-slate-800 dark:text-white">批量整理工作流</h3>
            <p class="mt-2 text-sm leading-relaxed text-slate-500 dark:text-slate-400">
              先设定目标目录与整理策略；可先刷新预览确认识别结果，也可直接提交后台整理任务并在任务中心查看进度。
            </p>
          </div>
          <div class="grid grid-cols-3 gap-3">
            <article class="rounded-2xl bg-slate-100 p-4 dark:bg-white/5">
              <span class="block text-xs text-slate-400 dark:text-slate-500">候选文件</span>
              <strong class="mt-2 block text-xl font-extrabold tabular-nums text-slate-800 dark:text-white">{{ candidateList.length }}</strong>
            </article>
            <article class="rounded-2xl bg-slate-100 p-4 dark:bg-white/5">
              <span class="block text-xs text-slate-400 dark:text-slate-500">手动修正</span>
              <strong class="mt-2 block text-xl font-extrabold tabular-nums text-slate-800 dark:text-white">{{ manualCount }}</strong>
            </article>
            <article class="rounded-2xl bg-slate-100 p-4 dark:bg-white/5">
              <span class="block text-xs text-slate-400 dark:text-slate-500">预览状态</span>
              <strong class="mt-2 block text-xl font-extrabold text-slate-800 dark:text-white">{{ hasPreview ? '已生成' : '待刷新' }}</strong>
            </article>
          </div>
        </section>

        <n-form :model="form" label-placement="top" class="mb-2">
          <div class="grid gap-x-4 sm:grid-cols-2">
            <n-form-item label="目标目录" class="sm:col-span-2">
              <TargetFolderPicker
                v-if="isCloud115"
                :cloud-115-id="currentSource?.cloud115_id || 0"
                :default-path="form.target_path"
                placeholder="请选择整理后的目标目录"
                @update:path="form.target_path = $event"
              />
              <n-input v-else v-model:value="form.target_path" placeholder="请输入整理后的目标目录" />
            </n-form-item>

            <n-form-item label="媒体类型">
              <n-select v-model:value="form.media_type" :options="mediaTypeOptions" />
            </n-form-item>

            <n-form-item label="冲突策略">
              <n-select v-model:value="form.conflict_policy" :options="conflictPolicyOptions" />
            </n-form-item>

            <n-form-item label="重命名预设">
              <div class="w-full">
                <n-select
                  v-model:value="form.preset_id"
                  placeholder="选择预设模板（可选）"
                  clearable
                  :options="presetOptions"
                  @update:value="handlePresetChange"
                />
                <p class="mt-1 text-xs leading-relaxed text-slate-400 dark:text-slate-500">选择预设后会自动填充重命名模板；留空则使用系统默认模板。</p>
              </div>
            </n-form-item>

            <n-form-item label="整理方式">
              <div class="w-full">
                <n-select v-model:value="form.operation_mode" :options="operationModeOptions" />
                <p v-if="isCloud115" class="mt-1 text-xs leading-relaxed text-slate-400 dark:text-slate-500">115 云盘为远程存储，不支持硬链接和软链接。</p>
              </div>
            </n-form-item>

            <n-form-item v-if="!isCloud115" label="刮削 NFO">
              <p class="text-xs leading-relaxed text-slate-400 dark:text-slate-500">整理任务在后台执行，NFO 刮削按后端全局配置自动处理。</p>
            </n-form-item>
          </div>
        </n-form>

        <n-alert
          v-if="!hasPreview"
          type="info"
          :show-icon="true"
          class="mb-4"
          :title="`已收集 ${candidateList.length} 个视频文件。你可以先刷新预览确认识别结果，也可以直接执行整理并在任务中心查看进度。`"
        />

        <n-alert
          v-if="hasPreview && summary"
          type="info"
          :show-icon="true"
          class="mb-4"
          :title="`共 ${summary.total} 项，可处理 ${summary.processable || 0} 项，冲突 ${summary.conflicts || 0} 项，识别失败 ${summary.failed || 0} 项`"
        />

        <n-alert
          v-if="hasPreview && manualCount > 0"
          type="warning"
          :show-icon="true"
          class="mb-4"
          :title="`已应用 ${manualCount} 项手动修正的识别结果，执行整理时将优先使用。`"
        />

        <div v-if="hasPreview && previewList.length > 0" class="overflow-x-auto">
          <n-data-table
            :columns="previewColumns"
            :data="previewList"
            :max-height="420"
            :striped="true"
            :row-key="previewRowKey"
            :scroll-x="1140"
          />
        </div>

        <div v-else-if="candidateList.length > 0" class="overflow-x-auto">
          <n-data-table
            :columns="candidateColumns"
            :data="candidateList"
            :max-height="420"
            :striped="true"
            :scroll-x="560"
          />
        </div>

        <EmptyState v-else title="暂无可显示内容" />
      </div>
    </n-spin>

    <template #action>
      <div class="flex flex-wrap justify-end gap-2">
        <n-button @click="handleCloseDialog">取消</n-button>
        <n-button :loading="previewLoading" @click="handlePreview">刷新预览</n-button>
        <n-button type="primary" :loading="executing" @click="handleExecute">执行整理</n-button>
      </div>
    </template>
  </n-modal>
</template>

<script setup>
import { computed, h, onBeforeUnmount, ref, watch } from 'vue'
import {
  NModal,
  NSpin,
  NForm,
  NFormItem,
  NInput,
  NSelect,
  NAlert,
  NDataTable,
  NButton,
  NTag,
  NIcon,
  useMessage
} from 'naive-ui'
import { CreateOutline } from '@vicons/ionicons5'
import EmptyState from '../common/EmptyState.vue'
import {
  executeOrganizeAsync,
  getOrganizePresets,
  previewOrganize,
  startPreviewTaskAsync,
  getPreviewTaskStatus,
  startOrganizeCandidatesTaskAsync,
  getOrganizeCandidatesTaskStatus
} from '../../utils/api/media'
import TargetFolderPicker from '../resource/TargetFolderPicker.vue'

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
const message = useMessage()
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

const mediaTypeOptions = [
  { label: '全部', value: 'all' },
  { label: '电影', value: 'movie' },
  { label: '剧集', value: 'tv' }
]

const conflictPolicyOptions = [
  { label: '跳过', value: 'skip' },
  { label: '覆盖', value: 'overwrite' },
  { label: '追加序号', value: 'suffix' }
]

const operationModeOptions = computed(() => [
  { label: '移动文件', value: 'move' },
  { label: '复制文件', value: 'copy' },
  { label: '硬链接', value: 'hardlink', disabled: props.isCloud115 },
  { label: '软链接', value: 'symlink', disabled: props.isCloud115 }
])

const presetOptions = computed(() => presets.value.map((preset) => ({
  label: `${preset.name} (${preset.media_type === 'tv' ? '剧集' : '电影'})`,
  value: preset.id
})))

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

onBeforeUnmount(() => {
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

const previewRowKey = (row) => getOverrideKey(row) || row.file_name

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
      message.error('启动候选扫描任务失败')
      return
    }

    currentCandidateTaskId.value = taskId
    const activePollToken = candidatePollToken
    const startedAt = Date.now()

    while (candidatePollToken === activePollToken && visible.value && currentCandidateTaskId.value === taskId) {
      if (Date.now() - startedAt >= CANDIDATE_POLL_TIMEOUT_MS) {
        message.warning('候选扫描超时，请重试')
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
            message.warning('当前选择范围内没有可整理的视频文件')
          }
          break
        }

        if (normalizedStatus === 'failed') {
          message.error(task.error || '候选扫描任务失败')
          break
        }
      } catch (pollError) {
        console.error('[OrganizeDialog] 获取候选扫描任务状态失败:', pollError)
        if (pollError?.response?.status === 404) {
          message.error('候选扫描任务不存在或已过期，请重新加载')
          break
        }
      }
    }
  } catch (error) {
    console.error('[OrganizeDialog] 加载整理候选文件失败:', error)
    candidateLoadingText.value = '候选扫描失败'
    const errorMsg = error.response?.data?.error || error.message || '加载整理候选文件失败'
    message.error(errorMsg)
  } finally {
    currentCandidateTaskId.value = null
    loading.value = false
  }
}

const pollTaskStatus = async (taskId, activePollToken) => {
  const startedAt = Date.now()

  while (previewPollToken === activePollToken && visible.value && currentTaskId.value === taskId) {
    if (Date.now() - startedAt >= PREVIEW_POLL_TIMEOUT_MS) {
      message.warning('预览任务超时，请重新刷新预览')
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
        message.success('预览完成')
        break
      }

      if (normalizedStatus === 'failed') {
        message.error(task.error || '预览任务失败')
        break
      }
    } catch (error) {
      console.error('[OrganizeDialog] 获取预览任务状态失败:', error)
      if (error?.response?.status === 404) {
        message.error('预览任务不存在或已过期，请重新刷新预览')
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
    message.warning('请输入目标目录')
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
      message.error('启动预览任务失败')
      previewLoading.value = false
      return
    }

    currentTaskId.value = taskId
    const activePollToken = previewPollToken
    await pollTaskStatus(taskId, activePollToken)
  } catch (error) {
    console.error('[OrganizeDialog] 预览整理失败:', error)
    const errorMsg = error.response?.data?.error || error.message || '预览整理失败'
    message.error(errorMsg)
    previewLoading.value = false
    currentTaskId.value = null
  }
}

const handleExecute = async () => {
  if (!form.value.target_path.trim()) {
    message.warning('请输入目标目录')
    return
  }

  executing.value = true
  try {
    const response = await executeOrganizeAsync(buildPayload())
    const payload = getPayload(response)
    const taskId = payload.task_id
    message.success(taskId ? `整理任务已提交，可在任务中心查看进度：${taskId}` : '整理任务已提交，可在任务中心查看进度')
    handleCloseDialog()
    emit('execute-success', {
      async: true,
      taskId
    })
  } catch (error) {
    console.error('[OrganizeDialog] 执行整理失败:', error)
    const errorMsg = error.response?.data?.error || error.message || '执行整理失败'
    message.error(errorMsg)
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
    message.warning('新文件名不能为空')
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
      episode: Number(overrideForm.episode || 0),
      metadata_source: overrideForm.metadata_source || '',
      metadata_id: overrideForm.metadata_id || '',
      metadata_provider: overrideForm.metadata_provider || ''
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
  message.success('当前行已更新到预览，无需重新刷新整批预览')
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
  message.success('当前行已恢复自动识别结果，无需重新刷新整批预览')
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
    message.success(taskId ? `重试任务已提交：${taskId}` : '重试任务已提交，可在任务中心查看进度')
    return {
      async: true,
      taskId
    }
  } catch (error) {
    console.error('[OrganizeDialog] 重试失败项失败:', error)
    const errorMsg = error.response?.data?.error || error.message || '重试失败项失败'
    message.error(errorMsg)
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

// 表格列定义(render 中读取的 ref 会被表格渲染副作用跟踪，自动响应更新)
const previewColumns = [
  { title: '原文件名', key: 'file_name', minWidth: 220 },
  {
    title: '识别结果',
    key: 'title',
    minWidth: 180,
    render: (row) => h('div', { class: 'flex flex-wrap items-center gap-2' }, [
      h('span', row.title || '-'),
      row.manual_override
        ? h(NTag, { type: 'warning', size: 'small' }, { default: () => '已手动修正' })
		: null,
	  row.ai_used
		? h(NTag, { type: 'info', size: 'small' }, { default: () => 'AI辅助' })
		: null
    ])
  },
  {
    title: '新文件名',
    key: 'new_name',
    minWidth: 220,
    render: (row) => {
      const key = getOverrideKey(row)
      if (editingNewNameKey.value === key) {
        return h(NInput, {
          value: row.new_name,
          size: 'small',
          onUpdateValue: (value) => { row.new_name = value },
          onBlur: () => handleConfirmEditNewName(row),
          onKeyup: (event) => {
            if (event.key === 'Enter') handleConfirmEditNewName(row)
          }
        })
      }
      return h('div', { class: 'group flex items-center gap-1' }, [
        h('span', row.new_name),
        h(NButton, {
          size: 'tiny',
          text: true,
          type: 'primary',
          'aria-label': '编辑新文件名',
          class: 'shrink-0 opacity-0 transition-opacity group-hover:opacity-100',
          onClick: () => handleStartEditNewName(row)
        }, { icon: () => h(NIcon, { component: CreateOutline }) })
      ])
    }
  },
  { title: '目标路径', key: 'new_path', minWidth: 260, ellipsis: { tooltip: true } },
  {
    title: '状态',
    key: 'status',
    width: 140,
    align: 'center',
    render: (row) => {
      if (updatingPreviewRowKey.value === getOverrideKey(row)) {
        return h(NTag, { type: 'info', size: 'small' }, { default: () => '更新中' })
      }
      if (row.identify_error) {
        return h(NTag, { type: 'error', size: 'small' }, { default: () => '识别失败' })
      }
      if (row.manual_override) {
        return h(NTag, { type: 'warning', size: 'small' }, { default: () => '已修正' })
      }
      if (row.conflict) {
        return h(NTag, { type: 'warning', size: 'small' }, { default: () => '存在冲突' })
      }
      return h(NTag, { type: 'success', size: 'small' }, { default: () => '可执行' })
    }
  },
  {
    title: '操作',
    key: 'actions',
    width: 120,
    align: 'center',
    render: (row) => h(NButton, {
      size: 'small',
      type: row.manual_override ? 'warning' : 'default',
      loading: updatingPreviewRowKey.value === getOverrideKey(row),
      onClick: () => emit('preview-identify', row)
    }, { default: () => (row.manual_override ? '重新修正' : '手动识别') })
  }
]

const candidateColumns = [
  { title: '候选视频文件', key: 'file_name', minWidth: 240 },
  { title: '源路径', key: 'file_path', minWidth: 320, ellipsis: { tooltip: true } }
]

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
