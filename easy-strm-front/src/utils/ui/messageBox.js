import { dialog } from './feedback'

/**
 * 确认/提示对话框(基于 Naive UI 离散 dialog API)
 * 保持与旧版 ElMessageBox 封装相同的 Promise 语义:
 * 确认 -> resolve,取消/关闭 -> reject
 */

const TYPE_MAP = {
  warning: 'warning',
  error: 'error',
  success: 'success',
  info: 'info'
}

export const showConfirmDialog = (message, title = '提示', options = {}) => {
  const type = TYPE_MAP[options.type] || 'warning'
  return new Promise((resolve, reject) => {
    dialog[type]({
      title,
      content: message,
      positiveText: options.confirmButtonText || '确定',
      negativeText: options.cancelButtonText || '取消',
      maskClosable: false,
      onPositiveClick: () => resolve('confirm'),
      onNegativeClick: () => reject('cancel'),
      onClose: () => reject('close')
    })
  })
}

export const showAlertDialog = (message, title = '提示', options = {}) => {
  const type = TYPE_MAP[options.type] || 'info'
  return new Promise((resolve) => {
    dialog[type]({
      title,
      content: message,
      positiveText: options.confirmButtonText || '确定',
      maskClosable: false,
      onPositiveClick: () => resolve('confirm'),
      onClose: () => resolve('close')
    })
  })
}
