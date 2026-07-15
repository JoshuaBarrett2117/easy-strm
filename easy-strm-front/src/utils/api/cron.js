import { api } from './request'

export const getCronTasks = () => {
  return api.get('/cron/tasks')
}

export const updateCronTask = (id, data) => {
  return api.put(`/cron/task/${id}`, data)
}

export const runCronTask = (id) => {
  return api.post(`/cron/task/${id}/run`)
}
