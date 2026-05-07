<!--
  FileBrowserDialog - 文件浏览对话框
  支持面包屑导航、搜索过滤、文件选择，以及识别、重命名、整理、刮削、删除等操作
  兼容本地存储和 115 云盘两种媒体源类型
-->
<template>
  <el-dialog
    v-model="visible"
    title="文件浏览"
    :width="isMobile ? '100%' : '92%'"
    top="4vh"
    append-to-body
  >
    <section class="workspace-overview">
      <div class="workspace-overview__copy">
        <h2>{{ currentDirectoryName || '文件浏览工作区' }}</h2>
        <p>
          当前正在浏览 {{ isCloud115 ? '115 云盘媒体源' : '本地媒体源' }}，
          可在这里完成目录切换、筛选、识别、重命名、整理和删除操作。
        </p>
      </div>
      <div class="workspace-overview__grid">
        <article v-for="card in browserOverviewCards" :key="card.label" class="overview-card">
          <span class="overview-card__label">{{ card.label }}</span>
          <strong class="overview-card__value">{{ card.value }}</strong>
          <p class="overview-card__hint">{{ card.hint }}</p>
        </article>
      </div>
    </section>

    <div class="breadcrumb-container">
      <el-breadcrumb separator="/">
        <el-breadcrumb-item>
          <span class="breadcrumb-link" @click="navigateToRoot">
            <el-icon style="margin-right: 4px;"><HomeFilled /></el-icon>
            {{ isCloud115 ? (currentSource?.name || '根目录') : '根目录' }}
          </span>
        </el-breadcrumb-item>
        <el-breadcrumb-item
          v-for="(item, index) in breadcrumbItems"
          :key="index"
        >
          <span class="breadcrumb-link" @click="navigateToPath(item.path)">
            <el-icon style="margin-right: 4px;"><Folder /></el-icon>
            {{ item.name }}
          </span>
        </el-breadcrumb-item>
      </el-breadcrumb>
      <div class="breadcrumb-actions" v-if="canGoBack">
        <el-button size="small" @click="navigateToRoot">
          <el-icon><HomeFilled /></el-icon>
          返回根目录
        </el-button>
        <el-button size="small" @click="navigateToParent">
          <el-icon><Back /></el-icon>
          返回上一级
        </el-button>
      </div>
    </div>

    <div class="filter-container">
      <div class="status-rail">
        <div class="status-rail__item">
          <span>当前源</span>
          <strong>{{ currentSourceName }}</strong>
        </div>
        <div class="status-rail__item">
          <span>当前显示路径</span>
          <strong>{{ currentDisplayPath }}</strong>
        </div>
        <div class="status-rail__item">
          <span>已选文件</span>
          <strong>{{ selectedFiles.length }} 项</strong>
        </div>
      </div>

      <div class="filter-search-row">
        <el-input
          v-model="searchKeyword"
          placeholder="搜索文件名"
          clearable
          :style="{ width: isMobile ? '100%' : '300px' }"
          @keyup.enter="handleSearch"
        >
          <template #prefix>
            <el-icon><Search /></el-icon>
          </template>
        </el-input>
        <el-select v-model="filterType" placeholder="文件类型" clearable :style="{ width: isMobile ? '100%' : '150px', marginLeft: isMobile ? '0' : '10px' }">
          <el-option label="全部" value="" />
          <el-option label="视频" value="video" />
          <el-option label="文件夹" value="folder" />
          <el-option label="已识别" value="identified" />
          <el-option label="未识别" value="unidentified" />
        </el-select>
        <el-button type="primary" @click="handleSearch" :style="{ marginLeft: isMobile ? '0' : '10px' }">
          <el-icon><Search /></el-icon>
          搜索
        </el-button>
        <el-button @click="handleRefresh">
          <el-icon><Refresh /></el-icon>
          刷新
        </el-button>
      </div>

      <div class="filter-action-row">
        <div v-if="!isMobile" class="batch-actions-desktop">
          <el-button type="success" @click="emit('batch-identify')" :disabled="selectedFiles.length === 0">
            <el-icon><MagicStick /></el-icon>
            批量识别
          </el-button>
          <el-button type="success" plain @click="emit('batch-directory-identify')" :disabled="selectedFiles.length === 0">
            <el-icon><FolderOpened /></el-icon>
            整目录识别
          </el-button>
          <el-tooltip :disabled="!isCloud115" content="115 云盘暂不支持单独批量重命名，请使用整理功能" placement="top">
            <el-button type="warning" @click="emit('batch-rename')" :disabled="selectedFiles.length === 0 || isCloud115">
              <el-icon><Edit /></el-icon>
              批量重命名
            </el-button>
          </el-tooltip>
          <el-tooltip :disabled="!isCloud115" content="115 云盘暂不支持刮削" placement="top">
            <el-button type="info" @click="emit('batch-scrape')" :disabled="selectedFiles.length === 0 || isCloud115" v-if="!isCloud115">
              <el-icon><Document /></el-icon>
              批量刮削
            </el-button>
          </el-tooltip>
          <el-button type="info" plain @click="emit('batch-directory-scrape')" :disabled="selectedFiles.length === 0 || isCloud115">
            <el-icon><FolderOpened /></el-icon>
            整目录刮削
          </el-button>
          <el-button class="organize-primary-btn" type="success" @click="emit('open-organize')" :disabled="selectedFiles.length === 0">
            <el-icon><Files /></el-icon>
            批量整理{{ selectedFiles.length ? ` (${selectedFiles.length})` : '' }}
          </el-button>
        </div>

        <div v-else class="batch-actions-mobile">
          <el-dropdown trigger="click">
            <el-button type="primary" size="small">
              批量操作 <el-icon class="el-icon--right"><ArrowDown /></el-icon>
            </el-button>
            <template #dropdown>
              <el-dropdown-menu>
                <el-dropdown-item @click="emit('batch-identify')" :disabled="selectedFiles.length === 0">
                  <el-icon><MagicStick /></el-icon> 批量识别
                </el-dropdown-item>
                <el-dropdown-item @click="emit('batch-directory-identify')" :disabled="selectedFiles.length === 0">
                  <el-icon><FolderOpened /></el-icon> 整目录识别
                </el-dropdown-item>
                <el-dropdown-item @click="emit('batch-rename')" :disabled="selectedFiles.length === 0 || isCloud115">
                  <el-icon><Edit /></el-icon> 批量重命名
                </el-dropdown-item>
                <el-dropdown-item v-if="!isCloud115" @click="emit('batch-scrape')" :disabled="selectedFiles.length === 0">
                  <el-icon><Document /></el-icon> 批量刮削
                </el-dropdown-item>
                <el-dropdown-item @click="emit('batch-directory-scrape')" :disabled="selectedFiles.length === 0 || isCloud115">
                  <el-icon><FolderOpened /></el-icon> 整目录刮削
                </el-dropdown-item>
                <el-dropdown-item @click="emit('open-organize')" :disabled="selectedFiles.length === 0">
                  <el-icon><Files /></el-icon> 批量整理{{ selectedFiles.length ? ` (${selectedFiles.length})` : '' }}
                </el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>
        </div>
      </div>
    </div>
    <div class="table-wrapper">
      <el-table
        :data="filteredFileList"
        border
        style="width: 100%"
        stripe
        class="custom-table"
        @selection-change="handleSelectionChange"
        @row-dblclick="handleRowDblClick"
        max-height="500"
        :row-class-name="getRowClassName"
      >
        <el-table-column type="selection" width="55" align="center" />
        <el-table-column prop="name" label="文件名" min-width="300">
          <template #default="scope">
            <div class="file-name" :class="{ 'is-directory': scope.row.is_dir }">
              <el-icon v-if="scope.row.is_dir" class="file-icon folder"><FolderOpened /></el-icon>
              <el-icon v-else-if="getFileType(scope.row.name) === 'video'" class="file-icon video"><VideoCamera /></el-icon>
              <el-icon v-else-if="getFileType(scope.row.name) === 'audio'" class="file-icon audio"><Headset /></el-icon>
              <el-icon v-else class="file-icon other"><Document /></el-icon>
              <span>{{ scope.row.name }}</span>
            </div>
          </template>
        </el-table-column>
        <el-table-column prop="size" label="大小" width="120" align="center">
          <template #default="scope">
            <span v-if="!scope.row.is_dir">{{ formatFileSize(scope.row.size) }}</span>
            <span v-else>-</span>
          </template>
        </el-table-column>
        <el-table-column prop="modify_time" label="修改时间" width="160" align="center" />
        <el-table-column prop="tmdb_title" label="TMDB识别" min-width="200">
          <template #default="scope">
            <span v-if="scope.row.tmdb_title">{{ scope.row.tmdb_title }}</span>
            <span v-else class="text-muted">未识别</span>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="340" fixed="right" align="center">
          <template #default="scope">
            <div class="action-buttons-desktop">
              <el-button type="primary" size="small" @click="emit('identify', scope.row)">
                <el-icon><MagicStick /></el-icon>
                识别
              </el-button>
          <el-tooltip :disabled="!isCloud115" content="115 云盘暂不支持单独批量重命名，请使用整理功能" placement="top">
                <el-button type="warning" size="small" @click="emit('rename', scope.row)" :disabled="isCloud115">
                  <el-icon><Edit /></el-icon>
                  重命名
                </el-button>
              </el-tooltip>
              <el-button
                type="success"
                size="small"
                @click="emit('single-organize', scope.row)"
                v-if="scope.row.is_dir || getFileType(scope.row.name) === 'video'"
              >
                <el-icon><Files /></el-icon>
                整理
              </el-button>
          <el-tooltip :disabled="!isCloud115" content="115 云盘暂不支持单独批量重命名，请使用整理功能" placement="top">
                <el-button
                  type="info"
                  size="small"
                  @click="emit('scrape', scope.row)"
                  :disabled="isCloud115 || scope.row.is_dir || getFileType(scope.row.name) !== 'video'"
                  v-if="!isCloud115 && !scope.row.is_dir && getFileType(scope.row.name) === 'video'"
                >
                  <el-icon><Document /></el-icon>
                  刮削
                </el-button>
              </el-tooltip>
              <el-button type="danger" size="small" @click="emit('delete-file', scope.row)">
                <el-icon><Delete /></el-icon>
                删除
              </el-button>
            </div>

            <div class="action-buttons-mobile">
              <el-dropdown trigger="click">
                <el-button type="primary" size="small">
                  操作 <el-icon class="el-icon--right"><ArrowDown /></el-icon>
                </el-button>
                <template #dropdown>
                  <el-dropdown-menu>
                    <el-dropdown-item @click="emit('identify', scope.row)">
                      <el-icon><MagicStick /></el-icon> 识别
                    </el-dropdown-item>
                    <el-dropdown-item @click="emit('rename', scope.row)" :disabled="isCloud115">
                      <el-icon><Edit /></el-icon> 重命名
                    </el-dropdown-item>
                    <el-dropdown-item
                      v-if="scope.row.is_dir || getFileType(scope.row.name) === 'video'"
                      @click="emit('single-organize', scope.row)"
                    >
                      <el-icon><Files /></el-icon> 整理
                    </el-dropdown-item>
                    <el-dropdown-item
                      v-if="!isCloud115 && !scope.row.is_dir && getFileType(scope.row.name) === 'video'"
                      @click="emit('scrape', scope.row)"
                    >
                      <el-icon><Document /></el-icon> 刮削
                    </el-dropdown-item>
                    <el-dropdown-item @click="emit('delete-file', scope.row)" divided>
                      <el-icon><Delete /></el-icon> 删除
                    </el-dropdown-item>
                  </el-dropdown-menu>
                </template>
              </el-dropdown>
            </div>
          </template>
        </el-table-column>
      </el-table>
    </div>

    <div class="pagination-container">
      <el-pagination
        v-model:current-page="currentPage"
        v-model:page-size="pageSize"
        :page-sizes="[20, 50, 100, 200]"
        layout="total, sizes, prev, pager, next, jumper"
        :total="total"
        @size-change="handleSizeChange"
        @current-change="handleCurrentChange"
      />
    </div>
  </el-dialog>
</template>

<script setup>
import { ref, computed, onMounted, onUnmounted } from 'vue'
import {
  FolderOpened,
  Folder,
  Document,
  Search,
  Refresh,
  MagicStick,
  Edit,
  Delete,
  VideoCamera,
  Headset,
  Files,
  HomeFilled,
  Back,
  ArrowDown
} from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
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
const searchKeyword = ref('')
const filterType = ref('')
const currentPage = ref(1)
const pageSize = ref(50)
const total = ref(0)

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
    ElMessage.error(errorMsg)
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

const handleSelectionChange = (selection) => {
  selectedFiles.value = selection
  emit('selection-change', selection)
}

const handleSizeChange = () => {
  fetchFileList()
}

const handleCurrentChange = () => {
  fetchFileList()
}

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

const getRowClassName = ({ row }) => {
  return row.is_dir ? 'directory-row' : 'file-row'
}

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

<style scoped>
.workspace-overview {
  display: grid;
  grid-template-columns: minmax(260px, 1fr) minmax(0, 2fr);
  gap: 18px;
  margin-bottom: 18px;
}

.workspace-overview__copy {
  padding: 20px;
  border-radius: 22px;
  background: linear-gradient(160deg, rgba(31, 111, 120, 0.12), rgba(242, 166, 90, 0.12));
  border: 1px solid rgba(31, 111, 120, 0.12);
}

.workspace-overview__copy h2 {
  margin: 0;
  font-size: 24px;
  color: #17313a;
}

.workspace-overview__copy p {
  margin: 12px 0 0;
  color: #6c6259;
  line-height: 1.7;
}

.workspace-overview__grid {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 14px;
}

.overview-card {
  padding: 18px;
  border-radius: 20px;
  background: linear-gradient(180deg, rgba(255, 255, 255, 0.94), rgba(247, 241, 231, 0.94));
  border: 1px solid rgba(120, 101, 72, 0.1);
}

.overview-card__label {
  display: block;
  color: #8a7b6d;
  font-size: 13px;
}

.overview-card__value {
  display: block;
  margin-top: 12px;
  font-size: 28px;
  color: #17313a;
  line-height: 1.1;
}

.overview-card__hint {
  margin: 10px 0 0;
  color: #73675d;
  line-height: 1.6;
}

.breadcrumb-container {
  margin-bottom: 15px;
  padding: 10px 15px;
  background-color: #f5f7fa;
  border-radius: 8px;
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.breadcrumb-container :deep(.el-breadcrumb__item) {
  cursor: pointer;
}

.breadcrumb-container :deep(.el-breadcrumb__item:hover) {
  color: #409eff;
}

.breadcrumb-container :deep(.el-breadcrumb__inner) {
  display: flex;
  align-items: center;
}

.breadcrumb-link {
  display: flex;
  align-items: center;
  cursor: pointer;
  transition: color 0.3s;
}

.breadcrumb-link:hover {
  color: #409eff;
}

.breadcrumb-actions {
  flex-shrink: 0;
}

.filter-container {
  display: flex;
  flex-direction: column;
  margin-bottom: 15px;
  gap: 10px;
}

.status-rail {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 12px;
}

.status-rail__item {
  padding: 14px 16px;
  border-radius: 18px;
  background: rgba(244, 239, 231, 0.88);
  border: 1px solid rgba(120, 101, 72, 0.08);
}

.status-rail__item span {
  display: block;
  color: #8a7b6d;
  font-size: 12px;
}

.status-rail__item strong {
  display: block;
  margin-top: 8px;
  color: #17313a;
  font-size: 16px;
  line-height: 1.5;
  word-break: break-all;
}

.filter-search-row {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 10px;
}

.filter-action-row {
  display: flex;
  align-items: center;
}

.batch-actions-desktop {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}

.batch-actions-mobile {
  display: none;
}

.organize-primary-btn {
  min-width: 132px;
  border: none;
  font-weight: 700;
  letter-spacing: 0.02em;
  background: linear-gradient(135deg, #12b981 0%, #079669 100%);
  box-shadow: 0 10px 22px rgba(18, 185, 129, 0.26);
}

.organize-primary-btn:not(.is-disabled):hover {
  transform: translateY(-1px);
  box-shadow: 0 14px 28px rgba(18, 185, 129, 0.34);
}

.organize-primary-btn.is-disabled {
  opacity: 0.58;
  box-shadow: 0 6px 16px rgba(18, 185, 129, 0.18);
}

/* 表格水平滚动容器 */
.table-wrapper {
  overflow-x: auto;
  -webkit-overflow-scrolling: touch;
}

.custom-table {
  border-radius: 8px;
  overflow: hidden;
}

.custom-table :deep(.el-table__header th) {
  background-color: #f8f9fa !important;
  color: #495057;
  font-weight: 600;
}

.custom-table :deep(.el-table__row:hover > td) {
  background-color: #e8f4fd !important;
}

.custom-table :deep(.directory-row) {
  cursor: pointer;
}

.custom-table :deep(.directory-row:hover > td) {
  background-color: #fff3e0 !important;
}

.custom-table :deep(.file-row) {
  cursor: default;
}

/* --- 操作按钮：桌面端 / 移动端切换 --- */
.action-buttons-mobile {
  display: none;
}

.action-buttons-desktop {
  display: flex;
  justify-content: center;
  gap: 4px;
  flex-wrap: wrap;
}

.file-name {
  display: flex;
  align-items: center;
  gap: 8px;
}

.file-name.is-directory {
  cursor: pointer;
}

.file-name.is-directory:hover {
  color: #409eff;
}

.file-icon {
  font-size: 18px;
  flex-shrink: 0;
}

.file-icon.folder {
  color: #f7b32b;
  font-size: 20px;
}

.file-icon.video {
  color: #409eff;
  font-size: 19px;
}

.file-icon.audio {
  color: #67c23a;
  font-size: 19px;
}

.file-icon.other {
  color: #909399;
  font-size: 17px;
}

.text-muted {
  color: #909399;
  font-size: 13px;
}

.pagination-container {
  display: flex;
  justify-content: flex-end;
  margin-top: 20px;
  padding: 10px 0;
}

:global(.dark) .workspace-overview__copy,
:global(.dark) .overview-card,
:global(.dark) .status-rail__item,
:global(.dark) .breadcrumb-container {
  background: rgba(16, 26, 37, 0.88);
  border-color: rgba(139, 163, 185, 0.12);
}

:global(.dark) .workspace-overview__copy h2,
:global(.dark) .overview-card__value,
:global(.dark) .status-rail__item strong,
:global(.dark) .file-name,
:global(.dark) .breadcrumb-link {
  color: #e8edf4;
}

:global(.dark) .workspace-overview__copy p,
:global(.dark) .overview-card__hint,
:global(.dark) .overview-card__label,
:global(.dark) .status-rail__item span,
:global(.dark) .text-muted {
  color: #9faebb;
}

/* ===== 响应式：移动端（< 768px） ===== */
@media (max-width: 768px) {
  .workspace-overview,
  .workspace-overview__grid,
  .status-rail {
    grid-template-columns: 1fr;
  }

  .breadcrumb-container {
    flex-direction: column;
    align-items: flex-start;
    gap: 8px;
    padding: 8px 12px;
  }

  .breadcrumb-actions {
    width: 100%;
    display: flex;
    gap: 8px;
  }

  .filter-search-row {
    width: 100%;
  }

  .filter-search-row .el-input,
  .filter-search-row .el-select {
    flex: 1;
  }

  /* 移动端：批量操作切换为下拉菜单 */
  .batch-actions-desktop {
    display: none;
  }

  .batch-actions-mobile {
    display: block;
  }

  /* 移动端：行操作按钮切换为下拉菜单 */
  .action-buttons-desktop {
    display: none;
  }

  .action-buttons-mobile {
    display: block;
  }

  /* 表格最小宽度确保可横向滚动 */
  .custom-table {
    min-width: 900px;
  }

  /* 分页简化 */
  .pagination-container {
    justify-content: center;
  }

  .pagination-container :deep(.el-pagination) {
    flex-wrap: wrap;
    justify-content: center;
  }
}
</style>
