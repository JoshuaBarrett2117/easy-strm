<!--
  MediaManager - 媒体源管理主页面
  负责协调文件浏览、TMDB 识别、重命名预览、批量整理、手动识别修正与整理结果展示等子对话框。
-->
<template>
  <div class="media-manager-container">
    <section class="workbench-hero">
      <div class="hero-copy">
        <el-tag type="success" effect="dark" round>媒体工作台</el-tag>
        <h1>围绕媒体源完成浏览、识别、整理与刮削</h1>
        <p>
          旧页面逻辑已经被收束到新的工作流入口中，核心后端能力仍然通过原有接口执行。
          先选择媒体源，再进入文件浏览器完成识别、批量整理或刮削。
        </p>
        <div class="hero-actions">
          <el-button type="primary" @click="sourceListRef?.handleAdd?.()">
            <el-icon><Plus /></el-icon>
            新增媒体源
          </el-button>
          <el-button @click="openCurrentSourceBrowser" :disabled="!currentSource">
            <el-icon><FolderOpened /></el-icon>
            打开当前媒体源
          </el-button>
          <el-button type="success" plain @click="handleOpenOrganize" :disabled="selectedFiles.length === 0">
            <el-icon><Files /></el-icon>
            批量整理
          </el-button>
        </div>
      </div>
      <div class="hero-panel">
        <div class="hero-panel__header">
          <span>当前工作上下文</span>
          <el-tag :type="currentSource ? 'success' : 'info'" round>
            {{ currentSource ? '已锁定媒体源' : '待选择媒体源' }}
          </el-tag>
        </div>
        <div class="context-list">
          <div class="context-item">
            <span>媒体源</span>
            <strong>{{ currentSource?.name || '未选择' }}</strong>
          </div>
          <div class="context-item">
            <span>源类型</span>
            <strong>{{ currentSource ? (isCloud115Source ? '115 云盘' : '本地存储') : '未选择' }}</strong>
          </div>
          <div class="context-item">
            <span>当前路径</span>
            <strong>{{ currentWorkbenchPath }}</strong>
          </div>
          <div class="context-item">
            <span>选中文件</span>
            <strong>{{ selectedFiles.length }} 项</strong>
          </div>
        </div>
      </div>
    </section>

    <section class="metric-grid">
      <article v-for="card in mediaSummaryCards" :key="card.label" class="metric-card">
        <span class="metric-card__label">{{ card.label }}</span>
        <strong class="metric-card__value">{{ card.value }}</strong>
        <p class="metric-card__hint">{{ card.hint }}</p>
      </article>
    </section>

    <section class="command-grid">
      <article class="command-card">
        <div class="command-card__header">
          <h2>快捷动作</h2>
          <span>围绕当前选择直接进入关键流程</span>
        </div>
        <div class="command-list">
          <button class="command-button" type="button" @click="openCurrentSourceBrowser" :disabled="!currentSource">
            <span>浏览文件</span>
            <small>进入当前媒体源目录</small>
          </button>
          <button class="command-button" type="button" @click="handleBatchIdentify" :disabled="selectedFiles.length === 0">
            <span>批量识别</span>
            <small>调用 TMDB 批量识别</small>
          </button>
          <button class="command-button" type="button" @click="handleBatchRename" :disabled="selectedFiles.length === 0 || isCloud115Source">
            <span>批量重命名</span>
            <small>生成重命名预览并执行</small>
          </button>
          <button class="command-button" type="button" @click="handleBatchScrape" :disabled="selectedFiles.length === 0 || isCloud115Source">
            <span>批量刮削</span>
            <small>为视频文件生成 NFO</small>
          </button>
        </div>
      </article>

      <article class="command-card">
        <div class="command-card__header">
          <h2>本轮选择</h2>
          <span>{{ selectedFiles.length ? '已准备好执行批量操作' : '还没有选择文件' }}</span>
        </div>
        <div v-if="selectedFiles.length" class="selection-list">
          <div v-for="item in selectionPreview" :key="item.id || item.path || item.name" class="selection-item">
            <strong>{{ item.name || item.file_name || '未命名文件' }}</strong>
            <span>{{ item.is_dir ? '目录' : getFileType(item.name || item.file_name) }}</span>
          </div>
        </div>
        <el-empty v-else description="进入文件浏览后选择文件，即可在这里看到本轮操作对象。" :image-size="90" />
      </article>
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

    <el-dialog
      v-model="singleRenameDialogVisible"
      title="重命名单个文件"
      width="500px"
      append-to-body
    >
      <el-form :model="singleRenameForm" label-width="100px">
        <el-form-item label="原文件名">
          <el-input v-model="singleRenameForm.original_name" disabled />
        </el-form-item>
        <el-form-item label="新文件名">
          <el-input v-model="singleRenameForm.new_name" placeholder="请输入新文件名" />
        </el-form-item>
      </el-form>
      <template #footer>
        <span class="dialog-footer">
          <el-button @click="singleRenameDialogVisible = false">取消</el-button>
          <el-button type="primary" @click="handleExecuteSingleRename" :loading="singleRenameLoading">确定</el-button>
        </span>
      </template>
    </el-dialog>

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
import { ElMessage } from 'element-plus'
import { Plus, FolderOpened, Files } from '@element-plus/icons-vue'

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

const mediaSummaryCards = computed(() => {
  const items = mediaSources.value
  const localCount = items.filter(item => item.source_type === 'local').length
  const cloudCount = items.filter(item => item.source_type === 'cloud115').length
  return [
    {
      label: '媒体源总数',
      value: items.length,
      hint: `${localCount} 个本地源，${cloudCount} 个 115 云源`
    },
    {
      label: '当前已选',
      value: currentSource.value?.name || '未选择',
      hint: currentSource.value ? `当前路径 ${currentWorkbenchPath.value}` : '从下方列表进入文件浏览'
    },
    {
      label: '待处理文件',
      value: selectedFiles.value.length,
      hint: selectedFiles.value.length ? '可直接发起识别、重命名或整理' : '进入浏览器后勾选文件'
    },
    {
      label: '整理模式',
      value: isCloud115Source.value ? '云盘整理' : '本地整理',
      hint: currentSource.value ? (isCloud115Source.value ? '115 云盘限制已自动适配' : '支持重命名、刮削与目录整理') : '将在选择媒体源后确定'
    }
  ]
})

const selectionPreview = computed(() => selectedFiles.value.slice(0, 5))

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

const openCurrentSourceBrowser = () => {
  if (!currentSource.value) {
    ElMessage.warning('请先从下方媒体源列表选择一个媒体源')
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
    ElMessage.warning('文件名为空，无法识别')
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
      ElMessage.warning(result.message || '自动识别未找到结果，请尝试手动搜索')
      openTmdbIdentifyDialog(row)
      return
    }

    candidatesMediaType.value = result.media_type || 'movie'
    candidatesList.value = result.candidates || []

    if (candidatesList.value.length === 0) {
      candidatesDialogVisible.value = false
      ElMessage.warning('未找到匹配的候选结果，请尝试手动搜索')
      openTmdbIdentifyDialog(row)
    }
  } catch (error) {
    console.error('[MediaManager] 自动识别失败:', error)
    candidatesDialogVisible.value = false
    const errorMsg = error.response?.data?.error || error.message || '自动识别失败'
    ElMessage.error(errorMsg)
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
    ElMessage.success('识别成功')
    candidatesDialogVisible.value = false
    fileBrowserRef.value?.fetchFileList()
    if (organizeDialogVisible.value) {
      organizeDialogRef.value?.handlePreview()
    }
  } catch (error) {
    console.error('[MediaManager] 绑定识别结果失败:', error)
    const errorMsg = error.response?.data?.error || error.message || '识别失败'
    ElMessage.error(errorMsg)
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
    ElMessage.success('识别成功')
    tmdbDialogVisible.value = false
    fileBrowserRef.value?.fetchFileList()
    if (organizeDialogVisible.value) {
      organizeDialogRef.value?.handlePreview()
    }
  } catch (error) {
    console.error('[MediaManager] 识别失败:', error)
    const errorMsg = error.response?.data?.error || error.message || '识别失败'
    ElMessage.error(errorMsg)
  }
}

const getBatchActionCounts = (payload, results) => {
  const safeResults = Array.isArray(results) ? results : []
  const successCount = typeof payload?.success === "number" ? payload.success : safeResults.filter(r => r.success).length
  const failedCount = typeof payload?.failed === "number" ? payload.failed : safeResults.filter(r => !r.success).length
  return { successCount, failedCount }
}

const notifyBatchActionResult = (label, successCount, failedCount) => {
  const message = `${label}完成：成功 ${successCount} 项，失败 ${failedCount} 项`
  if (failedCount > 0 && successCount === 0) {
    ElMessage.error(message)
  } else if (failedCount > 0) {
    ElMessage.warning(message)
  } else {
    ElMessage.success(message)
  }
}

const handleBatchIdentify = async () => {
  if (selectedFiles.value.length === 0) {
    ElMessage.warning('请先选择要识别的文件')
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
      ElMessage.success(`批量识别完成：成功 ${successCount} 项，失败 ${failedCount} 项`)
      fileBrowserRef.value?.fetchFileList()
    } catch (error) {
      console.error('[MediaManager] 批量识别失败:', error)
      const errorMsg = error.response?.data?.error || error.message || '批量识别失败'
      ElMessage.error(errorMsg)
    }
  }).catch(() => {})
}


const handleBatchDirectoryIdentify = async () => {
  if (selectedFiles.value.length === 0) {
    ElMessage.warning('请先选择要识别的目录')
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
      ElMessage.error(errorMsg)
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
    ElMessage.warning('请输入新文件名')
    return
  }

  singleRenameLoading.value = true
  try {
    await renameFile({
      source_id: singleRenameForm.value.source_id,
      file_id: singleRenameForm.value.file_id,
      new_name: singleRenameForm.value.new_name
    })
    ElMessage.success('重命名成功')
    singleRenameDialogVisible.value = false
    fileBrowserRef.value?.fetchFileList()
  } catch (error) {
    console.error('[MediaManager] 重命名失败:', error)
    const errorMsg = error.response?.data?.error || error.message || '重命名失败'
    ElMessage.error(errorMsg)
  } finally {
    singleRenameLoading.value = false
  }
}

const handleBatchRename = async () => {
  if (selectedFiles.value.length === 0) {
    ElMessage.warning('请先选择要重命名的文件')
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
    ElMessage.error(errorMsg)
  } finally {
    renameLoading.value = false
  }
}

const handleRenameSuccess = () => {
  fileBrowserRef.value?.fetchFileList()
}

const handleOpenOrganize = async () => {
  if (selectedFiles.value.length === 0) {
    ElMessage.warning('请先选择要整理的文件')
    return
  }

  const fileIds = selectedFiles.value.map(f => f.id)
  organizeDialogRef.value?.open(fileIds, currentSource.value)
}

const handleSingleOrganize = (row) => {
  selectedFiles.value = [row]
  handleOpenOrganize()
}

const handleOrganizeSuccess = ({ summary, resultList }) => {
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
      ElMessage.success(`刮削成功：${data.nfo_path || row.name}`)
    } else {
      ElMessage.warning('刮削未生成 NFO 文件')
    }
  } catch (error) {
    console.error('[MediaManager] 刮削失败:', error)
    const errorMsg = error.response?.data?.error || error.message || '刮削失败'
    ElMessage.error(errorMsg)
  }
}

const handleBatchScrape = async () => {
  if (selectedFiles.value.length === 0) {
    ElMessage.warning('请先选择要刮削的文件')
    return
  }

  const videoFiles = selectedFiles.value.filter(
    f => !f.is_dir && getFileType(f.name) === 'video'
  )
  if (videoFiles.length === 0) {
    ElMessage.warning('选中的文件中没有可刮削的视频文件')
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
      ElMessage.success(`批量刮削完成：成功 ${successCount} 项，失败 ${failedCount} 项`)
      fileBrowserRef.value?.fetchFileList()
    } catch (error) {
      console.error('[MediaManager] 批量刮削失败:', error)
      const errorMsg = error.response?.data?.error || error.message || '批量刮削失败'
      ElMessage.error(errorMsg)
    }
  }).catch(() => {})
}


const handleBatchDirectoryScrape = async () => {
  if (selectedFiles.value.length === 0) {
    ElMessage.warning('请先选择要刮削的目录')
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
      ElMessage.error(errorMsg)
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
      ElMessage.success('删除成功')
      fileBrowserRef.value?.fetchFileList()
    } catch (error) {
      console.error('[MediaManager] 删除文件失败:', error)
      const errorMsg = error.response?.data?.error || error.message || '删除失败'
      ElMessage.error(errorMsg)
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

<style scoped>
.media-manager-container {
  padding: 20px;
  min-height: calc(100vh - 100px);
  display: flex;
  flex-direction: column;
  gap: 20px;
}

.workbench-hero {
  display: grid;
  grid-template-columns: minmax(0, 2.1fr) minmax(320px, 1fr);
  gap: 20px;
  padding: 28px;
  border-radius: 28px;
  background:
    radial-gradient(circle at top left, rgba(70, 167, 137, 0.16), transparent 30%),
    radial-gradient(circle at bottom right, rgba(244, 176, 88, 0.18), transparent 28%),
    linear-gradient(135deg, #17313a 0%, #214852 48%, #2a6d73 100%);
  color: #f5f7f2;
  box-shadow: 0 28px 60px rgba(18, 39, 44, 0.24);
}

.hero-copy h1 {
  margin: 16px 0 10px;
  font-size: 32px;
  line-height: 1.2;
}

.hero-copy p {
  margin: 0;
  max-width: 760px;
  color: rgba(245, 247, 242, 0.82);
  line-height: 1.75;
}

.hero-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
  margin-top: 24px;
}

.hero-panel {
  padding: 20px;
  border-radius: 24px;
  background: rgba(255, 255, 255, 0.1);
  backdrop-filter: blur(12px);
  border: 1px solid rgba(255, 255, 255, 0.12);
}

.hero-panel__header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 12px;
  margin-bottom: 18px;
  font-weight: 600;
}

.context-list {
  display: grid;
  gap: 12px;
}

.context-item {
  padding: 12px 14px;
  border-radius: 18px;
  background: rgba(6, 15, 19, 0.18);
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.context-item span {
  font-size: 12px;
  color: rgba(245, 247, 242, 0.68);
}

.context-item strong {
  font-size: 15px;
  word-break: break-all;
}

.metric-grid {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 16px;
}

.metric-card {
  padding: 20px;
  border-radius: 22px;
  background: rgba(255, 252, 247, 0.88);
  border: 1px solid rgba(120, 101, 72, 0.12);
  box-shadow: 0 18px 40px rgba(58, 42, 24, 0.08);
}

.metric-card__label {
  display: block;
  color: #7d6f61;
  font-size: 13px;
}

.metric-card__value {
  display: block;
  margin-top: 10px;
  font-size: 28px;
  color: #17313a;
  line-height: 1.1;
}

.metric-card__hint {
  margin: 10px 0 0;
  color: #6c6259;
  line-height: 1.6;
}

.command-grid {
  display: grid;
  grid-template-columns: minmax(0, 1.2fr) minmax(0, 1fr);
  gap: 20px;
}

.command-card {
  padding: 22px;
  border-radius: 24px;
  background: rgba(255, 252, 247, 0.84);
  border: 1px solid rgba(120, 101, 72, 0.12);
  box-shadow: 0 18px 40px rgba(58, 42, 24, 0.08);
}

.command-card__header h2 {
  margin: 0;
  font-size: 20px;
  color: #17313a;
}

.command-card__header span {
  display: block;
  margin-top: 8px;
  color: #7b6e63;
}

.command-list {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 14px;
  margin-top: 18px;
}

.command-button {
  text-align: left;
  padding: 16px;
  border: 1px solid rgba(31, 111, 120, 0.12);
  background: linear-gradient(180deg, rgba(255, 255, 255, 0.92), rgba(244, 239, 231, 0.92));
  border-radius: 18px;
  cursor: pointer;
  transition: transform 0.2s ease, box-shadow 0.2s ease;
}

.command-button:hover:not(:disabled) {
  transform: translateY(-2px);
  box-shadow: 0 12px 24px rgba(30, 55, 62, 0.1);
}

.command-button:disabled {
  cursor: not-allowed;
  opacity: 0.56;
}

.command-button span,
.command-button small {
  display: block;
}

.command-button span {
  font-size: 16px;
  font-weight: 700;
  color: #17313a;
}

.command-button small {
  margin-top: 8px;
  color: #7f7469;
  line-height: 1.5;
}

.selection-list {
  display: grid;
  gap: 12px;
  margin-top: 18px;
}

.selection-item {
  display: flex;
  justify-content: space-between;
  gap: 12px;
  padding: 14px 16px;
  border-radius: 18px;
  background: #f8f3eb;
}

.selection-item strong {
  color: #17313a;
  word-break: break-all;
}

.selection-item span {
  color: #8a7b6d;
  white-space: nowrap;
}

.dialog-footer {
  display: flex;
  justify-content: flex-end;
  gap: 10px;
}

@media (max-width: 768px) {
  .media-manager-container {
    padding: 12px;
    min-height: calc(100vh - 60px);
  }

  .workbench-hero,
  .metric-grid,
  .command-grid,
  .command-list {
    grid-template-columns: 1fr;
  }

  .workbench-hero {
    padding: 20px;
  }

  .hero-copy h1 {
    font-size: 26px;
  }

  .selection-item {
    flex-direction: column;
  }
}
</style>


