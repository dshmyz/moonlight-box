<template>
  <el-card class="package-info-card">
    <template #header>
      <span class="card-title">包信息</span>
    </template>

    <el-descriptions :column="1" border size="small">
      <el-descriptions-item label="类型">
        <el-tag :type="getPackageTypeColor(pkg.type)" size="small">
          {{ getPackageTypeLabel(pkg.type) }}
        </el-tag>
      </el-descriptions-item>
      <el-descriptions-item label="最新版本">{{ pkg.latest_version || '-' }}</el-descriptions-item>
      <el-descriptions-item label="总下载量">{{ formatNumber(pkg.download_count || 0) }}</el-descriptions-item>
      <el-descriptions-item label="仓库">{{ pkg.repository || '-' }}</el-descriptions-item>
      <el-descriptions-item label="许可证">{{ pkg.license || '-' }}</el-descriptions-item>
    </el-descriptions>

    <div v-if="activeVersion" class="version-detail">
      <h4 class="section-title">
        版本详情
        <el-tag type="primary" size="small" effect="plain" class="version-tag">
          {{ activeVersion.version }}
        </el-tag>
      </h4>
      <el-descriptions :column="1" border size="small">
        <el-descriptions-item label="状态">
          <el-tag :type="getVersionStatusColor(activeVersion.status)" size="small">
            {{ getVersionStatusLabel(activeVersion.status) }}
          </el-tag>
        </el-descriptions-item>
        <el-descriptions-item label="发布时间">{{ formatDate(activeVersion.published_at) }}</el-descriptions-item>
        <el-descriptions-item label="大小">{{ formatSize(activeVersion.size_bytes) }}</el-descriptions-item>
        <el-descriptions-item label="下载量">{{ formatNumber(activeVersion.download_count) }}</el-descriptions-item>
        <el-descriptions-item label="SHA256">
          <span class="checksum-text" @click="copyText(activeVersion.checksum_sha256 || '')">{{ activeVersion.checksum_sha256 || '-' }}</span>
        </el-descriptions-item>
      </el-descriptions>
    </div>

    <div class="repo-config-section">
      <h4 class="section-title">仓库配置</h4>
      <div class="config-item">
        <span class="config-label">Registry URL</span>
        <code class="config-value" @click="copyText(registryUrl)">{{ registryUrl }}</code>
      </div>
      <div class="config-item">
        <span class="config-label">配置命令</span>
        <code class="config-value" @click="copyText(configCommand)">{{ configCommand }}</code>
      </div>
    </div>
  </el-card>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { ElMessage } from 'element-plus'
import { formatNumber, formatSize, formatDate } from '@/utils/format'
import { getPackageTypeColor, getPackageTypeLabel, normalizePackageType, getVersionStatusColor, getVersionStatusLabel } from '@/constants/package'
import type { PackageVersion } from '@/api/package'

const props = defineProps<{
  pkg: {
    name: string
    type: string
    latest_version?: string
    download_count?: number
    repository?: string
    license?: string
  }
  versions: PackageVersion[]
  selectedVersion: string
}>()

const activeVersion = computed(() => {
  return props.versions.find(v => v.version === props.selectedVersion) || null
})

const registryUrl = computed(() => {
  const base = window.location.origin
  const repo = props.pkg.repository || 'default'
  switch (normalizePackageType(props.pkg.type)) {
    case 'npm':
      return `${base}/repo/${repo}/`
    case 'pypi':
      return `${base}/repo/${repo}/simple`
    case 'nuget':
      return `${base}/repo/${repo}/v3/index.json`
    default:
      return `${base}/repo/${repo}/`
  }
})

const configCommand = computed(() => {
  const url = registryUrl.value
  const repo = props.pkg.repository || 'default'
  switch (normalizePackageType(props.pkg.type)) {
    case 'npm':
      return `npm config set registry ${url}`
    case 'pypi':
      return `pip config set global.index-url ${url}`
    case 'maven2':
      return url
    case 'go':
      return `GOPROXY=${url} go mod tidy`
    case 'nuget':
      return `dotnet nuget add source ${url} -n ${repo}`
    default:
      return url
  }
})

async function copyText(text: string) {
  if (!text) return
  try {
    await navigator.clipboard.writeText(text)
    ElMessage.success('已复制到剪贴板')
  } catch {
    ElMessage.error('复制失败')
  }
}
</script>

<style scoped>
.package-info-card {
  position: sticky;
  top: 72px;
}

.card-title {
  font-size: 16px;
  font-weight: 600;
  color: #303133;
}

.version-detail {
  margin-top: 16px;
  padding-top: 16px;
  border-top: 1px solid #e4e7ed;
}

.section-title {
  font-size: 14px;
  font-weight: 600;
  color: #303133;
  margin: 0 0 12px;
  display: flex;
  align-items: center;
  gap: 8px;
}

.version-tag {
  font-family: 'SF Mono', Monaco, 'Cascadia Code', monospace;
}

.checksum-text {
  font-family: 'SF Mono', Monaco, 'Cascadia Code', monospace;
  font-size: 12px;
  color: #909399;
  cursor: pointer;
  word-break: break-all;
}

.checksum-text:hover {
  color: #409eff;
}

.repo-config-section {
  margin-top: 16px;
  padding-top: 16px;
  border-top: 1px solid #e4e7ed;
}

.config-item {
  margin-bottom: 10px;
}

.config-item:last-child {
  margin-bottom: 0;
}

.config-label {
  display: block;
  font-size: 12px;
  color: #909399;
  margin-bottom: 4px;
}

.config-value {
  display: block;
  font-family: 'SF Mono', Monaco, 'Cascadia Code', monospace;
  font-size: 12px;
  color: #409eff;
  background: #f5f7fa;
  border-radius: 4px;
  padding: 6px 10px;
  word-break: break-all;
  cursor: pointer;
  transition: background 0.2s;
}

.config-value:hover {
  background: #ecf5ff;
}
</style>
