import request from '@/api/request'

export const casAuthApi = {
  getCASConfig() {
    return request.get('/auth/cas/config')
  },
  casCallback(ticket: string) {
    return request.get('/auth/cas/callback', { params: { ticket } })
  },
}
