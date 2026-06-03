import request from './request'

export interface UserRole {
  id: number
  name: string
}

export interface UserItem {
  id: number
  username: string
  display_name?: string
  email?: string
  is_active: boolean
  roles: UserRole[]
  created_at: string
  last_login_at?: string
}

export interface UserListResponse {
  items: UserItem[]
  pagination: {
    page: number
    page_size: number
    total: number
  }
}

export interface CreateUserPayload {
  username: string
  password: string
  display_name?: string
  email?: string
}

export interface RoleItem {
  id: number
  name: string
  description?: string
}

export const userApi = {
  list(params?: Record<string, unknown>) {
    return request.get<UserListResponse>('/users', { params })
  },

  create(data: CreateUserPayload) {
    return request.post<UserItem>('/users', data)
  },

  getRoles() {
    return request.get<RoleItem[]>('/roles')
  },

  assignRoles(userId: number, roleIds: number[]) {
    return request.put(`/users/${userId}/roles`, { role_ids: roleIds })
  },

  toggleStatus(userId: number, isActive: boolean) {
    return request.put(`/users/${userId}/status`, { is_active: isActive })
  },

  resetPassword(userId: number, password: string) {
    return request.put(`/users/${userId}/password`, { password })
  },

  delete(userId: number) {
    return request.delete(`/users/${userId}`)
  },
}