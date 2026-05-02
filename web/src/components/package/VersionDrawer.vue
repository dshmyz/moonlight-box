<template>
  <el-drawer
    v-model="visible"
    :title="`${packageName} - 版本列表`"
    direction="rtl"
    size="50%"
    @close="handleClose"
  >
    <div v-loading="loading" class="version-drawer-content">
      <el-table
        v-if="!loading && versions.length > 0"
        :data="versions"
        style="width: 100%"
      >
        <el-table-column prop="version" label="版本号" min-width="140">
          <template #default="{ row }">
            <span class="version-text">{{ row.version }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="status" label="状态" width="100" align="center">
          <template #default="{ row }">
            <el-tag :type="getVersionStatusColor(row.status)" size="small">
              {{ getVersionStatusLabel(row.status) }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="published_at" label="发布时间" width="120">
          <template #default="{ row }">
            {{ formatDate(row.published_at) }}
          </template>
        </el-table-column>
        <el-table-column prop="size_bytes" label="大小" width="90" align="right">
          <template #default="{ row }">
            {{ formatSize(row.size_bytes) }}
          </template>
        </el-table-column>
        <el-table-column prop="download_count" label="下载量" width="90" align="right">
          <template #default="{ row }">
            {{ formatNumber(row.download_count) }}
          </template>
        </el-table-column>
      </el-table>

      <el-empty v-if="!loading && versions.length === 0" description="暂无版本数据" />
    </div>
  </el-drawer>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { packageApi, type PackageVersion } from '@/api/package'
import { formatDate, formatNumber, formatSize } from '@/utils/format'
import { getVersionStatusColor, getVersionStatusLabel } from '@/constants/package'

const props = defineProps<{
  modelValue: boolean
  packageType: string
  packageName: string
}>()

const emit = defineEmits<{
  'update:modelValue': [value: boolean]
}>()

const visible = ref(false)
const loading = ref(false)
const versions = ref<PackageVersion[]>([])

watch(() => props.modelValue, (val) => {
  visible.value = val
  if (val) {
    loadVersions()
  }
})

watch(visible, (val) => {
  emit('update:modelValue', val)
})

async function loadVersions() {
  loading.value = true
  try {
    const response = await packageApi.getVersions(props.packageType, props.packageName)
    versions.value = response.versions || []
  } catch (error) {
    ElMessage.error('加载版本列表失败')
    console.error('Failed to load versions:', error)
  } finally {
    loading.value = false
  }
}

function handleClose() {
  visible.value = false
}
</script>

<style scoped>
.version-drawer-content {
  padding: 0 20px;
}

.version-text {
  font-weight: 500;
  color: #303133;
  font-family: 'SF Mono', Monaco, 'Cascadia Code', monospace;
  font-size: 13px;
}
</style>
