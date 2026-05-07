import { api } from './request'

/**
 * 媒体源管理 API
 */

// 获取媒体源列表
export const getMediaSources = (params) => api.get('/media/sources', { params })

// 根据ID获取媒体源详情
export const getMediaSourceById = (id) => api.get(`/media/sources/${id}`)

// 创建媒体源
export const createMediaSource = (data) => api.post('/media/sources', data)

// 更新媒体源
export const updateMediaSource = (id, data) => api.put(`/media/sources/${id}`, data)

// 删除媒体源
export const deleteMediaSource = (id) => api.delete(`/media/sources/${id}`)

/**
 * 文件浏览 API
 */

// 获取文件列表
export const getMediaFiles = (params) => api.get('/media/files', { params })

// 搜索文件
export const searchMediaFiles = (params) => api.get('/media/files/search', { params })

/**
 * 媒体库同步 API
 */

export const runFullMediaSync = (sourceId) => api.post(`/media/sources/${sourceId}/sync/full`)

export const runIncrementalMediaSync = (sourceId) => api.post(`/media/sources/${sourceId}/sync/incremental`)

export const getMediaSyncIndex = (sourceId, params) => api.get(`/media/sources/${sourceId}/sync/index`, { params })

export const runMediaLibraryPipeline = (sourceId) => api.post(`/media/sources/${sourceId}/pipeline`)

export const getMediaLibraryItems = (params) => api.get('/media/library/items', { params })

export const getMediaLibraryItem = (id) => api.get(`/media/library/items/${id}`)

export const runMediaLibraryItemPipeline = (id) => api.post(`/media/library/items/${id}/pipeline`)

export const generateMediaLibraryItemStrm = (id) => api.post(`/media/library/items/${id}/strm`)

export const refreshMediaLibraryItemServer = (id) => api.post(`/media/library/items/${id}/refresh-server`)

export const getPendingMediaItems = (params) => api.get('/media/pending', { params })

export const createPendingMediaItem = (data) => api.post('/media/pending', data)

export const identifyPendingMediaItem = (id, data) => api.post(`/media/pending/${id}/identify`, data)

export const runPendingMediaItem = (id) => api.post(`/media/pending/${id}/run`)

export const ignorePendingMediaItem = (id) => api.post(`/media/pending/${id}/ignore`)

/**
 * 文件操作 API（Phase 1 第二部分）
 */

// 移动文件
export const moveFiles = (data) => api.post('/media/files/move', data)

// 复制文件
export const copyFiles = (data) => api.post('/media/files/copy', data)

// 删除文件
export const deleteFile = (data) => api.post('/media/files/delete', data)

// 重命名文件
export const renameFile = (data) => api.post('/media/files/rename', data)

// 批量操作
export const batchOperation = (data) => api.post('/media/files/batch', data)

/**
 * TMDB 识别 API（Phase 3）
 */

// 搜索 TMDB
export const searchTmdb = (params) => api.get('/media/tmdb/search', { params })

// 识别文件
export const identifyFile = (data) => api.post('/media/tmdb/identify', data)

// 自动识别文件（返回 Top 3 候选，不写入缓存）
export const autoIdentifyFile = (data) => api.post('/media/tmdb/auto-identify', data)

// 批量识别文件
export const batchIdentifyTmdb = (data) => api.post('/media/tmdb/batch-identify', data)

// 获取电影详情
export const getMovieDetail = (id) => api.get(`/media/tmdb/movie/${id}`)

// 获取剧集详情
export const getTvDetail = (id) => api.get(`/media/tmdb/tv/${id}`)

// 获取 TMDB 配置
export const getTmdbConfig = (options = {}) => api.get('/media/tmdb/config', {
  skipGlobalErrorMessage: options.skipGlobalErrorMessage || false
})

// 更新 TMDB API Key
export const updateTmdbApiKey = (data, options = {}) => api.post('/media/tmdb/config', data, {
  skipGlobalErrorMessage: options.skipGlobalErrorMessage || false
})

/**
 * 更名预览 API（Phase 3）
 */

// 更名预览
export const previewRename = (data) => api.post('/media/organize/rename-preview', data)

// 执行更名
export const executeRename = (data) => api.post('/media/organize/rename-execute', data)

// 批量更名预览
export const batchPreviewRename = (data) => api.post('/media/organize/batch-rename-preview', data)

// 批量执行更名
export const batchExecuteRename = (data) => api.post('/media/organize/batch-rename-execute', data)

// 获取更名预设列表
export const getRenamePresets = (params) => api.get('/media/organize/presets', { params })

/**
 * 自动整理 API（Phase 3）
 */

// 获取整理候选文件
export const listOrganizeCandidates = (data) => api.post('/media/organize/candidates', data)

// 启动异步整理候选扫描任务
export const startOrganizeCandidatesTaskAsync = (data) => api.post('/media/organize/candidates/async', data)

// 获取异步整理候选扫描任务状态
export const getOrganizeCandidatesTaskStatus = (taskId) => api.get('/media/organize/candidates/status', { params: { task_id: taskId } })

// 预览整理结果
export const previewOrganize = (data) => api.post('/media/organize/preview', data)

// 启动异步预览任务
export const startPreviewTaskAsync = (data) => api.post('/media/organize/preview/async', data)

// 获取预览任务状态
export const getPreviewTaskStatus = (taskId) => api.get('/media/organize/preview/status', { params: { task_id: taskId } })

// 检查可恢复任务
export const checkRestorableTask = (data) => api.post('/media/organize/preview/check', data)

// 执行整理
export const executeOrganize = (data) => api.post('/media/organize/execute', data)

// 异步执行整理
export const executeOrganizeAsync = (data) => api.post('/media/organize/execute/async', data)

// 批量识别文件（整理服务）
export const batchIdentifyFiles = (data) => api.post('/media/organize/batch-identify', data)
export const batchIdentifyDirectoryFiles = (data) => api.post('/media/organize/batch-identify-directory', data)

// 批量更名预览（整理服务）
export const batchRenamePreview = (data) => api.post('/media/organize/batch-rename-preview', data)

// 批量执行更名（整理服务）
export const batchRenameExecute = (data) => api.post('/media/organize/batch-rename-execute', data)

// 识别单个文件（整理服务）
export const identifySingleFile = (data) => api.post('/media/organize/identify', data)

// 预览更名（整理服务）
export const previewRenameOrganize = (data) => api.post('/media/organize/rename-preview', data)

// 执行更名（整理服务）
export const executeRenameOrganize = (data) => api.post('/media/organize/rename-execute', data)

// 获取更名预设列表（整理服务）
export const getOrganizePresets = (params) => api.get('/media/organize/presets', { params })

/**
 * 媒体分类 API（Phase 5）
 */

// 获取所有媒体分类
export const getMediaCategories = (params) => api.get('/media/categories', { params })

// 创建媒体分类
export const createMediaCategory = (data) => api.post('/media/categories', data)

// 更新媒体分类
export const updateMediaCategory = (id, data) => api.put(`/media/categories/${id}`, data)

// 删除媒体分类
export const deleteMediaCategory = (id) => api.delete(`/media/categories/${id}`)

/**
 * NFO 刮削 API
 */

// 刮削单个文件 NFO
export const scrapeFile = (data) => api.post('/media/scrape/file', data)

// 批量刮削文件 NFO
export const scrapeFiles = (data) => api.post('/media/scrape/files', data)
export const scrapeDirectoryFiles = (data) => api.post('/media/scrape/directory', data)
