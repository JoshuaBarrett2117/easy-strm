import axios from 'axios'
import { message } from '../ui/feedback'

export const api = axios.create({
  baseURL: '/api',
  timeout: 60000,
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

let isRedirectingToLogin = false

const saveCurrentUrl = () => {
  localStorage.setItem('redirectUrl', window.location.href)
}

const clearCredentials = () => {
  localStorage.removeItem('token')
  localStorage.removeItem('user_id')
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
        message.error('登录已过期，请重新登录')
      }
      clearCredentials()
      redirectToLogin()
      return Promise.reject(new Error('登录已过期'))
    }

    if (error.config?.skipGlobalErrorMessage) {
      return Promise.reject(error)
    }

    if (error.response) {
      message.error(getResponseErrorMessage(error))
    } else {
      message.error('网络错误，请稍后重试')
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
