<template>
  <div class="header-content">
    <router-link to="/" class="logo-link">
      <svg class="logo-icon" viewBox="0 0 40 40" xmlns="http://www.w3.org/2000/svg">
        <defs>
          <linearGradient id="moonlight" x1="0%" y1="0%" x2="100%" y2="100%">
            <stop offset="0%" style="stop-color:#c4b5fd" />
            <stop offset="100%" style="stop-color:#a78bfa" />
          </linearGradient>
        </defs>
        <circle cx="20" cy="20" r="12" fill="url(#moonlight)" />
        <circle cx="24" cy="16" r="10" fill="var(--lunar-bg-surface)" />
        <circle cx="10" cy="10" r="1.5" fill="#c4b5fd" />
        <circle cx="32" cy="14" r="1" fill="#c4b5fd" opacity="0.6" />
        <circle cx="28" cy="30" r="1.2" fill="#c4b5fd" opacity="0.4" />
      </svg>
      <span class="logo-title">Moonlight Box</span>
    </router-link>

    <div class="header-actions">
      <router-link to="/help">
        <el-button class="lunar-btn lunar-btn--ghost">
          <el-icon><QuestionFilled /></el-icon>
          帮助
        </el-button>
      </router-link>
      <router-link to="/login">
        <el-button class="lunar-btn lunar-btn--accent">
          <el-icon><User /></el-icon>
          登录
        </el-button>
      </router-link>
    </div>
  </div>
</template>

<script setup lang="ts">
import { User, QuestionFilled } from '@element-plus/icons-vue'
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
  gap: 12px;
  text-decoration: none;
}

.logo-icon {
  width: 36px;
  height: 36px;
  flex-shrink: 0;
}

.logo-title {
  font-size: 16px;
  font-weight: 700;
  color: var(--lunar-silver);
  letter-spacing: -0.3px;
}

.header-actions {
  display: flex;
  align-items: center;
  gap: 12px;
}

.lunar-btn {
  height: 36px;
  padding: 0 16px;
  border-radius: 8px;
  font-size: 13px;
  font-weight: 600;
  transition: all 0.25s ease;
}

.lunar-btn--ghost {
  background: transparent;
  border: 1px solid var(--lunar-border);
  color: var(--lunar-silver-muted);
}

.lunar-btn--ghost:hover {
  border-color: var(--lunar-border-hover);
  color: var(--lunar-silver);
  background: var(--lunar-bg-glass);
}

.lunar-btn--accent {
  background: var(--lunar-gradient-btn);
  border: 1px solid var(--lunar-accent);
  color: var(--lunar-bg-deep);
}

.lunar-btn--accent:hover {
  filter: brightness(1.1);
  border-color: var(--lunar-accent);
  color: var(--lunar-bg-deep);
}

.lunar-btn--accent:active {
  transform: scale(0.97);
}
</style>