<template>
  <div class="system-config">
    <div class="page-header">
      <h2>系统配置</h2>
      <CustomButton type="primary" @click="saveConfigs" :loading="saving">
        <el-icon><Check /></el-icon> 保存配置
      </CustomButton>
    </div>

    <el-tabs v-model="activeCategory" type="border-card">
      <el-tab-pane
        v-for="category in categories"
        :key="category.name"
        :label="category.label"
        :name="category.name"
      >
        <el-form label-width="200px" class="config-form">
          <el-form-item
            v-for="config in category.configs"
            :key="config.key"
            :label="config.description"
          >
            <template v-if="config.value_type === 'boolean'">
              <el-switch v-model="configValues[config.key]" />
            </template>
            <template v-else-if="config.value_type === 'number'">
              <el-input-number v-model="configValues[config.key]" :min="0" />
            </template>
            <template v-else-if="config.value_type === 'json'">
              <el-input
                v-model="configValues[config.key]"
                type="textarea"
                :rows="5"
                placeholder="请输入 JSON 格式配置"
              />
            </template>
            <template v-else>
              <CustomInput v-model="configValues[config.key]" />
            </template>
            <div class="config-key">配置键: {{ config.key }}</div>
          </el-form-item>
        </el-form>
      </el-tab-pane>
    </el-tabs>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { Check } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import { systemApi, type SystemConfig } from '@/api/system'
import CustomButton from '@/components/ui/CustomButton.vue'
import CustomInput from '@/components/ui/CustomInput.vue'

const loading = ref(false)
const saving = ref(false)
const configs = ref<SystemConfig[]>([])
const configValues = ref<Record<string, any>>({})
const activeCategory = ref('')

/** 分类配置 */
const categories = computed(() => {
  const categoryMap: Record<string, SystemConfig[]> = {}

  configs.value.forEach((config) => {
    if (!categoryMap[config.category]) {
      categoryMap[config.category] = []
    }
    categoryMap[config.category].push(config)
  })

  const categoryLabels: Record<string, string> = {
    general: '通用配置',
    storage: '存储配置',
    cache: '缓存配置',
    security: '安全配置',
    network: '网络配置',
    log: '日志配置',
  }

  return Object.keys(categoryMap).map((name) => ({
    name,
    label: categoryLabels[name] || name,
    configs: categoryMap[name],
  }))
})

/** 加载配置列表 */
const loadConfigs = async () => {
  loading.value = true
  try {
    const res = await systemApi.getConfigs()
    const data = res as any
    configs.value = data?.list || []

    // 初始化配置值
    configs.value.forEach((config) => {
      let value: any = config.value

      // 根据类型转换值
      if (config.value_type === 'boolean') {
        value = value === 'true' || value === '1'
      } else if (config.value_type === 'number') {
        value = Number(value) || 0
      } else if (config.value_type === 'json') {
        try {
          value = JSON.stringify(JSON.parse(value), null, 2)
        } catch {
          // 保持原值
        }
      }

      configValues.value[config.key] = value
    })

    // 设置默认激活的分类
    if (categories.value.length > 0) {
      activeCategory.value = categories.value[0].name
    }
  } catch {
    ElMessage.error('加载配置失败')
  } finally {
    loading.value = false
  }
}

/** 保存配置 */
const saveConfigs = async () => {
  saving.value = true
  try {
    const configData = Object.keys(configValues.value).map((key) => {
      const config = configs.value.find((c) => c.key === key)
      let value = configValues.value[key]

      // 根据类型转换值
      if (config?.value_type === 'boolean') {
        value = value ? 'true' : 'false'
      } else if (config?.value_type === 'number') {
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
  padding: 20px;
}

.page-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 24px;
}

.page-header h2 {
  margin: 0;
  font-size: 22px;
  font-weight: 600;
}

.config-form {
  padding: 20px;
}

.config-key {
  margin-top: 4px;
  font-size: 12px;
  color: #909399;
}

:deep(.el-input),
:deep(.el-input-number) {
  width: 400px;
}

:deep(.el-textarea) {
  width: 600px;
}
</style>
