import { api } from './request'

export const getEmbyStatus = (options = {}) => {
  return api.get('/emby/status', {
    skipGlobalErrorMessage: options.skipGlobalErrorMessage || false
  })
}

export const getEmbyLibraries = (options = {}) => {
  return api.get('/emby/libraries', {
    skipGlobalErrorMessage: options.skipGlobalErrorMessage || false
  })
}

export const refreshEmbyLibrary = (libraryId = '', options = {}) => {
  return api.post('/emby/refresh', { library_id: libraryId }, {
    skipGlobalErrorMessage: options.skipGlobalErrorMessage || false
  })
}
