// 页面可见性感知的轮询工具：页面隐藏（切到其他标签页/最小化）时不发请求，
// 重新可见时立即执行一次，然后按原间隔恢复。
//
// - visibleTimeout(fn, ms)：替代 setTimeout，用于自行续约的轮询链；返回 { cancel }。
// - visibleDelay(ms)：替代 `await new Promise(r => setTimeout(r, ms))` 的轮询等待。
// - createPoller(fn, interval, options)：不依赖组件生命周期的轮询器（start/stop/refresh）。
// - usePolling(fn, interval, options)：组件内使用，作用域销毁时自动停止，可用 enabled 响应式开关。
import { getCurrentScope, isRef, onScopeDispose, unref, watch } from 'vue'

const hasDocument = () => typeof document !== 'undefined' && typeof document.addEventListener === 'function'
const isHidden = () => hasDocument() && document.hidden === true

/**
 * 类似 setTimeout，但页面隐藏期间不触发；隐藏后重新可见时立即触发。
 * 调用 cancel() 取消；触发或取消后会移除监听。
 */
export function visibleTimeout(fn, ms) {
  let timer = null
  let done = false
  const cleanup = () => {
    done = true
    clearTimeout(timer)
    timer = null
    if (hasDocument()) document.removeEventListener('visibilitychange', onVisibility)
  }
  const fire = () => {
    if (done) return
    cleanup()
    fn()
  }
  function onVisibility () {
    if (done) return
    if (isHidden()) {
      clearTimeout(timer)
      timer = null
    } else {
      fire()
    }
  }
  if (hasDocument()) document.addEventListener('visibilitychange', onVisibility)
  if (!isHidden()) {
    timer = setTimeout(() => {
      if (isHidden()) {
        timer = null
        return
      }
      fire()
    }, Math.max(0, Number(ms) || 0))
  }
  return { cancel: cleanup }
}

/** 取消 visibleTimeout 返回的句柄，兼容 null/undefined。 */
export const cancelVisibleTimeout = (handle) => {
  if (handle && typeof handle.cancel === 'function') handle.cancel()
}

/** 等待 ms 毫秒；页面隐藏期间一直等待，重新可见后立即继续。 */
export const visibleDelay = (ms) => new Promise(resolve => { visibleTimeout(resolve, ms) })

/**
 * 创建轮询器：fn 执行完才开始计下一次间隔（不会重叠）。
 * interval 可以是数字、ref 或返回数字的函数。
 * options.immediate：start() 时先执行一次。
 */
export function createPoller(fn, interval, { immediate = false } = {}) {
  let handle = null
  let active = false
  let running = false
  const resolveInterval = () => {
    const value = typeof interval === 'function' ? interval() : unref(interval)
    return Math.max(0, Number(value) || 0)
  }
  const schedule = () => {
    cancelVisibleTimeout(handle)
    handle = active ? visibleTimeout(tick, resolveInterval()) : null
  }
  async function tick () {
    if (!active) return
    running = true
    try {
      await fn()
    } catch (error) {
      // 轮询回调自己负责提示；这里只保证轮询不中断。
    } finally {
      running = false
      if (active) schedule()
    }
  }
  const start = () => {
    if (active) return
    active = true
    if (immediate) void tick()
    else schedule()
  }
  const stop = () => {
    active = false
    cancelVisibleTimeout(handle)
    handle = null
  }
  /** 立即执行一次并重新计时（未启动时只执行一次）。 */
  const refresh = async () => {
    cancelVisibleTimeout(handle)
    if (running) return
    if (active) await tick()
    else await fn()
  }
  return { start, stop, refresh, isActive: () => active }
}

/**
 * 组件内轮询。options.enabled 为 ref/getter 时随之自动启停；否则需手动 start()。
 * 所在作用域销毁（组件卸载）时自动停止。
 */
export function usePolling(fn, interval, { immediate = false, enabled } = {}) {
  const poller = createPoller(fn, interval, { immediate })
  if (enabled !== undefined) {
    const source = typeof enabled === 'function' || isRef(enabled) ? enabled : () => enabled
    watch(source, value => (value ? poller.start() : poller.stop()), { immediate: true })
  }
  if (getCurrentScope()) onScopeDispose(poller.stop)
  return poller
}
