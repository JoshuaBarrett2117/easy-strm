import { api, cookieUtils } from './request'

const handleLoginSuccess = (response) => {
  localStorage.setItem('token', response.token)
  localStorage.setItem('user_id', response.user_id)
  localStorage.setItem('user_name', response.name)
  cookieUtils.setCookie('token', response.token)

  const redirectUrl = localStorage.getItem('redirectUrl')
  if (redirectUrl) {
    localStorage.removeItem('redirectUrl')
    window.location.href = redirectUrl
  } else {
    window.location.href = '/dashboard/user-info'
  }
}

export const login = (data) => {
  return api.post('/auth/login', data).then(response => {
    handleLoginSuccess(response.data)
    return response
  })
}

export const getUserInfo = () => {
  return api.get('/auth/user/info')
}

export const logout = () => {
  localStorage.removeItem('token')
  localStorage.removeItem('user_id')
  localStorage.removeItem('user_name')
  cookieUtils.removeCookie('token')
  window.location.href = '/login'
}
