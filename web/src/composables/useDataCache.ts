import { shallowRef, ref, type Ref } from 'vue'

interface CacheEntry<T> {
  data: T
  timestamp: number
}

export function useDataCache<T>(
  fetcher: () => Promise<T>,
  ttlMs: number = 5 * 60 * 1000
) {
  const cache = shallowRef<CacheEntry<T> | null>(null)
  const loading: Ref<boolean> = ref(false)
  const error: Ref<string | null> = ref(null)

  function isExpired(): boolean {
    if (!cache.value) return true
    return Date.now() - cache.value.timestamp > ttlMs
  }

  async function fetch(force = false): Promise<T | null> {
    if (!force && cache.value && !isExpired()) {
      return cache.value.data
    }

    loading.value = true
    error.value = null

    try {
      const data = await fetcher()
      cache.value = { data, timestamp: Date.now() }
      return data
    } catch (err) {
      error.value = err instanceof Error ? err.message : '加载失败'
      return null
    } finally {
      loading.value = false
    }
  }

  function invalidate() {
    cache.value = null
  }

  function getData(): T | null {
    return cache.value?.data ?? null
  }

  return {
    cache,
    loading,
    error,
    fetch,
    invalidate,
    getData,
    isExpired,
  }
}