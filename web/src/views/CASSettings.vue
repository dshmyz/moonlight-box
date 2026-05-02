<template>
  <div class="settings-page">
    <div class="page-header">
      <h2>系统设置</h2>
    </div>

    <el-card class="settings-card">
      <template #header>
        <div class="card-header">
          <span>CAS 单点登录配置</span>
        </div>
      </template>

      <CASConfigForm v-model="casConfig" />

      <div class="form-actions">
        <el-button type="primary" @click="handleSave" :loading="saving">
          保存配置
        </el-button>
        <el-button @click="handleReset" :loading="loading">
          重置
        </el-button>
        <el-button type="danger" @click="handleDelete" :loading="deleting">
          删除配置
        </el-button>
      </div>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import CASConfigForm from '@/components/settings/CASConfigForm.vue'
import { casConfigApi, type CASConfig } from '@/api/casConfig'

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
  padding: 24px;
}

.page-header {
  margin-bottom: 24px;
}

.page-header h2 {
  font-size: 20px;
  font-weight: 600;
  color: #303133;
  margin: 0;
}

.settings-card {
  max-width: 800px;
}

.card-header {
  font-size: 16px;
  font-weight: 600;
}

.form-actions {
  margin-top: 24px;
  padding-top: 16px;
  border-top: 1px solid #ebeef5;
}
</style>
