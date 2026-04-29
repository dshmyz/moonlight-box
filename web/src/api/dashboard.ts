import request from './request'

/** 仓库状态信息接口 */
export interface RepoStatus {
  /** 仓库名称 */
  name: string
  /** 仓库类型 */
  type: string
  /** 包类型 */
  package_type: string
  /** 运行状态 */
  status: string
  /** 包数量 */
  package_count: number
  /** 当日下载量 */
  download_count_today: number
  /** 存储占用（字节） */
  storage_bytes: number
}

/** 存储信息接口 */
export interface StorageInfo {
  /** 总存储空间（字节） */
  total_bytes: number
  /** 已使用空间（字节） */
  used_bytes: number
  /** 使用百分比 */
  usage_percent: number
}

/** 缓存信息接口 */
export interface CacheInfo {
  /** 缓存命中率 */
  hit_rate: number
  /** 缓存总条目数 */
  total_entries: number
}

/** 热门包统计信息接口 */
export interface PackageTop {
  /** 包名称 */
  name: string
  /** 包类型 */
  type: string
  /** 下载次数 */
  download_count: number
  /** 描述 */
  description?: string
  /** 许可证 */
  license?: string
}

/** 仪表盘统计数据接口 */
export interface DashboardStats {
  /** 仓库列表 */
  repositories: RepoStatus[]
  /** 存储信息 */
  storage: StorageInfo
  /** 缓存信息 */
  cache: CacheInfo
  /** 近 7 天下载量数组 */
  downloads_last_7_days: number[]
  /** 热门包 Top 5 */
  top_packages: PackageTop[]
}

/** 仪表盘相关 API */
export const dashboardApi = {
  /**
   * 获取仪表盘统计数据
   */
  getStats() {
    return request.get<DashboardStats>('/dashboard/stats')
  },
}
