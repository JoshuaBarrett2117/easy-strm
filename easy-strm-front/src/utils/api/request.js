import axios from 'axios'
import { ElMessage } from 'element-plus'

export const api = axios.create({
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

export { cookieUtils }

const saveCurrentUrl = () => {
  localStorage.setItem('redirectUrl', window.location.href)
}

const clearCredentials = () => {
  localStorage.removeItem('token')
  localStorage.removeItem('user_id')
  cookieUtils.removeCookie('token')
}

const redirectToLogin = () => {
  saveCurrentUrl()
  window.location.href = '/login'
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
    if (error.response && error.response.status === 401) {
      ElMessage.error('登录已过期，请重新登录')
      clearCredentials()
      redirectToLogin()
      return Promise.reject(new Error('登录已过期'))
    }
    if (error.response) {
      const message = error.response.data.error || '请求失败'
      ElMessage.error(message)
    } else {
      ElMessage.error('网络错误，请稍后重试')
    }
    return Promise.reject(error)
  }
)

export const request = (url, options = {}) => {
  const method = options.method || 'GET'
  const data = options.data || {}
  if (method === 'GET') {
    return api.get(url, { params: data })
  }
  return api({ method, url, data })
}
