import { beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent, h } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { NDialogProvider, NMessageProvider } from 'naive-ui'

const nav = vi.hoisted(() => ({ push: vi.fn(), route: { path: '/dashboard/system-logs', query: {}, meta: { title: '系统日志' } } }))
vi.mock('vue-router', () => ({ useRoute: () => nav.route, useRouter: () => ({ push: nav.push }) }))
vi.mock('../../src/views/dashboard/SystemLogs.vue', () => ({ default: { render: () => null } }))
vi.mock('../../src/components/dashboard/PlaybackRecordsDrawer.vue', () => ({ default: { render: () => null } }))
vi.mock('../../src/utils/api/auth', () => ({ logout: vi.fn() }))

const { default: Dashboard } = await import('../../src/views/Dashboard.vue')

const mountDashboard = async () => {
  const Host = defineComponent({ render: () => h(NDialogProvider, null, { default: () => h(NMessageProvider, null, { default: () => h(Dashboard) }) }) })
  const wrapper = mount(Host, { global: { stubs: { 'router-view': true, RouterView: true } } })
  await flushPromises()
  return wrapper
}

describe('Dashboard 侧栏', () => {
  beforeEach(() => {
    localStorage.clear()
    nav.push.mockClear()
  })

  it('五组菜单，系统日志在“系统”组并高亮，点击菜单跳转', async () => {
    const wrapper = await mountDashboard()
    const text = wrapper.find('[data-testid="sidebar"]').text()
    for (const title of ['总览', '媒体整理', '115 与分享', 'STRM 与 Emby', '系统', '系统日志']) expect(text).toContain(title)
    expect(wrapper.text()).toContain('系统')
    const taskButton = wrapper.findAll('button[aria-label="任务中心"]')[0]
    await taskButton.trigger('click')
    expect(nav.push).toHaveBeenCalledWith('/dashboard/tasks')
    wrapper.unmount()
  })

  it('折叠状态写入 localStorage，重新挂载后保持', async () => {
    const wrapper = await mountDashboard()
    const sidebar = () => wrapper.find('[data-testid="sidebar"]')
    expect(sidebar().classes()).not.toContain('lg:w-[4.75rem]')
    await wrapper.find('[data-testid="sidebar-toggle"]').trigger('click')
    expect(sidebar().classes()).toContain('lg:w-[4.75rem]')
    expect(localStorage.getItem('easy-strm:sidebar-collapsed')).toBe('1')
    wrapper.unmount()

    const again = await mountDashboard()
    expect(again.find('[data-testid="sidebar"]').classes()).toContain('lg:w-[4.75rem]')
    await again.find('[data-testid="sidebar-toggle"]').trigger('click')
    expect(localStorage.getItem('easy-strm:sidebar-collapsed')).toBe('0')
    again.unmount()
  })
})
