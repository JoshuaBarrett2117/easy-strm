import axios from 'axios'
import { ElMessage } from 'element-plus'

// 创建Axios实例
const api = axios.create({
  baseURL: '/api',
  timeout: 10000,
  withCredentials: true // 允许携带凭证，用于跨域请求
})

// Cookie操作工具函数
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

// 保存当前页面URL到localStorage
const saveCurrentUrl = () => {
  localStorage.setItem('redirectUrl', window.location.href)
}

// 清除本地存储的凭证
const clearCredentials = () => {
  localStorage.removeItem('token')
  localStorage.removeItem('user_id')
  cookieUtils.removeCookie('token')
}

// 跳转到登录页面
const redirectToLogin = () => {
  saveCurrentUrl()
  window.location.href = '/login'
}

// 请求拦截器，自动添加token
api.interceptors.request.use(
  config => {
    const token = localStorage.getItem('token')
    if (token) {
      config.headers.Authorization = `Bearer ${token}`
    }
    return config
  },
  error => {
    return Promise.reject(error)
  }
)

// 响应拦截器，处理token验证失败的情况
api.interceptors.response.use(
  response => {
    return response
  },
  error => {
    // 处理token验证失败的情况
    if (error.response && error.response.status === 401) {
      ElMessage.error('登录已过期，请重新登录')
      clearCredentials()
      redirectToLogin()
      return Promise.reject(new Error('登录已过期'))
    }
    // 处理其他错误
    if (error.response) {
      const message = error.response.data.error || '请求失败'
      ElMessage.error(message)
    } else {
      ElMessage.error('网络错误，请稍后重试')
    }
    return Promise.reject(error)
  }
)

// 通用请求函数
export const request = (url, options = {}) => {
  const method = options.method || 'GET'
  const data = options.data || {}
  
  if (method === 'GET') {
    return api.get(url, { params: data })
  } else {
    return api({ method, url, data })
  }
}

// 登录成功后处理
const handleLoginSuccess = (response) => {
  // 存储token到本地存储
  localStorage.setItem('token', response.token)
  localStorage.setItem('user_id', response.user_id)
  localStorage.setItem('user_name', response.name)
  cookieUtils.setCookie('token', response.token)
  
  // 检查是否有重定向URL
  const redirectUrl = localStorage.getItem('redirectUrl')
  if (redirectUrl) {
    localStorage.removeItem('redirectUrl')
    window.location.href = redirectUrl
  } else {
    // 默认跳转到用户信息页面
    window.location.href = '/dashboard/user-info'
  }
}

// 登录API
export const login = (data) => {
  return api.post('/login', data).then(response => {
    handleLoginSuccess(response.data)
    return response
  })
}

// 获取用户信息API
export const getUserInfo = () => {
  return api.get('/user/info')
}

// 115云相关API已移动到 utils/api/cloud115.js

export { cookieUtils }

// 日志查看API（仅admin用户可访问）
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

// 任务相关API
export const getTaskList = () => {
  return api.get('/tasks')
}

export const getTaskDetail = (taskId) => {
  return api.get(`/strm/task/${taskId}`)
}

// 系统配置API
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

export const testNetworkConnectivity = () => {
  return api.get('/network/test')
}
