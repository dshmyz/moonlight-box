<template>
  <div class="system-config">
    <div class="page-header">
      <h2>系统配置</h2>
      <CustomButton type="primary" @click="saveConfigs" :loading="saving">
        <template #icon>
          <Check />
        </template>
        保存配置
      </CustomButton>
    </div>

    <CustomTabs v-model="activeCategory" :tabs="tabOptions" />

    <CustomCard>
      <el-form label-width="200px" class="config-form">
        <el-form-item
          v-for="config in currentCategoryConfigs"
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
            <CustomInput
              v-model="configValues[config.key]"
              type="textarea"
              :rows="5"
              placeholder="请输入 JSON 格式配置"
              style="width: 600px"
            />
          </template>
          <template v-else>
            <CustomInput v-model="configValues[config.key]" style="width: 400px" />
          </template>
          <div class="config-key">配置键: {{ config.key }}</div>
        </el-form-item>
      </el-form>
    </CustomCard>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { Check } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import { systemApi, type SystemConfig } from '@/api/system'
import CustomButton from '@/components/ui/CustomButton.vue'
import CustomInput from '@/components/ui/CustomInput.vue'
import CustomTabs from '@/components/ui/CustomTabs.vue'
import CustomCard from '@/components/ui/CustomCard.vue'

const loading = ref(false)
const saving = ref(false)
const configs = ref<SystemConfig[]>([])
const configValues = ref<Record<string, any>>({})
const activeCategory = ref('')

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
    const data = res as any
    configs.value = data?.list || []

    configs.value.forEach((config) => {
      let value: any = config.value

      if (config.value_type === 'boolean') {
        value = value === 'true' || value === '1'
      } else if (config.value_type === 'number') {
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
  padding: var(--spacing-xl);
}

.page-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: var(--spacing-2xl);
}

.page-header h2 {
  margin: 0;
  font-size: var(--font-size-xl);
  font-weight: var(--font-weight-semibold);
}

.config-form {
  padding: var(--spacing-xl);
}

.config-key {
  margin-top: var(--spacing-xs);
  font-size: var(--font-size-xs);
  color: var(--color-text-tertiary);
}
</style>
