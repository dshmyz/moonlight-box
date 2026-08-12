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
      <el-descriptions-item label="仓库">{{ pkg.group_repository || pkg.repository || '-' }}</el-descriptions-item>
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
        <el-descriptions-item label="版本许可证">{{ activeVersion.license || '-' }}</el-descriptions-item>
        <el-descriptions-item label="发布时间">{{ formatDate(activeVersion.published_at) }}</el-descriptions-item>
        <el-descriptions-item label="大小">{{ formatSize(activeVersion.size_bytes) }}</el-descriptions-item>
        <el-descriptions-item label="下载量">{{ formatNumber(activeVersion.download_count) }}</el-descriptions-item>
        <el-descriptions-item label="触发回源IP">{{ activeVersion.trigger_ip || '-' }}</el-descriptions-item>
        <el-descriptions-item label="SHA256">
          <span class="checksum-text" @click="copyText(activeVersion.checksum_sha256 || '')">{{ activeVersion.checksum_sha256 || '-' }}</span>
        </el-descriptions-item>
      </el-descriptions>
      <div v-if="protocolFields.length > 0" class="protocol-fields">
        <span v-for="field in protocolFields" :key="field.key" class="protocol-field">
          <span class="protocol-key">{{ field.key }}</span>
          <span class="protocol-value">{{ field.value }}</span>
        </span>
      </div>
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
import { formatNumber, formatSize, formatDate } from '@/utils/format'
import { getPackageTypeColor, getPackageTypeLabel, normalizePackageType, getVersionStatusColor, getVersionStatusLabel } from '@/constants/package'
import { copyToClipboard } from '@/utils/clipboard'
import type { PackageVersion } from '@/api/package'

const props = defineProps<{
  pkg: {
    name: string
    type: string
    latest_version?: string
    download_count?: number
    repository?: string
    group_repository?: string
    license?: string
  }
  versions: PackageVersion[]
  selectedVersion: string
}>()

const activeVersion = computed(() => {
  return props.versions.find(v => v.version === props.selectedVersion) || null
})

const protocolFields = computed(() => {
  const version = activeVersion.value
  if (!version) return []

  const skipped = new Set(['license', 'description', 'published_at'])
  const fields: Array<{ key: string; value: string }> = []
  const addFields = (source?: Record<string, unknown>) => {
    if (!source) return
    Object.entries(source).forEach(([key, value]) => {
      if (skipped.has(key) || value === undefined || value === null || value === '') return
      if (typeof value === 'object') return
      fields.push({ key, value: String(value) })
    })
  }

  addFields(version.qualifiers)
  addFields(version.attributes)
  return fields.slice(0, 8)
})

const registryUrl = computed(() => {
  const base = window.location.origin
  const repo = props.pkg.group_repository || props.pkg.repository || 'default'
  switch (normalizePackageType(props.pkg.type)) {
    case 'npm':
      return `${base}/repository/${repo}/`
    case 'pypi':
      return `${base}/repository/${repo}/simple`
    default:
      return `${base}/repository/${repo}/`
  }
})

const configCommand = computed(() => {
  const url = registryUrl.value
  switch (normalizePackageType(props.pkg.type)) {
    case 'npm':
      return `npm config set registry ${url}`
    case 'pypi':
      return `pip config set global.index-url ${url}`
    case 'maven2':
      return url
    case 'go':
      return `GOPROXY=${url} go mod tidy`
    default:
      return url
  }
})

function copyText(text: string) {
  if (!text) return
  copyToClipboard(text)
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
  color: var(--lunar-silver);
}

.version-detail {
  margin-top: 16px;
  padding-top: 16px;
  border-top: 1px solid var(--lunar-border);
}

.section-title {
  font-size: 14px;
  font-weight: 600;
  color: var(--lunar-silver);
  margin: 0 0 12px;
  display: flex;
  align-items: center;
  gap: 8px;
}

.version-tag {
  font-family: var(--font-family-mono);
}

.checksum-text {
  font-family: var(--font-family-mono);
  font-size: 12px;
  color: var(--lunar-silver-dim);
  cursor: pointer;
  word-break: break-all;
}

.checksum-text:hover {
  color: var(--lunar-accent);
}

.protocol-fields {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin-top: 10px;
}

.protocol-field {
  max-width: 100%;
  border: 1px solid var(--lunar-border);
  border-radius: 4px;
  padding: 4px 6px;
  font-size: 12px;
  line-height: 1.4;
  background: var(--lunar-bg-glass);
}

.protocol-key {
  color: var(--lunar-silver-dim);
  margin-right: 4px;
}

.protocol-value {
  color: var(--lunar-silver);
  word-break: break-word;
}

.repo-config-section {
  margin-top: 16px;
  padding-top: 16px;
  border-top: 1px solid var(--lunar-border);
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
  color: var(--lunar-silver-dim);
  margin-bottom: 4px;
}

.config-value {
  display: block;
  font-family: var(--font-family-mono);
  font-size: 12px;
  color: var(--lunar-accent);
  background: var(--lunar-bg-glass);
  border: 1px solid var(--lunar-border);
  border-radius: 4px;
  padding: 6px 10px;
  word-break: break-all;
  cursor: pointer;
  transition: background 0.2s;
}

.config-value:hover {
  background: rgba(196, 181, 253, 0.12);
}
</style>
