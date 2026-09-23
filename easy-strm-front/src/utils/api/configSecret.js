import { api } from './request'

export const revealConfigSecret = (key, serverId) => {
  const options = {
    params: serverId ? { server_id: serverId } : {},
    skipGlobalErrorMessage: true
  }
  return key.startsWith('emby_')
    ? api.get(`/emby/secrets/${encodeURIComponent(key)}`, options)
    : api.get(`/settings/secrets/${encodeURIComponent(key)}`, options)
}
