<template>
  <div class="cas-settings">
    <header class="list-header">
      <div class="header-content">
        <div class="header-icon">
          <i class="fa-solid fa-key"></i>
        </div>
        <div class="header-text">
          <h2>CAS 单点登录</h2>
          <p class="header-subtitle">配置 CAS SSO 认证服务器连接参数</p>
        </div>
      </div>
      <div class="header-actions">
        <el-button @click="testConnection" :loading="testing">
          <i class="fa-solid fa-plug"></i>
          <span>测试连接</span>
        </el-button>
        <el-button type="primary" @click="saveConfig" :loading="saving">
          <i class="fa-solid fa-check"></i>
          <span>保存配置</span>
        </el-button>
      </div>
    </header>

    <div class="content-panel" v-loading="loading">
      <el-form :model="config" label-width="160px" class="config-form">
        <el-form-item label="启用 CAS">
          <el-switch v-model="config.enabled" />
          <span class="form-hint">开启后登录页将显示 CAS 登录入口</span>
        </el-form-item>

        <el-form-item label="CAS 服务器地址" required>
          <el-input
            v-model="config.server_url"
            placeholder="https://cas.example.com"
            :disabled="!config.enabled"
          />
          <span class="form-hint">CAS 服务端基础 URL，不含路径</span>
        </el-form-item>

        <el-form-item label="服务回调地址">
          <el-input
            v-model="config.service_url"
            placeholder="https://your-moonlight-domain/login"
            :disabled="!config.enabled"
          />
          <span class="form-hint">本系统的 CAS 回调地址（前端登录页），需在 CAS 服务端注册。留空则按请求域名自动推导，此时请配置下方"允许的域名"</span>
        </el-form-item>

        <el-form-item label="允许的域名">
          <el-input
            v-model="config.allowed_hosts"
            placeholder="repo-a.corp.com, *.corp.com"
            :disabled="!config.enabled"
          />
          <span class="form-hint">service_url 留空时，按请求域名自动推导回跳地址的白名单（逗号分隔，支持 *.corp.com 通配）；需在 CAS 服务端注册对应 service</span>
        </el-form-item>

        <el-form-item label="登录路径">
          <el-input
            v-model="config.login_path"
            placeholder="/cas/login"
            :disabled="!config.enabled"
          />
          <span class="form-hint">CAS 服务端的登录端点路径，通常为 /cas/login</span>
        </el-form-item>

        <el-form-item label="Ticket 校验路径">
          <el-input
            v-model="config.validate_path"
            placeholder="/cas/serviceValidate"
            :disabled="!config.enabled"
          />
          <span class="form-hint">CAS 服务端的 ticket 校验端点，通常为 /cas/serviceValidate</span>
        </el-form-item>
      </el-form>

      <div v-if="testResult" class="test-result" :class="testResult.success ? 'success' : 'error'">
        <i :class="testResult.success ? 'fa-solid fa-circle-check' : 'fa-solid fa-circle-exclamation'"></i>
        <span>{{ testResult.message }}</span>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, reactive } from 'vue'
import { casConfigApi, type CASConfig } from '@/api/casConfig'
import { success, error } from '@/utils/message'

const loading = ref(false)
const saving = ref(false)
const testing = ref(false)
const testResult = ref<{ success: boolean; message: string } | null>(null)

const config = reactive({
  enabled: false,
  server_url: '',
  service_url: '',
  login_path: '/cas/login',
  validate_path: '/cas/serviceValidate',
  // 逗号分隔的域名白名单文本，提交时转换为数组
  allowed_hosts: '',
})

async function loadConfig() {
  loading.value = true
  try {
    const data = await casConfigApi.getConfig()
    if (data) {
      const { allowed_hosts, ...rest } = data
      Object.assign(config, rest)
      config.allowed_hosts = Array.isArray(allowed_hosts) ? allowed_hosts.join(', ') : ''
    }
  } catch (e: any) {
    error(e?.message || '加载 CAS 配置失败')
  } finally {
    loading.value = false
  }
}

async function saveConfig() {
  if (config.enabled) {
    if (!config.server_url) {
      error('启用 CAS 时，服务器地址为必填项')
      return
    }
    // 与后端不变量一致：静态 service_url 与动态 Host 白名单至少配置其一，
    // 否则 CAS 无法推导回跳地址，登录必然失败。
    if (!config.service_url && !config.allowed_hosts.trim()) {
      error('启用 CAS 时，服务回调地址与允许的域名至少填写一项')
      return
    }
  }

  saving.value = true
  try {
    const payload: CASConfig = {
      ...config,
      allowed_hosts: config.allowed_hosts
        .split(',')
        .map((s) => s.trim())
        .filter(Boolean),
    }
    await casConfigApi.updateConfig(payload)
    success('CAS 配置已保存')
  } catch (e: any) {
    error(e?.message || '保存 CAS 配置失败')
  } finally {
    saving.value = false
  }
}

async function testConnection() {
  if (!config.server_url) {
    error('请先填写 CAS 服务器地址')
    return
  }

  testing.value = true
  testResult.value = null
  try {
    await casConfigApi.testConnection()
    testResult.value = { success: true, message: 'CAS 服务器连接正常' }
  } catch (e: any) {
    testResult.value = { success: false, message: e?.message || 'CAS 服务器连接失败' }
  } finally {
    testing.value = false
  }
}

onMounted(() => {
  loadConfig()
})
</script>

<style scoped>
.cas-settings {
  padding: 24px;
}

.list-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 24px;
  padding: 20px 24px;
  background: #fff;
  border-radius: 12px;
  border: 1px solid #e2e8f0;
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
  font-size: 20px;
}

.header-text h2 {
  margin: 0;
  font-size: 20px;
  font-weight: 600;
  color: #1e293b;
}

.header-subtitle {
  margin: 4px 0 0;
  font-size: 13px;
  color: #64748b;
}

.header-actions {
  display: flex;
  gap: 12px;
}

.content-panel {
  background: #fff;
  border-radius: 12px;
  border: 1px solid #e2e8f0;
  padding: 32px;
}

.config-form {
  max-width: 640px;
}

.form-hint {
  display: block;
  margin-top: 4px;
  font-size: 12px;
  color: #94a3b8;
}

.test-result {
  margin-top: 24px;
  padding: 12px 16px;
  border-radius: 8px;
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 14px;
}

.test-result.success {
  background: #f0fdf4;
  color: #059669;
  border: 1px solid #bbf7d0;
}

.test-result.error {
  background: #fef2f2;
  color: #dc2626;
  border: 1px solid #fecaca;
}
</style>
