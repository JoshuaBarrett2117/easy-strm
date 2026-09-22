// 每个任务独立轮询，同一任务的重复订阅共享结果，卸载后忽略在途响应。
export function createShareTaskWatcher(fetchTask, onState, onComplete, interval = 1000) {
  const watchers = new Map()
  let disposed = false
  const watch = id => {
    if (disposed) return Promise.resolve(null)
    if (watchers.has(id)) return watchers.get(id).promise
    let resolve, reject
    const promise = new Promise((res, rej) => { resolve = res; reject = rej })
    const entry = { promise, resolve, timer: null }
    watchers.set(id, entry)
    const poll = async () => {
      try {
        const task = await fetchTask(id)
        if (disposed) return
        onState(task)
        if (['completed', 'failed', 'cancelled'].includes(task.status)) {
          watchers.delete(id)
          resolve(task)
          onComplete(task)
        } else {
          entry.timer = setTimeout(poll, interval)
        }
      } catch (error) {
        if (disposed) return
        watchers.delete(id)
        reject(error)
      }
    }
    void poll()
    return promise
  }
  const dispose = () => {
    disposed = true
    for (const entry of watchers.values()) {
      clearTimeout(entry.timer)
      entry.resolve(null)
    }
    watchers.clear()
  }
  return { watch, dispose }
}
