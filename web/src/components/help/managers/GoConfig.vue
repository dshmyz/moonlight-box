<template>
  <div class="go-config">
    <h3>Go 配置指南</h3>
    <el-alert type="info" :closable="false" style="margin-bottom: 20px">
      Go modules 是 Go 语言的依赖管理系统
    </el-alert>
    <p>配置详情请查看 <a href="/docs/client-configuration.md#go-配置" target="_blank">完整配置文档</a></p>
    <el-button type="primary" @click="downloadTemplate('go-env.sh')">
      下载环境变量脚本
    </el-button>
  </div>
</template>

<script setup lang="ts">
const downloadTemplate = (filename: string) => {
  window.open(`/docs/templates/${filename}`, '_blank')
}
</script>
