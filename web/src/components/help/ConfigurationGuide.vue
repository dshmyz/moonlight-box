<template>
  <div class="configuration-guide">
    <h2>📖 配置指南</h2>

    <el-alert
      type="info"
      :closable="false"
      show-icon
      style="margin-bottom: 20px"
    >
      选择您的包管理器查看详细配置说明
    </el-alert>

    <el-tabs v-model="activeManager" type="border-card">
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

      <el-tab-pane label="NuGet" name="nuget">
        <NuGetConfig />
      </el-tab-pane>

      <el-tab-pane label="Yum/APT" name="yum">
        <YumAPTConfig />
      </el-tab-pane>
    </el-tabs>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import NPMConfig from '@/components/help/managers/NPMConfig.vue'
import MavenConfig from '@/components/help/managers/MavenConfig.vue'
import PyPIConfig from '@/components/help/managers/PyPIConfig.vue'
import GoConfig from '@/components/help/managers/GoConfig.vue'
import NuGetConfig from '@/components/help/managers/NuGetConfig.vue'
import YumAPTConfig from '@/components/help/managers/YumAPTConfig.vue'

const activeManager = ref('npm')
</script>

<style scoped>
.configuration-guide {
  padding: 20px;
}

.configuration-guide h2 {
  margin-bottom: 20px;
}
</style>
