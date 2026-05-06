import request from '@/api/request'

export interface UserProfile {
  username: string
  display_name?: string
  email?: string
  roles?: string[]
}

export interface ChangePasswordPayload {
  old_password: string
  new_password: string
}

export interface UpdateProfilePayload {
  display_name?: string
  email?: string
}

export const authApi = {
  login(username: string, password: string) {
    return request.post<{ access_token: string }>('/auth/login', { username, password })
  },
  logout() {
    return request.post('/auth/logout')
  },
  refreshToken() {
    return request.post('/auth/refresh')
  },
  getProfile() {
    return request.get<UserProfile>('/auth/profile')
  },
  updateProfile(payload: UpdateProfilePayload) {
    return request.put<UserProfile>('/auth/profile', payload)
  },
  changePassword(payload: ChangePasswordPayload) {
    return request.post('/auth/password', payload)
  },
}
