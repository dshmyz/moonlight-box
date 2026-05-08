<template>
  <div class="configuration-guide">
    <div class="section-header">
      <h2>配置指南</h2>
      <p class="section-desc">选择您的包管理器，查看详细的配置说明和示例</p>
    </div>

    <div class="manager-tabs">
      <el-tabs v-model="activeManager" type="card" class="guide-tabs">
        <el-tab-pane label="NPM" name="npm">
          <NPMConfig />
        </el-tab-pane>

        <el-tab-pane label="Maven" name="maven">
          <MavenConfig />
        </el-tab-pane>

        <el-tab-pane label="PyPI" name="pypi">
          <PyPIConfig />
        </el-tab-pane>

        <el-tab-pane label="Go" name="go">
          <GoConfig />
        </el-tab-pane>

        <el-tab-pane label="Yum/APT" name="yum">
          <YumAPTConfig />
        </el-tab-pane>
      </el-tabs>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import NPMConfig from '@/components/help/managers/NPMConfig.vue'
import MavenConfig from '@/components/help/managers/MavenConfig.vue'
import PyPIConfig from '@/components/help/managers/PyPIConfig.vue'
import GoConfig from '@/components/help/managers/GoConfig.vue'
import YumAPTConfig from '@/components/help/managers/YumAPTConfig.vue'

const activeManager = ref('npm')
</script>

<style scoped>
.configuration-guide {
  padding: 0;
}

.section-header {
  padding: 32px 40px 24px;
}

.section-header h2 {
  font-size: 24px;
  font-weight: 600;
  color: #0f172a;
  margin: 0 0 8px;
}

.section-desc {
  color: #6b7280;
  font-size: 14px;
  line-height: 1.6;
  margin: 0;
}

.manager-tabs {
  background: #fff;
}

.guide-tabs {
  margin: 0;
}

.guide-tabs :deep(.el-tabs__header) {
  background: #fafbfc;
  border-bottom: 1px solid #e5e7eb;
  margin: 0;
  padding: 0 40px;
}

.guide-tabs :deep(.el-tabs__nav-wrap::after) {
  display: none;
}

.guide-tabs :deep(.el-tabs__item) {
  font-size: 14px;
  font-weight: 500;
  color: #6b7280;
  padding: 14px 20px;
  margin-right: 8px;
  border-radius: 6px 6px 0 0;
  transition: all 0.2s ease;
}

.guide-tabs :deep(.el-tabs__item:hover) {
  color: #2563eb;
}

.guide-tabs :deep(.el-tabs__item.is-active) {
  color: #2563eb;
  font-weight: 600;
  background: #fff;
  border-bottom-color: #fff;
}

.guide-tabs :deep(.el-tabs__content) {
  padding: 24px 0;
}

.guide-tabs :deep(.el-tab-pane) {
  padding: 0 40px;
  max-width: 100%;
}
</style>
