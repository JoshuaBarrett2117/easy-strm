import { api } from './request'

/**
 * 115分享链接转存 API
 */

// 解析115分享链接
export const parseShareLink = (url, password) => {
  return api.post('/v1/resource/115-share/parse', { url, password: password || '' })
}

// 获取分享文件列表
export const getShareFiles = (params = {}) => {
  return api.get('/v1/resource/115-share/files', { params })
}

// 提交转存任务
export const submitTransfer = (data) => {
  return api.post('/v1/resource/115-share/transfer', data)
}

// 查询转存任务进度
export const getTransferProgress = (taskId) => {
  return api.get(`/v1/resource/115-share/transfer/${taskId}`)
}

// 取消转存任务
export const cancelTransfer = (taskId) => {
  return api.post(`/v1/resource/115-share/transfer/${taskId}/cancel`)
}

// 重试转存失败文件
export const retryTransfer = (taskId) => {
  return api.post(`/v1/resource/115-share/transfer/${taskId}/retry`)
}
