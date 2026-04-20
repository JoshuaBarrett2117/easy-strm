import { api } from './request'

/**
 * Dashboard 数据概览 API
 */

// 获取 Dashboard 统计数据
export const getDashboardStats = () => api.get('/dashboard/stats')
