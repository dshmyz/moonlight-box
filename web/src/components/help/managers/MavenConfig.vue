<template>
  <div class="maven-config">
    <h3>Maven 配置指南</h3>
    <el-alert type="info" :closable="false" style="margin-bottom: 20px">
      Maven 是 Java 项目的构建和依赖管理工具
    </el-alert>
    <p>配置详情请查看 <a href="/docs/client-configuration.md#maven-配置" target="_blank">完整配置文档</a></p>
    <el-button type="primary" @click="downloadTemplate('settings.xml')">
      下载 settings.xml 模板
    </el-button>
  </div>
</template>

<script setup lang="ts">
const downloadTemplate = (filename: string) => {
  window.open(`/docs/templates/${filename}`, '_blank')
}
</script>
