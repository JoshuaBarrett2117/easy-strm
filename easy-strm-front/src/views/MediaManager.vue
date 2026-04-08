<template>
  <div class="media-manager-container">
    <!-- 媒体源管理卡片 -->
    <el-card shadow="hover" class="source-card">
      <template #header>
        <div class="card-header">
          <div class="header-title">
            <el-icon class="header-icon"><FolderOpened /></el-icon>
            <span>媒体源管理</span>
          </div>
          <el-button type="primary" @click="handleAddSource">
            <el-icon><Plus /></el-icon>
            新增媒体源
          </el-button>
        </div>
      </template>

      <el-table :data="mediaSources" border style="width: 100%" stripe class="custom-table">
        <el-table-column prop="id" label="ID" width="60" align="center" />
        <el-table-column prop="name" label="名称" min-width="120" />
        <el-table-column prop="source_type" label="类型" width="120" align="center">
          <template #default="scope">
            <el-tag :type="scope.row.source_type === 'local' ? 'success' : 'primary'" size="small">
              {{ scope.row.source_type === 'local' ? '本地存储' : '115云盘' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="path" label="路径" min-width="200" show-overflow-tooltip />
        <el-table-column prop="cloud115_name" label="关联账号" width="120" align="center">
          <template #default="scope">
            <span v-if="scope.row.cloud115_id">{{ scope.row.cloud115_name || '-' }}</span>
            <span v-else class="text-muted">-</span>
          </template>
        </el-table-column>
        <el-table-column prop="create_time" label="创建时间" width="160" align="center" />
        <el-table-column label="操作" width="200" fixed="right" align="center">
          <template #default="scope">
            <el-button type="primary" size="small" @click="handleBrowseFiles(scope.row)">
              <el-icon><Folder /></el-icon>
              浏览
            </el-button>
            <el-button type="warning" size="small" @click="handleEditSource(scope.row)">
              <el-icon><Edit /></el-icon>
              编辑
            </el-button>
            <el-button type="danger" size="small" @click="handleDeleteSource(scope.row)">
              <el-icon><Delete /></el-icon>
              删除
            </el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <!-- 文件浏览弹窗 -->
    <el-dialog
      v-model="fileDialogVisible"
      :title="`文件浏览 - ${currentSource?.name || ''}`"
      width="90%"
      top="5vh"
      :close-on-click-modal="false"
      destroy-on-close
    >
      <!-- 面包屑导航 -->
      <div class="breadcrumb-container">
        <el-breadcrumb separator="/">
          <el-breadcrumb-item>
            <span class="breadcrumb-link" @click="navigateToRoot">
              <el-icon style="margin-right: 4px;"><HomeFilled /></el-icon>
              根目录
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
          <el-button size="small" @click="navigateToParent">
            <el-icon><Back /></el-icon>
            返回上一级
          </el-button>
        </div>
      </div>

      <!-- 搜索和过滤 -->
      <div class="filter-container">
        <el-input
          v-model="searchKeyword"
          placeholder="搜索文件名"
          clearable
          style="width: 300px"
          @keyup.enter="handleSearch"
        >
          <template #prefix>
            <el-icon><Search /></el-icon>
          </template>
        </el-input>
        <el-select v-model="filterType" placeholder="文件类型" clearable style="width: 150px; margin-left: 10px">
          <el-option label="全部" value="" />
          <el-option label="视频" value="video" />
          <el-option label="文件夹" value="folder" />
          <el-option label="已识别" value="identified" />
          <el-option label="未识别" value="unidentified" />
        </el-select>
        <el-button type="primary" @click="handleSearch" style="margin-left: 10px">
          <el-icon><Search /></el-icon>
          搜索
        </el-button>
        <el-button @click="handleRefresh">
          <el-icon><Refresh /></el-icon>
          刷新
        </el-button>
        <div style="flex: 1"></div>
        <el-button type="success" @click="handleBatchIdentify" :disabled="selectedFiles.length === 0">
          <el-icon><MagicStick /></el-icon>
          批量识别
        </el-button>
        <el-tooltip :disabled="!isCloud115Source" content="115 云盘暂不支持单纯的批量重命名，请使用整理" placement="top">
          <el-button type="warning" @click="handleBatchRename" :disabled="selectedFiles.length === 0 || isCloud115Source">
            <el-icon><Edit /></el-icon>
            批量重命名
          </el-button>
        </el-tooltip>
        <el-button class="organize-primary-btn" type="success" @click="handleOpenOrganize" :disabled="selectedFiles.length === 0">
          <el-icon><Files /></el-icon>
          批量整理{{ selectedFiles.length ? ` (${selectedFiles.length})` : '' }}
        </el-button>
      </div>

      <!-- 文件列表 -->
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
            <div class="file-name" :class="{ 'is-directory': scope.row.is_dir || scope.row.is_directory }">
              <el-icon v-if="scope.row.is_dir || scope.row.is_directory" class="file-icon folder"><FolderOpened /></el-icon>
              <el-icon v-else-if="getFileType(scope.row.name) === 'video'" class="file-icon video"><VideoCamera /></el-icon>
              <el-icon v-else-if="getFileType(scope.row.name) === 'audio'" class="file-icon audio"><Headset /></el-icon>
              <el-icon v-else class="file-icon other"><Document /></el-icon>
              <span>{{ scope.row.name }}</span>
            </div>
          </template>
        </el-table-column>
        <el-table-column prop="size" label="大小" width="120" align="center">
          <template #default="scope">
            <span v-if="!(scope.row.is_dir || scope.row.is_directory)">{{ formatFileSize(scope.row.size) }}</span>
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
        <el-table-column label="操作" width="280" fixed="right" align="center">
          <template #default="scope">
            <el-button type="primary" size="small" @click="handleIdentify(scope.row)">
              <el-icon><MagicStick /></el-icon>
              识别
            </el-button>
            <el-tooltip :disabled="!isCloud115Source" content="115 云盘暂不支持重命名" placement="top">
              <el-button type="warning" size="small" @click="handleRename(scope.row)" :disabled="isCloud115Source">
                <el-icon><Edit /></el-icon>
                重命名
              </el-button>
            </el-tooltip>
            <el-button
              type="success"
              size="small"
              @click="handleSingleOrganize(scope.row)"
              v-if="(scope.row.is_dir || scope.row.is_directory) || getFileType(scope.row.name) === 'video'"
            >
              <el-icon><Files /></el-icon>
              整理
            </el-button>
            <el-button type="danger" size="small" @click="handleDeleteFile(scope.row)">
              <el-icon><Delete /></el-icon>
              删除
            </el-button>
          </template>
        </el-table-column>
      </el-table>

      <!-- 分页 -->
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

    <!-- 新增/编辑媒体源对话框 -->
    <el-dialog
      v-model="sourceDialogVisible"
      :title="sourceDialogTitle"
      width="600px"
    >
      <el-form ref="sourceFormRef" :model="sourceForm" :rules="sourceRules" label-width="120px">
        <el-form-item label="名称" prop="name">
          <el-input v-model="sourceForm.name" placeholder="请输入媒体源名称" />
        </el-form-item>
        <el-form-item label="类型" prop="source_type">
          <el-radio-group v-model="sourceForm.source_type" @change="handleSourceTypeChange">
            <el-radio label="local">本地存储</el-radio>
            <el-radio label="cloud115">115云盘</el-radio>
          </el-radio-group>
        </el-form-item>
        <el-form-item label="路径" prop="path">
          <el-input v-model="sourceForm.path" placeholder="请输入路径" />
        </el-form-item>
        <el-form-item v-if="sourceForm.source_type === 'cloud115'" label="关联账号" prop="cloud115_id">
          <el-select v-model="sourceForm.cloud115_id" placeholder="请选择115账号" style="width: 100%">
            <el-option
              v-for="account in cloud115List"
              :key="account.id"
              :label="account.name"
              :value="account.id"
            />
          </el-select>
        </el-form-item>
      </el-form>
      <template #footer>
        <span class="dialog-footer">
          <el-button @click="sourceDialogVisible = false">取消</el-button>
          <el-button type="primary" @click="handleSubmitSource" :loading="sourceLoading">确定</el-button>
        </span>
      </template>
    </el-dialog>

    <!-- TMDB识别对话框 -->
    <el-dialog
      v-model="tmdbDialogVisible"
      title="TMDB识别"
      width="800px"
    >
      <div class="tmdb-container">
        <div class="tmdb-search">
          <el-input
            v-model="tmdbSearchKeyword"
            placeholder="输入电影或剧集名称搜索"
            @keyup.enter="handleTmdbSearch"
          >
            <template #prefix>
              <el-icon><Search /></el-icon>
            </template>
          </el-input>
          <el-select v-model="tmdbType" placeholder="类型" style="width: 120px; margin-left: 10px">
            <el-option label="电影" value="movie" />
            <el-option label="剧集" value="tv" />
          </el-select>
          <el-button type="primary" @click="handleTmdbSearch" style="margin-left: 10px">搜索</el-button>
        </div>

        <div class="tmdb-results" v-loading="tmdbLoading">
          <div v-if="tmdbResults.length > 0" class="result-grid">
            <div
              v-for="item in tmdbResults"
              :key="item.tmdb_id || item.id"
              class="result-item"
              @click="handleSelectTmdb(item)"
            >
              <img v-if="item.poster_path" :src="item.poster_path.trim()" alt="poster" class="poster-img" />
              <div v-else class="no-poster">无海报</div>
              <div class="result-info">
                <div class="result-title">{{ item.title || item.name }}</div>
                <div class="result-year">{{ item.release_date || item.first_air_date || item.year || '-' }}</div>
                <div class="result-overview" v-if="item.overview">{{ item.overview }}</div>
                <div class="result-rating" v-if="item.vote_average">
                  <el-rate :model-value="item.vote_average / 2" disabled show-score :score-template="item.vote_average.toFixed(1)" />
                </div>
              </div>
            </div>
          </div>
          <el-empty v-else description="暂无搜索结果" />
        </div>
      </div>
      <template #footer>
        <el-button @click="tmdbDialogVisible = false">关闭</el-button>
      </template>
    </el-dialog>

    <!-- 重命名预览对话框 -->
    <el-dialog
      v-model="renameDialogVisible"
      title="重命名预览"
      width="900px"
    >
      <div class="rename-container" v-loading="renameLoading">
        <el-table :data="renamePreviewList" border style="width: 100%" stripe>
          <el-table-column prop="original_name" label="原文件名" min-width="300" />
          <el-table-column width="50" align="center">
            <template #default>
              <el-icon><Right /></el-icon>
            </template>
          </el-table-column>
          <el-table-column prop="new_name" label="新文件名" min-width="300" />
        </el-table>
      </div>
      <template #footer>
        <span class="dialog-footer">
          <el-button @click="renameDialogVisible = false">取消</el-button>
          <el-button type="primary" @click="handleExecuteRename" :loading="renameLoading">执行重命名</el-button>
        </span>
      </template>
    </el-dialog>

    <!-- 单个文件重命名对话框 -->
    <el-dialog
      v-model="singleRenameDialogVisible"
      title="重命名文件"
      width="500px"
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

    <el-dialog
      v-model="organizeDialogVisible"
      title="批量整理"
      width="1000px"
    >
      <div class="organize-container" v-loading="organizeLoading">
        <el-form :model="organizeForm" label-width="110px" class="organize-form">
          <el-form-item label="目标目录">
            <el-input v-model="organizeForm.target_path" placeholder="请输入整理后的目标目录" />
          </el-form-item>
          <el-form-item label="媒体类型">
            <el-select v-model="organizeForm.media_type" style="width: 100%">
              <el-option label="全部" value="all" />
              <el-option label="电影" value="movie" />
              <el-option label="剧集" value="tv" />
            </el-select>
          </el-form-item>
          <el-form-item label="冲突策略">
            <el-select v-model="organizeForm.conflict_policy" style="width: 100%">
              <el-option label="跳过" value="skip" />
              <el-option label="覆盖" value="overwrite" />
              <el-option label="追加序号" value="suffix" />
            </el-select>
          </el-form-item>
          <el-form-item label="整理方式">
            <el-select v-model="organizeForm.operation_mode" style="width: 100%">
              <el-option label="移动文件" value="move" />
              <el-option label="复制文件" value="copy" />
              <el-option label="硬链接" value="hardlink" :disabled="isCloud115Source" />
              <el-option label="软链接" value="symlink" :disabled="isCloud115Source" />
            </el-select>
          </el-form-item>
        </el-form>

        <el-alert
          v-if="!organizeHasPreview"
          :title="`已收集 ${organizeCandidateList.length} 个视频文件，点击“刷新预览”后再执行整理`"
          type="info"
          show-icon
          :closable="false"
          style="margin-bottom: 16px"
        />

        <el-alert
          v-if="organizeHasPreview && organizeSummary"
          :title="`共 ${organizeSummary.total} 项，可处理 ${organizeSummary.processable || 0} 项，冲突 ${organizeSummary.conflicts || 0} 项，识别失败 ${organizeSummary.failed || 0} 项`"
          type="info"
          show-icon
          :closable="false"
          style="margin-bottom: 16px"
        />

        <el-alert
          v-if="organizeHasPreview && organizeManualCount > 0"
          :title="`已应用 ${organizeManualCount} 项手动修正的识别结果，执行整理时将优先使用。`"
          type="warning"
          show-icon
          :closable="false"
          style="margin-bottom: 16px"
        />

        <el-table v-if="organizeHasPreview" :data="organizePreviewList" border stripe max-height="420">
          <el-table-column prop="file_name" label="原文件名" min-width="220" />
          <el-table-column prop="title" label="识别结果" min-width="180" />
          <el-table-column prop="new_name" label="新文件名" min-width="220" />
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
              <el-button size="small" @click="handleOrganizePreviewIdentify(scope.row)">手动识别</el-button>
            </template>
          </el-table-column>
        </el-table>

        <el-table v-else :data="organizeCandidateList" border stripe max-height="420">
          <el-table-column prop="file_name" label="候选视频文件" min-width="240" />
          <el-table-column prop="file_path" label="源路径" min-width="320" show-overflow-tooltip />
        </el-table>
      </div>
      <template #footer>
        <span class="dialog-footer">
          <el-button @click="organizeDialogVisible = false">取消</el-button>
          <el-button @click="handlePreviewOrganize" :loading="organizeLoading">刷新预览</el-button>
          <el-button type="primary" @click="handleExecuteOrganize" :loading="organizeExecuting">执行整理</el-button>
        </span>
      </template>
    </el-dialog>

    <el-dialog
      v-model="organizeIdentifyDialogVisible"
      title="修改识别结果"
      width="560px"
    >
      <el-form :model="organizeIdentifyForm" label-width="110px">
        <el-form-item label="原文件名">
          <el-input v-model="organizeIdentifyForm.file_name" disabled />
        </el-form-item>
        <el-form-item label="媒体类型">
          <el-select v-model="organizeIdentifyForm.media_type" style="width: 100%">
            <el-option label="电影" value="movie" />
            <el-option label="剧集" value="tv" />
          </el-select>
        </el-form-item>
        <el-form-item label="标题">
          <el-input v-model="organizeIdentifyForm.title" placeholder="请输入标题" />
        </el-form-item>
        <el-form-item label="年份">
          <el-input-number v-model="organizeIdentifyForm.year" :min="0" :max="9999" style="width: 100%" />
        </el-form-item>
        <el-form-item v-if="organizeIdentifyForm.media_type === 'tv'" label="季数">
          <el-input-number v-model="organizeIdentifyForm.season" :min="0" :max="999" style="width: 100%" />
        </el-form-item>
        <el-form-item v-if="organizeIdentifyForm.media_type === 'tv'" label="集数">
          <el-input-number v-model="organizeIdentifyForm.episode" :min="0" :max="9999" style="width: 100%" />
        </el-form-item>
        <el-form-item label="TMDB ID">
          <el-input-number v-model="organizeIdentifyForm.tmdb_id" :min="0" :max="999999999" style="width: 100%" />
        </el-form-item>
      </el-form>
      <template #footer>
        <span class="dialog-footer">
          <el-button @click="handleSearchTmdbForOrganizeEdit">从 TMDB 选择</el-button>
          <el-button v-if="organizeIdentifyForm.override_key" @click="handleClearOrganizeIdentifyOverride">清除修改</el-button>
          <el-button @click="organizeIdentifyDialogVisible = false">取消</el-button>
          <el-button type="primary" @click="handleApplyOrganizeIdentifyOverride">应用到预览</el-button>
        </span>
      </template>
    </el-dialog>

    <el-dialog
      v-model="organizeResultDialogVisible"
      title="整理结果"
      width="900px"
    >
      <el-alert
        v-if="organizeExecuteSummary"
        :title="`共 ${organizeExecuteSummary.total || 0} 项，成功 ${organizeExecuteSummary.success || 0} 项，跳过 ${organizeExecuteSummary.skipped || 0} 项，失败 ${organizeExecuteSummary.failed || 0} 项`"
        type="success"
        show-icon
        :closable="false"
        style="margin-bottom: 16px"
      />
      <el-table :data="organizeResultList" border stripe max-height="420">
        <el-table-column prop="file_name" label="文件名" min-width="220" />
        <el-table-column prop="message" label="结果" min-width="220" />
        <el-table-column prop="new_path" label="目标路径" min-width="280" show-overflow-tooltip />
        <el-table-column label="状态" width="120" align="center">
          <template #default="scope">
            <el-tag v-if="scope.row.skipped" type="warning">已跳过</el-tag>
            <el-tag v-else-if="scope.row.success" type="success">成功</el-tag>
            <el-tag v-else type="danger">失败</el-tag>
          </template>
        </el-table-column>
      </el-table>
      <template #footer>
        <span class="dialog-footer">
          <el-button type="primary" @click="organizeResultDialogVisible = false">关闭</el-button>
        </span>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, onMounted, computed } from 'vue'
import {
  FolderOpened,
  Plus,
  Edit,
  Delete,
  Folder,
  Document,
  Search,
  Refresh,
  MagicStick,
  Right,
  VideoCamera,
  Headset,
  Files,
  HomeFilled,
  Back
} from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  getMediaSources,
  createMediaSource,
  updateMediaSource,
  deleteMediaSource,
  getMediaFiles,
  searchTmdb,
  identifyFile,
  batchPreviewRename,
  batchExecuteRename,
  batchIdentifyFiles,
  listOrganizeCandidates,
  previewOrganize,
  executeOrganize,
  renameFile,
  deleteFile
} from '../utils/api/media'
import { getCloud115List } from '../utils/api/cloud115'

// ==================== 媒体源管理 ====================

/**
 * 媒体源列表
 */
const mediaSources = ref([])

/**
 * 115账号列表
 */
const cloud115List = ref([])

/**
 * 媒体源对话框
 */
const sourceDialogVisible = ref(false)
const sourceDialogTitle = ref('新增媒体源')
const sourceFormRef = ref(null)
const sourceLoading = ref(false)

/**
 * 媒体源表单数据
 */
const sourceForm = ref({
  id: null,
  name: '',
  source_type: 'local',
  path: '',
  cloud115_id: null
})

/**
 * 媒体源表单验证规则
 */
const sourceRules = {
  name: [{ required: true, message: '请输入媒体源名称', trigger: 'blur' }],
  source_type: [{ required: true, message: '请选择类型', trigger: 'change' }],
  path: [{ required: true, message: '请输入路径', trigger: 'blur' }]
}

/**
 * 获取媒体源列表
 */
const fetchMediaSources = async () => {
  try {
    const response = await getMediaSources()
    // 处理后端返回的嵌套数据结构: {state: true, data: {data: [...], total: N}}
    const apiData = response.data.data
    const sources = Array.isArray(apiData) ? apiData : (apiData?.data || [])
    mediaSources.value = [...sources].sort((a, b) => (b.id || 0) - (a.id || 0))
  } catch (error) {
    console.error('[MediaManager] 获取媒体源列表失败:', error)
    const errorMsg = error.response?.data?.error || error.message || '获取媒体源列表失败'
    ElMessage.error(errorMsg)
  }
}

/**
 * 获取115账号列表
 */
const fetchCloud115List = async () => {
  try {
    const response = await getCloud115List()
    const apiData = response.data.data
    cloud115List.value = Array.isArray(apiData) ? apiData : (apiData?.data || [])
  } catch (error) {
    console.error('[MediaManager] 获取115账号列表失败:', error)
    const errorMsg = error.response?.data?.error || error.message || '获取115账号列表失败'
    ElMessage.error(errorMsg)
  }
}

/**
 * 新增媒体源
 */
const handleAddSource = () => {
  sourceDialogTitle.value = '新增媒体源'
  resetSourceForm()
  sourceDialogVisible.value = true
}

/**
 * 编辑媒体源
 * @param {Object} row - 媒体源数据
 */
const handleEditSource = (row) => {
  sourceDialogTitle.value = '编辑媒体源'
  sourceForm.value = {
    id: row.id,
    name: row.name || '',
    source_type: row.source_type || 'local',
    path: row.path || '',
    cloud115_id: row.cloud115_id || null
  }
  sourceDialogVisible.value = true
}

/**
 * 删除媒体源
 * @param {Object} row - 媒体源数据
 */
const handleDeleteSource = (row) => {
  ElMessageBox.confirm(
    '确定要删除这个媒体源吗？',
    '删除确认',
    {
      confirmButtonText: '确定',
      cancelButtonText: '取消',
      type: 'warning'
    }
  ).then(async () => {
    try {
      await deleteMediaSource(row.id)
      ElMessage.success('删除成功')
      fetchMediaSources()
    } catch (error) {
      console.error('[MediaManager] 删除媒体源失败:', error)
      const errorMsg = error.response?.data?.error || error.message || '删除失败'
      ElMessage.error(errorMsg)
    }
  }).catch(() => {})
}

/**
 * 提交媒体源表单
 */
const handleSubmitSource = async () => {
  if (!sourceFormRef.value) return

  await sourceFormRef.value.validate(async (valid) => {
    if (valid) {
      sourceLoading.value = true
      try {
        if (sourceForm.value.id) {
          await updateMediaSource(sourceForm.value.id, sourceForm.value)
          ElMessage.success('编辑成功')
        } else {
          await createMediaSource(sourceForm.value)
          ElMessage.success('新增成功')
        }
        sourceDialogVisible.value = false
        fetchMediaSources()
        resetSourceForm()
      } catch (error) {
        console.error('[MediaManager] 提交媒体源失败:', error)
        const errorMsg = error.response?.data?.error || error.message || (sourceForm.value.id ? '编辑失败' : '新增失败')
        ElMessage.error(errorMsg)
      } finally {
        sourceLoading.value = false
      }
    }
  })
}

/**
 * 重置媒体源表单
 */
const resetSourceForm = () => {
  sourceForm.value = {
    id: null,
    name: '',
    source_type: 'local',
    path: '',
    cloud115_id: null
  }
  if (sourceFormRef.value) {
    sourceFormRef.value.resetFields()
  }
}

/**
 * 媒体源类型变化处理
 */
const handleSourceTypeChange = () => {
  sourceForm.value.cloud115_id = null
}

// ==================== 文件浏览 ====================

/**
 * 文件浏览弹窗是否可见
 */
const fileDialogVisible = ref(false)

/**
 * 当前选中的媒体源
 */
const currentSource = ref(null)

/**
 * 当前路径
 */
const currentPath = ref('/')

/**
 * 115 目录导航历史栈
 */
const directoryStack = ref([])

/**
 * 文件列表
 */
const fileList = ref([])

/**
 * 选中的文件
 */
const selectedFiles = ref([])

/**
 * 分页
 */
const currentPage = ref(1)
const pageSize = ref(50)
const total = ref(0)

/**
 * 搜索和过滤
 */
const searchKeyword = ref('')
const filterType = ref('')

/**
 * 面包屑导航项
 * 对于115云盘，由于使用目录ID导航，暂时只支持返回根目录
 */
/**
 * 是否为115云盘媒体源
 */
const isCloud115Source = computed(() => {
  return currentSource.value?.source_type === 'cloud115'
})

/**
 * 面包屑导航项
 * 对于115云盘，使用目录栈生成面包屑
 */
const breadcrumbItems = computed(() => {
  // 115云盘：从目录栈生成面包屑
  if (currentSource.value?.source_type === 'cloud115') {
    return directoryStack.value.map(item => ({
      name: item.name,
      path: item.cid
    }))
  }
  // 本地存储：支持路径面包屑导航
  if (currentPath.value === '/') return []
  const parts = currentPath.value.split('/').filter(p => p)
  return parts.map((part, index) => ({
    name: part,
    path: '/' + parts.slice(0, index + 1).join('/')
  }))
})

/**
 * 本地过滤文件列表（按已识别/未识别筛选）
 */
const filteredFileList = computed(() => {
  if (filterType.value === 'identified') {
    return fileList.value.filter(f => f.tmdb_title)
  }
  if (filterType.value === 'unidentified') {
    return fileList.value.filter(f => !f.tmdb_title && !(f.is_dir || f.is_directory))
  }
  return fileList.value
})

/**
 * 媒体源根路径（用于判断是否在根目录）
 */
const sourceRootPath = computed(() => {
  if (!currentSource.value) return '/'
  return currentSource.value.path || '/'
})

const currentCloud115DisplayPath = computed(() => {
  if (!isCloud115Source.value) return currentPath.value
  if (directoryStack.value.length === 0) return '/'
  return '/' + directoryStack.value.map(item => item.name).join('/')
})

/**
 * 当前目录名称（已废弃，面包屑统一使用 breadcrumbItems）
 */

/**
 * 是否可以返回上一级
 */
const canGoBack = computed(() => {
  if (!currentSource.value) return false
  if (currentSource.value.source_type === 'cloud115') {
    // 115云盘：目录栈有内容时可以返回
    return directoryStack.value.length > 0
  }
  // 本地存储：当前路径不是根目录时可以返回
  return currentPath.value !== '/'
})

/**
 * 导航到根目录
 */
const navigateToRoot = () => {
  if (currentSource.value?.source_type === 'cloud115') {
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
  if (!currentSource.value) return
  
  if (currentSource.value.source_type === 'cloud115') {
    // 115云盘：从目录栈弹出上一级
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
    // 本地存储：返回上一级目录
    if (currentPath.value === '/') return
    const parts = currentPath.value.split('/').filter(p => p)
    parts.pop()
    currentPath.value = parts.length > 0 ? '/' + parts.join('/') : '/'
  }
  currentPage.value = 1
  fetchFileList()
}

/**
 * 浏览文件
 * @param {Object} source - 媒体源数据
 */
const handleBrowseFiles = (source) => {
  currentSource.value = source
  currentPath.value = '/'
  currentPage.value = 1
  searchKeyword.value = ''
  filterType.value = ''
  directoryStack.value = []
  fileDialogVisible.value = true
  fetchFileList()
}

/**
 * 获取文件列表
 */
const fetchFileList = async () => {
  if (!currentSource.value) return

  try {
    // 只将后端可识别的筛选类型发送给服务器（identified/unidentified 由前端本地过滤）
    const serverFilterType = ['video', 'folder'].includes(filterType.value) ? filterType.value : ''
    const params = {
      source_id: currentSource.value.id,
      path: currentPath.value,
      page: currentPage.value,
      page_size: pageSize.value,
      keyword: searchKeyword.value,
      type: serverFilterType
    }

    const response = await getMediaFiles(params)
    // 处理后端返回的嵌套数据结构
    const apiData = response.data.data
    fileList.value = apiData?.files || []
    total.value = apiData?.total || 0
  } catch (error) {
    console.error('[MediaManager] 获取文件列表失败:', error)
    const errorMsg = error.response?.data?.error || error.message || '获取文件列表失败'
    ElMessage.error(errorMsg)
  }
}

/**
 * 导航到指定路径
 * @param {string} path - 目标路径
 */
const navigateToPath = (path) => {
  // 115云盘：截断目录栈到点击的面包屑位置
  if (currentSource.value?.source_type === 'cloud115') {
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
 * 双击行进入文件夹
 * 仅目录可以双击进入，文件不允许双击进入
 * @param {Object} row - 文件数据
 */
const handleRowDblClick = (row) => {
  const isDirectory = row.is_dir || row.is_directory
  // 只有目录可以双击进入
  if (!isDirectory) {
    return
  }
  
  // 根据媒体源类型使用不同的导航方式
  if (currentSource.value.source_type === 'cloud115') {
    // 115云盘：记录当前目录到导航栈，然后使用目录ID导航
    directoryStack.value.push({ name: row.name, cid: row.cid || row.id })
    currentPath.value = row.cid || row.id
  } else {
    // 本地存储：使用路径拼接
    currentPath.value = currentPath.value === '/'
      ? `/${row.name}`
      : `${currentPath.value}/${row.name}`
  }
  currentPage.value = 1
  fetchFileList()
}

/**
 * 搜索文件
 */
const handleSearch = () => {
  currentPage.value = 1
  fetchFileList()
}

/**
 * 刷新文件列表
 */
const handleRefresh = () => {
  fetchFileList()
}

/**
 * 选择变化
 * @param {Array} selection - 选中的文件
 */
const handleSelectionChange = (selection) => {
  selectedFiles.value = selection
}

/**
 * 分页大小变化
 * @param {number} size - 每页数量
 */
const handleSizeChange = (size) => {
  pageSize.value = size
  fetchFileList()
}

/**
 * 当前页变化
 * @param {number} page - 当前页码
 */
const handleCurrentChange = (page) => {
  currentPage.value = page
  fetchFileList()
}

/**
 * 格式化文件大小
 * @param {number} bytes - 字节数
 * @returns {string} 格式化后的文件大小
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
 * @param {string} filename - 文件名
 * @returns {string} 文件类型: video | audio | document | other
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

/**
 * 获取表格行的样式类名
 * 为目录行添加特殊样式,提示用户可以双击进入
 * @param {Object} params - 行数据参数
 * @returns {string} 样式类名
 */
const getRowClassName = ({ row }) => {
  const isDirectory = row.is_dir || row.is_directory
  return isDirectory ? 'directory-row' : 'file-row'
}

// ==================== TMDB识别 ====================

/**
 * TMDB对话框
 */
const tmdbDialogVisible = ref(false)
const tmdbSearchKeyword = ref('')
const tmdbType = ref('movie')
const tmdbResults = ref([])
const tmdbLoading = ref(false)
const currentIdentifyFile = ref(null)
const tmdbSelectMode = ref('cache')

const openTmdbIdentifyDialog = (row) => {
  currentIdentifyFile.value = row
  tmdbSelectMode.value = 'cache'
  tmdbSearchKeyword.value = (row.name || row.file_name || '').replace(/\.[^/.]+$/, '')
  tmdbResults.value = []
  tmdbDialogVisible.value = true
}

/**
 * 识别单个文件
 * @param {Object} row - 文件数据
 */
const handleIdentify = (row) => {
  openTmdbIdentifyDialog(row)
}

/**
 * 批量识别
 */
const handleBatchIdentify = async () => {
  if (selectedFiles.value.length === 0) {
    ElMessage.warning('请先选择要识别的文件')
    return
  }

  ElMessageBox.confirm(
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
      // 展示识别结果摘要
      const results = response.data.data?.data || response.data.data || []
      const successCount = Array.isArray(results) ? results.filter(r => r.success).length : 0
      const failedCount = Array.isArray(results) ? results.filter(r => !r.success).length : 0
      ElMessage.success(`批量识别完成：成功 ${successCount} 项，失败 ${failedCount} 项`)
      fetchFileList()
    } catch (error) {
      console.error('[MediaManager] 批量识别失败:', error)
      const errorMsg = error.response?.data?.error || error.message || '批量识别失败'
      ElMessage.error(errorMsg)
    }
  }).catch(() => {})
}

/**
 * 搜索TMDB
 */
const handleTmdbSearch = async () => {
  if (!tmdbSearchKeyword.value.trim()) {
    ElMessage.warning('请输入搜索关键词')
    return
  }

  tmdbLoading.value = true
  try {
    const response = await searchTmdb({
      keyword: tmdbSearchKeyword.value,
      type: tmdbType.value
    })
    console.log('[MediaManager] TMDB搜索响应:', response.data)
    tmdbResults.value = response.data.data?.data || []
  } catch (error) {
    console.error('[MediaManager] TMDB搜索失败:', error)
    const errorMsg = error.response?.data?.error || error.message || 'TMDB搜索失败'
    ElMessage.error(errorMsg)
  } finally {
    tmdbLoading.value = false
  }
}

/**
 * 选择TMDB结果
 * @param {Object} item - TMDB数据
 */
const handleSelectTmdb = async (item) => {
  if (!currentIdentifyFile.value) return

  if (tmdbSelectMode.value === 'organize') {
    organizeIdentifyForm.value = {
      ...organizeIdentifyForm.value,
      media_type: tmdbType.value,
      tmdb_id: item.tmdb_id || item.id || 0,
      title: item.title || item.name || '',
      year: item.year || 0
    }
    tmdbDialogVisible.value = false
    if (!organizeIdentifyDialogVisible.value) {
      organizeIdentifyDialogVisible.value = true
    }
    return
  }

  try {
    await identifyFile({
      file_id: currentIdentifyFile.value.identify_cache_key || currentIdentifyFile.value.id,
      tmdb_id: item.tmdb_id || item.id,
      tmdb_type: tmdbType.value,
      title: item.title || item.name,
      year: item.year,
      poster_url: item.poster_path
    })
    ElMessage.success('识别成功')
    tmdbDialogVisible.value = false
    fetchFileList()
    if (organizeDialogVisible.value) {
      await handlePreviewOrganize()
    }
  } catch (error) {
    console.error('[MediaManager] 识别失败:', error)
    const errorMsg = error.response?.data?.error || error.message || '识别失败'
    ElMessage.error(errorMsg)
  }
}

const handleOrganizePreviewIdentify = (row) => {
  openOrganizeIdentifyEditor({
    file_id: row.file_id,
    cloud_id: row.cloud_id || row.file_id,
    file_name: row.file_name,
    media_type: row.media_type,
    tmdb_id: row.tmdb_id,
    title: row.title,
    year: row.year,
    season: row.season,
    episode: row.episode
  })
}

// ==================== 重命名 ====================

/**
 * 重命名预览对话框
 */
const renameDialogVisible = ref(false)
const renamePreviewList = ref([])
const renameLoading = ref(false)

/**
 * 单个文件重命名对话框
 */
const singleRenameDialogVisible = ref(false)
const singleRenameForm = ref({
  source_id: null,
  file_id: null,
  original_name: '',
  new_name: ''
})
const singleRenameLoading = ref(false)

/**
 * 批量整理对话框
 */
const organizeDialogVisible = ref(false)
const organizeResultDialogVisible = ref(false)
const organizeLoading = ref(false)
const organizeExecuting = ref(false)
const organizeCandidateList = ref([])
const organizePreviewList = ref([])
const organizeHasPreview = ref(false)
const organizeSummary = ref(null)
const organizeResultList = ref([])
const organizeExecuteSummary = ref(null)
const organizeForm = ref({
  target_path: '',
  media_type: 'all',
  conflict_policy: 'skip',
  operation_mode: 'move'
})
const organizeManualOverrides = ref({})
const organizeIdentifyDialogVisible = ref(false)
const organizeIdentifyForm = ref({
  file_id: '',
  cloud_id: '',
  file_name: '',
  media_type: 'movie',
  tmdb_id: 0,
  title: '',
  year: 0,
  season: 0,
  episode: 0,
  override_key: ''
})
const organizeManualCount = computed(() => Object.keys(organizeManualOverrides.value).length)

/**
 * 重命名单个文件
 * @param {Object} row - 文件数据
 */
const handleRename = (row) => {
  singleRenameForm.value = {
    source_id: currentSource.value.id,
    file_id: row.id,
    original_name: row.name,
    new_name: row.name
  }
  singleRenameDialogVisible.value = true
}

/**
 * 执行单个文件重命名
 */
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
    fetchFileList()
  } catch (error) {
    console.error('[MediaManager] 重命名失败:', error)
    const errorMsg = error.response?.data?.error || error.message || '重命名失败'
    ElMessage.error(errorMsg)
  } finally {
    singleRenameLoading.value = false
  }
}

/**
 * 批量重命名
 */
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

/**
 * 执行批量重命名
 */
const handleExecuteRename = async () => {
  renameLoading.value = true
  try {
    const items = renamePreviewList.value.map(item => ({
      source_id: currentSource.value.id,
      file_id: item.file_id,
      new_name: item.new_name
    }))

    const response = await batchExecuteRename({ items })
    // 展示重命名结果摘要
    const data = response.data.data || response.data || {}
    const successCount = data.success || 0
    const failedCount = data.failed || 0
    ElMessage.success(`批量重命名完成：成功 ${successCount} 项，失败 ${failedCount} 项`)
    renameDialogVisible.value = false
    fetchFileList()
  } catch (error) {
    console.error('[MediaManager] 批量重命名失败:', error)
    const errorMsg = error.response?.data?.error || error.message || '批量重命名失败'
    ElMessage.error(errorMsg)
  } finally {
    renameLoading.value = false
  }
}

/**
 * 打开整理对话框
 */
const handleOpenOrganize = async () => {
  if (selectedFiles.value.length === 0) {
    ElMessage.warning('请先选择要整理的文件')
    return
  }

  organizeForm.value = {
    target_path: isCloud115Source.value ? currentCloud115DisplayPath.value : (currentSource.value?.path || ''),
    media_type: 'all',
    conflict_policy: 'skip',
    operation_mode: 'move'
  }
  organizeCandidateList.value = []
  organizePreviewList.value = []
  organizeHasPreview.value = false
  organizeSummary.value = null
  organizeResultList.value = []
  organizeExecuteSummary.value = null
  organizeManualOverrides.value = {}
  organizeDialogVisible.value = true
  await handleLoadOrganizeCandidates()
}

/**
 * 单个文件整理
 */
const handleSingleOrganize = (row) => {
  selectedFiles.value = [row]
  handleOpenOrganize()
}

const getOrganizeOverrideKey = (row) => {
  if (!row) return ''
  return row.cloud_id || row.file_id || row.cloudID || row.fileID || row.id || ''
}

const buildOrganizeManualItems = () => Object.values(organizeManualOverrides.value)

const buildOrganizePayload = () => ({
  source_id: currentSource.value.id,
  source_path: currentPath.value === '/' ? '' : currentPath.value,
  target_path: organizeForm.value.target_path,
  media_type: organizeForm.value.media_type,
  conflict_policy: organizeForm.value.conflict_policy,
  operation_mode: organizeForm.value.operation_mode,
  use_category: true,
  file_ids: selectedFiles.value.map(file => file.id),
  manual_items: buildOrganizeManualItems()
})

const hasOrganizeManualOverride = (row) => {
  const key = getOrganizeOverrideKey(row)
  return Boolean(key && organizeManualOverrides.value[key])
}

const formatOrganizeIdentifyLabel = (row) => {
  if (!row) return '-'
  if (row.identify_error && !row.title) return row.identify_error

  const parts = []
  if (row.title) parts.push(row.title)
  if (row.year) parts.push(String(row.year))
  if (row.media_type === 'tv') {
    if (Number.isInteger(row.season) && row.season > 0) parts.push(`S${String(row.season).padStart(2, '0')}`)
    if (Number.isInteger(row.episode) && row.episode > 0) parts.push(`E${String(row.episode).padStart(2, '0')}`)
  }
  return parts.join(' / ') || '未识别'
}

const openOrganizeIdentifyEditor = (row) => {
  const key = getOrganizeOverrideKey(row)
  const existing = key ? organizeManualOverrides.value[key] : null
  organizeIdentifyForm.value = {
    file_id: row.file_id || row.id || '',
    cloud_id: row.cloud_id || '',
    file_name: row.file_name || row.name || '',
    media_type: existing?.media_type || row.media_type || 'movie',
    tmdb_id: existing?.tmdb_id || row.tmdb_id || 0,
    title: existing?.title || row.title || '',
    year: existing?.year || row.year || 0,
    season: existing?.season || row.season || 0,
    episode: existing?.episode || row.episode || 0,
    override_key: key
  }
  organizeIdentifyDialogVisible.value = true
}

const handleEditOrganizeIdentify = (row) => {
  openOrganizeIdentifyEditor(row)
}

const handleSearchTmdbForOrganizeEdit = () => {
  currentIdentifyFile.value = {
    file_id: organizeIdentifyForm.value.file_id,
    cloud_id: organizeIdentifyForm.value.cloud_id,
    file_name: organizeIdentifyForm.value.file_name
  }
  tmdbSelectMode.value = 'organize'
  tmdbSearchKeyword.value = organizeIdentifyForm.value.title || organizeIdentifyForm.value.file_name.replace(/\.[^/.]+$/, '')
  tmdbType.value = organizeIdentifyForm.value.media_type || 'movie'
  tmdbResults.value = []
  tmdbDialogVisible.value = true
}

const handleApplyOrganizeIdentifyOverride = async () => {
  const form = organizeIdentifyForm.value
  if (!form.title.trim()) {
    ElMessage.warning('请输入识别标题')
    return
  }

  const key = form.override_key || form.cloud_id || form.file_id
  organizeManualOverrides.value = {
    ...organizeManualOverrides.value,
    [key]: {
      file_id: form.file_id,
      cloud_id: form.cloud_id,
      media_type: form.media_type,
      tmdb_id: Number(form.tmdb_id || 0),
      title: form.title.trim(),
      year: Number(form.year || 0),
      season: Number(form.season || 0),
      episode: Number(form.episode || 0)
    }
  }
  organizeIdentifyDialogVisible.value = false
  await handlePreviewOrganize()
}

const handleClearOrganizeIdentifyOverride = async () => {
  const key = organizeIdentifyForm.value.override_key
  if (!key) {
    organizeIdentifyDialogVisible.value = false
    return
  }

  const next = { ...organizeManualOverrides.value }
  delete next[key]
  organizeManualOverrides.value = next
  organizeIdentifyDialogVisible.value = false
  await handlePreviewOrganize()
}

/**
 * 加载整理候选文件
 */
const handleLoadOrganizeCandidates = async () => {
  organizeLoading.value = true
  try {
    const response = await listOrganizeCandidates({
      source_id: currentSource.value.id,
      source_path: currentPath.value === '/' ? '' : currentPath.value,
      media_type: organizeForm.value.media_type,
      file_ids: selectedFiles.value.map(file => file.id)
    })
    const payload = response.data.data || {}
    organizeCandidateList.value = payload.data || []
    organizeHasPreview.value = false
    if (organizeCandidateList.value.length === 0) {
      ElMessage.warning('当前选择范围内没有可整理的视频文件')
    }
  } catch (error) {
    console.error('[MediaManager] 加载整理候选文件失败:', error)
    const errorMsg = error.response?.data?.error || error.message || '加载整理候选文件失败'
    ElMessage.error(errorMsg)
  } finally {
    organizeLoading.value = false
  }
}

/**
 * 预览整理结果
 */
const handlePreviewOrganize = async () => {
  if (!organizeForm.value.target_path.trim()) {
    ElMessage.warning('请输入目标目录')
    return
  }

  organizeLoading.value = true
  try {
    const response = await previewOrganize(buildOrganizePayload())
    const payload = response.data.data || {}
    organizePreviewList.value = payload.data || []
    organizeSummary.value = payload.summary || null
    organizeHasPreview.value = true
  } catch (error) {
    console.error('[MediaManager] 预览整理失败:', error)
    const errorMsg = error.response?.data?.error || error.message || '预览整理失败'
    ElMessage.error(errorMsg)
  } finally {
    organizeLoading.value = false
  }
}

/**
 * 执行整理
 */
const handleExecuteOrganize = async () => {
  if (!organizeForm.value.target_path.trim()) {
    ElMessage.warning('请输入目标目录')
    return
  }
  if (!organizeHasPreview.value) {
    ElMessage.warning('请先刷新预览，确认识别结果后再执行整理')
    return
  }

  organizeExecuting.value = true
  try {
    const response = await executeOrganize(buildOrganizePayload())
    const payload = response.data.data || {}
    const summary = payload.summary || {}
    ElMessage.success(`整理完成：成功 ${summary.success || 0}，跳过 ${summary.skipped || 0}，失败 ${summary.failed || 0}`)
    organizeDialogVisible.value = false
    organizeResultList.value = payload.data || []
    organizeExecuteSummary.value = summary
    organizeResultDialogVisible.value = true
    fetchFileList()
  } catch (error) {
    console.error('[MediaManager] 执行整理失败:', error)
    const errorMsg = error.response?.data?.error || error.message || '执行整理失败'
    ElMessage.error(errorMsg)
  } finally {
    organizeExecuting.value = false
  }
}

// ==================== 文件操作 ====================

/**
 * 删除文件
 * @param {Object} row - 文件数据
 */
const handleDeleteFile = (row) => {
  ElMessageBox.confirm(
    `确定要删除 "${row.name}" 吗？`,
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
        file_id: row.id,
        file_path: row.path
      })
      ElMessage.success('删除成功')
      fetchFileList()
    } catch (error) {
      console.error('[MediaManager] 删除文件失败:', error)
      const errorMsg = error.response?.data?.error || error.message || '删除失败'
      ElMessage.error(errorMsg)
    }
  }).catch(() => {})
}

// ==================== 生命周期 ====================

/**
 * 组件挂载时初始化
 */
onMounted(() => {
  fetchMediaSources()
  fetchCloud115List()
})
</script>

<style scoped>
.media-manager-container {
  padding: 20px;
  min-height: calc(100vh - 100px);
}

.source-card {
  margin-bottom: 20px;
  border-radius: 12px;
  overflow: hidden;
}

.file-card {
  border-radius: 12px;
  overflow: hidden;
}

.main-card :deep(.el-card__header),
.source-card :deep(.el-card__header),
.file-card :deep(.el-card__header) {
  background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
  padding: 16px 20px;
}

.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.header-title {
  display: flex;
  align-items: center;
  gap: 10px;
  color: white;
  font-size: 18px;
  font-weight: 600;
}

.header-icon {
  font-size: 22px;
}

.header-actions {
  display: flex;
  gap: 10px;
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

.text-muted {
  color: #909399;
  font-size: 13px;
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
  align-items: center;
  margin-bottom: 15px;
  flex-wrap: wrap;
  gap: 10px;
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

.pagination-container {
  display: flex;
  justify-content: flex-end;
  margin-top: 20px;
  padding: 10px 0;
}

.dialog-footer {
  display: flex;
  justify-content: flex-end;
  gap: 10px;
}

/* TMDB样式 */
.tmdb-container {
  min-height: 400px;
}

.tmdb-search {
  display: flex;
  align-items: center;
  margin-bottom: 20px;
}

.tmdb-results {
  max-height: 500px;
  overflow-y: auto;
}

.result-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(200px, 1fr));
  gap: 20px;
}

.result-item {
  cursor: pointer;
  border: 1px solid #e4e7ed;
  border-radius: 8px;
  overflow: hidden;
  transition: all 0.3s;
  background: #fff;
}

.result-item:hover {
  border-color: #409eff;
  box-shadow: 0 4px 16px rgba(64, 158, 255, 0.2);
  transform: translateY(-2px);
}

.poster-img {
  width: 100%;
  height: 280px;
  object-fit: cover;
}

.no-poster {
  width: 100%;
  height: 280px;
  display: flex;
  align-items: center;
  justify-content: center;
  background-color: #f5f7fa;
  color: #909399;
  font-size: 14px;
}

.result-info {
  padding: 12px;
}

.result-title {
  font-size: 14px;
  font-weight: 600;
  color: #303133;
  margin-bottom: 6px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.result-year {
  font-size: 12px;
  color: #909399;
  margin-bottom: 6px;
}

.result-overview {
  font-size: 12px;
  color: #606266;
  line-height: 1.5;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
  margin-bottom: 8px;
}

.result-rating {
  margin-top: 4px;
}

/* 重命名样式 */
.rename-container {
  max-height: 500px;
  overflow-y: auto;
}

.organize-container {
  min-height: 320px;
}

.organize-form {
  margin-bottom: 16px;
}

.organize-identify-cell {
  display: flex;
  align-items: center;
  gap: 8px;
}
</style>
