import { describe, expect, it, vi } from 'vitest'

vi.mock('../../src/router/guard', () => ({ authGuard: () => true }))
const { menuSections, visibleMenuSections, sectionTitleFor } = await import('../../src/router/menu')
const { routes } = await import('../../src/router/index')

const items = menuSections.flatMap(section => section.items)

describe('侧栏菜单', () => {
  it('五个分组，顺序固定', () => {
    expect(menuSections.map(section => section.title)).toEqual(['总览', '媒体整理', '115 与分享', 'STRM 与 Emby', '系统'])
  })

  it('每项图标都不重复', () => {
    expect(new Set(items.map(item => item.icon)).size).toBe(items.length)
  })

  it('系统日志在“系统”组；所有业务路由都有菜单项且路径不变', () => {
    expect(sectionTitleFor('/dashboard/system-logs')).toBe('系统')
    const children = routes.find(route => route.path === '/dashboard').children.map(child => `/dashboard/${child.path}`)
    expect(items.map(item => item.path).sort()).toEqual(children.sort())
  })

  it('非管理员隐藏 Emby 管理与监控', () => {
    const paths = visibleMenuSections(false).flatMap(section => section.items.map(item => item.path))
    expect(paths).not.toContain('/dashboard/emby-management')
    expect(paths).not.toContain('/dashboard/emby-monitor')
    expect(visibleMenuSections(true).flatMap(section => section.items).length).toBe(items.length)
  })
})
