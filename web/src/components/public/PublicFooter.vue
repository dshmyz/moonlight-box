<template>
  <div class="footer-content">
    <div class="footer-left">
      <span class="footer-brand">Moonlight Registry</span>
      <span class="footer-divider">|</span>
      <span class="footer-desc">企业级组件仓库管理平台</span>
    </div>
    <div class="footer-right">
      <span class="footer-copyright"> 2026 Moonlight Registry. All rights reserved.</span>
    </div>
  </div>
</template>

<script setup lang="ts">
</script>

<style scoped>
.footer-content {
  max-width: 1200px;
  margin: 0 auto;
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 16px 20px;
}

.footer-left {
  display: flex;
  align-items: center;
  gap: 8px;
}

.footer-brand {
  font-weight: 600;
  color: #606266;
  font-size: 13px;
}

.footer-divider {
  color: #dcdfe6;
}

.footer-desc {
  color: #909399;
  font-size: 12px;
}

.footer-right {
  color: #c0c4cc;
  font-size: 12px;
}
</style>
