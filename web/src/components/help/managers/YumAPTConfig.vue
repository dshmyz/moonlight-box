<template>
  <div class="yum-apt-config">
    <el-alert type="info" :closable="false">
      Yum 和 APT 是 Linux 系统的包管理器，用于安装、更新和管理系统软件包
    </el-alert>
    <div class="config-content">
      <div class="config-section">
        <div class="section-row">
          <div class="section-text">
            <h4>Yum 配置</h4>
            <p>下载配置文件并查看完整文档，快速完成 Yum 仓库配置</p>
          </div>
          <div class="section-actions">
            <el-button type="primary" @click="downloadTemplate('moonlight.repo')">
              <i class="fa-solid fa-download"></i>
              下载 Yum 配置
            </el-button>
            <el-button @click="openYumDocs">
              <i class="fa-solid fa-book"></i>
              查看文档
            </el-button>
          </div>
        </div>
      </div>
      
      <el-divider />
      
      <div class="config-section">
        <div class="section-row">
          <div class="section-text">
            <h4>APT 配置</h4>
            <p>下载配置文件并查看完整文档，快速完成 APT 仓库配置</p>
          </div>
          <div class="section-actions">
            <el-button type="primary" @click="downloadTemplate('moonlight.list')">
              <i class="fa-solid fa-download"></i>
              下载 APT 配置
            </el-button>
            <el-button @click="openAptDocs">
              <i class="fa-solid fa-book"></i>
              查看文档
            </el-button>
          </div>
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

const openYumDocs = () => {
  router.push({ name: 'DocsViewer', params: { doc: 'client-configuration.md' } })
}

const openAptDocs = () => {
  router.push({ name: 'DocsViewer', params: { doc: 'client-configuration.md' } })
}
</script>

<style scoped>
.yum-apt-config {
  padding: 0;
  width: 100%;
}

.yum-apt-config :deep(.el-alert) {
  border-radius: 8px;
  padding: 12px 16px;
  font-size: 13px;
  margin: 20px 0;
}

.config-content {
  padding: 24px;
  background: #fafbfc;
  border-radius: 8px;
  border: 1px solid #e5e7eb;
}

.config-section {
  padding: 6px 0;
}

.section-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 20px;
}

.section-text {
  flex: 1;
}

.section-text h4 {
  margin: 0 0 6px;
  font-size: 15px;
  font-weight: 600;
  color: #0f172a;
}

.section-text p {
  margin: 0;
  color: #6b7280;
  font-size: 13px;
  line-height: 1.5;
}

.section-actions {
  display: flex;
  gap: 10px;
  flex-shrink: 0;
}

.section-actions :deep(.el-button) {
  border-radius: 6px;
  padding: 8px 16px;
  font-weight: 500;
  font-size: 13px;
}

.config-content :deep(.el-divider) {
  margin: 20px 0;
  border-color: #e5e7eb;
}

@media (max-width: 768px) {
  .section-row {
    flex-direction: column;
    align-items: flex-start;
  }

  .section-actions {
    width: 100%;
  }

  .section-actions :deep(.el-button) {
    flex: 1;
  }
}
</style>
