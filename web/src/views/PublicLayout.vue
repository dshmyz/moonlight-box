<template>
  <el-container class="public-layout" :class="themeClass">
    <div class="layout-noise" />
    <el-header class="public-header-wrapper">
      <PublicHeader />
    </el-header>
    <el-main class="public-main">
      <router-view />
    </el-main>
    <el-footer class="public-footer-wrapper">
      <PublicFooter />
    </el-footer>
    <AIAssistant />
  </el-container>
</template>

<script setup lang="ts">
import PublicHeader from '@/components/public/PublicHeader.vue'
import PublicFooter from '@/components/public/PublicFooter.vue'
import AIAssistant from '@/components/ai/AIAssistant.vue'
import { useLunarTheme } from '@/composables/useLunarTheme'

const { themeClass } = useLunarTheme()
</script>

<style scoped>
.public-layout {
  min-height: 100vh;
  background: var(--lunar-bg-deep);
  display: flex;
  flex-direction: column;
  position: relative;
}

.layout-noise {
  position: fixed;
  inset: 0;
  opacity: 0.03;
  pointer-events: none;
  z-index: 9999;
  background-image: url("data:image/svg+xml,%3Csvg viewBox='0 0 256 256' xmlns='http://www.w3.org/2000/svg'%3E%3Cfilter id='noise'%3E%3CfeTurbulence type='fractalNoise' baseFrequency='0.9' numOctaves='4' stitchTiles='stitch'/%3E%3C/filter%3E%3Crect width='100%25' height='100%25' filter='url(%23noise)'/%3E%3C/svg%3E");
  background-repeat: repeat;
  background-size: 256px;
  mix-blend-mode: overlay;
  transition: opacity 1s ease;
}

.public-layout.lunar-dawn .layout-noise {
  opacity: 0.02;
}

.public-header-wrapper {
  background: var(--lunar-bg-surface);
  border-bottom: 1px solid var(--lunar-border);
  padding: 0;
  height: 56px;
  position: sticky;
  top: 0;
  z-index: 100;
}

.public-main {
  flex: 1;
  max-width: 1200px;
  width: 100%;
  margin: 0 auto;
  padding: 24px 20px;
  background: transparent;
}

.public-footer-wrapper {
  background: var(--lunar-bg-surface);
  border-top: 1px solid var(--lunar-border);
  height: auto;
  padding: 0;
}
</style>