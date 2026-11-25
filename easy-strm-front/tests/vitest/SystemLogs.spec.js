import { beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent, h } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { NMessageProvider } from 'naive-ui'

const api = vi.hoisted(() => ({
  getLogFiles: vi.fn(),
  getLogFileContent: vi.fn(),
  getLogConfig: vi.fn(),
  updateLogConfig: vi.fn()
}))
vi.mock('../../src/utils/api/strm', () => api)

const { default: SystemLogs } = await import('../../src/views/dashboard/SystemLogs.vue')

const LINES = [
  '[2026-11-25 10:00:01][ERROR][req=req-0001][task=-][module=db][trace=-] 数据库迁移失败',
  '[2026-11-25 10:00:00][INFO][req=-][task=77][module=cron][trace=-] 任务完成',
  'raw stderr line'
].join('\n')

const mountLogs = async () => {
  const Host = defineComponent({ render: () => h(NMessageProvider, null, { default: () => h(SystemLogs) }) })
  const wrapper = mount(Host, { attachTo: document.body })
  await flushPromises()
  return wrapper
}

describe('SystemLogs 过滤', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    api.getLogFiles.mockResolvedValue({ data: { data: [
      { name: 'info_2026-11-24.log', type: 'info', size: 10 },
      { name: 'info_2026-11-25.log', type: 'info', size: 20 },
      { name: 'debug_2026-11-25.log', type: 'debug', size: 30 }
    ] } })
    api.getLogFileContent.mockResolvedValue({ data: { data: { content: LINES, matched: 3, truncated: false } } })
    api.getLogConfig.mockResolvedValue({ data: { data: { value: 7 } } })
  })

  it('默认打开最新日期的 debug 文件并按级别渲染行', async () => {
    const wrapper = await mountLogs()
    expect(api.getLogFileContent).toHaveBeenCalledWith('debug_2026-11-25.log', 500, { level: '', q: '' })
    const text = wrapper.text()
    expect(text).toContain('数据库迁移失败')
    expect(text).toContain('raw stderr line')
    expect(text).toContain('显示最近 3 条匹配日志')
    wrapper.unmount()
  })

  it('点击 req / task / module 标记按该值重新查询并显示筛选标签', async () => {
    const wrapper = await mountLogs()
    const tokens = wrapper.findAll('.log-token')
    const reqToken = tokens.find(token => token.text().startsWith('req='))
    await reqToken.trigger('click')
    await flushPromises()
    expect(api.getLogFileContent).toHaveBeenLastCalledWith('debug_2026-11-25.log', 500, { level: '', q: 'req=req-0001' })
    expect(wrapper.text()).toContain('筛选：req=req-0001')

    const taskToken = wrapper.findAll('.log-token').find(token => token.text().startsWith('task='))
    await taskToken.trigger('click')
    await flushPromises()
    expect(api.getLogFileContent).toHaveBeenLastCalledWith('debug_2026-11-25.log', 500, { level: '', q: 'task=77' })

    const moduleToken = wrapper.findAll('.log-token').find(token => token.text().includes('[cron]'))
    await moduleToken.trigger('click')
    await flushPromises()
    expect(api.getLogFileContent).toHaveBeenLastCalledWith('debug_2026-11-25.log', 500, { level: '', q: 'module=cron' })
    wrapper.unmount()
  })

  it('无匹配时显示空状态', async () => {
    api.getLogFileContent.mockResolvedValue({ data: { data: { content: '', matched: 0 } } })
    const wrapper = await mountLogs()
    expect(wrapper.text()).toContain('暂无日志内容')
    wrapper.unmount()
  })
})
