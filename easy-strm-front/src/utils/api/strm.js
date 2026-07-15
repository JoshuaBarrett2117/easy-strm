import { api } from './request'

export const getStrmConfigList = (params) => {
  return api.get('/strm/config', { params })
}

export const getStrmConfigById = (id) => {
  return api.get(`/strm/config/${id}`)
}

export const createStrmConfig = (data) => {
  return api.post('/strm/config', data)
}

export const updateStrmConfig = (id, data) => {
  return api.put(`/strm/config/${id}`, data)
}

export const deleteStrmConfig = (id) => {
  return api.delete(`/strm/config/${id}`)
}

export const generateFullStrmConfig = (id) => {
  return api.post(`/strm/config/${id}/generate/full`)
}

export const getStrmTaskStatus = (taskId) => {
  return api.get(`/strm/task/${taskId}`)
}

/**
 * 从整理结果生成 STRM 文件
 * 整理完成后触发，根据媒体源和目标路径自动匹配 STRM 配置并异步生成
 * @param {Object} data - 请求参数
 * @param {number} data.source_id - 媒体源 ID
 * @param {string[]} data.target_paths - 整理后的目标路径列表
 * @param {number} [data.strm_config_id=0] - STRM 配置 ID，0 表示自动匹配
 * @returns {Promise} 生成结果
 */
export const generateStrmFromOrganize = (data) => {
  return api.post('/strm/config/generate/from-organize', data)
}

export const getLogFiles = () => {
  return api.get('/logs')
}

export const getLogFileContent = (filename, lines = 500) => {
  return api.get(`/logs/${filename}`, { params: { lines } })
}

export const getLogConfig = () => {
  return api.get('/logs/config')
}

export const updateLogConfig = (value) => {
  return api.put('/logs/config', { value })
}
