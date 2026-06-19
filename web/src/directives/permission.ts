import type { Directive } from 'vue'
import { useAuthStore } from '@/stores/auth'

/**
 * v-permission 指令：根据当前用户权限控制元素显隐
 * 用法：v-permission="'resource:action'"
 * 无权限时从 DOM 移除元素（非 display:none，避免被绕过）
 */
export const permission: Directive<HTMLElement, string> = {
  mounted(el, binding) {
    const perm = binding.value
    if (!perm || typeof perm !== 'string') return

    const sep = perm.indexOf(':')
    if (sep <= 0) return

    const resource = perm.slice(0, sep)
    const action = perm.slice(sep + 1)

    const store = useAuthStore()
    if (!store.hasPermission(resource, action)) {
      el.parentNode?.removeChild(el)
    }
  },
}
