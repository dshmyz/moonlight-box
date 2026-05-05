<template>
  <div class="help-page">
    <el-page-header @back="goBack" content="帮助中心" />

    <div class="help-content">
      <el-tabs v-model="activeTab">
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
.help-page {
  padding: var(--spacing-5);
}

.help-content {
  margin-top: 20px;
  background: #fff;
  padding: var(--spacing-5);
  border-radius: 4px;
}
</style>
