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

// 115云相关API
export const getCloud115List = () => {
  return api.get('/cloud115')
}

export const createCloud115 = (data) => {
  return api.post('/cloud115', data)
}

export const updateCloud115 = (id, data) => {
  return api.put(`/cloud115/${id}`, data)
}

export const deleteCloud115 = (id) => {
  return api.delete(`/cloud115/${id}`)
}

// 115文件直链生成API
export const get115DirectLink = (params) => {
  return api.get('/115/direct-link', { params })
}

// 115文件列表API
export const get115Files = (params) => {
  return api.get('/115/files', { params })
}

// 115扫码登录相关API
// 获取支持的登录渠道列表
export const get115LoginChannels = () => {
  return api.get('/115/login/channels')
}

// 获取登录二维码
export const get115QRCode = () => {
  return api.get('/115/qrcode')
}

// 检查扫码登录状态
export const check115LoginStatus = (params) => {
  return api.get('/115/login/status', { params })
}

// 确认登录并保存凭据
export const confirm115Login = (data) => {
  return api.post('/115/login/confirm', data)
}

// 115 Open API扫码登录相关API（预留接口）
// 获取Open API登录二维码
export const get115OpenQRCode = () => {
  return api.get('/115/open/qrcode')
}

// 检查Open API扫码登录状态
export const check115OpenLoginStatus = (params) => {
  return api.get('/115/open/login/status', { params })
}

// 确认Open API登录并保存凭据
export const confirm115OpenLogin = (data) => {
  return api.post('/115/open/login/confirm', data)
}

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
