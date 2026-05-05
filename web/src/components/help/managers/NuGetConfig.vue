<template>
  <div class="nuget-config">
    <h3>NuGet 配置指南</h3>
    <el-alert type="info" :closable="false" style="margin-bottom: 20px">
      NuGet 是 .NET 的包管理器
    </el-alert>
    <p>配置详情请查看 <a href="/docs/client-configuration.md#nuget-配置" target="_blank">完整配置文档</a></p>
    <el-button type="primary" @click="downloadTemplate('NuGet.Config')">
      下载 NuGet.Config 模板
    </el-button>
  </div>
</template>

<script setup lang="ts">
const downloadTemplate = (filename: string) => {
  window.open(`/docs/templates/${filename}`, '_blank')
}
</script>
