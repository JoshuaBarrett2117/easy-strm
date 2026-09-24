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
export const assistIdentifyFile = (data) => api.post('/media/tmdb/assist-identify', data, { timeout: 125000 })

// 使用当前整理规则在本地解析文件名
export const parseMediaFilename = (data) => api.post('/media/tmdb/parse-filename', data)

// 获取文件名识别规则
export const getFilenameRecognitionRules = () => api.get('/media/tmdb/filename-rules')

// 保存文件名识别规则
export const updateFilenameRecognitionRules = (data) => api.put('/media/tmdb/filename-rules', data)

// 恢复内置常用文件名识别模板
export const resetFilenameRecognitionRules = () => api.post('/media/tmdb/filename-rules/reset')

// 批量识别文件
export const batchIdentifyTmdb = (data) => api.post('/media/tmdb/batch-identify', data)
export const getShareRecords = (params) => api.get('/media/share-records', { params })
// 任务总时限配置，0代表无限制。
export const getShareTaskSettings = () => api.get('/media/share-task-settings')
export const saveShareTaskSettings = data => api.put('/media/share-task-settings', data)
export const createShareRecord = (data) => api.post('/media/share-records', data)
// 解析混合分享文案，仅生成预览，不写入分享记录。
export const parseShareImport = (text) => api.post('/media/share-records/parse', { text })
export const updateShareRecord = (id, data) => api.put(`/media/share-records/${id}`, data)
export const deleteShareRecord = (id) => api.delete(`/media/share-records/${id}`)
// 清空媒体候选及识别内容，保留分享链接配置。
export const clearShareMedia = (id) => api.delete(`/media/share-records/${id}/media`)
export const clearAllShareMedia = () => api.delete('/media/share-records/media')
export const clearSelectedShareMedia = (shareIds) => api.post('/media/share-records/batch-clear', { share_ids: shareIds })
export const identifyShareMedia = (id, data) => api.post(`/media/share-records/media/${id}/identify`, data)
export const manualIdentifyShareMedia = (id, data) => api.post(`/media/share-records/media/${id}/manual-identify`, data)
export const deleteShareMedia = (shareId, mediaId) => api.delete(`/media/share-records/${shareId}/media/${mediaId}`)
export const batchIdentifyShareRecords = (data) => api.post('/media/share-records/batch-identify', data)
export const batchSyncShareRecords = (shareIds) => api.post('/media/share-records/batch-sync', { share_ids: shareIds })
export const identifyShareRecord = (id, pendingOnly = false, failedOnly = false, forceRefresh = false) => api.post(`/media/share-records/${id}/identify`, null, {params: {pending_only: pendingOnly, failed_only: failedOnly, force_refresh: forceRefresh}})
export const syncShareRecord = (id) => api.post(`/media/share-records/${id}/sync`)
export const getShareFiles = (id, params) => api.get(`/media/share-records/${id}/files`, { params })
export const getShareIdentifyTask = (taskId) => api.get(`/tasks/${taskId}`)

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

// 按数据库分页读取已识别媒体。
export const getShareMedia = (id, params) => api.get(`/media/share-records/${id}/media`, { params })
