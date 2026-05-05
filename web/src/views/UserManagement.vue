<template>
  <div class="user-management">
    <div class="page-header">
      <h2>用户管理</h2>
      <CustomButton type="primary" @click="showCreateDialog">
        <template #icon>
          <Plus />
        </template>
        创建用户
      </CustomButton>
    </div>

    <div class="filter-bar">
      <CustomInput
        v-model="keyword"
        placeholder="搜索用户名"
        clearable
        style="width: 200px"
        @keyup.enter="loadUsers"
        @clear="loadUsers"
      />
      <CustomSelect
        v-model="filterActive"
        placeholder="状态"
        clearable
        style="width: 100px"
        :options="statusOptions"
        @change="loadUsers"
      />
      <CustomButton type="primary" @click="loadUsers">搜索</CustomButton>
    </div>

    <CustomTable
      :columns="tableColumns"
      :data="users"
      :loading="loading"
      row-key="id"
    >
      <template #is_active="{ row }">
        <CustomTag :type="row.is_active ? 'success' : 'danger'" size="small">
          {{ row.is_active ? '启用' : '禁用' }}
        </CustomTag>
      </template>

      <template #created_at="{ row }">
        {{ formatDate(row.created_at) }}
      </template>

      <template #roles="{ row }">
        <div class="role-tags">
          <CustomTag
            v-for="role in row.roles"
            :key="role.id"
            size="small"
            type="primary"
          >
            {{ role.name }}
          </CustomTag>
          <span v-if="!row.roles || row.roles.length === 0" class="text-muted">未分配</span>
        </div>
      </template>

      <template #operations="{ row }">
        <div class="operation-buttons">
          <CustomButton size="small" @click="showRoleDialog(row)">分配角色</CustomButton>
          <CustomButton
            size="small"
            :type="row.is_active ? 'outline' : 'primary'"
            @click="toggleActive(row)"
          >
            {{ row.is_active ? '禁用' : '启用' }}
          </CustomButton>
        </div>
      </template>
    </CustomTable>

    <el-pagination
      v-if="total > pageSize"
      :current-page="page"
      :page-size="pageSize"
      :total="total"
      layout="total, prev, pager, next"
      class="pagination"
      @current-change="handlePageChange"
    />

    <CustomDialog v-model="createVisible" title="创建用户" width="500px">
      <el-form :model="createForm" label-width="80px">
        <el-form-item label="用户名">
          <CustomInput v-model="createForm.username" />
        </el-form-item>
        <el-form-item label="密码">
          <CustomInput v-model="createForm.password" type="password" />
        </el-form-item>
        <el-form-item label="显示名称">
          <CustomInput v-model="createForm.display_name" />
        </el-form-item>
        <el-form-item label="邮箱">
          <CustomInput v-model="createForm.email" />
        </el-form-item>
      </el-form>
      <template #footer>
        <CustomButton @click="createVisible = false">取消</CustomButton>
        <CustomButton type="primary" @click="createUser">确定</CustomButton>
      </template>
    </CustomDialog>

    <CustomDialog v-model="roleVisible" title="分配角色" width="500px">
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
        <CustomButton @click="roleVisible = false">取消</CustomButton>
        <CustomButton type="primary" @click="assignRoles">确定</CustomButton>
      </template>
    </CustomDialog>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { Plus } from '@element-plus/icons-vue'
import request from '@/api/request'
import { ElMessage } from 'element-plus'
import CustomButton from '@/components/ui/CustomButton.vue'
import CustomInput from '@/components/ui/CustomInput.vue'
import CustomSelect from '@/components/ui/CustomSelect.vue'
import CustomTable from '@/components/ui/CustomTable.vue'
import CustomTag from '@/components/ui/CustomTag.vue'
import CustomDialog from '@/components/ui/CustomDialog.vue'

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

const statusOptions = [
  { label: '启用', value: true },
  { label: '禁用', value: false },
]

const tableColumns = [
  { prop: 'id', label: 'ID', width: '60px' },
  { prop: 'username', label: '用户名', width: '150px' },
  { prop: 'display_name', label: '显示名称', width: '150px' },
  { prop: 'email', label: '邮箱' },
  { prop: 'is_active', label: '状态', width: '80px' },
  { prop: 'created_at', label: '创建时间', width: '180px' },
  { prop: 'roles', label: '角色' },
  { prop: 'operations', label: '操作', width: '200px' },
]

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

    const res = await request.get('/users', { params })
    const data = res as any
    users.value = data?.items || []
    total.value = data?.pagination?.total || 0
  } catch (e: any) {
    console.error(e)
  } finally {
    loading.value = false
  }
}

async function loadRoles() {
  try {
    const res = await request.get('/roles')
    allRoles.value = (res as any) || []
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
    await request.post('/users', createForm.value)
    ElMessage.success('用户创建成功')
    createVisible.value = false
    loadUsers()
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
  padding: var(--spacing-xl);
}

.page-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: var(--spacing-2xl);
}

.page-header h2 {
  margin: 0;
  font-size: var(--font-size-xl);
  font-weight: var(--font-weight-semibold);
}

.filter-bar {
  display: flex;
  gap: var(--spacing-md);
  margin-bottom: var(--spacing-xl);
}

.pagination {
  margin-top: var(--spacing-xl);
  display: flex;
  justify-content: flex-end;
}

.text-muted {
  color: var(--color-text-tertiary);
}

.role-tags {
  display: flex;
  gap: var(--spacing-xs);
  flex-wrap: wrap;
}

.operation-buttons {
  display: flex;
  gap: var(--spacing-sm);
}
</style>
