import { api } from './request'

export const getEmbyStatus = (options = {}) => {
  return api.get('/emby/status', {
    skipGlobalErrorMessage: options.skipGlobalErrorMessage || false
  })
}

export const getEmbyLibraries = (options = {}) => {
  return api.get('/emby/libraries', {
    skipGlobalErrorMessage: options.skipGlobalErrorMessage || false
  })
}

export const refreshEmbyLibrary = (libraryId = '', options = {}) => {
  return api.post('/emby/refresh', { library_id: libraryId }, {
    skipGlobalErrorMessage: options.skipGlobalErrorMessage || false
  })
}

export const getEmbyServers = () => api.get('/emby/servers')
export const createEmbyServer = data => api.post('/emby/servers', data)
export const updateEmbyServer = (serverId, data) => api.put(`/emby/servers/${serverId}`, data)
export const deleteEmbyServer = serverId => api.delete(`/emby/servers/${serverId}`)
export const testEmbyServer = serverId => api.post(`/emby/servers/${serverId}/test`)

export const getEmbyUsers = serverId => api.get(`/emby/servers/${serverId}/users`)
export const getEmbyUserLibraries = serverId => api.get(`/emby/servers/${serverId}/user-libraries`)
export const createEmbyUser = (serverId, data) => api.post(`/emby/servers/${serverId}/users`, data)
export const updateEmbyUser = (serverId, userId, data) => api.put(`/emby/servers/${serverId}/users/${userId}`, data)
export const setEmbyUserPassword = (serverId, userId, data) => api.put(`/emby/servers/${serverId}/users/${userId}/password`, data)
export const deleteEmbyUser = (serverId, userId) => api.delete(`/emby/servers/${serverId}/users/${userId}`)
export const getEmbyUserAvatar = (serverId, userId) => api.get(`/emby/servers/${serverId}/users/${userId}/avatar`, { responseType: 'blob', skipGlobalErrorMessage: true })
export const uploadEmbyUserAvatar = (serverId, userId, file) => {
  const form = new FormData()
  form.append('file', file)
  return api.post(`/emby/servers/${serverId}/users/${userId}/avatar`, form)
}

export const getManagedEmbyLibraries = serverId => api.get(`/emby/servers/${serverId}/libraries`)
export const getEmbyLibraryCover = (serverId, libraryId) => api.get(`/emby/servers/${serverId}/libraries/${libraryId}/cover`, { responseType: 'blob', skipGlobalErrorMessage: true })
export const createEmbyLibrary = (serverId, data) => api.post(`/emby/servers/${serverId}/libraries`, data)
export const updateEmbyLibrary = (serverId, libraryId, data) => api.put(`/emby/servers/${serverId}/libraries/${libraryId}`, data)
export const deleteEmbyLibrary = (serverId, libraryId, name) => api.delete(`/emby/servers/${serverId}/libraries/${libraryId}`, { params: { name } })
export const refreshManagedEmbyLibrary = (serverId, libraryId) => api.post(`/emby/servers/${serverId}/libraries/${libraryId}/refresh`)
export const refreshAllManagedEmbyLibraries = serverId => api.post(`/emby/servers/${serverId}/libraries/refresh-all`)
export const uploadEmbyLibraryCover = (serverId, libraryId, file) => {
  const form = new FormData()
  form.append('file', file)
  return api.post(`/emby/servers/${serverId}/libraries/${libraryId}/cover`, form)
}
export const generateEmbyLibraryCover = (serverId, libraryId, data) => api.post(`/emby/servers/${serverId}/libraries/${libraryId}/cover/generate`, data)
export const applyEmbyLibraryCover = (serverId, libraryId, taskId) => api.post(`/emby/servers/${serverId}/libraries/${libraryId}/cover/apply`, { task_id: taskId })
export const getEmbyCoverPreview = taskId => api.get(`/emby/cover-previews/${taskId}`, { responseType: 'blob' })

export const getStrmAssistantStatus = serverId => api.get(`/emby/servers/${serverId}/plugin/strm-assistant`)
export const runStrmAssistantTask = (serverId, data) => api.post(`/emby/servers/${serverId}/plugin/strm-assistant/tasks`, data)
export const getEmbyCoverAIConfig = () => api.get('/emby/cover-ai/config')
export const updateEmbyCoverAIConfig = data => api.put('/emby/cover-ai/config', data)
export const bindEmbyMediaSource = (serverId, sourceId, libraryId) => api.put(`/emby/servers/${serverId}/media-sources/${sourceId}`, { library_id: libraryId })

export const getEmbyMonitorOverview = serverId => api.get(`/emby/servers/${serverId}/monitor/overview`)
export const getEmbyMonitorRankings = (serverId, dimension, params) => api.get(`/emby/servers/${serverId}/monitor/rankings/${dimension}`, { params })
export const getEmbyMonitorHeatmap = (serverId, params) => api.get(`/emby/servers/${serverId}/monitor/heatmap`, { params })
export const getEmbyMonitorRecentItems = (serverId, params) => api.get(`/emby/servers/${serverId}/monitor/recent-items`, { params })
export const getEmbyMonitorItemImage = (serverId, itemId) => api.get(`/emby/servers/${serverId}/monitor/items/${itemId}/image`, { responseType: 'blob', skipGlobalErrorMessage: true })

export const getEmbyScheduledTasks = serverId => api.get(`/emby/servers/${serverId}/scheduled-tasks`)
export const updateEmbyScheduledTaskTriggers = (serverId, taskId, triggers) => api.put(`/emby/servers/${serverId}/scheduled-tasks/${encodeURIComponent(taskId)}/triggers`, { triggers })
export const startEmbyScheduledTask = (serverId, taskId) => api.post(`/emby/servers/${serverId}/scheduled-tasks/${encodeURIComponent(taskId)}/run`)
