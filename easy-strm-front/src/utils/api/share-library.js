import { api } from './request'
export const getShareLibrary = (params) => api.get('/media/share-library', { params })
export const getLibraryOptions = () => api.get('/media/share-library/options')
export const getLibrarySources = (params) => api.get('/media/share-library/sources', { params })
export const enrichLibrary = () => api.post('/media/share-library/enrich')
