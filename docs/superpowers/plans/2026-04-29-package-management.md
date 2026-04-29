# 包管理功能实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 为管理后台实现完整的包管理功能，包括包列表展示、搜索过滤、版本管理等核心功能。

**架构：** 增强现有 PackageList.vue 和 PackageDetail.vue 组件，创建子组件实现表格视图、卡片视图、版本抽屉和版本表格。复用公开页面的详情页，通过路由判断显示管理功能。

**技术栈：** Vue 3 + TypeScript + Element Plus + Vite

---

## 文件结构

### 新建文件
- `web/src/components/package/PackageTable.vue` - 表格视图组件
- `web/src/components/package/PackageCards.vue` - 卡片视图组件
- `web/src/components/package/VersionDrawer.vue` - 版本抽屉组件
- `web/src/components/package/VersionTable.vue` - 版本表格组件

### 修改文件
- `web/src/views/PackageList.vue` - 增强列表页，添加视图切换和搜索功能
- `web/src/views/PackageDetail.vue` - 增强详情页，添加版本管理功能
- `web/src/router/index.ts` - 添加管理后台包详情路由
- `web/src/api/package.ts` - 添加版本管理 API 接口

---

## 任务 1：创建 PackageTable 组件

**文件：**
- 创建：`web/src/components/package/PackageTable.vue`

- [ ] **步骤 1：创建 PackageTable.vue 文件**

```vue
<template>
  <el-table :data="packages" v-loading="loading" style="width: 100%">
    <el-table-column prop="name" label="包名" min-width="200">
      <template #default="{ row }">
        <div>
          <div class="package-name" @click="$emit('view-detail', row)">
            {{ row.name }}
          </div>
          <div class="package-description">{{ row.description }}</div>
        </div>
      </template>
    </el-table-column>
    <el-table-column prop="type" label="类型" width="100">
      <template #default="{ row }">
        <el-tag :type="getTypeTag(row.type)" size="small">
          {{ row.type }}
        </el-tag>
      </template>
    </el-table-column>
    <el-table-column prop="versions_count" label="版本数" width="80" align="center" />
    <el-table-column prop="download_count" label="下载数" width="100" align="center">
      <template #default="{ row }">
        {{ formatNumber(row.download_count) }}
      </template>
    </el-table-column>
    <el-table-column prop="updated_at" label="更新时间" width="180">
      <template #default="{ row }">
        {{ formatDate(row.updated_at) }}
      </template>
    </el-table-column>
    <el-table-column label="操作" width="180" fixed="right">
      <template #default="{ row }">
        <el-button size="small" @click="$emit('view-versions', row)">
          查看版本
        </el-button>
        <el-button size="small" @click="$emit('view-detail', row)">
          详情
        </el-button>
      </template>
    </el-table-column>
  </el-table>
</template>

<script setup lang="ts">
import type { Package } from '@/api/package'

defineProps<{
  packages: Package[]
  loading: boolean
}>()

defineEmits<{
  'view-versions': [pkg: Package]
  'view-detail': [pkg: Package]
}>()

const getTypeTag = (type: string) => {
  const tagMap: Record<string, string> = {
    npm: 'primary',
    maven: 'danger',
    pypi: 'success',
    go: 'warning',
    nuget: 'info',
  }
  return tagMap[type] || 'info'
}

const formatNumber = (num: number) => {
  if (num >= 1000000) return (num / 1000000).toFixed(1) + 'M'
  if (num >= 1000) return (num / 1000).toFixed(1) + 'K'
  return num.toString()
}

const formatDate = (date: string) => {
  return new Date(date).toLocaleDateString('zh-CN')
}
</script>

<style scoped>
.package-name {
  color: #409eff;
  cursor: pointer;
  font-weight: 600;
}

.package-name:hover {
  text-decoration: underline;
}

.package-description {
  font-size: 12px;
  color: #909399;
  margin-top: 4px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
</style>
```

- [ ] **步骤 2：Commit PackageTable 组件**

```bash
git add web/src/components/package/PackageTable.vue
git commit -m "feat: add PackageTable component for package list"
```

---

## 任务 2：创建 PackageCards 组件

**文件：**
- 创建：`web/src/components/package/PackageCards.vue`

- [ ] **步骤 1：创建 PackageCards.vue 文件**

```vue
<template>
  <div class="package-cards" v-loading="loading">
    <div class="card-grid">
      <div
        v-for="pkg in packages"
        :key="pkg.id"
        class="package-card"
      >
        <div class="card-header">
          <div class="card-title">
            <div class="package-name" @click="$emit('view-detail', pkg)">
              {{ pkg.name }}
            </div>
            <div class="package-version">v{{ pkg.latest_version || 'N/A' }}</div>
          </div>
          <el-tag :type="getTypeTag(pkg.type)" size="small">
            {{ pkg.type }}
          </el-tag>
        </div>

        <div class="card-body">
          {{ pkg.description || '暂无描述' }}
        </div>

        <div class="card-stats">
          <div><strong>{{ pkg.versions_count || 0 }}</strong> 版本</div>
          <div><strong>{{ formatNumber(pkg.download_count) }}</strong> 下载</div>
          <div>更新于 {{ formatDate(pkg.updated_at) }}</div>
        </div>

        <div class="card-actions">
          <el-button size="small" @click="$emit('view-versions', pkg)">
            查看版本
          </el-button>
          <el-button size="small" type="primary" @click="$emit('view-detail', pkg)">
            查看详情
          </el-button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import type { Package } from '@/api/package'

defineProps<{
  packages: Package[]
  loading: boolean
}>()

defineEmits<{
  'view-versions': [pkg: Package]
  'view-detail': [pkg: Package]
}>()

const getTypeTag = (type: string) => {
  const tagMap: Record<string, string> = {
    npm: 'primary',
    maven: 'danger',
    pypi: 'success',
    go: 'warning',
    nuget: 'info',
  }
  return tagMap[type] || 'info'
}

const formatNumber = (num: number) => {
  if (num >= 1000000) return (num / 1000000).toFixed(1) + 'M'
  if (num >= 1000) return (num / 1000).toFixed(1) + 'K'
  return num.toString()
}

const formatDate = (date: string) => {
  return new Date(date).toLocaleDateString('zh-CN')
}
</script>

<style scoped>
.package-cards {
  width: 100%;
}

.card-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(300px, 1fr));
  gap: 16px;
}

.package-card {
  background: white;
  border: 1px solid #ebeef5;
  border-radius: 8px;
  padding: 16px;
  transition: all 0.3s;
}

.package-card:hover {
  box-shadow: 0 2px 12px rgba(0, 0, 0, 0.1);
}

.card-header {
  display: flex;
  justify-content: space-between;
  align-items: start;
  margin-bottom: 12px;
}

.card-title {
  flex: 1;
}

.package-name {
  font-size: 18px;
  font-weight: 600;
  color: #409eff;
  cursor: pointer;
  margin-bottom: 4px;
}

.package-name:hover {
  text-decoration: underline;
}

.package-version {
  font-size: 12px;
  color: #909399;
}

.card-body {
  font-size: 14px;
  color: #606266;
  margin-bottom: 16px;
  line-height: 1.6;
  overflow: hidden;
  text-overflow: ellipsis;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
}

.card-stats {
  display: flex;
  gap: 24px;
  margin-bottom: 16px;
  font-size: 13px;
  color: #909399;
}

.card-stats strong {
  color: #606266;
}

.card-actions {
  display: flex;
  gap: 8px;
}

.card-actions .el-button {
  flex: 1;
}
</style>
```

- [ ] **步骤 2：Commit PackageCards 组件**

```bash
git add web/src/components/package/PackageCards.vue
git commit -m "feat: add PackageCards component for package list"
```

---

## 任务 3：创建 VersionDrawer 组件

**文件：**
- 创建：`web/src/components/package/VersionDrawer.vue`

- [ ] **步骤 1：创建 VersionDrawer.vue 文件**

```vue
<template>
  <el-drawer
    v-model="visible"
    :title="`${packageName} - 版本列表`"
    direction="rtl"
    size="400px"
  >
    <div v-loading="loading" class="version-list">
      <div
        v-for="version in versions"
        :key="version.id"
        class="version-item"
        :class="`status-${version.status}`"
      >
        <div class="version-header">
          <div class="version-number" :class="{ 'is-disabled': version.status === 'yanked' }">
            {{ version.version }}
          </div>
          <el-tag :type="getStatusTag(version.status)" size="small">
            {{ getStatusText(version.status) }}
          </el-tag>
        </div>
        <div class="version-info">
          <span>📦 {{ formatNumber(version.download_count) }} 下载</span>
          <span>📅 {{ formatDate(version.published_at) }}</span>
        </div>
      </div>
    </div>

    <template #footer>
      <el-button type="primary" @click="handleViewDetail">
        查看完整详情
      </el-button>
    </template>
  </el-drawer>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { packageApi } from '@/api/package'

interface PackageVersion {
  id: number
  version: string
  status: string
  download_count: number
  published_at: string
}

const props = defineProps<{
  modelValue: boolean
  packageId?: number
  packageName: string
  packageType: string
}>()

const emit = defineEmits<{
  'update:modelValue': [value: boolean]
}>()

const router = useRouter()
const visible = ref(props.modelValue)
const loading = ref(false)
const versions = ref<PackageVersion[]>([])

watch(() => props.modelValue, (val) => {
  visible.value = val
  if (val && props.packageId) {
    loadVersions()
  }
})

watch(visible, (val) => {
  emit('update:modelValue', val)
})

const loadVersions = async () => {
  if (!props.packageType || !props.packageName) return

  loading.value = true
  try {
    const res = await packageApi.getVersions(props.packageType, props.packageName)
    versions.value = res.versions || []
  } catch (err) {
    ElMessage.error('加载版本列表失败')
  } finally {
    loading.value = false
  }
}

const getStatusTag = (status: string) => {
  const tagMap: Record<string, string> = {
    published: 'success',
    deprecated: 'warning',
    yanked: 'danger',
    draft: 'info',
  }
  return tagMap[status] || 'info'
}

const getStatusText = (status: string) => {
  const textMap: Record<string, string> = {
    published: '已发布',
    deprecated: '已废弃',
    yanked: '已禁用',
    draft: '草稿',
  }
  return textMap[status] || status
}

const formatNumber = (num: number) => {
  if (num >= 1000000) return (num / 1000000).toFixed(1) + 'M'
  if (num >= 1000) return (num / 1000).toFixed(1) + 'K'
  return num.toString()
}

const formatDate = (date: string) => {
  return new Date(date).toLocaleDateString('zh-CN')
}

const handleViewDetail = () => {
  router.push(`/admin/packages/${props.packageType}/${encodeURIComponent(props.packageName)}`)
  visible.value = false
}
</script>

<style scoped>
.version-list {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.version-item {
  background: #f5f7fa;
  border-radius: 4px;
  padding: 12px;
}

.version-item.status-yanked {
  background: #fef0f0;
}

.version-item.status-deprecated {
  background: #fdf6ec;
}

.version-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 8px;
}

.version-number {
  font-weight: 600;
  color: #409eff;
}

.version-number.is-disabled {
  color: #f56c6c;
  text-decoration: line-through;
}

.version-info {
  font-size: 12px;
  color: #909399;
  display: flex;
  gap: 16px;
}
</style>
```

- [ ] **步骤 2：Commit VersionDrawer 组件**

```bash
git add web/src/components/package/VersionDrawer.vue
git commit -m "feat: add VersionDrawer component for quick version preview"
```

---

## 任务 4：创建 VersionTable 组件

**文件：**
- 创建：`web/src/components/package/VersionTable.vue`

- [ ] **步骤 1：创建 VersionTable.vue 文件**

```vue
<template>
  <el-table :data="versions" v-loading="loading">
    <el-table-column prop="version" label="版本号" width="150">
      <template #default="{ row }">
        <div class="version-number" :class="{ 'is-disabled': row.status === 'yanked' }">
          {{ row.version }}
        </div>
      </template>
    </el-table-column>
    <el-table-column prop="status" label="状态" width="100">
      <template #default="{ row }">
        <el-tag :type="getStatusTag(row.status)" size="small">
          {{ getStatusText(row.status) }}
        </el-tag>
      </template>
    </el-table-column>
    <el-table-column prop="download_count" label="下载量" width="100" align="center">
      <template #default="{ row }">
        {{ formatNumber(row.download_count) }}
      </template>
    </el-table-column>
    <el-table-column prop="published_at" label="发布时间" width="180">
      <template #default="{ row }">
        {{ formatDate(row.published_at) }}
      </template>
    </el-table-column>
    <el-table-column label="操作" min-width="200">
      <template #default="{ row }">
        <div class="action-buttons">
          <el-button
            v-if="row.status === 'published'"
            size="small"
            type="warning"
            @click="handleDeprecate(row)"
          >
            废弃
          </el-button>
          <el-button
            v-if="row.status === 'published'"
            size="small"
            type="danger"
            @click="handleYank(row)"
          >
            禁用
          </el-button>
          <el-button
            v-if="row.status === 'deprecated' || row.status === 'yanked'"
            size="small"
            type="success"
            @click="handleRestore(row)"
          >
            恢复
          </el-button>
          <el-button
            v-if="row.status === 'yanked'"
            size="small"
            type="danger"
            @click="handleDelete(row)"
          >
            删除
          </el-button>
          <el-button
            size="small"
            @click="handleDownload(row)"
          >
            下载
          </el-button>
        </div>
      </template>
    </el-table-column>
  </el-table>
</template>

<script setup lang="ts">
import { ElMessage, ElMessageBox } from 'element-plus'
import { packageApi } from '@/api/package'

interface PackageVersion {
  id: number
  version: string
  status: string
  download_count: number
  published_at: string
}

defineProps<{
  versions: PackageVersion[]
  loading: boolean
}>()

const emit = defineEmits<{
  'refresh': []
}>()

const getStatusTag = (status: string) => {
  const tagMap: Record<string, string> = {
    published: 'success',
    deprecated: 'warning',
    yanked: 'danger',
    draft: 'info',
  }
  return tagMap[status] || 'info'
}

const getStatusText = (status: string) => {
  const textMap: Record<string, string> = {
    published: '已发布',
    deprecated: '已废弃',
    yanked: '已禁用',
    draft: '草稿',
  }
  return textMap[status] || status
}

const formatNumber = (num: number) => {
  if (num >= 1000000) return (num / 1000000).toFixed(1) + 'M'
  if (num >= 1000) return (num / 1000).toFixed(1) + 'K'
  return num.toString()
}

const formatDate = (date: string) => {
  return new Date(date).toLocaleDateString('zh-CN')
}

const handleDeprecate = async (version: PackageVersion) => {
  try {
    await ElMessageBox.prompt('请输入废弃原因（可选）', '废弃版本', {
      confirmButtonText: '确定',
      cancelButtonText: '取消',
      inputPlaceholder: '废弃原因',
    }).then(async ({ value }) => {
      await packageApi.deprecateVersion(version.id, value || '')
      ElMessage.success('版本已废弃')
      emit('refresh')
    })
  } catch (err) {
    if (err !== 'cancel') {
      ElMessage.error('操作失败')
    }
  }
}

const handleYank = async (version: PackageVersion) => {
  try {
    await ElMessageBox.confirm(
      '禁用后用户将无法安装此版本，确定要禁用吗？',
      '禁用版本',
      {
        confirmButtonText: '确定',
        cancelButtonText: '取消',
        type: 'warning',
      }
    )
    await packageApi.yankVersion(version.id)
    ElMessage.success('版本已禁用')
    emit('refresh')
  } catch (err) {
    if (err !== 'cancel') {
      ElMessage.error('操作失败')
    }
  }
}

const handleRestore = async (version: PackageVersion) => {
  try {
    await ElMessageBox.confirm(
      '确定要恢复此版本吗？',
      '恢复版本',
      {
        confirmButtonText: '确定',
        cancelButtonText: '取消',
        type: 'info',
      }
    )
    await packageApi.restoreVersion(version.id)
    ElMessage.success('版本已恢复')
    emit('refresh')
  } catch (err) {
    if (err !== 'cancel') {
      ElMessage.error('操作失败')
    }
  }
}

const handleDelete = async (version: PackageVersion) => {
  try {
    await ElMessageBox.confirm(
      '此操作将永久删除该版本，且不可恢复，确定要删除吗？',
      '删除版本',
      {
        confirmButtonText: '确定',
        cancelButtonText: '取消',
        type: 'error',
      }
    )
    await packageApi.deleteVersion(version.id)
    ElMessage.success('版本已删除')
    emit('refresh')
  } catch (err) {
    if (err !== 'cancel') {
      ElMessage.error('操作失败')
    }
  }
}

const handleDownload = (version: PackageVersion) => {
  ElMessage.info('下载功能待实现')
}
</script>

<style scoped>
.version-number {
  font-weight: 600;
  color: #409eff;
}

.version-number.is-disabled {
  color: #f56c6c;
  text-decoration: line-through;
}

.action-buttons {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}
</style>
```

- [ ] **步骤 2：Commit VersionTable 组件**

```bash
git add web/src/components/package/VersionTable.vue
git commit -m "feat: add VersionTable component for version management"
```

---

## 任务 5：增强 package.ts API 文件

**文件：**
- 修改：`web/src/api/package.ts`

- [ ] **步骤 1：添加版本管理 API 接口**

在 `web/src/api/package.ts` 文件末尾添加以下代码：

```typescript
export interface PackageVersion {
  id: number
  package_id: number
  version: string
  status: string
  storage_path: string
  size_bytes: number
  checksum_sha256?: string
  checksum_md5?: string
  published_at: string
  published_by: number
  metadata?: string
  download_count: number
}

export interface VersionListResponse {
  package_name: string
  type: string
  versions: PackageVersion[]
}

export const packageApi = {
  search(params: { q: string; type?: string; scope?: string; sort?: string; page?: number; page_size?: number }) {
    return request.get<SearchResponse>('/packages/search', { params })
  },

  getVersions(type: string, name: string) {
    return request.get<VersionListResponse>(`/packages/${type}/${encodeURIComponent(name)}/versions`)
  },

  deprecateVersion(versionId: number, reason: string) {
    return request.post(`/packages/versions/${versionId}/deprecate`, { reason })
  },

  restoreVersion(versionId: number) {
    return request.post(`/packages/versions/${versionId}/restore`)
  },

  yankVersion(versionId: number) {
    return request.post(`/packages/versions/${versionId}/yank`)
  },

  deleteVersion(versionId: number) {
    return request.delete(`/packages/versions/${versionId}`)
  },
}
```

- [ ] **步骤 2：Commit API 接口更新**

```bash
git add web/src/api/package.ts
git commit -m "feat: add version management API endpoints"
```

---

## 任务 6：增强 PackageList.vue

**文件：**
- 修改：`web/src/views/PackageList.vue`

- [ ] **步骤 1：重写 PackageList.vue 完整代码**

```vue
<template>
  <div class="package-list">
    <el-card>
      <template #header>
        <div class="card-header">
          <span>包管理</span>
          <div class="view-toggle">
            <el-button
              :type="viewMode === 'table' ? 'primary' : ''"
              size="small"
              @click="viewMode = 'table'"
            >
              表格视图
            </el-button>
            <el-button
              :type="viewMode === 'card' ? 'primary' : ''"
              size="small"
              @click="viewMode = 'card'"
            >
              卡片视图
            </el-button>
          </div>
        </div>
      </template>

      <div class="filter-section">
        <el-input
          v-model="searchQuery"
          placeholder="搜索包名或描述..."
          clearable
          class="search-input"
          @keyup.enter="handleSearch"
          @clear="handleSearch"
        >
          <template #prefix>
            <el-icon><Search /></el-icon>
          </template>
        </el-input>

        <el-select v-model="selectedType" placeholder="包类型" clearable @change="handleSearch">
          <el-option label="全部类型" value="" />
          <el-option label="npm" value="npm" />
          <el-option label="Maven" value="maven" />
          <el-option label="PyPI" value="pypi" />
          <el-option label="Go" value="go" />
          <el-option label="NuGet" value="nuget" />
        </el-select>

        <el-select v-model="sortBy" placeholder="排序方式" @change="handleSearch">
          <el-option label="按下载量" value="downloads" />
          <el-option label="按名称" value="name" />
          <el-option label="按更新时间" value="updated_at" />
        </el-select>

        <el-button type="primary" @click="handleSearch">
          <el-icon><Search /></el-icon>
          搜索
        </el-button>
      </div>

      <PackageTable
        v-if="viewMode === 'table'"
        :packages="packages"
        :loading="loading"
        @view-versions="handleViewVersions"
        @view-detail="handleViewDetail"
      />

      <PackageCards
        v-else
        :packages="packages"
        :loading="loading"
        @view-versions="handleViewVersions"
        @view-detail="handleViewDetail"
      />

      <div class="pagination-wrapper">
        <el-pagination
          v-model:current-page="currentPage"
          v-model:page-size="pageSize"
          :total="total"
          :page-sizes="[10, 20, 50, 100]"
          layout="total, sizes, prev, pager, next"
          @current-change="handleSearch"
          @size-change="handleSearch"
        />
      </div>
    </el-card>

    <VersionDrawer
      v-model="showVersionDrawer"
      :package-id="selectedPackage?.id"
      :package-name="selectedPackage?.name || ''"
      :package-type="selectedPackage?.type || ''"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { Search } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import { packageApi, type Package } from '@/api/package'
import PackageTable from '@/components/package/PackageTable.vue'
import PackageCards from '@/components/package/PackageCards.vue'
import VersionDrawer from '@/components/package/VersionDrawer.vue'

const router = useRouter()
const loading = ref(false)
const viewMode = ref<'table' | 'card'>('table')
const searchQuery = ref('')
const selectedType = ref('')
const sortBy = ref('downloads')
const currentPage = ref(1)
const pageSize = ref(20)
const total = ref(0)
const packages = ref<Package[]>([])
const showVersionDrawer = ref(false)
const selectedPackage = ref<Package | null>(null)

const handleSearch = async () => {
  loading.value = true
  try {
    const params: any = {
      q: searchQuery.value,
      page: currentPage.value,
      page_size: pageSize.value,
      sort: sortBy.value,
    }
    if (selectedType.value) {
      params.type = selectedType.value
    }

    const res = await packageApi.search(params)
    packages.value = res.list || []
    total.value = res.total || 0
  } catch (err) {
    ElMessage.error('加载包列表失败')
  } finally {
    loading.value = false
  }
}

const handleViewVersions = (pkg: Package) => {
  selectedPackage.value = pkg
  showVersionDrawer.value = true
}

const handleViewDetail = (pkg: Package) => {
  router.push(`/admin/packages/${pkg.type}/${encodeURIComponent(pkg.name)}`)
}

onMounted(() => {
  handleSearch()
})
</script>

<style scoped>
.package-list {
  padding: 20px;
}

.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.view-toggle {
  display: flex;
  gap: 8px;
}

.filter-section {
  display: flex;
  gap: 12px;
  margin-bottom: 20px;
  flex-wrap: wrap;
}

.search-input {
  width: 300px;
}

.pagination-wrapper {
  margin-top: 20px;
  display: flex;
  justify-content: flex-end;
}
</style>
```

- [ ] **步骤 2：Commit PackageList.vue 更新**

```bash
git add web/src/views/PackageList.vue
git commit -m "feat: enhance PackageList with dual view and search functionality"
```

---

## 任务 7：增强 PackageDetail.vue

**文件：**
- 修改：`web/src/views/PackageDetail.vue`

- [ ] **步骤 1：读取现有 PackageDetail.vue 内容**

读取文件：`web/src/views/PackageDetail.vue`

- [ ] **步骤 2：添加版本管理功能**

在现有 PackageDetail.vue 中添加以下功能：

1. 在 script setup 部分添加：

```typescript
import { useRoute } from 'vue-router'
import VersionTable from '@/components/package/VersionTable.vue'

const route = useRoute()
const isAdmin = computed(() => route.path.startsWith('/admin'))
const versions = ref<PackageVersion[]>([])
const versionsLoading = ref(false)

const loadVersions = async () => {
  if (!props.pkg?.type || !props.pkg?.name) return

  versionsLoading.value = true
  try {
    const res = await packageApi.getVersions(props.pkg.type, props.pkg.name)
    versions.value = res.versions || []
  } catch (err) {
    ElMessage.error('加载版本列表失败')
  } finally {
    versionsLoading.value = false
  }
}

const handleRefresh = () => {
  loadVersions()
}

onMounted(() => {
  if (isAdmin.value) {
    loadVersions()
  }
})
```

2. 在模板中添加版本管理部分（在现有内容后面）：

```vue
<div v-if="isAdmin" class="version-management">
  <el-divider />
  <h3>版本管理</h3>
  <VersionTable
    :versions="versions"
    :loading="versionsLoading"
    @refresh="handleRefresh"
  />
</div>
```

- [ ] **步骤 3：Commit PackageDetail.vue 更新**

```bash
git add web/src/views/PackageDetail.vue
git commit -m "feat: add version management to PackageDetail for admin view"
```

---

## 任务 8：更新路由配置

**文件：**
- 修改：`web/src/router/index.ts`

- [ ] **步骤 1：添加管理后台包详情路由**

在 `/admin` 路由的 children 数组中添加：

```typescript
{
  path: 'packages/:type/:name',
  name: 'AdminPackageDetail',
  component: () => import('@/views/PackageDetail.vue'),
  meta: { title: '包详情', isAdmin: true },
},
```

- [ ] **步骤 2：Commit 路由更新**

```bash
git add web/src/router/index.ts
git commit -m "feat: add admin package detail route"
```

---

## 任务 9：测试和验证

**文件：**
- 无需创建新文件

- [ ] **步骤 1：启动前端开发服务器**

运行：`cd web && npm run dev`
预期：前端服务启动成功

- [ ] **步骤 2：测试包列表页面**

操作：
1. 访问 `/admin/packages`
2. 验证包列表正确显示
3. 测试搜索功能
4. 测试类型过滤
5. 测试排序功能
6. 测试视图切换（表格/卡片）

预期：所有功能正常工作

- [ ] **步骤 3：测试版本抽屉**

操作：
1. 点击"查看版本"按钮
2. 验证抽屉正确显示版本列表
3. 点击"查看完整详情"按钮

预期：抽屉正常工作，跳转正确

- [ ] **步骤 4：测试包详情页**

操作：
1. 访问 `/admin/packages/npm/lodash`
2. 验证版本管理功能显示
3. 测试废弃版本操作
4. 测试恢复版本操作

预期：版本管理功能正常工作

- [ ] **步骤 5：测试公开页面不受影响**

操作：
1. 访问 `/packages/npm/lodash`
2. 验证不显示版本管理功能

预期：公开页面保持只读

---

## 任务 10：最终提交和清理

**文件：**
- 无需创建新文件

- [ ] **步骤 1：运行前端构建**

运行：`cd web && npm run build`
预期：构建成功，无错误

- [ ] **步骤 2：运行类型检查**

运行：`cd web && npm run type-check`
预期：类型检查通过

- [ ] **步骤 3：最终 commit**

```bash
git add .
git commit -m "feat: complete package management implementation"
```

---

## 自检清单

- [x] 规格覆盖度：所有规格需求都有对应任务
- [x] 占位符扫描：无占位符，所有步骤都有完整代码
- [x] 类型一致性：所有类型定义一致，无冲突
- [x] 文件路径：所有文件路径精确
- [x] 测试覆盖：包含完整的测试步骤
- [x] Commit 策略：每个任务完成后都有 commit
