import { request } from './request'

export const getGlobalApiConfig = (options = {}) => request('/system-api/config', { skipGlobalErrorMessage: options.skipGlobalErrorMessage || false })
export const updateGlobalApiConfig = (data, options = {}) => request('/system-api/config', { method: 'PUT', data, skipGlobalErrorMessage: options.skipGlobalErrorMessage || false })
export const regenerateGlobalApiKey = (options = {}) => request('/system-api/key/regenerate', { method: 'POST', skipGlobalErrorMessage: options.skipGlobalErrorMessage || false })
