import { api } from './request'

export const getCronTasks = params => {
  return api.get('/cron/tasks', { params })
}

export const getCronHandlers = () => api.get('/cron/handlers')
export const createCronTask = data => api.post('/cron/task', data)
export const deleteCronTask = id => api.delete(`/cron/task/${id}`)
export const getCronRuns = (id, params) => api.get(`/cron/task/${id}/runs`, { params })

export const updateCronTask = (id, data) => {
  return api.put(`/cron/task/${id}`, data)
}

export const runCronTask = (id) => {
  return api.post(`/cron/task/${id}/run`)
}
