<template>
  <div class="login-container">
    <div class="login-bg">
      <div class="bg-blob blob-1"></div>
      <div class="bg-blob blob-2"></div>
      <div class="bg-blob blob-3"></div>
      <div class="stars">
        <span v-for="i in 50" :key="i" class="star" :style="getStarStyle(i)"></span>
      </div>
    </div>

    <div class="login-content">
      <div class="login-card">
        <div class="logo-section">
          <svg class="logo-svg" viewBox="0 0 40 40" xmlns="http://www.w3.org/2000/svg">
            <defs>
              <linearGradient id="login-moonlight" x1="0%" y1="0%" x2="100%" y2="100%">
                <stop offset="0%" style="stop-color:#c4b5fd" />
                <stop offset="100%" style="stop-color:#8b5cf6" />
              </linearGradient>
            </defs>
            <circle cx="20" cy="20" r="12" fill="url(#login-moonlight)"/>
            <circle cx="24" cy="16" r="10" fill="#fff"/>
            <circle cx="10" cy="10" r="1.5" fill="#c4b5fd"/>
            <circle cx="32" cy="14" r="1" fill="#c4b5fd" opacity="0.6"/>
            <circle cx="28" cy="30" r="1.2" fill="#c4b5fd" opacity="0.4"/>
          </svg>
          <h1 class="logo-title">Moonlight Box</h1>
          <p class="logo-subtitle">企业级组件仓库管理平台</p>
        </div>

        <template v-if="casEnabled">
          <el-button
            type="primary"
            size="large"
            class="cas-login-btn"
            @click="handleCASLogin"
            :loading="loading"
          >
            <i class="fa-solid fa-users"></i>
            <span>使用 CAS 单点登录</span>
          </el-button>

          <div class="divider">
            <span class="divider-line"></span>
            <span class="divider-text">或</span>
            <span class="divider-line"></span>
          </div>
        </template>

        <form ref="formRef" class="login-form" @submit.prevent="handleLogin">
          <div class="form-item">
            <div class="input-wrapper">
              <i class="fa-solid fa-user input-icon"></i>
              <input
                v-model="form.username"
                type="text"
                placeholder="用户名"
                class="native-input"
                @blur="validateUsername"
              />
            </div>
            <span v-if="usernameError" class="error-message">{{ usernameError }}</span>
          </div>

          <div class="form-item">
            <div class="input-wrapper">
              <i class="fa-solid fa-lock input-icon"></i>
              <input
                v-model="form.password"
                :type="showPassword ? 'text' : 'password'"
                placeholder="密码"
                class="native-input"
                @keyup.enter="handleLogin"
                @blur="validatePassword"
              />
              <button
                type="button"
                class="password-toggle"
                @click="togglePassword"
              >
                <i class="fa-solid" :class="showPassword ? 'fa-eye-slash' : 'fa-eye'"></i>
              </button>
            </div>
            <span v-if="passwordError" class="error-message">{{ passwordError }}</span>
          </div>

          <div class="form-item">
            <button
              type="submit"
              :disabled="loading"
              class="login-btn"
            >
              <span v-if="loading" class="loading-spinner"></span>
              <span>登 录</span>
            </button>
          </div>
        </form>

        <div class="login-footer">
          <router-link to="/" class="back-link">
            <i class="fa-solid fa-arrow-left"></i>
            <span>返回公共仓库</span>
          </router-link>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import { ElMessage } from 'element-plus'

import { casAuthApi } from '@/api/casConfig'

const router = useRouter()
const route = useRoute()
const authStore = useAuthStore()

const formRef = ref<HTMLFormElement>()
const loading = ref(false)
const casEnabled = ref(false)
const showPassword = ref(false)
const usernameError = ref('')
const passwordError = ref('')

const form = reactive({
  username: '',
  password: '',
})

onMounted(async () => {
  try {
    const data = await casAuthApi.getCASConfig()
    casEnabled.value = data?.enabled ?? false
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
  } catch (error: unknown) {
    const msg = error instanceof Error ? error.message : 'CAS 登录失败'
    ElMessage.error(msg)
    router.push({ path: '/login', query: { redirect: route.query.redirect } })
  } finally {
    loading.value = false
  }
}

function handleCASLogin() {
  const redirect = (route.query.redirect as string) || ''
  window.location.href = `/api/v1/auth/cas/login?redirect=${encodeURIComponent(redirect)}`
}

function togglePassword() {
  showPassword.value = !showPassword.value
}

function validateUsername() {
  if (!form.username.trim()) {
    usernameError.value = '请输入用户名'
    return false
  }
  usernameError.value = ''
  return true
}

function validatePassword() {
  if (!form.password.trim()) {
    passwordError.value = '请输入密码'
    return false
  }
  passwordError.value = ''
  return true
}

async function handleLogin() {
  const valid = validateUsername() && validatePassword()
  if (!valid) return

  loading.value = true
  try {
    await authStore.login(form.username, form.password)
    ElMessage.success('登录成功')
    const redirect = (route.query.redirect as string) || '/admin/dashboard'
    router.push(redirect)
  } catch (error: unknown) {
    const msg = error instanceof Error ? error.message : '登录失败'
    ElMessage.error(msg)
  } finally {
    loading.value = false
  }
}

function getStarStyle(_index: number) {
  const left = Math.random() * 100
  const top = Math.random() * 100
  const size = Math.random() * 2 + 1
  const delay = Math.random() * 3
  const duration = Math.random() * 2 + 1
  return {
    left: `${left}%`,
    top: `${top}%`,
    width: `${size}px`,
    height: `${size}px`,
    animationDelay: `${delay}s`,
    animationDuration: `${duration}s`,
  }
}
</script>

<style scoped>
.login-container {
  height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
  background: linear-gradient(135deg, #0f172a 0%, #1e1b4b 50%, #312e81 100%);
  overflow: hidden;
  position: relative;
}

.login-bg {
  position: absolute;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  overflow: hidden;
}

.bg-blob {
  position: absolute;
  border-radius: 50%;
  filter: blur(80px);
  opacity: 0.4;
}

.blob-1 {
  width: 600px;
  height: 600px;
  background: linear-gradient(135deg, #8b5cf6 0%, #6366f1 100%);
  top: -200px;
  left: -100px;
  animation: blobFloat 15s ease-in-out infinite;
}

.blob-2 {
  width: 500px;
  height: 500px;
  background: linear-gradient(135deg, #a855f7 0%, #ec4899 100%);
  bottom: -150px;
  right: -50px;
  animation: blobFloat 12s ease-in-out infinite reverse;
}

.blob-3 {
  width: 400px;
  height: 400px;
  background: linear-gradient(135deg, #6366f1 0%, #8b5cf6 100%);
  top: 50%;
  left: 50%;
  transform: translate(-50%, -50%);
  animation: blobFloat 18s ease-in-out infinite;
}

@keyframes blobFloat {
  0%, 100% {
    transform: translate(0, 0) scale(1);
  }
  33% {
    transform: translate(30px, -30px) scale(1.1);
  }
  66% {
    transform: translate(-20px, 20px) scale(0.95);
  }
}

.stars {
  position: absolute;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
}

.star {
  position: absolute;
  background: #fff;
  border-radius: 50%;
  animation: twinkle ease-in-out infinite;
}

@keyframes twinkle {
  0%, 100% {
    opacity: 0.3;
  }
  50% {
    opacity: 1;
  }
}

.login-content {
  position: relative;
  z-index: 10;
  width: 100%;
  max-width: 420px;
  padding: 0 20px;
}

.login-card {
  background: rgba(15, 23, 42, 0.85);
  backdrop-filter: blur(20px);
  border-radius: 20px;
  padding: 48px 40px;
  border: 1px solid rgba(255, 255, 255, 0.1);
  box-shadow: 0 25px 50px -12px rgba(0, 0, 0, 0.5);
}

.logo-section {
  text-align: center;
  margin-bottom: 40px;
}

.logo-svg {
  width: 88px;
  height: 88px;
  margin: 0 auto 24px;
}

.logo-title {
  font-size: 24px;
  font-weight: 700;
  color: #f1f5f9;
  margin: 0 0 8px;
  letter-spacing: -0.5px;
}

.logo-subtitle {
  font-size: 14px;
  color: #94a3b8;
  margin: 0;
}

.cas-login-btn {
  width: 100%;
  height: 48px;
  background: linear-gradient(135deg, #8b5cf6 0%, #6366f1 100%);
  border: none;
  border-radius: 12px;
  font-size: 15px;
  font-weight: 600;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  transition: all 0.3s ease;
  box-shadow: 0 4px 15px rgba(139, 92, 246, 0.4);
}

.cas-login-btn:hover {
  transform: translateY(-2px);
  box-shadow: 0 6px 20px rgba(139, 92, 246, 0.5);
}

.cas-login-btn i {
  font-size: 18px;
}

.divider {
  display: flex;
  align-items: center;
  gap: 16px;
  margin: 20px 0;
}

.divider-line {
  flex: 1;
  height: 1px;
  background: linear-gradient(90deg, transparent 0%, rgba(255, 255, 255, 0.2) 50%, transparent 100%);
}

.divider-text {
  font-size: 13px;
  color: #64748b;
}

.login-form {
  margin-top: 8px;
}

.form-item {
  margin-bottom: 20px;
}

.input-wrapper {
  position: relative;
  display: flex;
  align-items: center;
  width: 100%;
}

.input-icon {
  position: absolute;
  left: 16px;
  color: #64748b;
  font-size: 16px;
  z-index: 1;
}

.native-input {
  width: 100%;
  height: 48px;
  background: rgba(30, 41, 59, 0.8);
  border: 1px solid rgba(255, 255, 255, 0.1);
  border-radius: 12px;
  padding-left: 48px;
  padding-right: 48px;
  color: #f1f5f9;
  font-size: 14px;
  transition: all 0.3s ease;
  box-sizing: border-box;
  outline: none;
}

.native-input:hover {
  border-color: rgba(167, 139, 250, 0.4);
}

.native-input:focus {
  border-color: #8b5cf6;
  box-shadow: 0 0 0 3px rgba(139, 92, 246, 0.1);
}

.native-input::placeholder {
  color: #64748b;
}

.password-toggle {
  position: absolute;
  right: 16px;
  background: none;
  border: none;
  color: #64748b;
  cursor: pointer;
  font-size: 16px;
  z-index: 1;
  transition: color 0.3s ease;
}

.password-toggle:hover {
  color: #a78bfa;
}

.error-message {
  display: block;
  color: #ef4444;
  font-size: 12px;
  margin-top: 6px;
  padding-left: 48px;
}

.loading-spinner {
  display: inline-block;
  width: 18px;
  height: 18px;
  border: 2px solid rgba(255, 255, 255, 0.3);
  border-top-color: #fff;
  border-radius: 50%;
  animation: spin 0.8s linear infinite;
  margin-right: 8px;
}

@keyframes spin {
  to {
    transform: rotate(360deg);
  }
}

.login-btn {
  width: 100%;
  height: 48px;
  background: linear-gradient(135deg, #8b5cf6 0%, #6366f1 100%);
  border: none;
  border-radius: 12px;
  font-size: 15px;
  font-weight: 600;
  transition: all 0.3s ease;
  box-shadow: 0 4px 15px rgba(139, 92, 246, 0.4);
}

.login-btn:hover:not(:disabled) {
  transform: translateY(-2px);
  box-shadow: 0 6px 20px rgba(139, 92, 246, 0.5);
}

.login-btn:active:not(:disabled) {
  transform: translateY(0);
}

.login-footer {
  text-align: center;
  margin-top: 24px;
  padding-top: 24px;
  border-top: 1px solid rgba(255, 255, 255, 0.1);
}

.back-link {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  color: #94a3b8;
  text-decoration: none;
  font-size: 14px;
  transition: all 0.3s ease;
}

.back-link:hover {
  color: #c4b5fd;
  transform: translateX(-4px);
}

.back-link i {
  font-size: 14px;
}
</style>
