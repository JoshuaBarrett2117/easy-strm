import { api } from './request'

export const listShareSelections = (params, traceId) => api.get('/media/share-library/strm/selections', { params, traceId, skipGlobalErrorMessage: true })
export const getShareSelectionDetail = (media_item_key, traceId) => api.get('/media/share-library/strm/selections/detail', { params: { media_item_key }, traceId, skipGlobalErrorMessage: true })
export const changeShareSelection = (data, traceId) => api.put('/media/share-library/strm/selections', data, { traceId, skipGlobalErrorMessage: true })
export const refreshShareSelections = (traceId) => api.post('/media/share-library/strm/selections/refresh', null, { traceId, skipGlobalErrorMessage: true })
