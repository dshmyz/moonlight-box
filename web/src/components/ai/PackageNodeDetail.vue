<template>
  <el-dialog
    v-model="dialogVisible"
    :title="dialogTitle"
    width="520px"
    :close-on-click-modal="true"
    destroy-on-close
    class="package-node-dialog"
  >
    <div v-if="loading" class="detail-loading">
      <el-icon class="is-loading"><Loading /></el-icon>
      <span>加载包详情...</span>
    </div>
    
    <div v-else-if="error" class="detail-error">
      <el-icon><Warning /></el-icon>
      <span>{{ error }}</span>
    </div>
    
    <div v-else class="detail-content">
      <div v-if="navigationStack.length > 0" class="detail-nav">
        <el-button size="small" text @click="goBack">返回上一级</el-button>
        <span class="detail-nav-path">{{ currentNode.name }}</span>
      </div>

      <!-- 基本信息 -->
      <el-descriptions :column="1" border size="small">
        <el-descriptions-item label="类型">
          <el-tag size="small" :type="getPackageTypeTag(pkgInfo?.type)">{{ pkgInfo?.type }}</el-tag>
        </el-descriptions-item>
        <el-descriptions-item label="当前版本">{{ pkgInfo?.latestVersion || '-' }}</el-descriptions-item>
        <el-descriptions-item label="许可证">
          <el-tag v-if="pkgInfo?.license" size="small" effect="plain">{{ pkgInfo.license }}</el-tag>
          <span v-else>-</span>
        </el-descriptions-item>
        <el-descriptions-item label="下载量">{{ pkgInfo?.downloadCount?.toLocaleString() || 0 }}</el-descriptions-item>
      </el-descriptions>
      
      <!-- 描述 -->
      <div v-if="pkgInfo?.description" class="detail-section">
        <div class="section-title">📄 描述</div>
        <p class="description">{{ pkgInfo.description }}</p>
      </div>
      
      <!-- 安全状态 -->
      <div class="detail-section">
        <div class="section-title">🔒 安全状态</div>
        <el-alert
          v-if="!security"
          title="ℹ️ 暂无扫描结果"
          type="info"
          :closable="false"
          show-icon
        />
        <el-alert
          v-else-if="security.totalVulnerabilities === 0"
          title="✅ 未发现已知漏洞"
          type="success"
          :closable="false"
          show-icon
        />
        <el-alert
          v-else-if="security.criticalCount || security.highCount"
          :title="`⚠️ 发现 ${security.totalVulnerabilities} 个漏洞`"
          :description="`严重: ${security.criticalCount}, 高危: ${security.highCount}, 中危: ${security.mediumCount}, 低危: ${security.lowCount}`"
          type="error"
          :closable="false"
          show-icon
        />
        <el-alert
          v-else
          :title="`ℹ️ 发现 ${security?.totalVulnerabilities} 个低危漏洞`"
          type="warning"
          :closable="false"
          show-icon
        />
      </div>
      
      <!-- 版本历史 -->
      <div v-if="versions?.length" class="detail-section">
        <div class="section-title">📜 版本历史</div>
        <el-table :data="versions.slice(0, 5)" size="small" :show-header="false">
          <el-table-column prop="version" label="版本" width="100" />
          <el-table-column prop="publishedAt" label="发布时间" width="120">
            <template #default="{ row }">{{ formatDate(row.publishedAt) }}</template>
          </el-table-column>
          <el-table-column prop="downloadCount" label="下载">
            <template #default="{ row }">{{ row.downloadCount?.toLocaleString() }}</template>
          </el-table-column>
        </el-table>
        <div v-if="versions.length > 5" class="show-more">
          <el-button link size="small">显示更多版本 (共 {{ versions.length }})</el-button>
        </div>
      </div>
      
      <!-- 依赖关系 -->
      <div v-if="dependencies?.length" class="detail-section">
        <div class="section-title">🔗 直接依赖 ({{ dependencies.length }})</div>
        <div class="dependency-list">
          <el-tag
            v-for="dep in dependencies.slice(0, 8)"
            :key="dep.name"
            size="small"
            class="dep-tag"
            @click="handleDepClick(dep)"
          >
            {{ dep.name }}@{{ dep.version }}
          </el-tag>
          <el-tag v-if="dependencies.length > 8" size="small" class="dep-tag more">
            +{{ dependencies.length - 8 }} 更多
          </el-tag>
        </div>
      </div>
    </div>
    
    <template #footer>
      <div class="dialog-footer">
        <el-button @click="dialogVisible = false">关闭</el-button>
        <el-button 
          v-if="pkgInfo?.homepage" 
          type="primary" 
          link 
          @click="openHomepage"
        >
          访问主页 🔗
        </el-button>
        <el-button 
          type="primary" 
          @click="handleOptimizeClick"
        >
          优化建议 ✨
        </el-button>
      </div>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { ref, watch, computed } from 'vue'
import { ElMessage } from 'element-plus'
import { Loading, Warning } from '@element-plus/icons-vue'
import { packageApi, type Package, type PackageVersion } from '@/api/package'
import { securityApi, type ScanResult } from '@/api/security'

interface PackageInfo {
  name: string
  type: string
  displayName?: string
  description?: string
  homepage?: string
  license?: string
  downloadCount?: number
  latestVersion?: string
}

interface VersionInfo {
  version: string
  publishedAt: string
  downloadCount?: number
}

interface DependencyInfo {
  name: string
  version: string
  type: string
}

interface SecurityInfo {
  totalVulnerabilities: number
  criticalCount: number
  highCount: number
  mediumCount: number
  lowCount: number
  scanStatus: string
}

interface Props {
  visible: boolean
  packageName: string
  packageType?: string
}

interface Emits {
  (e: 'update:visible', value: boolean): void
  (e: 'optimize', pkgName: string, pkgType?: string): void
}

const props = defineProps<Props>()
const emit = defineEmits<Emits>()

const dialogVisible = computed({
  get: () => props.visible,
  set: (val: boolean) => emit('update:visible', val)
})

const loading = ref(false)
const error = ref<string>()
const pkgInfo = ref<PackageInfo>()
const versions = ref<VersionInfo[]>([])
const dependencies = ref<DependencyInfo[]>([])
const security = ref<SecurityInfo | null>(null)

interface PackageNodeRef {
  name: string
  type?: string
}

const navigationStack = ref<PackageNodeRef[]>([])
const currentNode = ref<PackageNodeRef>({
  name: props.packageName,
  type: props.packageType
})

const dialogTitle = computed(() => {
  return pkgInfo.value?.displayName || currentNode.value.name || '包详情'
})

const mapPackageInfo = (pkg: Package): PackageInfo => ({
  name: pkg.name,
  type: pkg.type,
  displayName: pkg.display_name,
  description: pkg.description,
  homepage: pkg.homepage,
  license: pkg.license,
  downloadCount: pkg.download_count,
  latestVersion: pkg.latest_version
})

const mapVersions = (versionList: PackageVersion[]): VersionInfo[] => {
  return versionList.map((item) => ({
    version: item.version,
    publishedAt: item.published_at,
    downloadCount: item.download_count
  }))
}

const mapDependencies = (deps: PackageVersion['dependencies'], fallbackType: string): DependencyInfo[] => {
  if (!deps || deps.length === 0) return []
  return deps.map((dep) => ({
    name: dep.dep_name,
    version: dep.dep_version_constraint,
    type: dep.package_type || fallbackType || 'generic'
  }))
}

const mapSecurity = (scan: ScanResult): SecurityInfo => ({
  totalVulnerabilities: scan.total_vulnerabilities,
  criticalCount: scan.critical_count,
  highCount: scan.high_count,
  mediumCount: scan.medium_count,
  lowCount: scan.low_count,
  scanStatus: scan.scan_status
})

const fetchPackageDetail = async (input?: PackageNodeRef) => {
  const target = input || currentNode.value
  if (!target?.name) return
  
  loading.value = true
  error.value = undefined
  pkgInfo.value = undefined
  versions.value = []
  dependencies.value = []
  security.value = null
  
  try {
    const searchResult = await packageApi.search({
      q: target.name,
      type: target.type,
      page: 1,
      page_size: 20
    })

    const normalizedName = target.name.toLowerCase()
    const normalizedType = (target.type || '').toLowerCase()
    const exactMatch = searchResult.list.find((item) =>
      item.name.toLowerCase() === normalizedName &&
      (!normalizedType || item.type.toLowerCase() === normalizedType)
    )
    const looseMatch = searchResult.list.find((item) =>
      item.name.toLowerCase().includes(normalizedName) &&
      (!normalizedType || item.type.toLowerCase() === normalizedType)
    )
    const targetPkg = exactMatch || looseMatch || searchResult.list[0]

    if (!targetPkg) {
      error.value = `未找到包: ${target.name}`
      return
    }

    currentNode.value = {
      name: targetPkg.name,
      type: targetPkg.type
    }
    pkgInfo.value = mapPackageInfo(targetPkg)

    const versionResult = await packageApi.getVersions(targetPkg.type, targetPkg.name)
    const versionList = versionResult.versions || []
    versions.value = mapVersions(versionList)

    const latestVersion =
      versionList.find((item) => item.version === targetPkg.latest_version) ||
      versionList[0]
    dependencies.value = mapDependencies(latestVersion?.dependencies, targetPkg.type)

    if (latestVersion?.id) {
      try {
        const scanResult = await securityApi.getScanResult(latestVersion.id) as ScanResult
        security.value = mapSecurity(scanResult)
      } catch {
        security.value = null
      }
    }
  } catch (e: any) {
    console.error('Failed to fetch package detail:', e)
    error.value = e.message || '加载失败'
    ElMessage.warning(error.value)
  } finally {
    loading.value = false
  }
}

const handleDepClick = (dep: DependencyInfo) => {
  if (!dep.name) return
  navigationStack.value.push({
    name: currentNode.value.name,
    type: currentNode.value.type
  })
  currentNode.value = {
    name: dep.name,
    type: dep.type || currentNode.value.type
  }
  fetchPackageDetail(currentNode.value)
}

const goBack = () => {
  const prev = navigationStack.value.pop()
  if (!prev) return
  currentNode.value = prev
  fetchPackageDetail(prev)
}

const openHomepage = () => {
  if (pkgInfo.value?.homepage) {
    window.open(pkgInfo.value.homepage, '_blank', 'noopener')
  }
}

const handleOptimizeClick = () => {
  emit('optimize', currentNode.value.name, currentNode.value.type)
}

const getPackageTypeTag = (type?: string): 'success' | 'warning' | 'info' | 'danger' => {
  const map: Record<string, any> = {
    npm: 'success',
    maven: 'warning',
    pypi: 'info',
    go: 'danger',
    nuget: 'success',
    generic: 'info'
  }
  return map[type || ''] || 'info'
}

const formatDate = (dateStr: string) => {
  return new Date(dateStr).toLocaleDateString('zh-CN', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit'
  })
}

watch(() => props.visible, (val) => {
  if (val && props.packageName) {
    navigationStack.value = []
    currentNode.value = {
      name: props.packageName,
      type: props.packageType
    }
    fetchPackageDetail(currentNode.value)
  }
})

watch([() => props.packageName, () => props.packageType], () => {
  if (props.visible && props.packageName) {
    navigationStack.value = []
    currentNode.value = {
      name: props.packageName,
      type: props.packageType
    }
    fetchPackageDetail(currentNode.value)
  }
})
</script>

<style scoped>
.package-node-dialog :deep(.el-dialog__body) {
  padding: 16px 20px;
  max-height: 60vh;
  overflow-y: auto;
}

.detail-loading,
.detail-error {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  padding: 40px;
  color: var(--el-text-color-secondary);
}

.detail-error {
  color: var(--el-color-danger);
}

.detail-content {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.detail-nav {
  display: flex;
  align-items: center;
  gap: 8px;
  padding-bottom: 8px;
  border-bottom: 1px solid var(--el-border-color-light);
}

.detail-nav-path {
  font-size: 12px;
  color: var(--el-text-color-secondary);
}

.detail-section {
  padding: 12px 0;
  border-bottom: 1px solid var(--el-border-color-light);
}

.detail-section:last-child {
  border-bottom: none;
}

.section-title {
  font-weight: 600;
  font-size: 14px;
  color: var(--el-text-color-primary);
  margin-bottom: 10px;
  display: flex;
  align-items: center;
  gap: 6px;
}

.description {
  font-size: 13px;
  color: var(--el-text-color-secondary);
  line-height: 1.5;
  margin: 0;
}

.show-more {
  text-align: center;
  margin-top: 8px;
}

.dependency-list {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.dep-tag {
  cursor: pointer;
  transition: all 0.2s;
}

.dep-tag:hover {
  transform: translateY(-1px);
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.1);
}

.dep-tag.more {
  cursor: default;
  background: var(--el-fill-color-light);
}

.dialog-footer {
  display: flex;
  justify-content: flex-end;
  gap: 12px;
}

:deep(.dark .el-descriptions__label) {
  background: #1e293b;
}

:deep(.dark .el-table) {
  --el-table-bg-color: #1e293b;
  --el-table-tr-bg-color: #1e293b;
}
</style>
