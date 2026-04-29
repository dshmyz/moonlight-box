<template>
  <div class="package-list">
    <el-card>
      <template #header>
        <div class="card-header">
          <span>包管理</span>
          <el-button type="primary" size="small">上传包</el-button>
        </div>
      </template>
      
      <el-table :data="packages" v-loading="loading">
        <el-table-column prop="name" label="包名" />
        <el-table-column prop="type" label="类型" width="100">
          <template #default="{ row }">
            <el-tag size="small">{{ row.type }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="description" label="描述" show-overflow-tooltip />
        <el-table-column prop="versions_count" label="版本数" width="80" align="center" />
        <el-table-column prop="download_count" label="下载数" width="100" align="center" />
        <el-table-column prop="updated_at" label="更新时间" width="180" />
      </el-table>
      
      <div class="pagination-wrapper">
        <el-pagination
          v-model:current-page="currentPage"
          :page-size="20"
          :total="total"
          layout="total, prev, pager, next"
        />
      </div>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'

const loading = ref(false)
const packages = ref<any[]>([])
const currentPage = ref(1)
const total = ref(0)

onMounted(async () => {
  loading.value = true
  try {
    // TODO: 加载包列表
  } finally {
    loading.value = false
  }
})
</script>

<style scoped>
.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.pagination-wrapper {
  margin-top: 20px;
  display: flex;
  justify-content: flex-end;
}
</style>
