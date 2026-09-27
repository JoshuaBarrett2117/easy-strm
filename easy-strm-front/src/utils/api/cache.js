import { request } from './request'

export const getCacheOverview = () => {
  return request('/cache/overview')
}

export const getCacheDetails = (params) => {
  return request('/cache/details', { params })
}

export const clearCacheGroup = (scope) => {
  return request('/cache/clear', {
    method: 'POST',
    data: { scope }
  })
}
