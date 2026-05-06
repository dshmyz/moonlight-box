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
      <el-button type="primary" class="save-btn" @click="saveConfigs" :loading="saving">
        <i class="fa-solid fa-check"></i>
        <span>保存配置</span>
      </el-button>
    </header>

    <div class="content-panel">
      <el-tabs v-model="activeCategory" class="config-tabs">
        <el-tab-pane v-for="tab in tabOptions" :key="tab.name" :label="tab.label" :name="tab.name" />
      </el-tabs>

      <div class="config-content">
        <el-form label-width="180px" class="config-form">
          <el-form-item
            v-for="config in currentCategoryConfigs"
            :key="config.key"
            :label="config.description"
          >
            <template v-if="config.value_type === 'bool' || config.value_type === 'boolean'">
              <el-switch v-model="configValues[config.key]" />
            </template>
            <template v-else-if="config.value_type === 'int' || config.value_type === 'number'">
              <el-input-number v-model="configValues[config.key]" :min="0" />
            </template>
            <template v-else-if="config.value_type === 'json'">
              <el-input
                v-model="configValues[config.key]"
                type="textarea"
                :rows="5"
                placeholder="请输入 JSON 格式配置"
                class="json-input"
              />
            </template>
            <template v-else>
              <el-input v-model="configValues[config.key]" class="config-input" />
            </template>
            <div class="config-key">
              <i class="fa-solid fa-key"></i>
              {{ config.key }}
            </div>
          </el-form-item>
        </el-form>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { systemApi, type SystemConfig } from '@/api/system'

const loading = ref(false)
const saving = ref(false)
const configs = ref<SystemConfig[]>([])
const configValues = ref<Record<string, any>>({})
const activeCategory = ref('')

const categories = computed(() => {
  const categoryMap: Record<string, SystemConfig[]> = {}

  configs.value.forEach((config) => {
    const cat = config.category?.trim() || 'other'
    if (!categoryMap[cat]) {
      categoryMap[cat] = []
    }
    categoryMap[cat].push(config)
  })

  const categoryLabels: Record<string, string> = {
    general: '通用配置',
    storage: '存储配置',
    cache: '缓存配置',
    security: '安全配置',
    login: '登录配置',
    network: '网络配置',
    log: '日志配置',
    other: '其他配置',
  }

  return Object.keys(categoryMap).map((name) => ({
    name,
    label: categoryLabels[name] || name,
    configs: categoryMap[name],
  }))
})

const tabOptions = computed(() => {
  return categories.value.map((cat) => ({
    name: cat.name,
    label: cat.label,
  }))
})

const currentCategoryConfigs = computed(() => {
  const category = categories.value.find((cat) => cat.name === activeCategory.value)
  return category?.configs || []
})

const loadConfigs = async () => {
  loading.value = true
  try {
    const res = await systemApi.getConfigs()
    configs.value = (res as any) || []

    configs.value.forEach((config) => {
      let value: any = config.value

      if (config.value_type === 'bool' || config.value_type === 'boolean') {
        value = value === 'true' || value === '1'
      } else if (config.value_type === 'int' || config.value_type === 'number') {
        value = Number(value) || 0
      } else if (config.value_type === 'json') {
        try {
          value = JSON.stringify(JSON.parse(value), null, 2)
        } catch {
        }
      }

      configValues.value[config.key] = value
    })

    if (categories.value.length > 0) {
      activeCategory.value = categories.value[0].name
    }
  } catch {
    ElMessage.error('加载配置失败')
  } finally {
    loading.value = false
  }
}

const saveConfigs = async () => {
  saving.value = true
  try {
    const configData = Object.keys(configValues.value).map((key) => {
      const config = configs.value.find((c) => c.key === key)
      let value = configValues.value[key]

      if (config?.value_type === 'bool' || config?.value_type === 'boolean') {
        value = value ? 'true' : 'false'
      } else if (config?.value_type === 'int' || config?.value_type === 'number') {
        value = String(value)
      } else if (config?.value_type === 'json') {
        try {
          JSON.parse(value)
        } catch {
          ElMessage.error(`配置 ${config.description} 的 JSON 格式不正确`)
          throw new Error('Invalid JSON')
        }
      } else {
        value = String(value)
      }

      return { key, value }
    })

    await systemApi.updateConfigs({ configs: configData })
    ElMessage.success('配置保存成功')
    await loadConfigs()
  } catch (error) {
    if (error instanceof Error && error.message !== 'Invalid JSON') {
      ElMessage.error('保存配置失败')
    }
  } finally {
    saving.value = false
  }
}

onMounted(loadConfigs)
</script>

<style scoped>
.system-config {
  min-height: 100%;
  background: linear-gradient(180deg, #f8fafc 0%, #f1f5f9 100%);
}

.list-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
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

.save-btn:hover {
  background: linear-gradient(135deg, #1d4ed8 0%, #1e40af 100%);
  transform: translateY(-1px);
  box-shadow: 0 4px 12px rgba(37, 99, 235, 0.3);
}

.content-panel {
  background: #fff;
  border-radius: 16px;
  padding: 24px;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.04);
}

.config-tabs {
  margin-bottom: 24px;
}

:deep(.el-tabs__item) {
  font-size: 14px;
  font-weight: 500;
  padding: 0 20px;
  height: 40px;
  line-height: 40px;
}

:deep(.el-tabs__item.is-active) {
  color: #2563eb;
}

:deep(.el-tabs__active-bar) {
  height: 3px;
  border-radius: 3px;
}

.config-content {
  padding: 8px 0;
}

.config-form {
  max-width: 800px;
}

.config-form :deep(.el-form-item) {
  margin-bottom: 24px;
  padding-bottom: 16px;
  border-bottom: 1px solid #f3f4f6;
}

.config-form :deep(.el-form-item:last-child) {
  border-bottom: none;
  margin-bottom: 0;
}

.config-form :deep(.el-form-item__label) {
  font-weight: 500;
  color: #374151;
}

.config-input {
  max-width: 400px;
}

.json-input {
  max-width: 600px;
}

.config-key {
  margin-top: 8px;
  font-size: 12px;
  color: #9ca3af;
  display: flex;
  align-items: center;
  gap: 4px;
}

.config-key i {
  font-size: 10px;
}
</style>
