import request from '@/api/request'

export const authApi = {
  login(username: string, password: string) {
    return request.post('/auth/login', { username, password })
  },
  logout() {
    return request.post('/auth/logout')
  },
  refreshToken() {
    return request.post('/auth/refresh')
  },
  getProfile() {
    return request.get('/auth/profile')
  },
}
