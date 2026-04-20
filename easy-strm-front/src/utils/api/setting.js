import { request } from './request'

export const getSettings = (options = {}) => {
  return request('/settings', {
    skipGlobalErrorMessage: options.skipGlobalErrorMessage || false
  })
}

export const getSetting = (key, options = {}) => {
  return request(`/settings/${key}`, {
    skipGlobalErrorMessage: options.skipGlobalErrorMessage || false
  })
}

export const updateSetting = (key, value, options = {}) => {
  return request(`/settings/${key}`, {
    method: 'PUT',
    data: { value },
    skipGlobalErrorMessage: options.skipGlobalErrorMessage || false
  })
}

export const updateSettings = (settings, options = {}) => {
  return request('/settings', {
    method: 'PUT',
    data: settings,
    skipGlobalErrorMessage: options.skipGlobalErrorMessage || false
  })
}

export const testNetworkConnectivity = (options = {}) => {
  return request('/network/test', {
    skipGlobalErrorMessage: options.skipGlobalErrorMessage || false
  })
}
