<template>
  <el-card class="package-info-card">
    <template #header>
      <span class="card-title">包信息</span>
    </template>

    <el-descriptions :column="1" border size="small">
      <el-descriptions-item label="类型">
        <el-tag :type="getTypeColor(pkg.type)" size="small">
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
          <el-tag :type="getStatusType(activeVersion.status)" size="small">
            {{ getStatusLabel(activeVersion.status) }}
          </el-tag>
        </el-descriptions-item>
        <el-descriptions-item label="发布时间">{{ activeVersion.published_at || '-' }}</el-descriptions-item>
        <el-descriptions-item label="大小">{{ formatSize(activeVersion.size) }}</el-descriptions-item>
        <el-descriptions-item label="下载量">{{ formatNumber(activeVersion.downloads) }}</el-descriptions-item>
        <el-descriptions-item label="SHA256">
          <span class="checksum-text" @click="copyText(activeVersion.checksum)">{{ activeVersion.checksum }}</span>
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

const props = defineProps<{
  pkg: {
    name: string
    type: string
    latest_version?: string
    download_count?: number
    repository?: string
    license?: string
  }
  versions: Array<{
    version: string
    published_at: string
    downloads: number
    is_latest?: boolean
    size: number
    checksum: string
    status: string
  }>
  selectedVersion: string
}>()

const activeVersion = computed(() => {
  return props.versions.find(v => v.version === props.selectedVersion) || null
})

function normalizeType(type: string) {
  return type === 'maven' ? 'maven2' : type
}

const registryUrl = computed(() => {
  const base = `${window.location.origin}/api/v1`
  const repo = props.pkg.repository || 'default'
  switch (normalizeType(props.pkg.type)) {
    case 'npm':
      return `${base}/repository/${repo}/`
    case 'pypi':
      return `${base}/repository/${repo}/simple`
    case 'nuget':
      return `${base}/repository/${repo}/v3/index.json`
    default:
      return `${base}/repository/${repo}/`
  }
})

const configCommand = computed(() => {
  const url = registryUrl.value
  const repo = props.pkg.repository || 'default'
  switch (normalizeType(props.pkg.type)) {
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

function getTypeColor(type: string) {
  const colors: Record<string, string> = {
    npm: '',
    maven2: 'success',
    pypi: 'warning',
    go: 'info',
  }
  return colors[normalizeType(type)] || 'info'
}

function getPackageTypeLabel(type: string) {
  const labels: Record<string, string> = {
    npm: 'npm',
    maven2: 'Maven',
    pypi: 'PyPI',
    go: 'Go',
  }
  return labels[normalizeType(type)] || type
}

function getStatusType(status: string) {
  const map: Record<string, string> = {
    published: 'success',
    deprecated: 'warning',
    yanked: 'danger',
    draft: 'info',
  }
  return map[status] || 'info'
}

function getStatusLabel(status: string) {
  const map: Record<string, string> = {
    published: '已发布',
    deprecated: '已弃用',
    yanked: '已撤回',
    draft: '草稿',
  }
  return map[status] || status
}

function formatNumber(num: number) {
  if (num >= 1000000) return `${(num / 1000000).toFixed(1)}M`
  if (num >= 1000) return `${(num / 1000).toFixed(1)}K`
  return String(num)
}

function formatSize(bytes: number) {
  if (bytes >= 1048576) return `${(bytes / 1048576).toFixed(1)} MB`
  if (bytes >= 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${bytes} B`
}

async function copyText(text: string) {
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
