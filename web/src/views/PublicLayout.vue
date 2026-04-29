<template>
  <el-container class="public-layout">
    <el-header class="public-header-wrapper">
      <PublicHeader />
    </el-header>
    <el-main class="public-main">
      <router-view />
    </el-main>
    <el-footer class="public-footer-wrapper">
      <PublicFooter />
    </el-footer>
  </el-container>
</template>

<script setup lang="ts">
import PublicHeader from '@/components/public/PublicHeader.vue'
import PublicFooter from '@/components/public/PublicFooter.vue'
</script>

<style scoped>
.public-layout {
  min-height: 100vh;
  background: #f5f7fa;
  display: flex;
  flex-direction: column;
}

.public-header-wrapper {
  background: #ffffff;
  border-bottom: 1px solid #e4e7ed;
  padding: 0;
  height: 56px;
  box-shadow: 0 1px 4px rgba(0, 0, 0, 0.04);
}

.public-main {
  flex: 1;
  max-width: 1200px;
  width: 100%;
  margin: 0 auto;
  padding: 24px 20px;
}

.public-footer-wrapper {
  background: #ffffff;
  border-top: 1px solid #e4e7ed;
  height: auto;
  padding: 0;
}
</style>
