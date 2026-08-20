import { api } from './request'

// 获取文件管理器可用位置。
export const getFileManagerLocations = () => api.get('/file-manager/locations')

// 浏览本地媒体源或115账号目录。
export const browseFileManager = (params) => api.get('/file-manager/files', { params })

// 创建复制或剪切粘贴任务。
export const createFileManagerTransfer = (data) => api.post('/file-manager/transfers', data)

// 删除文件管理器中选中的文件或目录。
export const deleteFileManagerEntries = (data) => api.post('/file-manager/delete', data)
