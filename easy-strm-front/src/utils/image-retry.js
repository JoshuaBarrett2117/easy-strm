import { reactive } from 'vue'

// 图片加载重试：单次加载失败（被中断、限流、瞬时网络抖动）不应变成永久「暂无海报」。
// 旧实现一旦 @error 就把 URL 写进失败集合且永不清理，只有刷新页面重建组件才恢复。
const MAX_ATTEMPTS = 3

export const createImageRetry = () => {
  const attempts = reactive({})
  const failed = reactive({})

  const bump = url => {
    if (!url || failed[url]) return
    const next = (attempts[url] || 0) + 1
    if (next >= MAX_ATTEMPTS) {
      failed[url] = true
      return
    }
    // 延迟重试：避免 error 事件立即触发下一轮而瞬间耗尽次数。
    setTimeout(() => { attempts[url] = next }, 400 * next)
  }

  const src = url => {
    if (!url) return url
    const attempt = attempts[url] || 0
    if (!attempt) return url
    return `${url}${url.includes('?') ? '&' : '?'}retry=${attempt}`
  }

  const reset = () => {
    Object.keys(attempts).forEach(key => delete attempts[key])
    Object.keys(failed).forEach(key => delete failed[key])
  }

  return { attempts, failed, bump, src, reset }
}
