export const maskSensitive = (text) => {
  if (!text) return '-'
  if (text.length <= 20) return '******'
  return `${text.substring(0, 8)}...${text.substring(text.length - 8)}`
}

export const accountTypeMap = {
  resource: { name: '资源号', tag: 'primary' },
  vip: { name: 'VIP观影号', tag: 'success' },
  both: { name: '兼顾', tag: 'warning' }
}

export const statusMap = {
  active: { name: '正常', tag: 'success' },
  cooling: { name: '冷却中', tag: 'warning' },
  disabled: { name: '禁用', tag: 'danger' }
}

export const transferMethodMap = {
  '115driver': { name: '115driver', tag: 'primary' },
  go115: { name: 'go115', tag: 'success' },
  alist: { name: 'alist', tag: 'warning' }
}

export const channelTips = {
  web: '请使用 115 网页版扫描二维码登录',
  android: '请使用 115 安卓 APP 扫描二维码登录',
  ios: '请使用 115 iOS APP 扫描二维码登录',
  tv: '请使用 115 电视版扫描二维码登录',
  alipaymini: '请使用支付宝小程序扫描二维码登录',
  wechatmini: '请使用微信小程序扫描二维码登录',
  qandroid: '请使用 115 安卓 Q 版扫描二维码登录'
}

export const loginStatusMap = {
  0: { text: '等待扫码', type: 'info' },
  1: { text: '已扫码，等待确认', type: 'warning' },
  2: { text: '登录成功', type: 'success' },
  3: { text: '二维码已过期', type: 'danger' },
  4: { text: '登录失败', type: 'danger' }
}

export const getAccountTypeName = (type) => accountTypeMap[type]?.name || '资源号'

export const getAccountTypeTag = (type) => accountTypeMap[type]?.tag || 'primary'

export const getStatusName = (status) => statusMap[status]?.name || '正常'

export const getStatusTag = (status) => statusMap[status]?.tag || 'success'

export const getPriorityTag = (priority) => {
  if (priority >= 8) return 'danger'
  if (priority >= 6) return 'warning'
  if (priority >= 4) return 'info'
  return 'success'
}

export const getTransferMethodName = (method) => {
  if (!method || method === '') return ''
  return transferMethodMap[method]?.name || ''
}

export const getTransferMethodTag = (method) => {
  if (!method || method === '') return 'info'
  return transferMethodMap[method]?.tag || 'info'
}

export const isDialogCancelAction = (error) => {
  return error === 'cancel' || error === 'close' || error?.message === 'cancel' || error?.message === 'close'
}

export const formatExpireTime = (seconds) => {
  if (seconds <= 0) return '已过期'
  const minutes = Math.floor(seconds / 60)
  const secs = seconds % 60
  return `${minutes}分 ${secs}秒`
}

export const sortChannelsWithWechatFirst = (channels) => {
  return [...channels].sort((a, b) => {
    if (a.value === 'wechatmini') return -1
    if (b.value === 'wechatmini') return 1
    return 0
  })
}
