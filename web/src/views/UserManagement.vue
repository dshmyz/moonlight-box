<template>
  <div class="user-management">
    <div class="page-header">
      <h2>用户管理</h2>
      <el-button type="primary" @click="showCreateDialog">
        <el-icon><Plus /></el-icon> 创建用户
      </el-button>
    </div>

    <div class="filter-bar">
      <el-input v-model="keyword" placeholder="搜索用户名" clearable style="width: 200px" @keyup.enter="loadUsers" @clear="loadUsers" />
      <el-select v-model="filterActive" placeholder="状态" clearable style="width: 100px" @change="loadUsers">
        <el-option label="启用" :value="true" />
        <el-option label="禁用" :value="false" />
      </el-select>
      <el-button type="primary" @click="loadUsers">搜索</el-button>
    </div>

    <el-table :data="users" v-loading="loading" style="width: 100%">
      <el-table-column prop="id" label="ID" width="60" />
      <el-table-column prop="username" label="用户名" width="150" />
      <el-table-column prop="display_name" label="显示名称" width="150" />
      <el-table-column prop="email" label="邮箱" min-width="180" />
      <el-table-column prop="is_active" label="状态" width="80">
        <template #default="{ row }">
          <el-tag :type="row.is_active ? 'success' : 'danger'" size="small">
            {{ row.is_active ? '启用' : '禁用' }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column prop="created_at" label="创建时间" width="180">
        <template #default="{ row }">
          {{ formatDate(row.created_at) }}
        </template>
      </el-table-column>
      <el-table-column label="角色" min-width="200">
        <template #default="{ row }">
          <el-tag v-for="role in row.roles" :key="role.id" size="small" style="margin-right: 4px">{{ role.name }}</el-tag>
          <span v-if="!row.roles || row.roles.length === 0" class="text-muted">未分配</span>
        </template>
      </el-table-column>
      <el-table-column label="操作" width="200" fixed="right">
        <template #default="{ row }">
          <el-button size="small" @click="showRoleDialog(row)">分配角色</el-button>
          <el-button size="small" :type="row.is_active ? 'warning' : 'success'" @click="toggleActive(row)">
            {{ row.is_active ? '禁用' : '启用' }}
          </el-button>
        </template>
      </el-table-column>
    </el-table>

    <el-pagination
      v-if="total > pageSize"
      :current-page="page"
      :page-size="pageSize"
      :total="total"
      layout="total, prev, pager, next"
      class="pagination"
      @current-change="handlePageChange"
    />

    <el-dialog v-model="createVisible" title="创建用户" width="500px">
      <el-form :model="createForm" label-width="80px">
        <el-form-item label="用户名">
          <el-input v-model="createForm.username" />
        </el-form-item>
        <el-form-item label="密码">
          <el-input v-model="createForm.password" type="password" show-password />
        </el-form-item>
        <el-form-item label="显示名称">
          <el-input v-model="createForm.display_name" />
        </el-form-item>
        <el-form-item label="邮箱">
          <el-input v-model="createForm.email" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="createVisible = false">取消</el-button>
        <el-button type="primary" @click="createUser">确定</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="roleVisible" title="分配角色" width="500px">
      <el-form label-width="80px">
        <el-form-item label="用户">
          <span>{{ currentUser?.username }}</span>
        </el-form-item>
        <el-form-item label="角色">
          <el-checkbox-group v-model="selectedRoles">
            <el-checkbox v-for="role in allRoles" :key="role.id" :label="role.id">
              {{ role.name }}
            </el-checkbox>
          </el-checkbox-group>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="roleVisible = false">取消</el-button>
        <el-button type="primary" @click="assignRoles">确定</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { Plus } from '@element-plus/icons-vue'
import request from '@/api/request'
import { ElMessage } from 'element-plus'

const loading = ref(false)
const users = ref<any[]>([])
const allRoles = ref<any[]>([])
const page = ref(1)
const pageSize = ref(20)
const total = ref(0)
const keyword = ref('')
const filterActive = ref<boolean | null>(null)

const createVisible = ref(false)
const createForm = ref({ username: '', password: '', display_name: '', email: '' })

const roleVisible = ref(false)
const currentUser = ref<any>(null)
const selectedRoles = ref<number[]>([])

function formatDate(d: string): string {
  if (!d) return '-'
  return new Date(d).toLocaleString('zh-CN')
}

async function loadUsers() {
  loading.value = true
  try {
    const params: Record<string, any> = { page: page.value, page_size: pageSize.value }
    if (keyword.value) params.keyword = keyword.value
    if (filterActive.value !== null) params.is_active = filterActive.value

    const res = await request.get('/api/v1/users', { params })
    if (res.data.code === 200) {
      users.value = res.data.data.items || []
      total.value = res.data.data.pagination?.total || 0
    }
  } catch (e: any) {
    console.error(e)
  } finally {
    loading.value = false
  }
}

async function loadRoles() {
  try {
    const res = await request.get('/api/v1/roles')
    if (res.data.code === 200) {
      allRoles.value = res.data.data || []
    }
  } catch (e: any) {
    console.error(e)
  }
}

function handlePageChange(p: number) {
  page.value = p
  loadUsers()
}

function showCreateDialog() {
  createForm.value = { username: '', password: '', display_name: '', email: '' }
  createVisible.value = true
}

async function createUser() {
  try {
    const res = await request.post('/api/v1/users', createForm.value)
    if (res.data.code === 200 || res.data.code === 201) {
      ElMessage.success('用户创建成功')
      createVisible.value = false
      loadUsers()
    }
  } catch (e: any) {
    ElMessage.error('创建用户失败')
  }
}

function showRoleDialog(user: any) {
  currentUser.value = user
  selectedRoles.value = (user.roles || []).map((r: any) => r.id)
  roleVisible.value = true
}

async function assignRoles() {
  if (!currentUser.value) return
  try {
    await request.put(`/api/v1/users/${currentUser.value.id}/roles`, { role_ids: selectedRoles.value })
    ElMessage.success('角色分配成功')
    roleVisible.value = false
    loadUsers()
  } catch (e: any) {
    ElMessage.error('角色分配失败')
  }
}

async function toggleActive(user: any) {
  try {
    await request.put(`/api/v1/users/${user.id}/status`, { is_active: !user.is_active })
    ElMessage.success('状态更新成功')
    loadUsers()
  } catch (e: any) {
    ElMessage.error('状态更新失败')
  }
}

onMounted(() => {
  loadUsers()
  loadRoles()
})
</script>

<style scoped>
.user-management {
  padding: 20px;
}

.page-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 24px;
}

.page-header h2 {
  margin: 0;
  font-size: 22px;
  font-weight: 600;
}

.filter-bar {
  display: flex;
  gap: 12px;
  margin-bottom: 16px;
}

.pagination {
  margin-top: 16px;
  display: flex;
  justify-content: flex-end;
}

.text-muted {
  color: #909399;
}
</style>
