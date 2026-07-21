import { ElMessageBox, ElMessage } from 'element-plus'

export interface ConfirmOptions {
  title?: string
  message: string
  type?: 'info' | 'warning' | 'error' | 'success'
  confirmText?: string
  cancelText?: string
}

export async function confirm(options: ConfirmOptions): Promise<boolean> {
  const {
    title = '提示',
    message,
    type = 'warning',
    confirmText = '确定',
    cancelText = '取消',
  } = options

  try {
    await ElMessageBox.confirm(message, title, {
      confirmButtonText: confirmText,
      cancelButtonText: cancelText,
      type,
    })
    return true
  } catch {
    return false
  }
}

export function success(message: string) {
  ElMessage.success(message)
}

export function error(message: string) {
  ElMessage.error(message)
}
