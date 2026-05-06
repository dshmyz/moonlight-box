<template>
  <div class="pypi-config">
    <el-alert type="info" :closable="false">
      PyPI 是 Python 的包索引和依赖管理工具，用于安装、发布和管理 Python 包
    </el-alert>
    <div class="config-content">
      <div class="content-row">
        <div class="content-text">
          <h4>快速开始</h4>
          <p>下载配置模板并查看完整文档，快速完成 PyPI 仓库配置</p>
        </div>
        <div class="content-actions">
          <el-button type="primary" @click="downloadTemplate('pip.conf')">
            <i class="fa-solid fa-download"></i>
            下载 pip.conf
          </el-button>
          <el-button @click="openDocs">
            <i class="fa-solid fa-book"></i>
            查看完整文档
          </el-button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useRouter } from 'vue-router'

const router = useRouter()

const downloadTemplate = (filename: string) => {
  window.open(`/docs/templates/${filename}`, '_blank')
}

const openDocs = () => {
  router.push({ name: 'DocsViewer', params: { doc: 'client-configuration.md' } })
}
</script>

<style scoped>
.pypi-config {
  padding: 0;
  width: 100%;
}

.pypi-config :deep(.el-alert) {
  border-radius: 8px;
  padding: 12px 16px;
  font-size: 13px;
  margin: 20px 0;
}

.config-content {
  background: #fafbfc;
  border-radius: 8px;
  border: 1px solid #e5e7eb;
  padding: 24px;
}

.content-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 20px;
}

.content-text {
  flex: 1;
}

.content-text h4 {
  margin: 0 0 6px;
  font-size: 15px;
  font-weight: 600;
  color: #0f172a;
}

.content-text p {
  margin: 0;
  color: #6b7280;
  font-size: 13px;
  line-height: 1.5;
}

.content-actions {
  display: flex;
  gap: 10px;
  flex-shrink: 0;
}

.content-actions :deep(.el-button) {
  border-radius: 6px;
  padding: 8px 16px;
  font-weight: 500;
  font-size: 13px;
}

@media (max-width: 768px) {
  .content-row {
    flex-direction: column;
    align-items: flex-start;
  }

  .content-actions {
    width: 100%;
  }

  .content-actions :deep(.el-button) {
    flex: 1;
  }
}
</style>
