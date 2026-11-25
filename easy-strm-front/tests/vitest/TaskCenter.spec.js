import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent, h } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { NMessageProvider, NSelect } from 'naive-ui'

const api = vi.hoisted(() => ({
  getUnifiedTaskList: vi.fn(),
  getTaskDetail: vi.fn(),
  cancelTask: vi.fn(),
  resumeTask: vi.fn(),
  getTaskTypes: vi.fn()
}))
vi.mock('../../src/utils/api/task', () => api)
const nav = vi.hoisted(() => ({ push: vi.fn(), route: { path: '/dashboard/tasks', query: {} } }))
vi.mock('vue-router', () => ({ useRoute: () => nav.route, useRouter: () => ({ push: nav.push }) }))

const { default: TaskCenter } = await import('../../src/views/dashboard/TaskCenter.vue')

const TASKS = [
  { task_id: 'strm_run', task_type: 'strm_generate', task_name: 'STRM 运行中', status: 'running', progress: 30, create_time: '2026-11-25 10:00:00', update_time: '2026-11-25 10:00:05', metadata: {} },
  { task_id: 'strm_fail', task_type: 'strm_generate', task_name: 'STRM 失败', status: 'failed', create_time: '2026-11-25 09:00:00', update_time: '2026-11-25 09:01:30', metadata: {} },
  { task_id: 'emby_ok', task_type: 'emby_refresh', task_name: 'Emby 刷新', status: 'success', create_time: '2026-11-25 08:00:00', update_time: '2026-11-25 08:00:10', metadata: { server_id: 1, server_name: '家里' } }
]

const mountCenter = async () => {
  const Host = defineComponent({ render: () => h(NMessageProvider, null, { default: () => h(TaskCenter) }) })
  const wrapper = mount(Host, { attachTo: document.body })
  await flushPromises()
  return wrapper
}
const statValue = (wrapper, key) => wrapper.find(`[data-testid="stat-${key}"]`).text()
const cardNames = (wrapper) => wrapper.findAll('[data-testid="task-card"]').map(card => card.text())

describe('任务中心', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    nav.route = { path: '/dashboard/tasks', query: {} }
    api.getUnifiedTaskList.mockResolvedValue({ data: { data: { data: TASKS } } })
    api.getTaskTypes.mockRejectedValue(new Error('offline'))
  })
  afterEach(() => {
    vi.useRealTimers()
    document.body.innerHTML = ''
  })

  it('类型筛选覆盖全部后端类型，不只 Emby；页头没有开发备注', async () => {
    const wrapper = await mountCenter()
    const select = wrapper.findAllComponents(NSelect).find(item => item.attributes('data-testid') === 'task-type-filter')
    const values = select.props('options').flatMap(group => group.children.map(option => option.value))
    expect(values).toEqual(expect.arrayContaining(['strm_generate', 'organize', 'share_sync', 'offline_download', 'emby_refresh']))
    expect(values.length).toBeGreaterThan(7)
    expect(wrapper.text()).not.toContain('保留现有任务取消')
    wrapper.unmount()
  })

  it('统计卡随筛选变化，点击统计卡按状态筛选', async () => {
    const wrapper = await mountCenter()
    expect(statValue(wrapper, 'all')).toContain('3')
    expect(statValue(wrapper, 'active')).toContain('1')
    expect(statValue(wrapper, 'failed')).toContain('1')
    expect(statValue(wrapper, 'done')).toContain('1')

    const select = wrapper.findAllComponents(NSelect).find(item => item.attributes('data-testid') === 'task-type-filter')
    select.vm.$emit('update:value', 'strm_generate')
    await flushPromises()
    expect(statValue(wrapper, 'all')).toContain('2')
    expect(statValue(wrapper, 'done')).toContain('0')

    await wrapper.find('[data-testid="stat-failed"]').trigger('click')
    await flushPromises()
    expect(cardNames(wrapper)).toHaveLength(1)
    expect(cardNames(wrapper)[0]).toContain('STRM 失败')
    expect(wrapper.find('[data-testid="stat-failed"]').attributes('aria-pressed')).toBe('true')

    await wrapper.find('[data-testid="stat-all"]').trigger('click')
    await flushPromises()
    expect(cardNames(wrapper)).toHaveLength(2)
    wrapper.unmount()
  })

  it('详情状态中文化、显示耗时，“查看日志”跳到系统日志并带 task 筛选', async () => {
    api.getTaskDetail.mockResolvedValue({ data: { data: { ...TASKS[1], display_status: 'partial_success' } } })
    const wrapper = await mountCenter()
    const card = wrapper.findAll('[data-testid="task-card"]').find(item => item.text().includes('STRM 失败'))
    await card.find('button').trigger('click')
    await flushPromises()
    expect(document.body.querySelector('[data-testid="task-detail-status"]').textContent).toBe('部分成功')
    expect(document.body.querySelector('[data-testid="task-detail-duration"]').textContent).toBe('1 分 30 秒')
    document.body.querySelector('[data-testid="task-view-logs"]').click()
    await flushPromises()
    expect(nav.push).toHaveBeenCalledWith({ path: '/dashboard/system-logs', query: { q: 'task=strm_fail' } })
    wrapper.unmount()
  })

  it('只有存在排队/执行中任务时才自动刷新列表', async () => {
    vi.useFakeTimers()
    api.getUnifiedTaskList.mockResolvedValue({ data: { data: { data: TASKS.slice(1) } } })
    const idle = await mountCenter()
    await vi.advanceTimersByTimeAsync(10000)
    expect(api.getUnifiedTaskList).toHaveBeenCalledTimes(1)
    idle.unmount()

    api.getUnifiedTaskList.mockClear()
    api.getUnifiedTaskList.mockResolvedValue({ data: { data: { data: TASKS } } })
    const busy = await mountCenter()
    await vi.advanceTimersByTimeAsync(6500)
    expect(api.getUnifiedTaskList.mock.calls.length).toBeGreaterThanOrEqual(3)
    busy.unmount()
  })
})
