import { api } from './request'

export const getTaskList = () => {
  return api.get('/auth/strm/task/all')
}

export const getTaskById = (taskId) => {
  return api.get(`/auth/strm/task/${taskId}`)
}

export const deleteTask = (taskId) => {
  return api.delete(`/auth/strm/task/${taskId}`, { params: { task_id: taskId } })
}
