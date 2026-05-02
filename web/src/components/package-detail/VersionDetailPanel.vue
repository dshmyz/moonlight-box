<template>
  <div v-if="version" class="version-detail-panel">
    <div class="panel-header">
      <h4>版本详情: {{ version.version }}</h4>
      <el-button link @click="$emit('close')">
        <el-icon><Close /></el-icon>
      </el-button>
    </div>

    <div class="panel-content">
      <div class="info-cards">
        <div class="info-card">
          <div class="label">发布时间</div>
          <div class="value">{{ formatDate(version.published_at) }}</div>
        </div>
        <div class="info-card">
          <div class="label">下载量</div>
          <div class="value">{{ formatNumber(version.download_count) }}</div>
        </div>
      </div>

      <div class="section">
        <h5>快速操作</h5>
        <div class="quick-actions">
          <el-button 
            size="small" 
            :disabled="!getInstallCommand()"
            :loading="copying"
            @click="copyInstallCommand"
          >
            复制安装命令
          </el-button>
        </div>
      </div>

      <div v-if="version.dependencies && version.dependencies.length > 0" class="section">
        <h5>依赖</h5>
        <div class="dependency-list">
          <div v-for="dep in version.dependencies" :key="dep.name" class="dependency-item">
            <span class="name">{{ dep.name }}</span>
            <span class="version">{{ dep.version }}</span>
          </div>
        </div>
      </div>

      <div class="section">
        <h5>文件列表</h5>
        <div class="file-list">
          <div v-for="file in version.files" :key="file.id" class="file-item">
            <span class="filename">{{ file.filename }}</span>
            <span class="size">{{ formatSize(file.size_bytes) }}</span>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { Close } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import { formatNumber, formatSize, formatDate } from '@/utils/format'
import type { PackageVersion, Package } from '@/api/package'

const props = defineProps<{
  version: PackageVersion | null
  pkg: Package | null
}>()

defineEmits<{
  close: []
}>()

const copying = ref(false)

async function copyInstallCommand() {
  const command = getInstallCommand()
  if (!command) {
    ElMessage.warning('无法生成安装命令')
    return
  }
  
  copying.value = true
  try {
    if (!navigator.clipboard) {
      const textarea = document.createElement('textarea')
      textarea.value = command
      document.body.appendChild(textarea)
      textarea.select()
      document.execCommand('copy')
      document.body.removeChild(textarea)
      ElMessage.success('安装命令已复制')
      return
    }
    
    await navigator.clipboard.writeText(command)
    ElMessage.success('安装命令已复制')
  } catch (error) {
    console.error('Failed to copy:', error)
    ElMessage.error('复制失败，请手动复制')
  } finally {
    copying.value = false
  }
}

function getInstallCommand(): string {
  if (!props.version || !props.pkg) return ''
  
  const { type, name } = props.pkg
  const { version } = props.version
  
  switch (type) {
    case 'npm':
      return `npm install ${name}@${version}`
    case 'maven': {
      const parts = name.split(':')
      if (parts.length === 2 && parts[0].trim() && parts[1].trim()) {
        return `<dependency>
  <groupId>${parts[0].trim()}</groupId>
  <artifactId>${parts[1].trim()}</artifactId>
  <version>${version}</version>
</dependency>`
      }
      return ''
    }
    case 'pypi':
      return `pip install ${name}==${version}`
    case 'go':
      return `go get ${name}@${version}`
    default:
      return ''
  }
}
</script>

<style scoped>
.version-detail-panel {
  background: #fff;
  border: 2px solid #ff9800;
  border-radius: 8px;
  padding: 16px;
  height: fit-content;
}

.panel-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 16px;
}

.panel-header h4 {
  margin: 0;
  font-size: 16px;
}

.info-cards {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 8px;
  margin-bottom: 16px;
}

.info-card {
  background: #f5f5f5;
  padding: 12px;
  border-radius: 4px;
}

.info-card .label {
  font-size: 12px;
  color: #666;
  margin-bottom: 4px;
}

.info-card .value {
  font-size: 14px;
  font-weight: 500;
}

.section {
  margin-bottom: 16px;
}

.section h5 {
  margin: 0 0 8px 0;
  font-size: 14px;
}

.quick-actions {
  display: flex;
  gap: 8px;
}

.dependency-list {
  background: #f5f5f5;
  border-radius: 4px;
  padding: 8px;
}

.dependency-item {
  display: flex;
  justify-content: space-between;
  padding: 4px 8px;
  font-size: 13px;
}

.dependency-item .version {
  color: #666;
}

.file-list {
  background: #f5f5f5;
  border-radius: 4px;
  padding: 8px;
}

.file-item {
  display: flex;
  justify-content: space-between;
  padding: 8px;
  border-bottom: 1px solid #e0e0e0;
}

.file-item:last-child {
  border-bottom: none;
}

.filename {
  font-size: 13px;
}

.size {
  font-size: 12px;
  color: #666;
}
</style>
