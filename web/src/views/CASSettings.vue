<template>
  <div class="settings-page">
    <div class="page-header">
      <h2>系统设置</h2>
    </div>

    <CustomCard title="CAS 单点登录配置" hoverable class="settings-card">
      <CASConfigForm v-model="casConfig" />

      <div class="form-actions">
        <CustomButton type="primary" @click="handleSave" :loading="saving">
          保存配置
        </CustomButton>
        <CustomButton type="secondary" @click="handleReset" :loading="loading">
          重置
        </CustomButton>
        <CustomButton type="outline" @click="handleDelete" :loading="deleting">
          删除配置
        </CustomButton>
      </div>
    </CustomCard>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import CASConfigForm from '@/components/settings/CASConfigForm.vue'
import { casConfigApi, type CASConfig } from '@/api/casConfig'
import CustomCard from '@/components/ui/CustomCard.vue'
import CustomButton from '@/components/ui/CustomButton.vue'

const loading = ref(false)
const saving = ref(false)
const deleting = ref(false)

const defaultCASConfig: CASConfig = {
  enabled: false,
  server_url: '',
  service_url: '',
  login_path: '/cas/login',
  validate_path: '/cas/serviceValidate',
}

const casConfig = ref<CASConfig>({ ...defaultCASConfig })

async function loadConfig() {
  loading.value = true
  try {
    const res = await casConfigApi.getConfig()
    if (res.data) {
      casConfig.value = { ...defaultCASConfig, ...res.data }
    } else {
      casConfig.value = { ...defaultCASConfig }
    }
  } catch {
    ElMessage.warning('未找到 CAS 配置，已加载默认配置')
    casConfig.value = { ...defaultCASConfig }
  } finally {
    loading.value = false
  }
}

async function handleSave() {
  saving.value = true
  try {
    await casConfigApi.updateConfig(casConfig.value)
    ElMessage.success('CAS 配置已保存')
    await loadConfig()
  } catch (error: any) {
    ElMessage.error(error.message || '保存配置失败')
  } finally {
    saving.value = false
  }
}

async function handleReset() {
  await loadConfig()
  ElMessage.info('已重置配置')
}

async function handleDelete() {
  try {
    await ElMessageBox.confirm('确定要删除 CAS 配置吗？此操作不可恢复。', '删除确认', {
      type: 'warning',
    })
    deleting.value = true
    await casConfigApi.deleteConfig()
    ElMessage.success('CAS 配置已删除')
    casConfig.value = { ...defaultCASConfig }
  } catch (error: any) {
    if (error !== 'cancel') {
      ElMessage.error(error.message || '删除配置失败')
    }
  } finally {
    deleting.value = false
  }
}

onMounted(loadConfig)
</script>

<style scoped>
.settings-page {
  padding: var(--spacing-xl);
}

.page-header {
  margin-bottom: var(--spacing-2xl);
}

.page-header h2 {
  font-size: var(--font-size-xl);
  font-weight: var(--font-weight-semibold);
  color: var(--color-text-primary);
  margin: 0;
}

.settings-card {
  max-width: 800px;
}

.form-actions {
  margin-top: var(--spacing-2xl);
  padding-top: var(--spacing-lg);
  border-top: 1px solid var(--color-border);
  display: flex;
  gap: var(--spacing-md);
}
</style>
