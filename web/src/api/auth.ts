import request from '@/api/request'

export const authApi = {
  login(username: string, password: string) {
    return request.post('/auth/login', { username, password })
  },
  getCASConfig() {
    return request.get('/auth/cas/config')
  },
  casCallback(ticket: string) {
    return request.get('/auth/cas/callback', { params: { ticket } })
  },
}
