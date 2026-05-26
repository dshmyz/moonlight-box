import request from '@/api/request'

export interface Permission {
  resource: string
  action: string
}

export interface UserProfile {
  id: number
  username: string
  display_name?: string
  email?: string
  avatar_url?: string
  roles: string[]
  permissions: Permission[]
  is_active: boolean
  created_at: string
  last_login_at?: string
}

export interface AuthResponse {
  access_token: string
  refresh_token: string
  expires_in: number
  user: UserProfile
}

export interface ChangePasswordPayload {
  old_password: string
  new_password: string
}

export interface UpdateProfilePayload {
  display_name?: string
  email?: string
  avatar_url?: string
}

export const authApi = {
  login(username: string, password: string) {
    return request.post<AuthResponse>('/auth/login', { username, password })
  },
  logout() {
    return request.post('/auth/logout')
  },
  refreshToken(refresh_token: string) {
    return request.post<AuthResponse>('/auth/refresh', { refresh_token })
  },
  getProfile() {
    return request.get<UserProfile>('/auth/profile')
  },
  updateProfile(payload: UpdateProfilePayload) {
    return request.put<UserProfile>('/auth/profile', payload)
  },
  changePassword(payload: ChangePasswordPayload) {
    return request.put('/auth/password', payload)
  },
}
