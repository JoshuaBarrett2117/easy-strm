<!--
  MediaManager - 媒体源管理主页面
  负责协调文件浏览、TMDB 识别、重命名预览、批量整理、手动识别修正与整理结果展示等子对话框。
-->
<template>
  <div class="space-y-4">
    <!-- 工作台头部 -->
    <section class="grid gap-4 rounded-2xl border border-slate-200 bg-white p-5 shadow-sm dark:border-white/5 dark:bg-ink-900 lg:grid-cols-[minmax(0,2fr)_minmax(300px,1fr)] lg:p-6">
      <div>
        <n-tag type="success" size="small" round>媒体工作台</n-tag>
        <h1 class="mt-3 text-xl font-bold text-slate-800 dark:text-white lg:text-2xl">
          围绕媒体源完成浏览、识别、整理与刮削
        </h1>
        <p class="mt-2 max-w-2xl text-sm leading-relaxed text-slate-400 dark:text-slate-500">
          先选择媒体源，再进入文件浏览器完成识别、批量整理或刮削。核心后端能力均通过原有接口执行。
        </p>
        <div class="mt-5 flex flex-wrap items-center gap-2">
          <n-button type="primary" @click="sourceListRef?.handleAdd?.()">
            <template #icon>
              <n-icon :component="AddOutline" />
            </template>
            新增媒体源
          </n-button>
          <n-button :disabled="!currentSource" @click="openCurrentSourceBrowser">
            <template #icon>
              <n-icon :component="FolderOpenOutline" />
            </template>
            打开当前媒体源
          </n-button>
          <n-button type="success" ghost :disabled="selectedFiles.length === 0" @click="handleOpenOrganize">
            <template #icon>
              <n-icon :component="FileTrayFullOutline" />
            </template>
            批量整理
          </n-button>
        </div>
      </div>

      <!-- 当前工作上下文 -->
      <div class="rounded-xl border border-slate-200 bg-slate-50 p-4 dark:border-white/5 dark:bg-ink-800/60">
        <div class="mb-3 flex items-center justify-between gap-3">
          <span class="text-sm font-semibold text-slate-700 dark:text-slate-200">当前工作上下文</span>
          <n-tag :type="currentSource ? 'success' : 'default'" size="small" round>
            {{ currentSource ? '已锁定媒体源' : '待选择媒体源' }}
          </n-tag>
        </div>
        <div class="space-y-2">
          <div
            v-for="ctx in contextItems"
            :key="ctx.label"
            class="flex items-center justify-between gap-3 rounded-lg bg-white px-3 py-2 text-sm dark:bg-ink-900/70"
          >
            <span class="shrink-0 text-slate-400 dark:text-slate-500">{{ ctx.label }}</span>
            <strong class="truncate font-semibold text-slate-700 dark:text-slate-200">{{ ctx.value }}</strong>
          </div>
        </div>
      </div>
    </section>

    <!-- 概览指标 -->
    <section class="grid grid-cols-2 gap-3 lg:grid-cols-4 lg:gap-4">
      <StatCard
        v-for="card in mediaSummaryCards"
        :key="card.label"
        :label="card.label"
        :value="card.value"
        :hint="card.hint"
        :icon="card.icon"
        :tone="card.tone"
      />
    </section>

    <!-- 快捷动作 + 本轮选择 -->
    <section class="grid gap-4 lg:grid-cols-2">
      <PageCard title="快捷动作" subtitle="围绕当前选择直接进入关键流程">
        <div class="grid grid-cols-1 gap-2 sm:grid-cols-2">
          <button
            v-for="action in quickActions"
            :key="action.label"
            type="button"
            class="flex flex-col gap-1 rounded-xl border border-slate-200 px-4 py-3 text-left transition-colors enabled:hover:border-cyan-400/60 enabled:hover:bg-cyan-500/5 disabled:cursor-not-allowed disabled:opacity-45 dark:border-white/10"
            :disabled="action.disabled"
            @click="action.handler"
          >
            <span class="text-sm font-semibold text-slate-700 dark:text-slate-200">{{ action.label }}</span>
            <small class="text-xs text-slate-400 dark:text-slate-500">{{ action.hint }}</small>
          </button>
        </div>
      </PageCard>

      <PageCard title="本轮选择" :subtitle="selectedFiles.length ? '已准备好执行批量操作' : '还没有选择文件'">
        <div v-if="selectedFiles.length" class="space-y-2">
          <div
            v-for="item in selectionPreview"
            :key="item.id || item.path || item.name"
            class="flex items-center justify-between gap-3 rounded-lg bg-slate-50 px-3 py-2 text-sm dark:bg-ink-800/60"
          >
            <strong class="truncate font-medium text-slate-700 dark:text-slate-200">
              {{ item.name || item.file_name || '未命名文件' }}
            </strong>
            <n-tag size="small" :bordered="false">
              {{ item.is_dir ? '目录' : getFileType(item.name || item.file_name) }}
            </n-tag>
          </div>
          <p v-if="selectedFiles.length > selectionPreview.length" class="text-xs text-slate-400 dark:text-slate-500">
            等共 {{ selectedFiles.length }} 项…
          </p>
        </div>
        <EmptyState
          v-else
          title="暂无选中文件"
          description="进入文件浏览后选择文件，即可在这里看到本轮操作对象。"
        />
      </PageCard>
    </section>

    <MediaSourceList ref="sourceListRef" @browse="handleBrowseFiles" />

    <FileBrowser
      ref="fileBrowserRef"
      :current-source="currentSource"
      @selection-change="handleSelectionChange"
      @identify="handleIdentify"
      @rename="handleRename"
      @single-organize="handleSingleOrganize"
      @scrape="handleScrapeFile"
      @delete-file="handleDeleteFile"
      @batch-identify="handleBatchIdentify"
      @batch-directory-identify="handleBatchDirectoryIdentify"
      @batch-rename="handleBatchRename"
      @batch-scrape="handleBatchScrape"
      @batch-directory-scrape="handleBatchDirectoryScrape"
      @open-organize="handleOpenOrganize"
    />

    <TmdbCandidatesDialog
      v-model:visible="candidatesDialogVisible"
      v-model:loading="candidatesLoading"
      :candidates-list="candidatesList"
      :media-type="candidatesMediaType"
      @select="handleSelectCandidate"
      @manual-search="handleManualSearchFromCandidates"
    />

    <TmdbIdentifyDialog
      ref="tmdbDialogRef"
      v-model:visible="tmdbDialogVisible"
      :select-mode="tmdbSelectMode"
      :current-file="currentIdentifyFile"
      @select="handleTmdbSelect"
    />

    <RenamePreviewDialog
      v-model:visible="renameDialogVisible"
      v-model:loading="renameLoading"
      :preview-list="renamePreviewList"
      :source-id="currentSource?.id"
      @execute-success="handleRenameSuccess"
    />

    <!-- 单文件重命名 -->
    <n-modal
      v-model:show="singleRenameDialogVisible"
      preset="card"
      title="重命名单个文件"
      class="w-[92vw] max-w-lg"
    >
      <n-form :model="singleRenameForm" label-placement="top">
        <n-form-item label="原文件名">
          <n-input v-model:value="singleRenameForm.original_name" disabled />
        </n-form-item>
        <n-form-item label="新文件名">
          <n-input v-model:value="singleRenameForm.new_name" placeholder="请输入新文件名" />
        </n-form-item>
      </n-form>
      <template #action>
        <div class="flex justify-end gap-2">
          <n-button @click="singleRenameDialogVisible = false">取消</n-button>
          <n-button type="primary" :loading="singleRenameLoading" @click="handleExecuteSingleRename">确定</n-button>
        </div>
      </template>
    </n-modal>

    <OrganizeDialog
      ref="organizeDialogRef"
      v-model:visible="organizeDialogVisible"
      :current-source="currentSource"
      :current-path="fileBrowserRef?.currentPath || '/'"
      :is-cloud115="isCloud115Source"
      :current-display-path="fileBrowserRef?.currentCloud115DisplayPath || '/'"
      @execute-success="handleOrganizeSuccess"
      @preview-identify="handleOrganizePreviewIdentify"
    />

    <ManualIdentifyDialog
      v-model:visible="organizeIdentifyDialogVisible"
      :form="organizeIdentifyForm"
      @apply="handleApplyOrganizeIdentifyOverride"
      @clear="handleClearOrganizeIdentifyOverride"
      @search-tmdb="handleSearchTmdbForOrganizeEdit"
    />

    <OrganizeResultDialog
      v-model:visible="organizeResultDialogVisible"
      :summary="organizeExecuteSummary"
      :result-list="organizeResultList"
      :current-source="currentSource"
      :executing="organizeExecuting"
      @retry-failed="handleRetryFailedItems"
    />
  </div>
</template>

<script setup>
import { ref, computed } from 'vue'
import { NButton, NTag, NIcon, NModal, NForm, NFormItem, NInput, useMessage } from 'naive-ui'
import {
  AddOutline,
  FolderOpenOutline,
  FileTrayFullOutline,
  ServerOutline,
  LocateOutline,
  CheckboxOutline,
  OptionsOutline
} from '@vicons/ionicons5'

import PageCard from '../components/common/PageCard.vue'
import StatCard from '../components/common/StatCard.vue'
import EmptyState from '../components/common/EmptyState.vue'
import MediaSourceList from '../components/media/MediaSourceList.vue'
import FileBrowser from '../components/media/FileBrowser.vue'
import TmdbCandidatesDialog from '../components/media/TmdbCandidatesDialog.vue'
import TmdbIdentifyDialog from '../components/media/TmdbIdentifyDialog.vue'
import RenamePreviewDialog from '../components/media/RenamePreviewDialog.vue'
import OrganizeDialog from '../components/media/OrganizeDialog.vue'
import ManualIdentifyDialog from '../components/media/ManualIdentifyDialog.vue'
import OrganizeResultDialog from '../components/media/OrganizeResultDialog.vue'
import { showConfirmDialog } from '../utils/ui/messageBox'

import {
  autoIdentifyFile,
  identifyFile,
  batchPreviewRename,
  batchIdentifyFiles,
  batchIdentifyDirectoryFiles,
  renameFile,
  deleteFile,
  scrapeFile,
  scrapeFiles,
  scrapeDirectoryFiles
} from '../utils/api/media'

const message = useMessage()

const sourceListRef = ref(null)
const fileBrowserRef = ref(null)
const tmdbDialogRef = ref(null)
const organizeDialogRef = ref(null)

const currentSource = ref(null)

const isCloud115Source = computed(() => {
  return currentSource.value?.source_type === 'cloud115'
})

const selectedFiles = ref([])

const mediaSources = computed(() => {
  const items = sourceListRef.value?.mediaSources
  return Array.isArray(items) ? items : []
})

const currentWorkbenchPath = computed(() => {
  if (!currentSource.value) return '请先选择媒体源'
  return fileBrowserRef.value?.getCurrentDisplayPath?.() || currentSource.value.path || '/'
})

const contextItems = computed(() => [
  { label: '媒体源', value: currentSource.value?.name || '未选择' },
  { label: '源类型', value: currentSource.value ? (isCloud115Source.value ? '115 云盘' : '本地存储') : '未选择' },
  { label: '当前路径', value: currentWorkbenchPath.value },
  { label: '选中文件', value: `${selectedFiles.value.length} 项` }
])

const mediaSummaryCards = computed(() => {
  const items = mediaSources.value
  const localCount = items.filter(item => item.source_type === 'local').length
  const cloudCount = items.filter(item => item.source_type === 'cloud115').length
  return [
    {
      label: '媒体源总数',
      value: items.length,
      hint: `${localCount} 个本地源，${cloudCount} 个 115 云源`,
      icon: ServerOutline,
      tone: 'cyan'
    },
    {
      label: '当前已选',
      value: currentSource.value?.name || '未选择',
      hint: currentSource.value ? `当前路径 ${currentWorkbenchPath.value}` : '从下方列表进入文件浏览',
      icon: LocateOutline,
      tone: 'violet'
    },
    {
      label: '待处理文件',
      value: selectedFiles.value.length,
      hint: selectedFiles.value.length ? '可直接发起识别、重命名或整理' : '进入浏览器后勾选文件',
      icon: CheckboxOutline,
      tone: 'amber'
    },
    {
      label: '整理模式',
      value: isCloud115Source.value ? '云盘整理' : '本地整理',
      hint: currentSource.value ? (isCloud115Source.value ? '115 云盘限制已自动适配' : '支持重命名、刮削与目录整理') : '将在选择媒体源后确定',
      icon: OptionsOutline,
      tone: 'green'
    }
  ]
})

const selectionPreview = computed(() => selectedFiles.value.slice(0, 5))

const quickActions = computed(() => [
  {
    label: '浏览文件',
    hint: '进入当前媒体源目录',
    disabled: !currentSource.value,
    handler: openCurrentSourceBrowser
  },
  {
    label: '批量识别',
    hint: '调用 TMDB 批量识别',
    disabled: selectedFiles.value.length === 0,
    handler: handleBatchIdentify
  },
  {
    label: '批量重命名',
    hint: '生成重命名预览并执行',
    disabled: selectedFiles.value.length === 0 || isCloud115Source.value,
    handler: handleBatchRename
  },
  {
    label: '批量刮削',
    hint: '为视频文件生成 NFO',
    disabled: selectedFiles.value.length === 0 || isCloud115Source.value,
    handler: handleBatchScrape
  }
])

const tmdbDialogVisible = ref(false)
const tmdbSelectMode = ref('cache')
const currentIdentifyFile = ref(null)

const candidatesDialogVisible = ref(false)
const candidatesLoading = ref(false)
const candidatesList = ref([])
const candidatesMediaType = ref('movie')

const renameDialogVisible = ref(false)
const renamePreviewList = ref([])
const renameLoading = ref(false)

const singleRenameDialogVisible = ref(false)
const singleRenameForm = ref({
  source_id: null,
  file_id: null,
  original_name: '',
  new_name: ''
})
const singleRenameLoading = ref(false)

const organizeDialogVisible = ref(false)
const organizeResultDialogVisible = ref(false)
const organizeExecuting = ref(false)
const organizeResultList = ref([])
const organizeExecuteSummary = ref(null)

const organizeIdentifyDialogVisible = ref(false)
const organizeIdentifyForm = ref({
  file_id: '',
  cloud_id: '',
  file_name: '',
  media_type: 'movie',
  tmdb_id: 0,
  title: '',
  original_title: '',
  year: 0,
  season: 0,
  episode: 0,
  override_key: ''
})

const handleBrowseFiles = (source) => {
  currentSource.value = source
  fileBrowserRef.value?.open(source)
}

function openCurrentSourceBrowser() {
  if (!currentSource.value) {
    message.warning('请先从下方媒体源列表选择一个媒体源')
    return
  }
  fileBrowserRef.value?.open(currentSource.value)
}

const handleSelectionChange = (selection) => {
  selectedFiles.value = selection
}

const handleIdentify = async (row) => {
  currentIdentifyFile.value = row
  const filename = row.name || row.file_name || ''
  if (!filename) {
    message.warning('文件名为空，无法识别')
    return
  }

  candidatesLoading.value = true
  candidatesDialogVisible.value = true
  candidatesList.value = []

  try {
    const response = await autoIdentifyFile({ filename })
    const result = response.data.data || response.data

    if (!result.success) {
      candidatesDialogVisible.value = false
      message.warning(result.message || '自动识别未找到结果，请尝试手动搜索')
      openTmdbIdentifyDialog(row)
      return
    }

    candidatesMediaType.value = result.media_type || 'movie'
    candidatesList.value = result.candidates || []

    if (candidatesList.value.length === 0) {
      candidatesDialogVisible.value = false
      message.warning('未找到匹配的候选结果，请尝试手动搜索')
      openTmdbIdentifyDialog(row)
    }
  } catch (error) {
    console.error('[MediaManager] 自动识别失败:', error)
    candidatesDialogVisible.value = false
    const errorMsg = error.response?.data?.error || error.message || '自动识别失败'
    message.error(errorMsg)
    openTmdbIdentifyDialog(row)
  } finally {
    candidatesLoading.value = false
  }
}

const openTmdbIdentifyDialog = (row) => {
  currentIdentifyFile.value = row
  tmdbSelectMode.value = 'cache'
  tmdbDialogVisible.value = true
  const filename = (row.name || row.file_name || '').replace(/\.[^/.]+$/, '')
  tmdbDialogRef.value?.setKeyword(filename, 'movie')
}

const handleSelectCandidate = async (candidate) => {
  if (!currentIdentifyFile.value) return

  try {
    await identifyFile({
      file_id: currentIdentifyFile.value.identify_cache_key || currentIdentifyFile.value.id,
      tmdb_id: candidate.tmdb_id,
      tmdb_type: candidate.media_type || candidatesMediaType.value,
      title: candidate.title,
      year: candidate.year,
      poster_url: candidate.poster_path
    })
    message.success('识别成功')
    candidatesDialogVisible.value = false
    fileBrowserRef.value?.fetchFileList()
    if (organizeDialogVisible.value) {
      organizeDialogRef.value?.handlePreview()
    }
  } catch (error) {
    console.error('[MediaManager] 绑定识别结果失败:', error)
    const errorMsg = error.response?.data?.error || error.message || '识别失败'
    message.error(errorMsg)
  }
}

const handleManualSearchFromCandidates = () => {
  candidatesDialogVisible.value = false
  openTmdbIdentifyDialog(currentIdentifyFile.value)
}

const handleTmdbSelect = async ({ item, mode, searchType }) => {
  if (mode === 'organize') {
    organizeIdentifyForm.value = {
      ...organizeIdentifyForm.value,
      media_type: searchType,
      tmdb_id: item.tmdb_id || item.id || 0,
      title: item.title || item.name || '',
      original_title: item.original_title || '',
      year: item.year || 0
    }
    tmdbDialogVisible.value = false
    if (!organizeIdentifyDialogVisible.value) {
      organizeIdentifyDialogVisible.value = true
    }
    return
  }

  if (!currentIdentifyFile.value) return

  try {
    await identifyFile({
      file_id: currentIdentifyFile.value.identify_cache_key || currentIdentifyFile.value.id,
      tmdb_id: item.tmdb_id || item.id,
      tmdb_type: searchType,
      title: item.title || item.name,
      year: item.year,
      poster_url: item.poster_path
    })
    message.success('识别成功')
    tmdbDialogVisible.value = false
    fileBrowserRef.value?.fetchFileList()
    if (organizeDialogVisible.value) {
      organizeDialogRef.value?.handlePreview()
    }
  } catch (error) {
    console.error('[MediaManager] 识别失败:', error)
    const errorMsg = error.response?.data?.error || error.message || '识别失败'
    message.error(errorMsg)
  }
}

const getBatchActionCounts = (payload, results) => {
  const safeResults = Array.isArray(results) ? results : []
  const successCount = typeof payload?.success === 'number' ? payload.success : safeResults.filter(r => r.success).length
  const failedCount = typeof payload?.failed === 'number' ? payload.failed : safeResults.filter(r => !r.success).length
  return { successCount, failedCount }
}

const notifyBatchActionResult = (label, successCount, failedCount) => {
  const text = `${label}完成：成功 ${successCount} 项，失败 ${failedCount} 项`
  if (failedCount > 0 && successCount === 0) {
    message.error(text)
  } else if (failedCount > 0) {
    message.warning(text)
  } else {
    message.success(text)
  }
}

function handleBatchIdentify() {
  if (selectedFiles.value.length === 0) {
    message.warning('请先选择要识别的文件')
    return
  }

  showConfirmDialog(
    `确定要批量识别选中的 ${selectedFiles.value.length} 个文件吗？`,
    '批量识别确认',
    {
      confirmButtonText: '确定',
      cancelButtonText: '取消',
      type: 'info'
    }
  ).then(async () => {
    try {
      const fileIds = selectedFiles.value.map(f => f.id)
      const response = await batchIdentifyFiles({
        source_id: currentSource.value.id,
        file_ids: fileIds
      })
      const results = response.data.data?.data || response.data.data || []
      const successCount = Array.isArray(results) ? results.filter(r => r.success).length : 0
      const failedCount = Array.isArray(results) ? results.filter(r => !r.success).length : 0
      message.success(`批量识别完成：成功 ${successCount} 项，失败 ${failedCount} 项`)
      fileBrowserRef.value?.fetchFileList()
    } catch (error) {
      console.error('[MediaManager] 批量识别失败:', error)
      const errorMsg = error.response?.data?.error || error.message || '批量识别失败'
      message.error(errorMsg)
    }
  }).catch(() => {})
}

const handleBatchDirectoryIdentify = () => {
  if (selectedFiles.value.length === 0) {
    message.warning('请先选择要识别的目录')
    return
  }

  showConfirmDialog(
    `确定要递归识别选中的 ${selectedFiles.value.length} 个目录文件吗？`,
    '整目录识别确认',
    {
      confirmButtonText: '确定',
      cancelButtonText: '取消',
      type: 'info'
    }
  ).then(async () => {
    try {
      const fileIds = selectedFiles.value.map(f => f.id).filter(Boolean)
      const response = await batchIdentifyDirectoryFiles({
        source_id: currentSource.value.id,
        source_path: fileBrowserRef.value?.getCurrentPath() || '/',
        file_ids: fileIds
      })
      const payload = response.data.data || {}
      const results = payload.data || []
      const { successCount, failedCount } = getBatchActionCounts(payload, results)
      notifyBatchActionResult('整目录识别', successCount, failedCount)
      fileBrowserRef.value?.fetchFileList()
    } catch (error) {
      console.error('[MediaManager] 整目录识别失败:', error)
      const errorMsg = error.response?.data?.error || error.message || '整目录识别失败'
      message.error(errorMsg)
    }
  }).catch(() => {})
}

const handleRename = (row) => {
  singleRenameForm.value = {
    source_id: currentSource.value.id,
    file_id: row.id,
    original_name: row.name,
    new_name: row.name
  }
  singleRenameDialogVisible.value = true
}

const handleExecuteSingleRename = async () => {
  if (!singleRenameForm.value.new_name.trim()) {
    message.warning('请输入新文件名')
    return
  }

  singleRenameLoading.value = true
  try {
    await renameFile({
      source_id: singleRenameForm.value.source_id,
      file_id: singleRenameForm.value.file_id,
      new_name: singleRenameForm.value.new_name
    })
    message.success('重命名成功')
    singleRenameDialogVisible.value = false
    fileBrowserRef.value?.fetchFileList()
  } catch (error) {
    console.error('[MediaManager] 重命名失败:', error)
    const errorMsg = error.response?.data?.error || error.message || '重命名失败'
    message.error(errorMsg)
  } finally {
    singleRenameLoading.value = false
  }
}

async function handleBatchRename() {
  if (selectedFiles.value.length === 0) {
    message.warning('请先选择要重命名的文件')
    return
  }

  renameLoading.value = true
  try {
    const items = selectedFiles.value.map(f => ({
      source_id: currentSource.value.id,
      file_id: f.id
    }))

    const response = await batchPreviewRename({ items })
    renamePreviewList.value = response.data.data?.data || response.data.data || []
    renameDialogVisible.value = true
  } catch (error) {
    console.error('[MediaManager] 预览重命名失败:', error)
    const errorMsg = error.response?.data?.error || error.message || '预览重命名失败'
    message.error(errorMsg)
  } finally {
    renameLoading.value = false
  }
}

const handleRenameSuccess = () => {
  fileBrowserRef.value?.fetchFileList()
}

function handleOpenOrganize() {
  if (selectedFiles.value.length === 0) {
    message.warning('请先选择要整理的文件')
    return
  }

  const fileIds = selectedFiles.value.map(f => f.id)
  organizeDialogRef.value?.open(fileIds, currentSource.value)
}

const handleSingleOrganize = (row) => {
  selectedFiles.value = [row]
  handleOpenOrganize()
}

const handleOrganizeSuccess = ({ async: asyncTask, summary, resultList }) => {
  if (asyncTask) {
    // 异步整理只提交任务，结果统一在任务中心追踪，避免打开空的同步结果弹窗。
    fileBrowserRef.value?.fetchFileList()
    return
  }
  organizeResultList.value = resultList
  organizeExecuteSummary.value = summary
  organizeResultDialogVisible.value = true
  fileBrowserRef.value?.fetchFileList()
}

const handleOrganizePreviewIdentify = (row) => {
  const key = organizeDialogRef.value?.getOverrideKey(row) || ''
  const existing = key ? organizeDialogRef.value?.manualOverrides[key] : null
  organizeIdentifyForm.value = {
    file_id: row.file_id || row.id || '',
    cloud_id: row.cloud_id || '',
    file_name: row.file_name || row.name || '',
    media_type: existing?.media_type || row.media_type || 'movie',
    tmdb_id: existing?.tmdb_id || row.tmdb_id || 0,
    title: existing?.title || row.title || '',
    original_title: existing?.original_title || row.original_title || '',
    year: existing?.year || row.year || 0,
    season: existing?.season || row.season || 0,
    episode: existing?.episode || row.episode || 0,
    override_key: key
  }
  organizeIdentifyDialogVisible.value = true
}

const handleApplyOrganizeIdentifyOverride = async (form) => {
  await organizeDialogRef.value?.applyManualOverride(form)
  organizeIdentifyDialogVisible.value = false
}

const handleClearOrganizeIdentifyOverride = async (key) => {
  await organizeDialogRef.value?.clearManualOverride(key)
  organizeIdentifyDialogVisible.value = false
}

const handleSearchTmdbForOrganizeEdit = (form) => {
  currentIdentifyFile.value = {
    file_id: form.file_id,
    cloud_id: form.cloud_id,
    file_name: form.file_name
  }
  tmdbSelectMode.value = 'organize'
  tmdbDialogVisible.value = true
  const keyword = form.title || form.file_name.replace(/\.[^/.]+$/, '')
  tmdbDialogRef.value?.setKeyword(keyword, form.media_type || 'movie')
}

const handleRetryFailedItems = async (failedItems) => {
  const result = await organizeDialogRef.value?.retryFailedItems(failedItems)
  if (result) {
    const retryFileIds = new Set(result.resultList.map(r => r.file_id || r.cloud_id))
    const mergedResults = organizeResultList.value.filter(
      item => item.success || item.skipped || !retryFileIds.has(item.file_id || item.cloud_id)
    )
    mergedResults.push(...result.resultList)
    organizeResultList.value = mergedResults

    organizeExecuteSummary.value = {
      total: mergedResults.length,
      success: mergedResults.filter(r => r.success).length,
      skipped: mergedResults.filter(r => r.skipped).length,
      failed: mergedResults.filter(r => !r.success && !r.skipped).length
    }

    fileBrowserRef.value?.fetchFileList()
  }
}

const handleScrapeFile = async (row) => {
  try {
    const response = await scrapeFile({
      source_id: currentSource.value.id,
      file_path: row.path || row.name
    })
    const data = response.data.data
    if (data?.success) {
      message.success(`刮削成功：${data.nfo_path || row.name}`)
    } else {
      message.warning('刮削未生成 NFO 文件')
    }
  } catch (error) {
    console.error('[MediaManager] 刮削失败:', error)
    const errorMsg = error.response?.data?.error || error.message || '刮削失败'
    message.error(errorMsg)
  }
}

function handleBatchScrape() {
  if (selectedFiles.value.length === 0) {
    message.warning('请先选择要刮削的文件')
    return
  }

  const videoFiles = selectedFiles.value.filter(
    f => !f.is_dir && getFileType(f.name) === 'video'
  )
  if (videoFiles.length === 0) {
    message.warning('选中的文件中没有可刮削的视频文件')
    return
  }

  showConfirmDialog(
    `确定要批量刮削选中的 ${videoFiles.length} 个视频文件的 NFO 吗？`,
    '批量刮削确认',
    {
      confirmButtonText: '确定',
      cancelButtonText: '取消',
      type: 'info'
    }
  ).then(async () => {
    try {
      const filePaths = videoFiles.map(f => f.path || f.name)
      const response = await scrapeFiles({
        source_id: currentSource.value.id,
        file_paths: filePaths
      })
      const data = response.data.data
      const successCount = data?.success || 0
      const failedCount = data?.failed || 0
      message.success(`批量刮削完成：成功 ${successCount} 项，失败 ${failedCount} 项`)
      fileBrowserRef.value?.fetchFileList()
    } catch (error) {
      console.error('[MediaManager] 批量刮削失败:', error)
      const errorMsg = error.response?.data?.error || error.message || '批量刮削失败'
      message.error(errorMsg)
    }
  }).catch(() => {})
}

const handleBatchDirectoryScrape = () => {
  if (selectedFiles.value.length === 0) {
    message.warning('请先选择要刮削的目录')
    return
  }

  showConfirmDialog(
    `确定要递归刮削选中的 ${selectedFiles.value.length} 个目录文件吗？`,
    '整目录刮削确认',
    {
      confirmButtonText: '确定',
      cancelButtonText: '取消',
      type: 'info'
    }
  ).then(async () => {
    try {
      const fileIds = selectedFiles.value.map(f => f.id).filter(Boolean)
      const response = await scrapeDirectoryFiles({
        source_id: currentSource.value.id,
        source_path: fileBrowserRef.value?.getCurrentPath() || '/',
        file_ids: fileIds
      })
      const payload = response.data.data || {}
      const results = payload.results || []
      const { successCount, failedCount } = getBatchActionCounts(payload, results)
      notifyBatchActionResult('整目录刮削', successCount, failedCount)
      fileBrowserRef.value?.fetchFileList()
    } catch (error) {
      console.error('[MediaManager] 整目录刮削失败:', error)
      const errorMsg = error.response?.data?.error || error.message || '整目录刮削失败'
      message.error(errorMsg)
    }
  }).catch(() => {})
}

const handleDeleteFile = (row) => {
  showConfirmDialog(
    `确定要删除“${row.name}”吗？`,
    '删除确认',
    {
      confirmButtonText: '确定',
      cancelButtonText: '取消',
      type: 'warning'
    }
  ).then(async () => {
    try {
      await deleteFile({
        source_id: currentSource.value.id,
        file_ids: [row.id || row.path].filter(Boolean)
      })
      message.success('删除成功')
      fileBrowserRef.value?.fetchFileList()
    } catch (error) {
      console.error('[MediaManager] 删除文件失败:', error)
      const errorMsg = error.response?.data?.error || error.message || '删除失败'
      message.error(errorMsg)
    }
  }).catch(() => {})
}

const getFileType = (filename) => {
  if (!filename) return 'other'
  const ext = filename.split('.').pop()?.toLowerCase() || ''
  const videoExts = ['mp4', 'mkv', 'avi', 'mov', 'wmv', 'flv', 'webm', 'm4v', 'rmvb', 'rm', 'ts', 'm2ts', 'mpg', 'mpeg']
  const audioExts = ['mp3', 'flac', 'wav', 'aac', 'ogg', 'wma', 'm4a', 'ape', 'alac']
  const docExts = ['txt', 'pdf', 'doc', 'docx', 'xls', 'xlsx', 'ppt', 'pptx', 'md', 'json', 'xml']
  if (videoExts.includes(ext)) return 'video'
  if (audioExts.includes(ext)) return 'audio'
  if (docExts.includes(ext)) return 'document'
  return 'other'
}
</script>
