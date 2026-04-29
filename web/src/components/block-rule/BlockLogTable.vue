<template>
  <el-table :data="logs" v-loading="loading" style="width: 100%">
    <el-table-column prop="created_at" label="时间" width="180">
      <template #default="{ row }">
        {{ formatTime(row.created_at) }}
      </template>
    </el-table-column>
    <el-table-column prop="resource_name" label="包名@版本" min-width="200" />
    <el-table-column prop="ip_address" label="客户端 IP" width="150" />
    <el-table-column prop="details" label="阻断原因" min-width="200">
      <template #default="{ row }">
        {{ parseReason(row.details) }}
      </template>
    </el-table-column>
    <el-table-column prop="user_agent" label="User-Agent" min-width="200" show-overflow-tooltip />
  </el-table>

  <div class="pagination-wrapper" v-if="total > 0">
    <el-pagination
      v-model:current-page="currentPage"
      :page-size="pageSize"
      :total="total"
      layout="prev, pager, next"
      @current-change="loadLogs"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import request from '@/api/request'

interface BlockLog {
  id: number
  action: string
  resource_name: string
  ip_address: string
  user_agent: string
  details: string
  created_at: string
}

const loading = ref(false)
const logs = ref<BlockLog[]>([])
const total = ref(0)
const currentPage = ref(1)
const pageSize = 20

const formatTime = (t: string) => {
  if (!t) return ''
  return new Date(t).toLocaleString('zh-CN')
}

const parseReason = (details: string) => {
  try {
    const obj = JSON.parse(details)
    return obj.reason || '-'
  } catch {
    return '-'
  }
}

const loadLogs = async () => {
  loading.value = true
  try {
    const res = await request.get('/block-rules/logs', {
      params: { page: currentPage.value, page_size: pageSize },
    })
    logs.value = res.data?.items || []
    total.value = res.data?.pagination?.total || 0
  } catch (err) {
    console.error('加载阻断日志失败:', err)
    ElMessage.error('加载阻断日志失败')
  } finally {
    loading.value = false
  }
}

onMounted(loadLogs)
</script>

<style scoped>
.pagination-wrapper {
  display: flex;
  justify-content: flex-end;
  margin-top: 16px;
}
</style>
