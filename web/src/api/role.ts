import request from './request'

export interface Permission {
  id: number
  resource: string
  action: string
}

export interface Role {
  id: number
  name: string
  description: string
  is_system_role: boolean
  permissions: Permission[]
  created_at: string
}

export interface RoleListResponse {
  items: Role[]
  pagination: {
    total: number
    page: number
    page_size: number
  }
}

export interface CreateRoleRequest {
  name: string
  description?: string
}

export interface UpdateRoleRequest {
  description?: string
}

export interface UpdatePermissionsRequest {
  permission_ids: number[]
}

export const roleApi = {
  list() {
    return request.get<Role[]>('/roles')
  },

  get(id: number) {
    return request.get<Role>(`/roles/${id}`)
  },

  create(data: CreateRoleRequest) {
    return request.post<Role>('/roles', data)
  },

  update(id: number, data: UpdateRoleRequest) {
    return request.put<Role>(`/roles/${id}`, data)
  },

  delete(id: number) {
    return request.delete(`/roles/${id}`)
  },

  listPermissions() {
    return request.get<Permission[]>('/roles/permissions')
  },

  updatePermissions(id: number, data: UpdatePermissionsRequest) {
    return request.put<Role>(`/roles/${id}/permissions`, data)
  },
}
