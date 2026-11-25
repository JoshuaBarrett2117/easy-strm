import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const { messageError } = vi.hoisted(() => ({ messageError: vi.fn() }))
vi.mock('../../src/utils/ui/feedback', () => ({ message: { error: messageError, success: vi.fn() } }))

const { api, request } = await import('../../src/utils/api/request')

const respond = (handler) => {
  api.defaults.adapter = async (config) => handler(config)
}

const httpError = (config, status, data = {}) => {
  const error = new Error(`Request failed with status code ${status}`)
  error.config = config
  error.response = { status, data, config, headers: {} }
  error.isAxiosError = true
  return Promise.reject(error)
}

describe('request 包装', () => {
  let replace
  const originalLocation = window.location

  beforeEach(() => {
    localStorage.clear()
    messageError.mockClear()
    replace = vi.fn()
    Object.defineProperty(window, 'location', { configurable: true, value: { href: 'http://localhost/dashboard/tasks', replace } })
  })

  afterEach(() => {
    Object.defineProperty(window, 'location', { configurable: true, value: originalLocation })
  })

  it('GET 请求把 data 作为查询参数，并附带 token 与追踪 ID', async () => {
    localStorage.setItem('token', 'abc')
    let seen
    respond(config => {
      seen = config
      return { data: { ok: true }, status: 200, statusText: 'OK', headers: {}, config }
    })
    const response = await request('/tasks', { data: { page: 2 } })
    expect(response.data).toEqual({ ok: true })
    expect(seen.method).toBe('get')
    expect(seen.params).toEqual({ page: 2 })
    expect(seen.baseURL).toBe('/api')
    expect(seen.headers.Authorization).toBe('Bearer abc')
    expect(seen.headers['X-Trace-Id']).toBeTruthy()
  })

  it('非 GET 请求把 data 作为请求体并透传 traceId', async () => {
    let seen
    respond(config => {
      seen = config
      return { data: {}, status: 200, statusText: 'OK', headers: {}, config }
    })
    await request('/tasks', { method: 'POST', data: { name: 'x' }, traceId: 'trace-1' })
    expect(seen.method).toBe('post')
    expect(JSON.parse(seen.data)).toEqual({ name: 'x' })
    expect(seen.headers['X-Trace-Id']).toBe('trace-1')
  })

  it('401 清理凭据并跳转登录页', async () => {
    localStorage.setItem('token', 'expired')
    localStorage.setItem('user_id', '1')
    respond(config => httpError(config, 401))
    await expect(request('/tasks')).rejects.toThrow('登录已过期')
    expect(localStorage.getItem('token')).toBeNull()
    expect(localStorage.getItem('user_id')).toBeNull()
    expect(localStorage.getItem('redirectUrl')).toBe('http://localhost/dashboard/tasks')
    expect(replace).toHaveBeenCalledWith('/login')
  })

  it('其他错误默认提示后端错误信息，skipGlobalErrorMessage 时不提示', async () => {
    respond(config => httpError(config, 500, { error: '服务异常' }))
    await expect(request('/tasks')).rejects.toBeTruthy()
    expect(messageError).toHaveBeenCalledWith('服务异常')

    messageError.mockClear()
    await expect(request('/tasks', { skipGlobalErrorMessage: true })).rejects.toBeTruthy()
    expect(messageError).not.toHaveBeenCalled()
  })
})
