import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { effectScope, nextTick, ref } from 'vue'
import { createPoller, usePolling, visibleDelay, visibleTimeout } from '../../src/composables/usePolling'

let hidden = false
const setHidden = (value) => {
  hidden = value
  document.dispatchEvent(new Event('visibilitychange'))
}

beforeEach(() => {
  vi.useFakeTimers()
  hidden = false
  Object.defineProperty(document, 'hidden', { configurable: true, get: () => hidden })
})
afterEach(() => {
  vi.useRealTimers()
})

describe('visibleTimeout', () => {
  it('页面可见时按时触发', () => {
    const fn = vi.fn()
    visibleTimeout(fn, 1000)
    vi.advanceTimersByTime(999)
    expect(fn).not.toHaveBeenCalled()
    vi.advanceTimersByTime(1)
    expect(fn).toHaveBeenCalledTimes(1)
  })

  it('隐藏期间不触发，重新可见时立即触发一次', () => {
    const fn = vi.fn()
    visibleTimeout(fn, 1000)
    setHidden(true)
    vi.advanceTimersByTime(10000)
    expect(fn).not.toHaveBeenCalled()
    setHidden(false)
    expect(fn).toHaveBeenCalledTimes(1)
    setHidden(true)
    setHidden(false)
    expect(fn).toHaveBeenCalledTimes(1)
  })

  it('隐藏时创建的定时器等到可见才触发', () => {
    hidden = true
    const fn = vi.fn()
    visibleTimeout(fn, 10)
    vi.advanceTimersByTime(1000)
    expect(fn).not.toHaveBeenCalled()
    setHidden(false)
    expect(fn).toHaveBeenCalledTimes(1)
  })

  it('cancel 后不触发', () => {
    const fn = vi.fn()
    const handle = visibleTimeout(fn, 100)
    handle.cancel()
    vi.advanceTimersByTime(1000)
    setHidden(true)
    setHidden(false)
    expect(fn).not.toHaveBeenCalled()
  })

  it('visibleDelay 在隐藏期间保持等待', async () => {
    const done = vi.fn()
    visibleDelay(100).then(done)
    setHidden(true)
    vi.advanceTimersByTime(1000)
    await Promise.resolve()
    expect(done).not.toHaveBeenCalled()
    setHidden(false)
    await Promise.resolve()
    expect(done).toHaveBeenCalled()
  })
})

describe('createPoller / usePolling', () => {
  it('按间隔执行、隐藏时暂停、可见时立即刷新后恢复', async () => {
    const fn = vi.fn().mockResolvedValue()
    const poller = createPoller(fn, 1000)
    poller.start()
    await vi.advanceTimersByTimeAsync(3000)
    expect(fn).toHaveBeenCalledTimes(3)
    setHidden(true)
    await vi.advanceTimersByTimeAsync(10000)
    expect(fn).toHaveBeenCalledTimes(3)
    setHidden(false)
    await vi.advanceTimersByTimeAsync(0)
    expect(fn).toHaveBeenCalledTimes(4)
    await vi.advanceTimersByTimeAsync(1000)
    expect(fn).toHaveBeenCalledTimes(5)
    poller.stop()
    await vi.advanceTimersByTimeAsync(5000)
    expect(fn).toHaveBeenCalledTimes(5)
  })

  it('上一次未完成时不会重叠执行，回调抛错不中断轮询', async () => {
    let release
    const fn = vi.fn()
      .mockImplementationOnce(() => new Promise(resolve => { release = resolve }))
      .mockRejectedValueOnce(new Error('boom'))
      .mockResolvedValue()
    const poller = createPoller(fn, 100)
    poller.start()
    await vi.advanceTimersByTimeAsync(1000)
    expect(fn).toHaveBeenCalledTimes(1)
    release()
    await vi.advanceTimersByTimeAsync(100)
    expect(fn).toHaveBeenCalledTimes(2)
    await vi.advanceTimersByTimeAsync(100)
    expect(fn).toHaveBeenCalledTimes(3)
    poller.stop()
  })

  it('enabled 响应式启停，作用域销毁时停止', async () => {
    const fn = vi.fn().mockResolvedValue()
    const enabled = ref(false)
    const scope = effectScope()
    scope.run(() => usePolling(fn, 100, { enabled }))
    await vi.advanceTimersByTimeAsync(500)
    expect(fn).not.toHaveBeenCalled()
    enabled.value = true
    await nextTick()
    await vi.advanceTimersByTimeAsync(250)
    expect(fn).toHaveBeenCalledTimes(2)
    enabled.value = false
    await nextTick()
    await vi.advanceTimersByTimeAsync(500)
    expect(fn).toHaveBeenCalledTimes(2)
    enabled.value = true
    await nextTick()
    scope.stop()
    await vi.advanceTimersByTimeAsync(500)
    expect(fn).toHaveBeenCalledTimes(2)
  })
})
