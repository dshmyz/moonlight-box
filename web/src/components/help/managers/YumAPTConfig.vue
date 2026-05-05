<template>
  <div class="yum-apt-config">
    <h3>Yum/APT 配置指南</h3>
    <el-alert type="info" :closable="false" style="margin-bottom: 20px">
      Yum 和 APT 是 Linux 系统的包管理器
    </el-alert>
    <p>配置详情请查看 <a href="/docs/client-configuration.md#yum-配置" target="_blank">Yum 配置文档</a></p>
    <p>配置详情请查看 <a href="/docs/client-configuration.md#apt-配置" target="_blank">APT 配置文档</a></p>
    <el-button type="primary" @click="downloadTemplate('moonlight.repo')">
      下载 Yum 配置
    </el-button>
    <el-button type="primary" @click="downloadTemplate('moonlight.list')">
      下载 APT 配置
    </el-button>
  </div>
</template>

<script setup lang="ts">
const downloadTemplate = (filename: string) => {
  window.open(`/docs/templates/${filename}`, '_blank')
}
</script>
