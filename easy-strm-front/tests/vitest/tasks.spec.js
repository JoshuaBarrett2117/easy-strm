import { beforeEach, describe, expect, it, vi } from 'vitest'

const api = vi.hoisted(() => ({ getTaskTypes: vi.fn() }))
vi.mock('../../src/utils/api/task', () => api)

const tasks = await import('../../src/utils/tasks')

describe('任务类型目录', () => {
  beforeEach(() => {
    tasks.taskTypeCatalog.value = [...tasks.TASK_TYPE_FALLBACK]
  })

  it('内置目录覆盖非 Emby 类型，下拉按组且补充未登记类型', () => {
    const options = tasks.taskTypeSelectOptions(['strm_generate', 'brand_new_type'])
    const values = options.flatMap(group => group.children.map(item => item.value))
    for (const type of ['strm_generate', 'organize', 'watch_auto_organize', 'share_sync', 'share_transfer', 'offline_download', 'cleanup', 'emby_refresh']) {
      expect(values).toContain(type)
    }
    expect(values.filter(value => value.startsWith('emby_')).length).toBe(7)
    expect(values.length).toBeGreaterThan(7)
    expect(options.find(group => group.label === '其他').children).toEqual([{ label: 'brand_new_type', value: 'brand_new_type' }])
    expect(new Set(values).size).toBe(values.length)
  })

  it('loadTaskTypes 以后端目录为准，失败时保留内置目录', async () => {
    api.getTaskTypes.mockResolvedValueOnce({ data: { data: [{ value: 'x_new', label: '新任务', group: '系统' }] } })
    await tasks.loadTaskTypes()
    expect(tasks.taskTypeLabel('x_new')).toBe('新任务')
    expect(tasks.taskTypeLabel('unknown_type')).toBe('unknown_type')
  })
})

describe('状态与耗时', () => {
  it('全部状态中文化', () => {
    for (const status of ['running', 'success', 'failed', 'cancelled', 'partial_success', 'pending', 'completed', 'unknown', 'skipped']) {
      expect(tasks.taskStatusLabel(status)).toMatch(/[\u4e00-\u9fa5]/)
    }
    expect(tasks.taskStatusLabel('partial_success')).toBe('部分成功')
    expect(tasks.taskStatusLabel('cancelled')).toBe('已取消')
    expect(tasks.effectiveTaskStatus({ status: 'failed', display_status: 'partial_success' })).toBe('partial_success')
    expect(tasks.effectiveTaskStatus({ status: 'completed', metadata: { outcome: 'skipped' } })).toBe('skipped')
  })

  it('已结束任务耗时 = 更新时间 - 创建时间；运行中 = 至今', () => {
    expect(tasks.taskDuration({ status: 'completed', create_time: '2026-11-25 10:00:00', update_time: '2026-11-25 11:02:05' })).toBe('1 小时 2 分')
    expect(tasks.taskDuration({ status: 'failed', create_time: '2026-11-25 10:00:00', update_time: '2026-11-25 10:00:42' })).toBe('42 秒')
    const now = Date.parse('2026-11-25T10:03:10')
    expect(tasks.taskDuration({ status: 'running', create_time: '2026-11-25 10:00:00', update_time: '2026-11-25 10:00:01' }, now)).toBe('3 分 10 秒（进行中）')
    expect(tasks.taskDuration({ status: 'completed' })).toBe('-')
  })

  it('跳转路由', () => {
    expect(tasks.taskLogsRoute('abc')).toEqual({ path: '/dashboard/system-logs', query: { q: 'task=abc' } })
    expect(tasks.taskCenterRoute('abc')).toEqual({ path: '/dashboard/tasks', query: { task_id: 'abc' } })
  })
})
