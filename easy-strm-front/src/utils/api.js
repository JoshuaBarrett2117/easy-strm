import axios from 'axios'
import { ElMessage } from 'element-plus'

const api = axios.create({
  baseURL: '/api',
  timeout: 10000,
  withCredentials: true
})

const cookieUtils = {
  setCookie(name, value, days = 7) {
    const date = new Date()
    date.setTime(date.getTime() + days * 24 * 60 * 60 * 1000)
    document.cookie = `${name}=${encodeURIComponent(value)};expires=${date.toUTCString()};path=/`
  },
  getCookie(name) {
    const match = document.cookie.match(new RegExp(`(^| )${name}=([^;]+)`))
    return match ? decodeURIComponent(match[2]) : null
  },
  removeCookie(name) {
    this.setCookie(name, '', -1)
  }
}

let isRedirectingToLogin = false

const saveCurrentUrl = () => {
  localStorage.setItem('redirectUrl', window.location.href)
}

const clearCredentials = () => {
  localStorage.removeItem('token')
  localStorage.removeItem('user_id')
  localStorage.removeItem('user_name')
  localStorage.removeItem('redirectUrl')
  cookieUtils.removeCookie('token')
}

const redirectToLogin = () => {
  if (isRedirectingToLogin) {
    return
  }

  isRedirectingToLogin = true
  saveCurrentUrl()
  window.location.replace('/login')
}

const getResponseErrorMessage = (error) => {
  return error?.response?.data?.error || error?.message || '请求失败'
}

api.interceptors.request.use(
  config => {
    const token = localStorage.getItem('token')
    if (token) {
      config.headers.Authorization = `Bearer ${token}`
    }
    return config
  },
  error => Promise.reject(error)
)

api.interceptors.response.use(
  response => response,
  error => {
    if (axios.isCancel(error)) {
      return Promise.reject(error)
    }

    if (error.response?.status === 401) {
      if (!isRedirectingToLogin) {
        ElMessage.error('登录已过期，请重新登录')
      }
      clearCredentials()
      redirectToLogin()
      return Promise.reject(new Error('登录已过期'))
    }

    if (error.config?.skipGlobalErrorMessage) {
      return Promise.reject(error)
    }

    if (error.response) {
      ElMessage.error(getResponseErrorMessage(error))
    } else {
      ElMessage.error('网络错误，请稍后重试')
    }

    return Promise.reject(error)
  }
)

export const request = (url, options = {}) => {
  const method = options.method || 'GET'
  const data = options.data || {}
  const requestOptions = {
    skipGlobalErrorMessage: options.skipGlobalErrorMessage || false
  }

  if (method === 'GET') {
    return api.get(url, { ...requestOptions, params: data })
  }

  return api({ ...requestOptions, method, url, data })
}

const handleLoginSuccess = (response) => {
  localStorage.setItem('token', response.token)
  localStorage.setItem('user_id', response.user_id)
  localStorage.setItem('user_name', response.name)
  cookieUtils.setCookie('token', response.token)

  const redirectUrl = localStorage.getItem('redirectUrl')
  if (redirectUrl) {
    localStorage.removeItem('redirectUrl')
    window.location.replace(redirectUrl)
  } else {
    window.location.replace('/dashboard/home')
  }
}

export const login = (data, options = {}) => {
  return api.post('/login', data, {
    skipGlobalErrorMessage: options.skipGlobalErrorMessage || false
  }).then(response => {
    handleLoginSuccess(response.data)
    return response
  })
}

export const getUserInfo = (options = {}) => {
  return api.get('/user/info', {
    skipGlobalErrorMessage: options.skipGlobalErrorMessage || false
  })
}

export { cookieUtils }

export const logout = () => {
  clearCredentials()
  window.location.replace('/login')
}

export const getLogFiles = () => {
  return api.get('/logs')
}

export const getLogFileContent = (filename, lines = 500) => {
  return api.get(`/logs/${filename}`, { params: { lines } })
}

export const getLogConfig = () => {
  return api.get('/logs/config')
}

export const updateLogConfig = (value) => {
  return api.put('/logs/config', { value })
}

export const getTaskList = () => {
  return api.get('/tasks/unified')
}

export const getTaskDetail = (taskId) => {
  return api.get(`/strm/task/${taskId}`)
}

export const cancelTask = (taskId) => {
  return api.post(`/tasks/${taskId}/cancel`)
}

export const resumeTask = (taskId) => {
  return api.post(`/tasks/${taskId}/resume`)
}

export const getSettings = () => {
  return api.get('/settings')
}

export const getSetting = (key) => {
  return api.get(`/settings/${key}`)
}

export const updateSetting = (key, value) => {
  return api.put(`/settings/${key}`, { value })
}

export const updateSettings = (settings) => {
  return api.put('/settings', settings)
}

export const getNetworkProbeSites = () => {
  return api.get('/network/test', { params: { mode: 'list' } })
}

export const testNetworkConnectivity = (params = {}) => {
  return api.get('/network/test', { params })
}

