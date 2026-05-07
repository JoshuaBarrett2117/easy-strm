import { api } from './request'

/**
 * Dashboard 数据概览 API
 */

// 获取 Dashboard 统计数据
export const getDashboardStats = () => api.get('/dashboard/stats')

// 获取 Dashboard 首页总览
export const getDashboardOverview = () => api.get('/dashboard/overview')

// 获取 Dashboard 资源监控
export const getDashboardResourceMonitor = () => api.get('/dashboard/resource-monitor')

// 获取 Dashboard 趋势
export const getDashboardTrend = (kind, days = 7) => {
  return api.get(`/dashboard/trends/${kind}`, { params: { days } })
}
