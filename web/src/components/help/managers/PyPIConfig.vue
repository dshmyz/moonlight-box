<template>
  <div class="pypi-config">
    <h3>PyPI 配置指南</h3>
    <el-alert type="info" :closable="false" style="margin-bottom: 20px">
      PyPI 是 Python 的包索引和依赖管理工具
    </el-alert>
    <p>配置详情请查看 <a href="/docs/client-configuration.md#pypi-配置" target="_blank">完整配置文档</a></p>
    <el-button type="primary" @click="downloadTemplate('pip.conf')">
      下载 pip.conf 模板
    </el-button>
  </div>
</template>

<script setup lang="ts">
const downloadTemplate = (filename: string) => {
  window.open(`/docs/templates/${filename}`, '_blank')
}
</script>
