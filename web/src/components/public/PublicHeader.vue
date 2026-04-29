<template>
  <div class="header-content">
    <router-link to="/" class="logo-link">
      <svg class="logo-icon" viewBox="0 0 40 40" xmlns="http://www.w3.org/2000/svg">
        <defs>
          <linearGradient id="moonlight" x1="0%" y1="0%" x2="100%" y2="100%">
            <stop offset="0%" style="stop-color:#c4b5fd" />
            <stop offset="100%" style="stop-color:#8b5cf6" />
          </linearGradient>
        </defs>
        <!-- 圆月 -->
        <circle cx="20" cy="20" r="12" fill="url(#moonlight)"/>
        <!-- 月牙阴影 -->
        <circle cx="24" cy="16" r="10" fill="#fff"/>
        <!-- 小星星 -->
        <circle cx="10" cy="10" r="1.5" fill="#c4b5fd"/>
        <circle cx="32" cy="14" r="1" fill="#c4b5fd" opacity="0.6"/>
        <circle cx="28" cy="30" r="1.2" fill="#c4b5fd" opacity="0.4"/>
      </svg>
      <div class="logo-text">
        <span class="logo-title">Moonlight Registry</span>
      </div>
    </router-link>
    <div class="header-actions">
      <el-input
        v-model="searchQuery"
        placeholder="搜索包..."
        class="header-search"
        clearable
        @keyup.enter="handleQuickSearch"
        @clear="handleQuickSearch"
      >
        <template #prefix>
          <el-icon><Search /></el-icon>
        </template>
      </el-input>
      <router-link to="/login">
        <el-button type="primary" plain>
          <el-icon><User /></el-icon>
          登录
        </el-button>
      </router-link>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { Search, User } from '@element-plus/icons-vue'

const router = useRouter()
const searchQuery = ref('')

function handleQuickSearch() {
  if (searchQuery.value.trim()) {
    router.push({ path: '/', query: { q: searchQuery.value.trim() } })
  }
}
</script>

<style scoped>
.header-content {
  display: flex;
  align-items: center;
  justify-content: space-between;
  width: 100%;
  max-width: 1200px;
  margin: 0 auto;
  padding: 0 20px;
  height: 100%;
}

.logo-link {
  display: flex;
  align-items: center;
  gap: 10px;
  text-decoration: none;
  flex-shrink: 0;
}

.logo-icon {
  width: 36px;
  height: 36px;
  flex-shrink: 0;
}

.logo-text {
  display: flex;
  flex-direction: column;
  justify-content: center;
}

.logo-title {
  font-size: 16px;
  font-weight: 700;
  color: #303133;
  letter-spacing: -0.3px;
}

.header-actions {
  display: flex;
  align-items: center;
  gap: 12px;
}

.header-search {
  width: 260px;
}

.header-search :deep(.el-input__wrapper) {
  background-color: #f5f7fa;
}
</style>
