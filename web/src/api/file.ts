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

export interface StorageBackendOption {
  id: number
  name: string
  type: string
  is_default: boolean
}

export const fileApi = {
  getBackends() {
    return request.get<StorageBackendOption[]>('/files/backends')
  },

  browse(path: string = '/', backendId: number = 0) {
    return request.get<BrowseResponse>('/files/browse', {
      params: { path, backend_id: backendId || undefined }
    })
  },

  stats(path: string, backendId: number = 0) {
    return request.get('/files/stats', {
      params: { path, backend_id: backendId || undefined }
    })
  },

  download(path: string, backendId: number = 0) {
    return request.get<DownloadResponse>('/files/download', {
      params: { path, backend_id: backendId || undefined },
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
