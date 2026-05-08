import axios, { type AxiosInstance, type AxiosResponse, type InternalAxiosRequestConfig } from 'axios'
import { ElMessage } from 'element-plus'
import router from '@/router'

interface ApiResponse<T = unknown> {
  code: number
  message: string
  data: T
}

const request: AxiosInstance = axios.create({
  baseURL: '/api/v1',
  timeout: 30000,
})

request.interceptors.request.use(
  (config: InternalAxiosRequestConfig) => {
    const token = localStorage.getItem('token')
    if (token) {
      config.headers.Authorization = `Bearer ${token}`
    }
    return config
  },
  (error) => Promise.reject(error)
)

request.interceptors.response.use(
  (response: AxiosResponse<ApiResponse>) => {
    const res = response.data
    if (res.code !== undefined && res.code !== 200 && res.code !== 201) {
      ElMessage.error(res.message || '请求失败')
      return Promise.reject(new Error(res.message))
    }
    return response
  },
  (error) => {
    if (error.response?.status === 401) {
      localStorage.removeItem('token')
      if (window.location.pathname.startsWith('/admin')) {
        router.push('/login')
        ElMessage.error('登录已过期，请重新登录')
      }
    } else if (error.response?.status === 409) {
      ElMessage.error(error.response?.data?.message || '资源已存在')
    } else if (error.response?.status !== 404) {
      ElMessage.error(error.response?.data?.message || '网络错误')
    }
    return Promise.reject(error)
  }
)

interface RequestWrapper {
  get<T = unknown>(url: string, config?: object): Promise<T>
  post<T = unknown>(url: string, data?: unknown, config?: object): Promise<T>
  put<T = unknown>(url: string, data?: unknown, config?: object): Promise<T>
  delete<T = unknown>(url: string, config?: object): Promise<T>
}

const api: RequestWrapper = {
  get<T>(url: string, config?: object): Promise<T> {
    return request.get<ApiResponse<T>>(url, config).then((res) => res.data.data)
  },
  post<T>(url: string, data?: unknown, config?: object): Promise<T> {
    return request.post<ApiResponse<T>>(url, data, config).then((res) => res.data.data)
  },
  put<T>(url: string, data?: unknown, config?: object): Promise<T> {
    return request.put<ApiResponse<T>>(url, data, config).then((res) => res.data.data)
  },
  delete<T>(url: string, config?: object): Promise<T> {
    return request.delete<ApiResponse<T>>(url, config).then((res) => res.data.data)
  },
}

export default api
