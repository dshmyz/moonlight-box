import { ref, reactive, computed, onMounted, onUnmounted } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { packageApi, type Package } from '@/api/package'

export type PackageSort = 'updated_at' | 'download_count' | 'name'
export type PackageSource = 'all' | 'local' | 'proxy'

// 与 packageApi.search 参数类型保持一致
interface SearchParams {
  q?: string
  type?: string
  name?: string
  version?: string
  repository?: string
  sort?: string
  page?: number
  page_size?: number
}

export interface PackageQuery {
  q: string
  type: string
  repository: string
  version: string
  source: PackageSource
  sort: PackageSort
  page: number
  pageSize: number
}

export interface UsePackageSearchOptions {
  mode: 'admin' | 'public'
  initialQuery?: Partial<PackageQuery>
  syncUrl?: boolean
  pageSizeOptions?: number[]
  defaultPageSize?: number
  recentSearchKey?: string
}

function normalizePackage(pkg: any): Package {
  return {
    ...pkg,
    type: pkg.type || pkg.package_type || pkg.format || 'generic',
  }
}

export function usePackageSearch(options: UsePackageSearchOptions) {
  const router = useRouter()
  const route = useRoute()

  const defaultPageSize = options.defaultPageSize ?? 20
  const syncUrl = options.syncUrl ?? true

  const query = reactive<PackageQuery>({
    q: options.initialQuery?.q ?? '',
    type: options.initialQuery?.type ?? 'all',
    repository: options.initialQuery?.repository ?? '',
    version: options.initialQuery?.version ?? '',
    source: options.initialQuery?.source ?? 'all',
    sort: options.initialQuery?.sort ?? 'updated_at',
    page: options.initialQuery?.page ?? 1,
    pageSize: defaultPageSize,
  })

  const packages = ref<Package[]>([])
  const total = ref(0)
  const loading = ref(false)
  const error = ref<string | null>(null)
  const searchTime = ref(0)
  const recentSearches = ref<string[]>([])

  const isEmpty = computed(() => !loading.value && packages.value.length === 0)
  // source 筛选暂未启用（后端不支持），保留字段供未来扩展
  const hasActiveFilter = computed(() =>
    !!query.repository || !!query.version || query.source !== 'all'
  )

  function buildApiParams(): SearchParams {
    const params: SearchParams = {
      q: query.q,
      page: query.page,
      page_size: query.pageSize,
      sort: query.sort,
    }
    if (query.type !== 'all') params.type = query.type
    if (query.repository) params.repository = query.repository
    if (query.version) params.version = query.version
    return params
  }

  async function search() {
    loading.value = true
    error.value = null
    try {
      const res = await packageApi.search(buildApiParams())
      packages.value = (res.list || []).map(normalizePackage)
      total.value = res.total || 0
      searchTime.value = res.search_time_ms || 0
    } catch (e) {
      error.value = e instanceof Error ? e.message : '加载失败'
      packages.value = []
      total.value = 0
    } finally {
      loading.value = false
    }
  }

  async function refresh() {
    await search()
    // 删除后页码修正：当前页无数据时连续回退，直到有数据或到第 1 页
    while (packages.value.length === 0 && query.page > 1) {
      query.page--
      await search()
    }
  }

  function readFromUrl() {
    const q = route.query
    if (q.q) query.q = String(q.q)
    if (q.type) query.type = String(q.type)
    if (q.sort) query.sort = String(q.sort) as PackageSort
    if (q.page) query.page = parseInt(String(q.page)) || 1
    if (q.page_size) query.pageSize = parseInt(String(q.page_size)) || defaultPageSize
    if (q.repo) query.repository = String(q.repo)
    if (q.version) query.version = String(q.version)
  }

  function syncToUrl(changed: Partial<PackageQuery>) {
    if (!syncUrl) return
    const next: Record<string, string> = {}
    if (query.q) next.q = query.q
    if (query.type !== 'all') next.type = query.type
    if (query.sort !== 'updated_at') next.sort = query.sort
    if (query.page !== 1) next.page = String(query.page)
    if (query.pageSize !== defaultPageSize) next.page_size = String(query.pageSize)
    if (query.repository) next.repo = query.repository
    if (query.version) next.version = query.version

    const isPageChange = 'page' in changed
    if (isPageChange) {
      router.push({ query: next })
    } else {
      router.replace({ query: next })
    }
  }

  function setQuery(patch: Partial<PackageQuery>) {
    const isPageChange = 'page' in patch
    Object.assign(query, patch)
    if (!isPageChange && query.page !== 1) {
      query.page = 1
    }
    syncToUrl(patch)
  }

  function resetFilters() {
    query.repository = ''
    query.version = ''
    query.source = 'all'
    query.page = 1
    syncToUrl({})
  }

  // 最近搜索
  const recentKey = options.recentSearchKey ?? `package-explorer:recent:${options.mode}`

  function loadRecentSearches(): string[] {
    try {
      const stored = localStorage.getItem(recentKey)
      return stored ? JSON.parse(stored) : []
    } catch {
      return []
    }
  }

  function saveRecentSearches(list: string[]) {
    try {
      localStorage.setItem(recentKey, JSON.stringify(list))
    } catch {
      // localStorage 不可用时静默失败
    }
  }

  function addRecentSearch(term: string) {
    const trimmed = term.trim()
    if (!trimmed) return
    const next = [trimmed, ...recentSearches.value.filter(s => s !== trimmed)].slice(0, 5)
    recentSearches.value = next
    saveRecentSearches(next)
  }

  function clearRecentSearches() {
    recentSearches.value = []
    saveRecentSearches([])
  }

  // 键盘快捷键
  // 全局快捷键：本 composable 设计为单实例使用（PackageExplorer 唯一调用方）
  // 多实例同时挂载会导致快捷键重复触发
  function handleKeydown(e: KeyboardEvent) {
    const target = e.target as HTMLElement
    const isInputFocused = target.tagName === 'INPUT' || target.tagName === 'TEXTAREA'

    if (e.key === 'Escape') {
      if (query.q) {
        setQuery({ q: '' })
        search()
      }
      return
    }

    if (e.key === '/' && !isInputFocused) {
      e.preventDefault()
      const input = document.querySelector<HTMLInputElement>('.package-search-bar input')
      input?.focus()
    }
  }

  onMounted(() => {
    readFromUrl()
    recentSearches.value = loadRecentSearches()
    document.addEventListener('keydown', handleKeydown)
  })

  onUnmounted(() => {
    document.removeEventListener('keydown', handleKeydown)
  })

  return {
    query,
    packages,
    total,
    loading,
    error,
    searchTime,
    recentSearches,
    isEmpty,
    hasActiveFilter,
    search,
    refresh,
    resetFilters,
    setQuery,
    addRecentSearch,
    clearRecentSearches,
  }
}
