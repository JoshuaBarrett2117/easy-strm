import { api } from './request'

/**
 * 115云下载（离线下载） API
 */

// 提交批量云下载（支持 ed2k / 磁力 / http 等链接）
export const submitOfflineDownload = (data) => {
  return api.post('/v1/resource/115-offline/submit', data)
}

// 分页查询云下载记录
export const getOfflineDownloadTasks = (params = {}) => {
  return api.get('/v1/resource/115-offline/tasks', { params })
}

// 删除云下载记录（deleteFiles=true 时同时删除云端已下载文件）
export const deleteOfflineDownloadTask = (id, deleteFiles = false) => {
  return api.delete(`/v1/resource/115-offline/tasks/${id}`, {
    params: { delete_files: deleteFiles }
  })
}
