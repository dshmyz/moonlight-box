<template>
  <div class="system-config">
    <header class="list-header">
      <div class="header-content">
        <div class="header-icon">
          <i class="fa-solid fa-gears"></i>
        </div>
        <div class="header-text">
          <h2>系统配置</h2>
          <p class="header-subtitle">配置系统各项参数和功能</p>
        </div>
      </div>
      <div class="header-actions" v-if="activeSection === 'params'">
        <el-input
          v-model="search"
          placeholder="搜索配置项 / key"
          clearable
          class="search-input"
        >
          <template #prefix>
            <i class="fa-solid fa-magnifying-glass search-icon"></i>
          </template>
        </el-input>
        <el-button
          type="primary"
          class="save-btn"
          :loading="saving"
          :disabled="!dirtyKeys.length"
          @click="saveConfigs"
        >
          <i class="fa-solid fa-check"></i>
          <span>保存{{ dirtyKeys.length ? ` (${dirtyKeys.length})` : '' }}</span>
        </el-button>
      </div>
    </header>

    <div class="content-panel" v-loading="loading && activeSection === 'params'">
      <el-tabs v-model="activeSection" class="section-tabs">
        <el-tab-pane label="系统参数" name="params">
          <div class="config-hint">
            <i class="fa-solid fa-circle-info"></i>
            <span>此处只保留运行时生效的系统参数。备份、调度与 Maven SNAPSHOT 参数、日志清理分别在「备份管理」「定时任务」「下载日志」页维护；CAS 登录见上方「CAS 登录」标签页。</span>
          </div>
          <div class="config-layout">
        <nav v-if="!isSearching" class="config-nav">
          <button
            v-for="category in categories"
            :key="category.name"
            class="nav-item"
            :class="{ active: category.name === activeCategory }"
            @click="activeCategory = category.name"
          >
            <span class="nav-label">{{ category.label }}</span>
            <span class="nav-count">{{ category.items.length }}</span>
          </button>
        </nav>

        <main class="config-main">
          <p v-if="!displayGroups.length" class="empty-tip">
            {{ isSearching ? '没有匹配的配置项' : '该分类下暂无配置项' }}
          </p>

          <section v-for="group in displayGroups" :key="group.key" class="config-group">
            <h4 class="group-title">{{ group.label }}</h4>

            <div
              v-for="config in group.items"
              :key="config.key"
              class="config-item"
              :class="{ dirty: isDirty(config.key) }"
            >
              <div class="item-info">
                <div class="item-label">
                  {{ config.description || config.key }}
                  <span v-if="isDirty(config.key)" class="dirty-flag">未保存</span>
                </div>
                <div class="item-key">{{ config.key }}</div>
              </div>

              <div class="item-control">
                <el-switch
                  v-if="isBool(config)"
                  v-model="configValues[config.key]"
                />
                <el-input-number
                  v-else-if="isInt(config)"
                  v-model="configValues[config.key]"
                  :min="0"
                  controls-position="right"
                  class="number-input"
                />
                <el-input
                  v-else-if="config.value_type === 'json'"
                  v-model="configValues[config.key]"
                  type="textarea"
                  :rows="5"
                  placeholder="请输入 JSON 格式配置"
                  class="json-input"
                />
                <el-input
                  v-else
                  v-model="configValues[config.key]"
                  :placeholder="placeholderFor(config.key)"
                  class="text-input"
                />
              </div>
            </div>
          </section>
        </main>
          </div>
        </el-tab-pane>
        <el-tab-pane label="CAS 登录" name="cas" lazy>
          <CasSettingsPanel />
        </el-tab-pane>
      </el-tabs>
    </div>

    <div v-if="activeSection === 'params' && dirtyKeys.length" class="dirty-bar">
      <span class="dirty-summary">{{ dirtyKeys.length }} 项修改尚未保存</span>
      <div class="dirty-actions">
        <el-button size="small" @click="resetDirty">撤销修改</el-button>
        <el-button size="small" type="primary" :loading="saving" @click="saveConfigs">
          保存配置
        </el-button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted } from 'vue'
import { onBeforeRouteLeave, useRoute } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { systemApi, type SystemConfig } from '@/api/system'
import CasSettingsPanel from '@/components/settings/CasSettingsPanel.vue'

const CATEGORY_ORDER = [
  'general',
  'network',
  'cache',
  'storage',
  'security',
  'auth',
  'log',
  'cleanup',
  'other',
]

const CATEGORY_LABELS: Record<string, string> = {
  general: '通用',
  network: '网络与健康检查',
  cache: '缓存',
  storage: '存储',
  security: '安全扫描',
  auth: '认证与会话',
  log: '日志',
  cleanup: '数据清理',
  other: '其他',
}

// 同一分类内按 key 前缀再分组，避免通用配置里平铺几十项
const GROUP_LABELS: Record<string, string> = {
  system: '基础信息',
  metrics: '监控',
  webhook: 'Webhook 通知',
  health_check: '健康检查',
  cache: '代理缓存',
  storage: '存储',
  security: '漏洞扫描与阻断',
  auth: '认证与会话',
  log_cleanup: '日志清理',
  maven_snapshot_cleanup: 'Maven SNAPSHOT 清理',
}

// 时长类配置的值必须是 Go duration（如 24h、30m、10s），写错会让配置静默回退默认值
const DURATION_KEY_PATTERN = /(ttl|expiry|timeout)$/
const DURATION_VALUE_PATTERN = /^\d+(\.\d+)?(ns|us|µs|ms|s|m|h)$/

const loading = ref(false)
const saving = ref(false)
const search = ref('')
const configs = ref<SystemConfig[]>([])
const configValues = reactive<Record<string, any>>({})
const originalValues = reactive<Record<string, any>>({})
const activeCategory = ref('')

// 旧的「CAS 设置」入口重定向到这里时，直接打开 CAS 标签页
const route = useRoute()
const activeSection = ref(route.query.tab === 'cas' ? 'cas' : 'params')

const isBool = (config: SystemConfig) =>
  config.value_type === 'bool' || config.value_type === 'boolean'
const isInt = (config: SystemConfig) =>
  config.value_type === 'int' || config.value_type === 'number'
const isSearching = computed(() => search.value.trim() !== '')

const prefixOf = (key: string) => key.split('.')[0]
const groupLabelOf = (prefix: string) => GROUP_LABELS[prefix] || prefix

const categories = computed(() => {
  const grouped: Record<string, SystemConfig[]> = {}
  configs.value.forEach((config) => {
    const category = config.category || 'other'
    ;(grouped[category] ||= []).push(config)
  })

  return Object.keys(grouped)
    .sort((a, b) => {
      const ai = CATEGORY_ORDER.indexOf(a)
      const bi = CATEGORY_ORDER.indexOf(b)
      return (ai === -1 ? CATEGORY_ORDER.length : ai) - (bi === -1 ? CATEGORY_ORDER.length : bi)
    })
    .map((name) => ({
      name,
      label: CATEGORY_LABELS[name] || name,
      items: grouped[name],
    }))
})

interface ConfigGroup {
  key: string
  label: string
  items: SystemConfig[]
}

const groupByPrefix = (items: SystemConfig[]): ConfigGroup[] => {
  const groups = new Map<string, ConfigGroup>()
  items.forEach((config) => {
    const prefix = prefixOf(config.key)
    if (!groups.has(prefix)) {
      groups.set(prefix, { key: prefix, label: groupLabelOf(prefix), items: [] })
    }
    groups.get(prefix)!.items.push(config)
  })
  return Array.from(groups.values())
}

const searchGroups = computed<ConfigGroup[]>(() => {
  const keyword = search.value.trim().toLowerCase()
  if (!keyword) return []
  const matched = configs.value.filter(
    (config) =>
      config.key.toLowerCase().includes(keyword) ||
      (config.description || '').toLowerCase().includes(keyword),
  )
  // 搜索时按分类聚合并保留分类名，避免用户不知道匹配项属于哪个模块
  const byCategory = new Map<string, SystemConfig[]>()
  matched.forEach((config) => {
    const category = config.category || 'other'
    if (!byCategory.has(category)) byCategory.set(category, [])
    byCategory.get(category)!.push(config)
  })
  return Array.from(byCategory.entries()).map(([category, items]) => ({
    key: category,
    label: CATEGORY_LABELS[category] || category,
    items,
  }))
})

const displayGroups = computed<ConfigGroup[]>(() => {
  if (isSearching.value) return searchGroups.value
  const category = categories.value.find((item) => item.name === activeCategory.value)
  return category ? groupByPrefix(category.items) : []
})

const dirtyKeys = computed(() =>
  Object.keys(originalValues).filter(
    (key) => configValues[key] !== originalValues[key],
  ),
)

const isDirty = (key: string) => configValues[key] !== originalValues[key]

const placeholderFor = (key: string) =>
  DURATION_KEY_PATTERN.test(key) ? '如 24h / 30m / 10s' : ''

const normalizeConfigValue = (config: SystemConfig): any => {
  const raw = config.value
  if (isBool(config)) return raw === 'true' || raw === '1'
  if (isInt(config)) return Number(raw) || 0
  if (config.value_type === 'json') {
    try {
      return JSON.stringify(JSON.parse(raw), null, 2)
    } catch {
      return raw
    }
  }
  return raw
}

const loadConfigs = async () => {
  loading.value = true
  try {
    const res = await systemApi.getConfigs()
    configs.value = (res as any) || []

    Object.keys(configValues).forEach((key) => {
      delete configValues[key]
    })
    Object.keys(originalValues).forEach((key) => {
      delete originalValues[key]
    })

    configs.value.forEach((config) => {
      const value = normalizeConfigValue(config)
      configValues[config.key] = value
      originalValues[config.key] = value
    })

    // 保存后重载不应把用户踢回第一个分类
    if (!categories.value.some((item) => item.name === activeCategory.value)) {
      activeCategory.value = categories.value[0]?.name || ''
    }
  } catch {
    ElMessage.error('加载配置失败')
  } finally {
    loading.value = false
  }
}

const serializeValue = (key: string, value: any): string | null => {
  const config = configs.value.find((item) => item.key === key)
  if (!config) return null

  if (isBool(config)) return value ? 'true' : 'false'
  // el-input-number 清空后是 null/undefined，直接 String() 会写成 "null"
  if (isInt(config)) return String(Number(value) || 0)
  if (config.value_type === 'json') {
    try {
      JSON.parse(value)
    } catch {
      ElMessage.error(`配置 ${config.description || config.key} 的 JSON 格式不正确`)
      return null
    }
  }
  if (DURATION_KEY_PATTERN.test(key) && !DURATION_VALUE_PATTERN.test(String(value))) {
    ElMessage.error(`配置 ${config.description || config.key} 需填写时长，如 24h、30m、10s`)
    return null
  }
  return String(value)
}

const saveConfigs = async () => {
  const keys = dirtyKeys.value
  if (!keys.length) return

  const payload: Array<{ key: string; value: string }> = []
  for (const key of keys) {
    const value = serializeValue(key, configValues[key])
    if (value === null) return
    payload.push({ key, value })
  }

  saving.value = true
  try {
    await systemApi.updateConfigs({ configs: payload })
    payload.forEach(({ key }) => {
      originalValues[key] = configValues[key]
    })
    ElMessage.success(`已保存 ${payload.length} 项配置`)
  } catch {
    ElMessage.error('保存配置失败')
  } finally {
    saving.value = false
  }
}

const resetDirty = () => {
  dirtyKeys.value.forEach((key) => {
    configValues[key] = originalValues[key]
  })
}

onMounted(loadConfigs)

// 有未保存修改时离开页面先确认，避免切换菜单丢失编辑
onBeforeRouteLeave(async () => {
  if (!dirtyKeys.value.length) return true
  try {
    await ElMessageBox.confirm(
      `有 ${dirtyKeys.value.length} 项配置未保存，确定离开吗？`,
      '未保存的修改',
      { type: 'warning', confirmButtonText: '离开', cancelButtonText: '继续编辑' },
    )
    return true
  } catch {
    return false
  }
})
</script>

<style scoped>
.system-config {
  min-height: 100%;
  background: linear-gradient(180deg, #f8fafc 0%, #f1f5f9 100%);
  padding-bottom: 8px;
}

.list-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 16px;
  flex-wrap: wrap;
  padding: 20px 24px;
  background: #fff;
  border-radius: 16px;
  margin-bottom: 16px;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.04);
}

.header-content {
  display: flex;
  align-items: center;
  gap: 16px;
  flex-shrink: 0;
}

.header-icon {
  width: 48px;
  height: 48px;
  border-radius: 12px;
  background: linear-gradient(135deg, #6366f1 0%, #4f46e5 100%);
  display: flex;
  align-items: center;
  justify-content: center;
  color: #fff;
  font-size: 24px;
}

.header-text h2 {
  font-size: 20px;
  font-weight: 600;
  margin: 0;
  color: #1f2937;
  letter-spacing: -0.2px;
}

.header-subtitle {
  font-size: 13px;
  color: #9ca3af;
  margin: 4px 0 0;
}

.header-actions {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
}

.search-input {
  width: 240px;
}

:deep(.search-input .el-input__wrapper) {
  border-radius: 10px;
  box-shadow: none;
  border: 1px solid #e2e8f0;
  background: #f8fafc;
  height: 40px;
}

:deep(.search-input .el-input__wrapper.is-focus) {
  border-color: #6366f1;
  box-shadow: 0 0 0 3px rgba(99, 102, 241, 0.1);
  background: #fff;
}

.search-icon {
  color: #94a3b8;
  font-size: 13px;
}

.save-btn {
  height: 40px;
  padding: 0 24px;
  border-radius: 10px;
  font-weight: 500;
  font-size: 14px;
  display: flex;
  align-items: center;
  gap: 8px;
  background: linear-gradient(135deg, #2563eb 0%, #1d4ed8 100%);
  border-color: transparent;
  transition: all 0.2s ease;
}

.save-btn:hover:not(.is-disabled) {
  background: linear-gradient(135deg, #1d4ed8 0%, #1e40af 100%);
  transform: translateY(-1px);
  box-shadow: 0 4px 12px rgba(37, 99, 235, 0.3);
}

.content-panel {
  background: #fff;
  border-radius: 16px;
  padding: 8px;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.04);
  min-height: 320px;
}

.config-hint {
  display: flex;
  align-items: center;
  gap: 8px;
  margin: 8px 8px 0;
  padding: 10px 14px;
  border-radius: 10px;
  background: #f0f9ff;
  border: 1px solid #e0f2fe;
  color: #0369a1;
  font-size: 12px;
}

.config-layout {
  display: flex;
  align-items: stretch;
  min-height: 320px;
}

.config-nav {
  width: 190px;
  flex-shrink: 0;
  padding: 12px 8px;
  border-right: 1px solid #f1f5f9;
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.nav-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  padding: 9px 12px;
  border: none;
  border-radius: 8px;
  background: transparent;
  color: #475569;
  font-size: 13px;
  font-weight: 500;
  cursor: pointer;
  text-align: left;
  transition: all 0.15s ease;
}

.nav-item:hover {
  background: #f8fafc;
  color: #1e293b;
}

.nav-item.active {
  background: #eef2ff;
  color: #4f46e5;
  font-weight: 600;
}

.nav-count {
  font-size: 11px;
  color: #94a3b8;
  background: #f1f5f9;
  border-radius: 10px;
  padding: 1px 7px;
}

.nav-item.active .nav-count {
  background: #e0e7ff;
  color: #4f46e5;
}

.config-main {
  flex: 1;
  min-width: 0;
  padding: 12px 24px 20px;
}

.empty-tip {
  color: #94a3b8;
  font-size: 13px;
  text-align: center;
  padding: 48px 0;
}

.config-group + .config-group {
  margin-top: 12px;
}

.group-title {
  margin: 12px 0 4px;
  font-size: 12px;
  font-weight: 600;
  color: #94a3b8;
  text-transform: uppercase;
  letter-spacing: 0.4px;
}

.config-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 24px;
  padding: 14px 12px;
  border-radius: 10px;
  border-bottom: 1px solid #f3f4f6;
}

.config-item:last-child {
  border-bottom: none;
}

.config-item.dirty {
  background: #fffbeb;
}

.item-info {
  min-width: 0;
  flex: 1;
}

.item-label {
  font-size: 13px;
  font-weight: 500;
  color: #374151;
  display: flex;
  align-items: center;
  gap: 8px;
}

.dirty-flag {
  font-size: 11px;
  font-weight: 500;
  color: #b45309;
  background: #fef3c7;
  border-radius: 4px;
  padding: 1px 6px;
}

.item-key {
  margin-top: 3px;
  font-size: 11px;
  color: #b0b7c3;
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
}

.item-control {
  flex-shrink: 0;
  display: flex;
  justify-content: flex-end;
}

.text-input {
  width: 320px;
}

.number-input {
  width: 160px;
}

.json-input {
  width: 420px;
}

.dirty-bar {
  position: sticky;
  bottom: 12px;
  margin-top: 16px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  padding: 12px 20px;
  border-radius: 12px;
  background: #1e293b;
  box-shadow: 0 8px 24px rgba(15, 23, 42, 0.24);
}

.dirty-summary {
  font-size: 13px;
  color: #e2e8f0;
}

.dirty-actions {
  display: flex;
  gap: 8px;
}

@media (max-width: 900px) {
  .config-layout {
    flex-direction: column;
  }

  .config-nav {
    width: 100%;
    flex-direction: row;
    flex-wrap: wrap;
    border-right: none;
    border-bottom: 1px solid #f1f5f9;
  }

  .config-item {
    flex-direction: column;
    align-items: stretch;
    gap: 10px;
  }

  .item-control {
    justify-content: flex-start;
  }

  .text-input,
  .json-input,
  .number-input {
    width: 100%;
  }
}
</style>
