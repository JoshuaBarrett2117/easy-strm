import { request } from './request'

export const getSettings = () => {
  return request('/settings')
}

export const getSetting = (key) => {
  return request(`/settings/${key}`)
}

export const updateSetting = (key, value) => {
  return request(`/settings/${key}`, {
    method: 'PUT',
    data: { value }
  })
}

export const updateSettings = (settings) => {
  return request('/settings', {
    method: 'PUT',
    data: settings
  })
}

export const testNetworkConnectivity = () => {
  return request('/network/test')
}
