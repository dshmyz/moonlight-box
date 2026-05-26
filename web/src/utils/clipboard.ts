import { ElMessage } from 'element-plus'

/**
 * 复制文本到剪贴板，带有降级方案
 * 优先使用 navigator.clipboard，失败时使用 execCommand 降级
 */
export async function copyToClipboard(text: string, successMsg = '已复制到剪贴板'): Promise<boolean> {
  if (!text) {
    ElMessage.warning('复制内容为空')
    return false
  }

  // 优先使用现代 Clipboard API
  if (navigator.clipboard && window.isSecureContext) {
    try {
      await navigator.clipboard.writeText(text)
      ElMessage.success(successMsg)
      return true
    } catch {
      // 降级到 execCommand
    }
  }

  // 降级方案：使用 textarea + execCommand
  try {
    const textarea = document.createElement('textarea')
    textarea.value = text
    textarea.style.position = 'fixed'
    textarea.style.left = '-9999px'
    textarea.style.top = '-9999px'
    document.body.appendChild(textarea)
    textarea.focus()
    textarea.select()
    const success = document.execCommand('copy')
    document.body.removeChild(textarea)
    if (success) {
      ElMessage.success(successMsg)
      return true
    }
  } catch {
    // execCommand 也失败了
  }

  ElMessage.error('复制失败')
  return false
}
