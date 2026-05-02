import request from './request'

export const fileApi = {
  browse(path: string = '/') {
    return request.get('/files/browse', {
      params: { path }
    })
  },

  stats(path: string) {
    return request.get('/files/stats', {
      params: { path }
    })
  },

  download(path: string) {
    return request.get('/files/download', {
      params: { path },
      responseType: 'blob'
    })
  }
}
