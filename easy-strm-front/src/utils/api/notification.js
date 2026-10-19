import { request } from './request'

export const getNotificationTaskScope = (options = {}) => {
  return request('/notify/task-scope', {
    skipGlobalErrorMessage: options.skipGlobalErrorMessage || false
  })
}

export const updateNotificationTaskScope = (data, options = {}) => {
  return request('/notify/task-scope', {
    method: 'PUT',
    data,
    skipGlobalErrorMessage: options.skipGlobalErrorMessage || false
  })
}

export const getTelegramConfig = (options = {}) => {
  return request('/notify/telegram/config', {
    skipGlobalErrorMessage: options.skipGlobalErrorMessage || false
  })
}

export const updateTelegramConfig = (data, options = {}) => {
  return request('/notify/telegram/config', {
    method: 'PUT',
    data,
    skipGlobalErrorMessage: options.skipGlobalErrorMessage || false
  })
}

export const getTelegramStatus = (options = {}) => {
  return request('/notify/telegram/status', {
    skipGlobalErrorMessage: options.skipGlobalErrorMessage || false
  })
}

export const testTelegram = (options = {}) => {
  return request('/notify/telegram/test', {
    method: 'POST',
    skipGlobalErrorMessage: options.skipGlobalErrorMessage || false
  })
}

export const getWeComConfig = (options = {}) => {
  return request('/notify/wecom/config', {
    skipGlobalErrorMessage: options.skipGlobalErrorMessage || false
  })
}

export const updateWeComConfig = (data, options = {}) => {
  return request('/notify/wecom/config', {
    method: 'PUT',
    data,
    skipGlobalErrorMessage: options.skipGlobalErrorMessage || false
  })
}

export const testWeCom = (options = {}) => {
  return request('/notify/wecom/test', {
    method: 'POST',
    skipGlobalErrorMessage: options.skipGlobalErrorMessage || false
  })
}
