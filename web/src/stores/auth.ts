import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import request from '@/api/request'
import { authApi } from '@/api/auth'
import { casAuthApi } from '@/api/casConfig'

interface Permission {
  resource: string
  action: string
}

interface UserInfo {
  id: number
  username: string
  email: string
  display_name: string
  roles: string[]
  permissions: Permission[]
}

export const useAuthStore = defineStore('auth', () => {
  const token = ref<string>(localStorage.getItem('token') || '')
  const user = ref<UserInfo | null>(null)

  const isLoggedIn = computed(() => !!token.value)
  const isAdmin = computed(() => user.value?.roles?.includes('admin') ?? false)

  function hasPermission(resource: string, action: string): boolean {
    if (isAdmin.value) return true
    if (!user.value?.permissions) return false
    
    return user.value.permissions.some(
      p => p.resource === resource && p.action === action
    )
  }

  async function login(username: string, password: string) {
    const res = await authApi.login(username, password)
    const data = res as any
    
    token.value = data.access_token
    user.value = data.user
    localStorage.setItem('token', data.access_token)
    
    return data
  }

  async function casLogin(ticket: string) {
    const res = await casAuthApi.casCallback(ticket)
    const data = res as any
    if (!data || !data.access_token) {
      throw new Error(data?.message || 'CAS 登录失败')
    }
    
    token.value = data.access_token
    user.value = data.user
    localStorage.setItem('token', data.access_token)
    
    return data
  }

  async function logout() {
    try {
      await request.post('/auth/logout')
    } finally {
      token.value = ''
      user.value = null
      localStorage.removeItem('token')
    }
  }

  async function fetchProfile() {
    try {
      const res = await request.get('/auth/profile')
      user.value = res as any
    } catch {
      token.value = ''
      localStorage.removeItem('token')
    }
  }

  return {
    token,
    user,
    isLoggedIn,
    isAdmin,
    hasPermission,
    login,
    casLogin,
    logout,
    fetchProfile,
  }
})
