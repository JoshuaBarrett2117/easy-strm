import { request } from './request'

export const getSettings = (options = {}) => {
  return request('/settings', {
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

export const testMdcConnection = (url, options = {}) => request('/settings/mdc/test', {
  method: 'POST',
  data: { url },
  skipGlobalErrorMessage: options.skipGlobalErrorMessage || false
})

export const testNetworkConnectivity = (options = {}) => {
  return request('/network/test', {
    data: options,
    skipGlobalErrorMessage: options.skipGlobalErrorMessage || false
  })
}

export const getNetworkProbeSites = (options = {}) => {
  return request('/network/test', {
    data: { mode: 'list' },
    skipGlobalErrorMessage: options.skipGlobalErrorMessage || false
  })
}
