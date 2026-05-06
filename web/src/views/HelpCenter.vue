<template>
  <div class="help-center">
    <header class="page-header">
      <div class="header-content">
        <div class="header-icon">
          <i class="fa-solid fa-question-circle"></i>
        </div>
        <div class="header-text">
          <h2>帮助中心</h2>
          <p class="header-subtitle">查找使用文档和常见问题解答</p>
        </div>
      </div>
      <el-button class="back-btn" @click="goBack">
        <i class="fa-solid fa-arrow-left"></i>
        <span>返回</span>
      </el-button>
    </header>

    <div class="content-panel">
      <el-tabs v-model="activeTab" class="help-tabs">
        <el-tab-pane label="快速开始" name="quickstart">
          <QuickStart />
        </el-tab-pane>

        <el-tab-pane label="配置指南" name="configuration">
          <ConfigurationGuide />
        </el-tab-pane>

        <el-tab-pane label="常见问题" name="faq">
          <FAQ />
        </el-tab-pane>
      </el-tabs>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import QuickStart from '@/components/help/QuickStart.vue'
import ConfigurationGuide from '@/components/help/ConfigurationGuide.vue'
import FAQ from '@/components/help/FAQ.vue'

const router = useRouter()
const activeTab = ref('quickstart')

const goBack = () => {
  router.back()
}
</script>

<style scoped>
.help-center {
  min-height: 100%;
  background: linear-gradient(180deg, #f8fafc 0%, #f1f5f9 100%);
}

.page-header {
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
  background: linear-gradient(135deg, #f59e0b 0%, #d97706 100%);
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

.back-btn {
  height: 40px;
  padding: 0 20px;
  border-radius: 10px;
  font-weight: 500;
  font-size: 14px;
  display: flex;
  align-items: center;
  gap: 8px;
  background: #fff;
  border-color: #e5e7eb;
  color: #374151;
}

.back-btn:hover {
  background: #f9fafb;
  border-color: #d1d5db;
}

.content-panel {
  background: #fff;
  border-radius: 20px;
  padding: 0;
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.04), 0 8px 24px rgba(0, 0, 0, 0.02);
  overflow: hidden;
  border: 1px solid rgba(0, 0, 0, 0.04);
}

.help-tabs :deep(.el-tabs__header) {
  padding: 0 40px;
  border-bottom: 1px solid #e5e7eb;
  margin-bottom: 0;
  background: #fafbfc;
}

.help-tabs :deep(.el-tabs__nav) {
  gap: 12px;
}

.help-tabs :deep(.el-tabs__item) {
  padding: 18px 28px;
  border-radius: 10px 10px 0 0;
  margin-right: 12px;
  font-weight: 500;
  font-size: 15px;
  color: #6b7280;
  transition: all 0.25s ease;
}

.help-tabs :deep(.el-tabs__item:hover) {
  color: #2563eb;
  background: rgba(37, 99, 235, 0.04);
}

.help-tabs :deep(.el-tabs__item.is-active) {
  background: #fff;
  color: #2563eb;
  border-bottom-color: #fff;
  box-shadow: 0 -2px 8px rgba(0, 0, 0, 0.04);
}

.help-tabs :deep(.el-tabs__active-bar) {
  display: none;
}

.help-tabs :deep(.el-tabs__content) {
  padding: 0;
}
</style>
