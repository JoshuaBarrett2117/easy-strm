import { api } from './request'

export const getCloud115List = () => {
  return api.get('/auth/cloud115/list')
}

export const getCloud115ById = (id) => {
  return api.get(`/auth/cloud115/${id}`)
}

export const createCloud115 = (data) => {
  return api.post('/auth/cloud115', data)
}

export const updateCloud115 = (id, data) => {
  return api.put(`/auth/cloud115/${id}`, data)
}

export const deleteCloud115 = (id) => {
  return api.delete(`/auth/cloud115/${id}`)
}

export const get115LoginChannels = () => {
  return api.get('/auth/cloud115/login/channels')
}

export const get115QRCode = () => {
  return api.get('/auth/cloud115/login/qrcode')
}

export const check115LoginStatus = (params) => {
  return api.get('/auth/cloud115/login/status', { params })
}

export const confirm115Login = (data) => {
  return api.post('/auth/cloud115/login/confirm', data)
}

export const get115DirectLink = (params) => {
  return api.get('/115/direct-link', { params })
}

export const get115Files = (params) => {
  return api.get('/115/files', { params })
}
