// 任务中心共用的类型名、状态名、耗时与跳转工具。
// 任务类型以后端 GET /tasks/types（domain.TaskTypeCatalog）为准；这里的 FALLBACK 只在接口未返回前/失败时使用，
// 后端新增类型时 loadTaskTypes() 会自动补上，无需改前端。
import { ref } from 'vue'
import { getTaskTypes } from './api/task'

export const TASK_TYPE_FALLBACK = [
  { value: 'strm_generate', label: 'STRM 生成', group: 'STRM' },
  { value: 'incremental_sync', label: 'STRM 增量同步', group: 'STRM' },
  { value: 'share_baseline_adopt', label: '分享 STRM 基线采纳', group: 'STRM' },
  { value: 'organize', label: '媒体整理', group: '媒体整理' },
  { value: 'watch_auto_organize', label: '自动整理', group: '媒体整理' },
  { value: 'share_transfer', label: '115 分享转存', group: '115 与分享' },
  { value: 'share_transfer_organize', label: '转存后整理', group: '115 与分享' },
  { value: 'share_transfer_scrape', label: '转存后刮削', group: '115 与分享' },
  { value: 'share_sync', label: '分享同步', group: '115 与分享' },
  { value: 'share_identify', label: '分享媒体识别', group: '115 与分享' },
  { value: 'share_delete', label: '删除分享', group: '115 与分享' },
  { value: 'share_clear', label: '清空分享文件记录', group: '115 与分享' },
  { value: 'share_media_delete', label: '删除分享文件记录', group: '115 与分享' },
  { value: 'offline_download', label: '115 云下载', group: '115 与分享' },
  { value: 'file_transfer', label: '文件传输', group: '115 与分享' },
  { value: 'emby_refresh', label: 'Emby 库刷新', group: 'Emby' },
  { value: 'emby_server', label: 'Emby 实例管理', group: 'Emby' },
  { value: 'emby_user', label: 'Emby 用户管理', group: 'Emby' },
  { value: 'emby_library', label: 'Emby 媒体库管理', group: 'Emby' },
  { value: 'emby_cover', label: 'Emby 媒体库封面', group: 'Emby' },
  { value: 'emby_plugin', label: '神医助手任务', group: 'Emby' },
  { value: 'emby_scheduled', label: 'Emby 定时任务触发', group: 'Emby' },
  { value: 'cleanup', label: '定时清理', group: '系统' }
]

/** 当前任务类型目录（响应式）。 */
export const taskTypeCatalog = ref([...TASK_TYPE_FALLBACK])
let loadPromise = null

/** 从后端加载任务类型目录（全局只请求一次；失败时保留内置目录，下次调用重试）。 */
export const loadTaskTypes = () => {
  if (!loadPromise) {
    loadPromise = getTaskTypes()
      .then(response => {
        const list = response?.data?.data
        if (Array.isArray(list) && list.length) {
          taskTypeCatalog.value = list.filter(item => item && item.value).map(item => ({
            value: String(item.value),
            label: String(item.label || item.value),
            group: String(item.group || '其他')
          }))
        }
        return taskTypeCatalog.value
      })
      .catch(() => {
        loadPromise = null
        return taskTypeCatalog.value
      })
  }
  return loadPromise
}

/** 任务类型中文名；未登记时原样返回类型值。 */
export const taskTypeLabel = (type) => {
  if (!type) return '未知任务'
  return taskTypeCatalog.value.find(item => item.value === type)?.label || type
}

/**
 * 任务类型筛选下拉：按分组，覆盖目录中全部类型，另外补上列表里出现但目录未登记的类型。
 * @param {string[]} extraTypes 当前任务列表中出现的类型
 */
export const taskTypeSelectOptions = (extraTypes = []) => {
  const groups = new Map()
  const known = new Set()
  for (const item of taskTypeCatalog.value) {
    known.add(item.value)
    if (!groups.has(item.group)) groups.set(item.group, [])
    groups.get(item.group).push({ label: item.label, value: item.value })
  }
  const unknown = [...new Set(extraTypes.filter(type => type && !known.has(type)))]
  if (unknown.length) groups.set('其他', unknown.map(type => ({ label: type, value: type })))
  return [...groups.entries()].map(([group, children]) => ({ type: 'group', label: group, key: group, children }))
}

export const TASK_STATUS_LABELS = {
  pending: '等待执行',
  queued: '排队中',
  scheduled: '已调度',
  running: '执行中',
  completed: '已完成',
  success: '成功',
  partial_success: '部分成功',
  partial_failed: '部分失败',
  failed: '失败',
  cancelled: '已取消',
  skipped: '已跳过',
  unknown: '结果未知'
}

export const taskStatusLabel = (status) => TASK_STATUS_LABELS[status] || (status ? `未知状态（${status}）` : '-')

export const taskStatusTagType = (status) => ({
  pending: 'info',
  queued: 'info',
  scheduled: 'default',
  running: 'warning',
  completed: 'success',
  success: 'success',
  partial_success: 'warning',
  partial_failed: 'warning',
  unknown: 'warning',
  failed: 'error',
  cancelled: 'default',
  skipped: 'default'
})[status] || 'default'

/** 任务展示状态（优先 display_status，跳过的任务显示 skipped）。 */
export const effectiveTaskStatus = (task) => {
  if (!task) return ''
  if (task.metadata?.outcome === 'skipped') return 'skipped'
  return task.display_status || task.status || ''
}

/** 统计卡分组：active=排队/执行中，failed=失败，done=已完成（含部分成功）。 */
export const TASK_STATUS_GROUPS = {
  active: ['pending', 'queued', 'running'],
  failed: ['failed', 'partial_failed', 'unknown'],
  done: ['completed', 'success', 'partial_success', 'skipped']
}

export const isTaskActive = (task) => ['pending', 'queued', 'running'].includes(task?.status)

const parseTime = (value) => {
  if (!value) return NaN
  if (typeof value === 'number') return value
  return Date.parse(String(value).trim().replace(' ', 'T'))
}

/** 毫秒数 → “1 小时 3 分”“2 分 5 秒”“12 秒”。 */
export const formatDurationMs = (ms) => {
  if (!Number.isFinite(ms) || ms < 0) return '-'
  const total = Math.floor(ms / 1000)
  const hours = Math.floor(total / 3600)
  const minutes = Math.floor((total % 3600) / 60)
  const seconds = total % 60
  if (hours > 0) return `${hours} 小时 ${minutes} 分`
  if (minutes > 0) return `${minutes} 分 ${seconds} 秒`
  return `${seconds} 秒`
}

/**
 * 任务耗时：运行中为“创建至今”，已结束为“创建至最后更新”。
 * 任务记录没有单独的开始时间，排队等待也计入。
 */
export const taskDuration = (task, now = Date.now()) => {
  if (!task) return '-'
  const start = parseTime(task.start_time || task.create_time)
  if (!Number.isFinite(start)) return '-'
  const end = isTaskActive(task) ? now : parseTime(task.end_time || task.update_time)
  if (!Number.isFinite(end)) return '-'
  const text = formatDurationMs(end - start)
  return isTaskActive(task) && text !== '-' ? `${text}（进行中）` : text
}

/** 任务中心定位某任务的路由。 */
export const taskCenterRoute = (taskId) => ({ path: '/dashboard/tasks', query: taskId ? { task_id: String(taskId) } : {} })

/** 系统日志页按任务筛选的路由。 */
export const taskLogsRoute = (taskId) => ({ path: '/dashboard/system-logs', query: { q: `task=${taskId}` } })
