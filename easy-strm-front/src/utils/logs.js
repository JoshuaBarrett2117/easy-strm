// 系统日志页的纯函数：解析单行日志、排序日志文件、格式化大小。与组件解耦以便单元测试。

// 统一单行格式：[时间][级别][req=…][task=…][module=…][trace=…] 消息
const HUMAN_LINE = /^\[([^\]]*)\]\[(DEBUG|INFO|WARN|ERROR)\]\[req=([^\]]*)\]\[task=([^\]]*)\]\[module=([^\]]*)\]\[trace=([^\]]*)\]\s?(.*)$/

/** 解析一行日志；支持人类可读格式与 JSON 格式，无法解析时 parsed=false 原样返回。 */
export const parseLogLine = (raw) => {
  const match = HUMAN_LINE.exec(raw)
  if (match) {
    return { parsed: true, raw, time: match[1], level: match[2], req: match[3], task: match[4], module: match[5], message: match[7] }
  }
  if (raw.startsWith('{')) {
    try {
      const record = JSON.parse(raw)
      const extra = [
        record.fields && Object.keys(record.fields).length ? JSON.stringify(record.fields) : '',
        record.error_chain && record.error_chain.length ? `error_chain=${JSON.stringify(record.error_chain)}` : ''
      ].filter(Boolean).join(' ')
      return {
        parsed: true, raw, time: record.timestamp, level: record.level || 'INFO',
        req: record.request_id || '-', task: record.task_id || '-', module: record.module || '-',
        message: `${record.message || ''}${extra ? ' ' + extra : ''}`
      }
    } catch (error) {
      // 非 JSON，按原文显示
    }
  }
  return { parsed: false, raw }
}

/** 按日期倒序，同一天 debug（完整时间线）排在 info 前面；返回新数组。 */
export const sortLogFiles = (files) => [...files].sort((a, b) => {
  const dateA = a.name.replace(/^(info|debug)_/, '')
  const dateB = b.name.replace(/^(info|debug)_/, '')
  if (dateA !== dateB) return dateB.localeCompare(dateA)
  if (a.type === b.type) return 0
  return a.type === 'debug' ? -1 : 1
})

export const formatFileSize = (bytes) => {
  if (!bytes) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB']
  const level = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1)
  return `${(bytes / (1024 ** level)).toFixed(level === 0 ? 0 : 2)} ${units[level]}`
}

/** 过长的请求 ID 只显示首尾，完整值仍用于筛选。 */
export const shortLogId = (id) => (id.length > 13 ? `${id.slice(0, 8)}…${id.slice(-4)}` : id)
