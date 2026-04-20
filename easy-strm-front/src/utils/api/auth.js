import { api, cookieUtils } from './request'

const clearCredentials = () => {
  localStorage.removeItem('token')
  localStorage.removeItem('user_id')
  localStorage.removeItem('user_name')
  localStorage.removeItem('redirectUrl')
  cookieUtils.removeCookie('token')
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
    window.location.replace('/dashboard/user-info')
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

export const logout = () => {
  clearCredentials()
  window.location.replace('/login')
}
