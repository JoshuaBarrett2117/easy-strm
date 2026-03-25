import { api } from './request'

export const getStrmConfigList = (params) => {
  return api.get('/auth/strm/config', { params })
}

export const getStrmConfigById = (id) => {
  return api.get(`/auth/strm/config/${id}`)
}

export const createStrmConfig = (data) => {
  return api.post('/auth/strm/config', data)
}

export const updateStrmConfig = (id, data) => {
  return api.put(`/auth/strm/config/${id}`, data)
}

export const deleteStrmConfig = (id) => {
  return api.delete(`/auth/strm/config/${id}`)
}

export const getStrmFilesByConfigId = (id) => {
  return api.get(`/auth/strm/config/${id}/files`)
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
