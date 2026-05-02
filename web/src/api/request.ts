import axios, { type AxiosResponse } from 'axios'
import { ElMessage } from 'element-plus'
import router from '@/router'

const request = axios.create({
  baseURL: '/api/v1',
  timeout: 30000,
})

request.interceptors.request.use(
  (config) => {
    const token = localStorage.getItem('token')
    if (token) {
      config.headers.Authorization = `Bearer ${token}`
    }
    return config
  },
  (error) => Promise.reject(error)
)

request.interceptors.response.use(
  (response) => {
    const res = response.data
    if (res.code !== undefined && res.code !== 200 && res.code !== 201) {
      ElMessage.error(res.message || '请求失败')
      return Promise.reject(new Error(res.message))
    }
    return res.data as unknown as AxiosResponse
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

export default request as unknown as {
  get<T = unknown>(url: string, config?: object): Promise<T>
  post<T = unknown>(url: string, data?: unknown, config?: object): Promise<T>
  put<T = unknown>(url: string, data?: unknown, config?: object): Promise<T>
  delete<T = unknown>(url: string, config?: object): Promise<T>
}
