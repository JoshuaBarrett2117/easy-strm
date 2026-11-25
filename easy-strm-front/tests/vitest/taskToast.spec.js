import { describe, expect, it, vi } from 'vitest'
import { flushPromises } from '@vue/test-utils'

const discrete = vi.hoisted(() => ({ calls: [] }))
vi.mock('../../src/utils/ui/feedback', () => {
  const make = type => vi.fn((content, options) => {
    const handle = { destroy: vi.fn() }
    discrete.calls.push({ type, content, options, handle })
    return handle
  })
  return { message: { success: make('success'), info: make('info'), warning: make('warning') } }
})

const { notifyTaskSubmitted } = await import('../../src/utils/ui/taskToast')

const findLink = (vnode) => vnode.children.find(child => child.props?.['data-testid'] === 'task-toast-link')

describe('notifyTaskSubmitted', () => {
  it('提示带“查看任务”链接，点击跳任务中心并定位任务', async () => {
    const router = { push: vi.fn() }
    notifyTaskSubmitted('整理任务已提交', 'organize_1', { router })
    const call = discrete.calls.at(-1)
    expect(call.type).toBe('success')
    expect(call.options.closable).toBe(true)
    const vnode = call.content()
    expect(vnode.children[0].children).toBe('整理任务已提交')
    const link = findLink(vnode)
    expect(link.children).toBe('查看任务')
    link.props.onClick({ preventDefault () {} })
    await flushPromises()
    expect(router.push).toHaveBeenCalledWith({ path: '/dashboard/tasks', query: { task_id: 'organize_1' } })
    expect(call.handle.destroy).toHaveBeenCalled()
  })

  it('没有任务 ID 时链接到任务中心；可指定类型', () => {
    notifyTaskSubmitted('已提交', '', { type: 'info', router: { push: vi.fn() } })
    const call = discrete.calls.at(-1)
    expect(call.type).toBe('info')
    expect(findLink(call.content()).children).toBe('前往任务中心')
  })
})
