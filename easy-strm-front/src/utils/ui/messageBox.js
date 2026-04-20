import { ElMessageBox } from 'element-plus'
import 'element-plus/es/components/message-box/style/css'

const MESSAGE_BOX_Z_INDEX = 4000

const getAppendTarget = () => {
  if (typeof document === 'undefined') {
    return undefined
  }
  return document.body
}

const buildOptions = (options = {}) => ({
  appendTo: getAppendTarget(),
  customClass: 'app-message-box',
  modalClass: 'app-message-box-overlay',
  closeOnClickModal: false,
  closeOnPressEscape: false,
  distinguishCancelAndClose: true,
  lockScroll: false,
  zIndex: MESSAGE_BOX_Z_INDEX,
  ...options
})

export const showConfirmDialog = (message, title = '提示', options = {}) => {
  return ElMessageBox.confirm(message, title, buildOptions(options))
}

export const showAlertDialog = (message, title = '提示', options = {}) => {
  return ElMessageBox.alert(message, title, buildOptions(options))
}
