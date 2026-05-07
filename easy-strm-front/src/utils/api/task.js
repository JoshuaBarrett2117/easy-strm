import { api } from './request'

/**
 * 任务管理 API
 */

// 获取所有任务
export const getTaskList = () => {
  return api.get('/strm/task/all')
}

// 获取统一任务列表（按优先级排序）
export const getUnifiedTaskList = () => {
  return api.get('/tasks/unified')
}

// 获取单个任务
export const getTaskById = (taskId) => {
  return api.get(`/tasks/${taskId}`)
}

// 获取任务详情
export const getTaskDetail = (taskId) => {
  return api.get(`/tasks/${taskId}`)
}

// 取消运行中的任务
export const cancelTask = (taskId) => {
  return api.post(`/tasks/${taskId}/cancel`)
}

// 恢复已取消或失败的任务
export const resumeTask = (taskId) => {
  return api.post(`/tasks/${taskId}/resume`)
}

// 删除任务
export const deleteTask = (taskId) => {
  return api.delete(`/strm/task/${taskId}`, { params: { task_id: taskId } })
}
