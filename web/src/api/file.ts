import request from './request'
import axios from 'axios'

export interface BrowseResponse {
  files: Array<{
    name: string
    path: string
    is_dir: boolean
    size: number
    mod_time: string
  }>
}

export interface DownloadResponse {
  data: Blob
}

export const fileApi = {
  browse(path: string = '/') {
    return request.get<BrowseResponse>('/files/browse', {
      params: { path }
    })
  },

  stats(path: string) {
    return request.get('/files/stats', {
      params: { path }
    })
  },

  download(path: string) {
    return request.get<DownloadResponse>('/files/download', {
      params: { path },
      responseType: 'blob'
    })
  },

  upload(file: File, path?: string, onProgress?: (percent: number) => void) {
    const formData = new FormData()
    formData.append('file', file)
    if (path) {
      formData.append('path', path)
    }
    const token = localStorage.getItem('token')

    return axios.post('/files/upload', formData, {
      headers: {
        'Content-Type': 'multipart/form-data',
        ...(token ? { 'Authorization': `Bearer ${token}` } : {})
      },
      onUploadProgress: (progressEvent) => {
        if (onProgress && progressEvent.total) {
          const percent = Math.round((progressEvent.loaded * 100) / progressEvent.total)
          onProgress(percent)
        }
      }
    })
  }
}
