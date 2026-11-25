// 后台任务提交成功提示：统一带“查看任务”链接，点击跳到任务中心并打开该任务详情。
import { h } from 'vue'
import { message as discreteMessage } from './feedback'
import { taskCenterRoute } from '../tasks'

let routerPromise = null
const getRouter = () => {
  if (!routerPromise) routerPromise = import('../../router').then(module => module.default)
  return routerPromise
}

/** 跳转到任务中心并定位任务。 */
export const openTaskInCenter = async (taskId, router) => {
  const target = router || await getRouter()
  return target.push(taskCenterRoute(taskId))
}

/**
 * 提示后台任务已提交。
 * @param {string} text 提示文案，如“整理任务已提交”
 * @param {string} [taskId] 任务 ID；为空时只显示文案和“任务中心”链接
 * @param {object} [options] { type: 'success'|'info'|'warning', duration, router }
 */
export const notifyTaskSubmitted = (text, taskId, options = {}) => {
  const { type = 'success', duration = 6000, router } = options
  let reactive = null
  const onClick = (event) => {
    event?.preventDefault?.()
    reactive?.destroy?.()
    void openTaskInCenter(taskId, router)
  }
  const render = () => h('span', { class: 'inline-flex flex-wrap items-center gap-x-2' }, [
    h('span', text),
    h('a', {
      href: '#',
      class: 'font-semibold text-indigo-500 underline-offset-2 hover:underline',
      'data-testid': 'task-toast-link',
      onClick
    }, taskId ? '查看任务' : '前往任务中心')
  ])
  const method = typeof discreteMessage[type] === 'function' ? type : 'success'
  reactive = discreteMessage[method](render, { duration, closable: true, keepAliveOnHover: true })
  return reactive
}
