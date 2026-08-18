import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { authApi, type UserProfile, type Permission, type AuthResponse } from '@/api/auth'
import { casAuthApi } from '@/api/casConfig'

export type { UserProfile, Permission }

export const useAuthStore = defineStore('auth', () => {
  const token = ref<string>(localStorage.getItem('token') || '')
  const user = ref<UserProfile | null>(null)

  const isLoggedIn = computed(() => !!token.value)
  const isAdmin = computed(() => user.value?.roles?.includes('admin') ?? false)

  function hasPermission(resource: string, action: string): boolean {
    if (isAdmin.value) return true
    if (!user.value?.permissions) return false
    
    return user.value.permissions.some(
      p => p.resource === resource && p.action === action
    )
  }

  async function login(username: string, password: string): Promise<AuthResponse> {
    const data = await authApi.login(username, password)
    
    token.value = data.access_token
    user.value = data.user
    localStorage.setItem('token', data.access_token)
    
    return data
  }

  async function casLogin(ticket: string, redirect?: string): Promise<AuthResponse> {
    const data = await casAuthApi.casCallback(ticket, redirect)
    if (!data || !data.access_token) {
      throw new Error('CAS 登录失败')
    }
    
    token.value = data.access_token
    user.value = data.user
    localStorage.setItem('token', data.access_token)
    
    return data
  }

  async function logout() {
    try {
      await authApi.logout()
    } finally {
      token.value = ''
      user.value = null
      localStorage.removeItem('token')
    }
  }

  async function fetchProfile() {
    try {
      const data = await authApi.getProfile()
      user.value = data
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
