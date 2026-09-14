import { api } from './request'

// AI配置、模型发现和试运行统一使用后端接口。
export const getAIConfig = () => api.get('/settings/ai-recognition')
export const saveAIConfig = config => api.put('/settings/ai-recognition', config)
export const getAIModels = config => api.post('/settings/ai-recognition/models', config, { timeout: 125000 })
export const testAIRecognition = (config, filename) => api.post('/settings/ai-recognition/test', { config, filename }, { timeout: 125000 })
export const assistMediaFilename = filename => api.post('/settings/ai-recognition/assist', { filename }, { timeout: 125000 })
