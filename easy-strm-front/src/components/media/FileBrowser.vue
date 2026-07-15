<!--
  FileBrowserDialog - 文件浏览对话框
  支持面包屑导航、搜索过滤、文件选择，以及识别、重命名、整理、刮削、删除等操作
  兼容本地存储和 115 云盘两种媒体源类型
-->
<template>
  <n-modal
    v-model:show="visible"
    preset="card"
    title="文件浏览"
    class="w-[96vw] max-w-[1400px]"
  >
    <div class="space-y-4">
      <!-- 工作区概览 -->
      <section class="grid gap-4 lg:grid-cols-[minmax(260px,1fr)_minmax(0,2fr)]">
        <div class="rounded-2xl border border-cyan-500/10 bg-gradient-to-br from-cyan-500/10 to-amber-400/10 p-4 lg:p-5">
          <h2 class="text-xl font-bold text-slate-800 dark:text-white">{{ currentDirectoryName || '文件浏览工作区' }}</h2>
          <p class="mt-2 text-sm leading-relaxed text-slate-500 dark:text-slate-400">
            当前正在浏览 {{ isCloud115 ? '115 云盘媒体源' : '本地媒体源' }}，
            可在这里完成目录切换、筛选、识别、重命名、整理和删除操作。
          </p>
        </div>
        <div class="grid grid-cols-2 gap-3 lg:grid-cols-4">
          <article
            v-for="card in browserOverviewCards"
            :key="card.label"
            class="rounded-2xl bg-slate-100 p-4 dark:bg-white/5"
          >
            <span class="block text-xs text-slate-400 dark:text-slate-500">{{ card.label }}</span>
            <strong class="mt-2 block break-all text-xl font-extrabold tabular-nums text-slate-800 dark:text-white">{{ card.value }}</strong>
            <p class="mt-1 text-xs leading-relaxed text-slate-400 dark:text-slate-500">{{ card.hint }}</p>
          </article>
        </div>
      </section>

      <!-- 面包屑导航 -->
      <div class="flex flex-col gap-2 rounded-2xl bg-slate-100 px-4 py-3 md:flex-row md:items-center md:justify-between dark:bg-white/5">
        <n-breadcrumb separator="/">
          <n-breadcrumb-item>
            <span class="inline-flex cursor-pointer items-center gap-1 transition-colors hover:text-cyan-500" @click="navigateToRoot">
              <n-icon :component="HomeOutline" />
              {{ isCloud115 ? (currentSource?.name || '根目录') : '根目录' }}
            </span>
          </n-breadcrumb-item>
          <n-breadcrumb-item
            v-for="(item, index) in breadcrumbItems"
            :key="index"
          >
            <span class="inline-flex cursor-pointer items-center gap-1 transition-colors hover:text-cyan-500" @click="navigateToPath(item.path)">
              <n-icon :component="FolderOutline" />
              {{ item.name }}
            </span>
          </n-breadcrumb-item>
        </n-breadcrumb>
        <div v-if="canGoBack" class="flex shrink-0 flex-wrap gap-2">
          <n-button size="small" @click="navigateToRoot">
            <template #icon><n-icon :component="HomeOutline" /></template>
            返回根目录
          </n-button>
          <n-button size="small" @click="navigateToParent">
            <template #icon><n-icon :component="ArrowBackOutline" /></template>
            返回上一级
          </n-button>
        </div>
      </div>

      <!-- 状态栏 -->
      <div class="grid gap-3 md:grid-cols-3">
        <div class="rounded-2xl border border-slate-200 bg-white px-4 py-3 dark:border-white/5 dark:bg-ink-900">
          <span class="block text-xs text-slate-400 dark:text-slate-500">当前源</span>
          <strong class="mt-1 block break-all text-sm font-semibold text-slate-800 dark:text-white">{{ currentSourceName }}</strong>
        </div>
        <div class="rounded-2xl border border-slate-200 bg-white px-4 py-3 dark:border-white/5 dark:bg-ink-900">
          <span class="block text-xs text-slate-400 dark:text-slate-500">当前显示路径</span>
          <strong class="mt-1 block break-all text-sm font-semibold text-slate-800 dark:text-white">{{ currentDisplayPath }}</strong>
        </div>
        <div class="rounded-2xl border border-slate-200 bg-white px-4 py-3 dark:border-white/5 dark:bg-ink-900">
          <span class="block text-xs text-slate-400 dark:text-slate-500">已选文件</span>
          <strong class="mt-1 block text-sm font-semibold text-slate-800 dark:text-white">{{ selectedFiles.length }} 项</strong>
        </div>
      </div>

      <!-- 搜索与筛选 -->
      <div class="flex flex-wrap items-center gap-3">
        <n-input
          v-model:value="searchKeyword"
          placeholder="搜索文件名"
          clearable
          class="w-full md:w-[300px]"
          @keyup.enter="handleSearch"
        >
          <template #prefix>
            <n-icon :component="SearchOutline" />
          </template>
        </n-input>
        <n-select
          v-model:value="filterType"
          :options="filterTypeOptions"
          placeholder="文件类型"
          clearable
          class="w-full md:w-[150px]"
        />
        <n-button type="primary" @click="handleSearch">
          <template #icon><n-icon :component="SearchOutline" /></template>
          搜索
        </n-button>
        <n-button @click="handleRefresh">
          <template #icon><n-icon :component="RefreshOutline" /></template>
          刷新
        </n-button>
      </div>

      <!-- 批量操作：桌面端按钮组 -->
      <div class="hidden flex-wrap items-center gap-2 md:flex">
        <n-button type="success" :disabled="selectedFiles.length === 0" @click="emit('batch-identify')">
          <template #icon><n-icon :component="SparklesOutline" /></template>
          批量识别
        </n-button>
        <n-button type="success" ghost :disabled="selectedFiles.length === 0" @click="emit('batch-directory-identify')">
          <template #icon><n-icon :component="FolderOpenOutline" /></template>
          整目录识别
        </n-button>
        <n-tooltip :disabled="!isCloud115" placement="top">
          <template #trigger>
            <n-button type="warning" :disabled="selectedFiles.length === 0 || isCloud115" @click="emit('batch-rename')">
              <template #icon><n-icon :component="CreateOutline" /></template>
              批量重命名
            </n-button>
          </template>
          115 云盘暂不支持单独批量重命名，请使用整理功能
        </n-tooltip>
        <n-button v-if="!isCloud115" type="info" :disabled="selectedFiles.length === 0" @click="emit('batch-scrape')">
          <template #icon><n-icon :component="DocumentTextOutline" /></template>
          批量刮削
        </n-button>
        <n-button type="info" ghost :disabled="selectedFiles.length === 0 || isCloud115" @click="emit('batch-directory-scrape')">
          <template #icon><n-icon :component="FolderOpenOutline" /></template>
          整目录刮削
        </n-button>
        <n-button type="success" class="min-w-[132px] font-bold" :disabled="selectedFiles.length === 0" @click="emit('open-organize')">
          <template #icon><n-icon :component="FileTrayFullOutline" /></template>
          批量整理{{ selectedFiles.length ? ` (${selectedFiles.length})` : '' }}
        </n-button>
      </div>

      <!-- 批量操作：移动端下拉菜单 -->
      <div class="md:hidden">
        <n-dropdown trigger="click" :options="batchActionOptions" @select="handleBatchAction">
          <n-button type="primary" size="small" icon-placement="right">
            <template #icon><n-icon :component="ChevronDownOutline" /></template>
            批量操作
          </n-button>
        </n-dropdown>
      </div>

      <!-- 文件列表 -->
      <div class="overflow-x-auto">
        <n-data-table
          :columns="columns"
          :data="filteredFileList"
          :loading="loading"
          :max-height="500"
          :striped="true"
          :row-key="rowKey"
          :row-props="rowProps"
          :checked-row-keys="checkedRowKeys"
          :scroll-x="1180"
          @update:checked-row-keys="handleCheckedRowKeys"
        />
      </div>

      <!-- 分页 -->
      <div class="flex flex-wrap justify-center md:justify-end">
        <n-pagination
          :page="currentPage"
          :page-size="pageSize"
          :item-count="total"
          :page-sizes="[20, 50, 100, 200]"
          show-size-picker
          show-quick-jumper
          :prefix="paginationPrefix"
          @update:page="handleCurrentChange"
          @update:page-size="handleSizeChange"
        />
      </div>
    </div>
  </n-modal>
</template>

<script setup>
import { ref, computed, onMounted, onUnmounted, h, watch } from 'vue'
import {
  NModal,
  NBreadcrumb,
  NBreadcrumbItem,
  NButton,
  NIcon,
  NInput,
  NSelect,
  NTooltip,
  NDropdown,
  NDataTable,
  NPagination,
  useMessage
} from 'naive-ui'
import {
  HomeOutline,
  FolderOutline,
  FolderOpenOutline,
  ArrowBackOutline,
  SearchOutline,
  RefreshOutline,
  SparklesOutline,
  CreateOutline,
  DocumentTextOutline,
  FileTrayFullOutline,
  TrashOutline,
  VideocamOutline,
  HeadsetOutline,
  ChevronDownOutline
} from '@vicons/ionicons5'
import { getMediaFiles } from '../../utils/api/media'

const props = defineProps({
  /** 当前媒体源 */
  currentSource: {
    type: Object,
    default: null
  }
})

const emit = defineEmits([
  'selection-change',
  'identify',
  'rename',
  'single-organize',
  'scrape',
  'delete-file',
  'batch-identify',
  'batch-rename',
  'batch-scrape',
  'batch-directory-identify',
  'batch-directory-scrape',
])
const visible = defineModel('visible', { type: Boolean, default: false })
const message = useMessage()
const activeSource = ref(null)

// --- 响应式布局状态 ---
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

// --- 导航状态 ---
const currentPath = ref('/')
const directoryStack = ref([])

// --- 文件列表状态 ---
const fileList = ref([])
const selectedFiles = ref([])
const checkedRowKeys = ref([])
const loading = ref(false)
const searchKeyword = ref('')
const filterType = ref('')
const currentPage = ref(1)
const pageSize = ref(50)
const total = ref(0)

const filterTypeOptions = [
  { label: '全部', value: '' },
  { label: '视频', value: 'video' },
  { label: '文件夹', value: 'folder' },
  { label: '已识别', value: 'identified' },
  { label: '未识别', value: 'unidentified' }
]

// --- 计算属性 ---
/** 是否为 115 云盘媒体源 */
const isCloud115 = computed(() => {
  return (activeSource.value || props.currentSource)?.source_type === 'cloud115'
})

/** 媒体源根路径 */
const sourceRootPath = computed(() => {
  const source = activeSource.value || props.currentSource
  if (!source) return '/'
  return source.path || '/'
})

/** 115 云盘当前显示路径 */
const currentCloud115DisplayPath = computed(() => {
  if (!isCloud115.value) return currentPath.value
  if (directoryStack.value.length === 0) return '/'
  return '/' + directoryStack.value.map(item => item.name).join('/')
})

/** 面包屑导航项 */
const breadcrumbItems = computed(() => {
  const source = activeSource.value || props.currentSource
  if (source?.source_type === 'cloud115') {
    return directoryStack.value.map(item => ({
      name: item.name,
      path: item.cid
    }))
  }
  if (currentPath.value === '/') return []
  const parts = currentPath.value.split('/').filter(p => p)
  return parts.map((part, index) => ({
    name: part,
    path: '/' + parts.slice(0, index + 1).join('/')
  }))
})

/** 当前目录名称 */
const currentDirectoryName = computed(() => {
  if (!props.currentSource) return ''
  if (isCloud115.value) {
    if (directoryStack.value.length === 0) return props.currentSource.name || '根目录'
    return directoryStack.value[directoryStack.value.length - 1].name
  }
  if (currentPath.value === '/') return props.currentSource.name || '根目录'
  const parts = currentPath.value.split('/').filter(p => p)
  return parts[parts.length - 1] || props.currentSource.name || '根目录'
})

const currentSourceName = computed(() => {
  const source = activeSource.value || props.currentSource
  return source?.name || '未选择媒体源'
})

const currentDisplayPath = computed(() => {
  return isCloud115.value ? currentCloud115DisplayPath.value : currentPath.value
})

/** 是否可以返回上一级 */
const canGoBack = computed(() => {
  const source = activeSource.value || props.currentSource
  if (!source) return false
  if (source.source_type === 'cloud115') {
    return directoryStack.value.length > 0
  }
  return currentPath.value !== '/'
})

/** 本地过滤文件列表 */
const filteredFileList = computed(() => {
  if (filterType.value === 'identified') {
    return fileList.value.filter(f => f.tmdb_title)
  }
  if (filterType.value === 'unidentified') {
    return fileList.value.filter(f => !f.tmdb_title && !f.is_dir)
  }
  return fileList.value
})

const directoryCount = computed(() => filteredFileList.value.filter(item => item.is_dir).length)
const videoCount = computed(() => filteredFileList.value.filter(item => !item.is_dir && getFileType(item.name) === 'video').length)
const identifiedCount = computed(() => filteredFileList.value.filter(item => item.tmdb_title).length)
const browserOverviewCards = computed(() => [
  {
    label: '当前目录项目',
    value: total.value || filteredFileList.value.length,
    hint: `${directoryCount.value} 个目录，${videoCount.value} 个视频文件`
  },
  {
    label: 'TMDB 已识别',
    value: identifiedCount.value,
    hint: filteredFileList.value.length ? '可继续执行整理或刮削' : '进入目录后开始处理文件'
  },
  {
    label: '筛选模式',
    value: filterType.value || '全部',
    hint: searchKeyword.value ? `关键词：${searchKeyword.value}` : '当前未设置关键词搜索'
  },
  {
    label: '批量操作状态',
    value: selectedFiles.value.length ? '已就绪' : '待选择',
    hint: selectedFiles.value.length ? `已选 ${selectedFiles.value.length} 项` : '勾选文件后可直接批量处理'
  }
])

/** 移动端批量操作下拉菜单 */
const batchActionOptions = computed(() => {
  const noneSelected = selectedFiles.value.length === 0
  const options = [
    { label: '批量识别', key: 'batch-identify', disabled: noneSelected, icon: renderIcon(SparklesOutline) },
    { label: '整目录识别', key: 'batch-directory-identify', disabled: noneSelected, icon: renderIcon(FolderOpenOutline) },
    { label: '批量重命名', key: 'batch-rename', disabled: noneSelected || isCloud115.value, icon: renderIcon(CreateOutline) }
  ]
  if (!isCloud115.value) {
    options.push({ label: '批量刮削', key: 'batch-scrape', disabled: noneSelected, icon: renderIcon(DocumentTextOutline) })
  }
  options.push({ label: '整目录刮削', key: 'batch-directory-scrape', disabled: noneSelected || isCloud115.value, icon: renderIcon(FolderOpenOutline) })
  options.push({
    label: `批量整理${selectedFiles.value.length ? ` (${selectedFiles.value.length})` : ''}`,
    key: 'open-organize',
    disabled: noneSelected,
    icon: renderIcon(FileTrayFullOutline)
  })
  return options
})

const handleBatchAction = (key) => {
  emit(key)
}

// --- 方法 ---
/**
 * 归一化文件项字段
 * 后端 115 与本地存储返回的字段名存在差异，这里统一处理
 */
const normalizeFileItem = (file) => ({
  ...file,
  is_dir: Boolean(file.is_dir || file.is_directory),
  id: file.id || file.cid || ''
})

/**
 * 获取文件列表
 */
const fetchFileList = async () => {
  const source = activeSource.value || props.currentSource
  if (!source) return

  loading.value = true
  try {
    const serverFilterType = ['video', 'folder'].includes(filterType.value) ? filterType.value : ''
    const params = {
      source_id: source.id,
      path: currentPath.value,
      page: currentPage.value,
      page_size: pageSize.value,
      keyword: searchKeyword.value,
      type: serverFilterType
    }

    const response = await getMediaFiles(params)
    const apiData = response.data.data
    fileList.value = (apiData?.files || []).map(normalizeFileItem)
    total.value = apiData?.total || 0
  } catch (error) {
    console.error('[FileBrowser] 获取文件列表失败:', error)
    const errorMsg = error.response?.data?.error || error.message || '获取文件列表失败'
    message.error(errorMsg)
  } finally {
    loading.value = false
  }
}

/**
 * 导航到根目录
 */
const navigateToRoot = () => {
  const source = activeSource.value || props.currentSource
  if (source?.source_type === 'cloud115') {
    currentPath.value = sourceRootPath.value || '/'
    directoryStack.value = []
  } else {
    currentPath.value = '/'
  }
  currentPage.value = 1
  fetchFileList()
}

/**
 * 导航到上一级目录
 */
const navigateToParent = () => {
  const source = activeSource.value || props.currentSource
  if (!source) return

  if (source.source_type === 'cloud115') {
    if (directoryStack.value.length > 0) {
      directoryStack.value.pop()
      const parent = directoryStack.value.length > 0
        ? directoryStack.value[directoryStack.value.length - 1].cid
        : (sourceRootPath.value || '/')
      currentPath.value = parent
    } else {
      currentPath.value = sourceRootPath.value || '/'
    }
  } else {
    if (currentPath.value === '/') return
    const parts = currentPath.value.split('/').filter(p => p)
    parts.pop()
    currentPath.value = parts.length > 0 ? '/' + parts.join('/') : '/'
  }
  currentPage.value = 1
  fetchFileList()
}

/**
 * 导航到指定路径
 */
const navigateToPath = (path) => {
  const source = activeSource.value || props.currentSource
  if (source?.source_type === 'cloud115') {
    const idx = directoryStack.value.findIndex(item => item.cid === path)
    if (idx >= 0) {
      directoryStack.value = directoryStack.value.slice(0, idx + 1)
    }
  }
  currentPath.value = path
  currentPage.value = 1
  fetchFileList()
}

/**
 * 双击进入文件夹
 */
const handleRowDblClick = (row) => {
  if (!row.is_dir) return

  const source = activeSource.value || props.currentSource
  if (source?.source_type === 'cloud115') {
    directoryStack.value.push({ name: row.name, cid: row.cid || row.id })
    currentPath.value = row.cid || row.id
  } else {
    currentPath.value = currentPath.value === '/'
      ? `/${row.name}`
      : `${currentPath.value}/${row.name}`
  }
  currentPage.value = 1
  fetchFileList()
}

const handleSearch = () => {
  currentPage.value = 1
  fetchFileList()
}

const handleRefresh = () => {
  fetchFileList()
}

/** 多选行 key（本地文件可能没有 id，回退到 path/name） */
const rowKey = (row) => row.id || row.path || row.name

const handleCheckedRowKeys = (keys) => {
  checkedRowKeys.value = keys
  selectedFiles.value = fileList.value.filter(file => keys.includes(rowKey(file)))
  emit('selection-change', selectedFiles.value)
}

// 数据变化（重新加载 / 客户端筛选切换）时清空选择，与原 el-table 行为一致
watch(filteredFileList, () => {
  if (checkedRowKeys.value.length > 0) {
    checkedRowKeys.value = []
    selectedFiles.value = []
    emit('selection-change', [])
  }
})

const handleSizeChange = (size) => {
  pageSize.value = size
  fetchFileList()
}

const handleCurrentChange = (page) => {
  currentPage.value = page
  fetchFileList()
}

const paginationPrefix = ({ itemCount }) => `共 ${itemCount} 项`

/**
 * 格式化文件大小
 */
const formatFileSize = (bytes) => {
  if (!bytes || bytes === 0) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i]
}

/**
 * 根据文件扩展名获取文件类型
 */
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

// --- 表格渲染 ---
const renderIcon = (icon) => () => h(NIcon, { component: icon })

const renderRowIcon = (row) => {
  if (row.is_dir) {
    return h(NIcon, { component: FolderOpenOutline, class: 'shrink-0 text-xl text-amber-500' })
  }
  const type = getFileType(row.name)
  if (type === 'video') {
    return h(NIcon, { component: VideocamOutline, class: 'shrink-0 text-lg text-cyan-500' })
  }
  if (type === 'audio') {
    return h(NIcon, { component: HeadsetOutline, class: 'shrink-0 text-lg text-emerald-500' })
  }
  return h(NIcon, { component: DocumentTextOutline, class: 'shrink-0 text-lg text-slate-400' })
}

/** 行操作：桌面端按钮组 */
const renderDesktopRowActions = (row) => {
  const buttons = [
    h(NButton, {
      size: 'small',
      type: 'primary',
      onClick: () => emit('identify', row)
    }, { icon: renderIcon(SparklesOutline), default: () => '识别' }),
    h(NTooltip, { disabled: !isCloud115.value, placement: 'top' }, {
      trigger: () => h(NButton, {
        size: 'small',
        type: 'warning',
        disabled: isCloud115.value,
        onClick: () => emit('rename', row)
      }, { icon: renderIcon(CreateOutline), default: () => '重命名' }),
      default: () => '115 云盘暂不支持单独批量重命名，请使用整理功能'
    })
  ]
  if (row.is_dir || getFileType(row.name) === 'video') {
    buttons.push(h(NButton, {
      size: 'small',
      type: 'success',
      onClick: () => emit('single-organize', row)
    }, { icon: renderIcon(FileTrayFullOutline), default: () => '整理' }))
  }
  if (!isCloud115.value && !row.is_dir && getFileType(row.name) === 'video') {
    buttons.push(h(NButton, {
      size: 'small',
      type: 'info',
      onClick: () => emit('scrape', row)
    }, { icon: renderIcon(DocumentTextOutline), default: () => '刮削' }))
  }
  buttons.push(h(NButton, {
    size: 'small',
    type: 'error',
    onClick: () => emit('delete-file', row)
  }, { icon: renderIcon(TrashOutline), default: () => '删除' }))
  return h('div', { class: 'flex flex-wrap items-center justify-center gap-1' }, buttons)
}

/** 行操作：移动端下拉菜单 */
const buildRowActionOptions = (row) => {
  const options = [
    { label: '识别', key: 'identify', icon: renderIcon(SparklesOutline) },
    { label: '重命名', key: 'rename', disabled: isCloud115.value, icon: renderIcon(CreateOutline) }
  ]
  if (row.is_dir || getFileType(row.name) === 'video') {
    options.push({ label: '整理', key: 'single-organize', icon: renderIcon(FileTrayFullOutline) })
  }
  if (!isCloud115.value && !row.is_dir && getFileType(row.name) === 'video') {
    options.push({ label: '刮削', key: 'scrape', icon: renderIcon(DocumentTextOutline) })
  }
  options.push({ type: 'divider', key: 'divider-delete' })
  options.push({ label: '删除', key: 'delete-file', icon: renderIcon(TrashOutline) })
  return options
}

const renderMobileRowActions = (row) => h(NDropdown, {
  trigger: 'click',
  options: buildRowActionOptions(row),
  onSelect: (key) => emit(key, row)
}, {
  default: () => h(NButton, {
    size: 'small',
    type: 'primary',
    iconPlacement: 'right'
  }, { icon: renderIcon(ChevronDownOutline), default: () => '操作' })
})

// render 中读取的 ref（isCloud115 / isMobile）会被表格渲染副作用跟踪，自动响应更新
const columns = computed(() => [
  { type: 'selection', width: 48 },
  {
    title: '文件名',
    key: 'name',
    minWidth: 300,
    render: (row) => h('div', {
      class: [
        'flex items-center gap-2',
        row.is_dir ? 'cursor-pointer font-medium text-slate-800 dark:text-white' : ''
      ]
    }, [
      renderRowIcon(row),
      h('span', { class: 'break-all' }, row.name)
    ])
  },
  {
    title: '大小',
    key: 'size',
    width: 110,
    align: 'center',
    render: (row) => (row.is_dir ? '-' : formatFileSize(row.size))
  },
  { title: '修改时间', key: 'modify_time', width: 170, align: 'center' },
  {
    title: 'TMDB识别',
    key: 'tmdb_title',
    minWidth: 200,
    render: (row) => (row.tmdb_title
      ? h('span', row.tmdb_title)
      : h('span', { class: 'text-xs text-slate-400 dark:text-slate-500' }, '未识别'))
  },
  {
    title: '操作',
    key: 'actions',
    width: isMobile.value ? 120 : 360,
    align: 'center',
    fixed: 'right',
    render: (row) => (isMobile.value ? renderMobileRowActions(row) : renderDesktopRowActions(row))
  }
])

/** 行属性：目录行手型光标 + 双击进入 */
const rowProps = (row) => ({
  style: row.is_dir ? 'cursor: pointer;' : '',
  onDblclick: () => handleRowDblClick(row)
})

// --- 暴露方法供父组件调用 ---
/**
 * 打开文件浏览对话框并初始化
 */
const open = (source) => {
  activeSource.value = source || props.currentSource || null
  currentPath.value = '/'
  currentPage.value = 1
  searchKeyword.value = ''
  filterType.value = ''
  directoryStack.value = []
  visible.value = true
  fetchFileList()
}

/**
 * 刷新当前文件列表
 */
const refresh = () => {
  fetchFileList()
}

/**
 * 获取当前路径，供父组件使用
 */
const getCurrentPath = () => currentPath.value

/**
 * 获取当前显示路径，供父组件使用
 */
const getCurrentDisplayPath = () => currentCloud115DisplayPath.value

/**
 * 获取选中的文件列表
 */
const getSelectedFiles = () => selectedFiles.value

defineExpose({
  open,
  refresh,
  fetchFileList,
  getCurrentPath,
  getCurrentDisplayPath,
  getSelectedFiles,
  currentPath,
  directoryStack,
  selectedFiles,
  isCloud115,
  currentCloud115DisplayPath
})
</script>
