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

// 批量识别文件
export const batchIdentifyTmdb = (data) => api.post('/media/tmdb/batch-identify', data)

// 获取电影详情
export const getMovieDetail = (id) => api.get(`/media/tmdb/movie/${id}`)

// 获取剧集详情
export const getTvDetail = (id) => api.get(`/media/tmdb/tv/${id}`)

// 获取 TMDB 配置
export const getTmdbConfig = () => api.get('/media/tmdb/config')

// 更新 TMDB API Key
export const updateTmdbApiKey = (data) => api.post('/media/tmdb/config', data)

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

// 预览整理结果
export const previewOrganize = (data) => api.post('/media/organize/preview', data)

// 执行整理
export const executeOrganize = (data) => api.post('/media/organize/execute', data)

// 批量识别文件（整理服务）
export const batchIdentifyFiles = (data) => api.post('/media/organize/batch-identify', data)

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

