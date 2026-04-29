<template>
  <div class="login-container">
    <div class="login-card">
      <div class="logo">
        <div class="logo-icon">
          <el-icon><Box /></el-icon>
        </div>
        <h1>Moonlight Registry</h1>
        <p>企业级组件仓库管理平台</p>
      </div>

      <template v-if="casEnabled">
        <el-button
          type="primary"
          size="large"
          class="cas-login-btn"
          @click="handleCASLogin"
        >
          <el-icon><Switch /></el-icon>
          使用 CAS 单点登录
        </el-button>

        <el-divider>或</el-divider>
      </template>

      <el-form ref="formRef" :model="form" :rules="rules" @submit.prevent="handleLogin">
        <el-form-item prop="username">
          <el-input
            v-model="form.username"
            placeholder="用户名"
            size="large"
          >
            <template #prefix>
              <el-icon><User /></el-icon>
            </template>
          </el-input>
        </el-form-item>

        <el-form-item prop="password">
          <el-input
            v-model="form.password"
            type="password"
            placeholder="密码"
            size="large"
            show-password
            @keyup.enter="handleLogin"
          >
            <template #prefix>
              <el-icon><Lock /></el-icon>
            </template>
          </el-input>
        </el-form-item>

        <el-form-item>
          <el-button
            type="primary"
            size="large"
            :loading="loading"
            class="login-btn"
            @click="handleLogin"
          >
            登 录
          </el-button>
        </el-form-item>
      </el-form>

      <div class="login-footer">
        <router-link to="/" class="back-link">
          <el-icon><Back /></el-icon>
          返回公共仓库
        </router-link>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import { ElMessage } from 'element-plus'
import { Box, User, Lock, Back, Switch } from '@element-plus/icons-vue'
import type { FormInstance, FormRules } from 'element-plus'
import { authApi } from '@/api/auth'

const router = useRouter()
const route = useRoute()
const authStore = useAuthStore()

const formRef = ref<FormInstance>()
const loading = ref(false)
const casEnabled = ref(false)

const form = reactive({
  username: '',
  password: '',
})

const rules: FormRules = {
  username: [{ required: true, message: '请输入用户名', trigger: 'blur' }],
  password: [{ required: true, message: '请输入密码', trigger: 'blur' }],
}

onMounted(async () => {
  try {
    const res = await authApi.getCASConfig()
    casEnabled.value = res.data?.enabled ?? false
  } catch {
    casEnabled.value = false
  }

  const ticket = route.query.ticket as string
  if (ticket) {
    await handleCASCallback(ticket)
  }
})

async function handleCASCallback(ticket: string) {
  loading.value = true
  try {
    await authStore.casLogin(ticket)
    ElMessage.success('CAS 登录成功')
    const redirect = (route.query.redirect as string) || '/admin/dashboard'
    router.push(redirect)
  } catch (error: any) {
    ElMessage.error(error.message || 'CAS 登录失败')
    router.push({ path: '/login', query: { redirect: route.query.redirect } })
  } finally {
    loading.value = false
  }
}

function handleCASLogin() {
  const redirect = (route.query.redirect as string) || ''
  window.location.href = `/api/v1/auth/cas/login?redirect=${encodeURIComponent(redirect)}`
}

async function handleLogin() {
  const valid = await formRef.value?.validate().catch(() => false)
  if (!valid) return

  loading.value = true
  try {
    await authStore.login(form.username, form.password)
    ElMessage.success('登录成功')
    const redirect = (route.query.redirect as string) || '/admin/dashboard'
    router.push(redirect)
  } catch (error: any) {
    ElMessage.error(error.message || '登录失败')
  } finally {
    loading.value = false
  }
}
</script>

<style scoped>
.login-container {
  height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
  background: #f5f7fa;
}

.login-card {
  width: 420px;
  padding: 40px;
  background: white;
  border-radius: 12px;
  box-shadow: 0 4px 20px rgba(0, 0, 0, 0.08);
}

.logo {
  text-align: center;
  margin-bottom: 32px;
}

.logo-icon {
  width: 48px;
  height: 48px;
  background: linear-gradient(135deg, #409eff 0%, #337ecc 100%);
  border-radius: 12px;
  display: flex;
  align-items: center;
  justify-content: center;
  color: #fff;
  font-size: 24px;
  margin: 0 auto 16px;
}

.logo h1 {
  font-size: 22px;
  font-weight: 700;
  margin: 0 0 6px;
  color: #303133;
}

.logo p {
  font-size: 13px;
  color: #909399;
  margin: 0;
}

.cas-login-btn {
  width: 100%;
  margin-bottom: 0;
}

.login-btn {
  width: 100%;
}

.login-footer {
  text-align: center;
  margin-top: 16px;
}

.back-link {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  color: #909399;
  text-decoration: none;
  font-size: 13px;
  transition: color 0.2s;
}

.back-link:hover {
  color: #409eff;
}
</style>
