import { describe, expect, it, vi } from 'vitest'
import { authGuard, resolveAuthRedirect } from '../../src/router/guard'

const storageWith = (values) => ({ getItem: key => (key in values ? values[key] : null) })

describe('resolveAuthRedirect', () => {
  it('未登录访问业务页跳转登录页', () => {
    expect(resolveAuthRedirect({ path: '/dashboard/home', meta: {} }, storageWith({}))).toBe('/login')
  })

  it('未登录访问登录页放行', () => {
    expect(resolveAuthRedirect({ path: '/login', meta: {} }, storageWith({}))).toBeNull()
  })

  it('已登录访问登录页回首页', () => {
    expect(resolveAuthRedirect({ path: '/login', meta: {} }, storageWith({ token: 't' }))).toBe('/dashboard/home')
  })

  it('adminOnly 页面仅 admin 可访问', () => {
    const to = { path: '/emby', meta: { adminOnly: true } }
    expect(resolveAuthRedirect(to, storageWith({ token: 't', user_name: 'guest' }))).toBe('/dashboard/home')
    expect(resolveAuthRedirect(to, storageWith({ token: 't', user_name: 'admin' }))).toBeNull()
  })

  it('路由缺少 meta 时不报错', () => {
    expect(resolveAuthRedirect({ path: '/dashboard/home' }, storageWith({ token: 't' }))).toBeNull()
  })
})

describe('authGuard', () => {
  it('读取 localStorage 并调用 next', () => {
    localStorage.clear()
    const next = vi.fn()
    authGuard({ path: '/dashboard/home', meta: {} }, {}, next)
    expect(next).toHaveBeenCalledWith('/login')

    localStorage.setItem('token', 't')
    const allow = vi.fn()
    authGuard({ path: '/dashboard/home', meta: {} }, {}, allow)
    expect(allow).toHaveBeenCalledWith()
    localStorage.clear()
  })
})
