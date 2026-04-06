import { api } from './request'

export const getCloud115List = () => {
  return api.get('/cloud115')
}

export const getCloud115ById = (id) => {
  return api.get(`/cloud115/${id}`)
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

export const get115LoginChannels = () => {
  return api.get('/115/login/channels')
}

export const get115QRCode = () => {
  return api.get('/115/qrcode')
}

export const check115LoginStatus = (params) => {
  return api.get('/115/login/status', { params, timeout: 60000 })
}

export const confirm115Login = (data) => {
  return api.post('/115/login/confirm', data)
}

export const get115DirectLink = (params) => {
  return api.get('/115/direct-link', { params })
}

export const get115Files = (params) => {
  return api.get('/115/files', { params })
}

export const get115OpenQRCode = () => {
  return api.get('/115/open/qrcode')
}

export const check115OpenLoginStatus = (params) => {
  return api.get('/115/open/login/status', { params })
}

export const confirm115OpenLogin = (data) => {
  return api.post('/115/open/login/confirm', data)
}
