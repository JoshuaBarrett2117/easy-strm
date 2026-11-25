// 全局路由守卫：未登录只能去登录页，已登录访问登录页回首页，adminOnly 页面仅 admin 可见。

/** 返回需要重定向的路径；放行时返回 null。storage 默认 localStorage，便于测试注入。 */
export const resolveAuthRedirect = (to, storage = localStorage) => {
  const token = storage.getItem('token')
  const isLoginPage = to.path === '/login'
  if (!token && !isLoginPage) return '/login'
  if (token && isLoginPage) return '/dashboard/home'
  if (to.meta?.adminOnly && storage.getItem('user_name') !== 'admin') return '/dashboard/home'
  return null
}

export const authGuard = (to, from, next) => {
  const redirect = resolveAuthRedirect(to)
  if (redirect) next(redirect)
  else next()
}
