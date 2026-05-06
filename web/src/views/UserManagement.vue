<template>
  <div class="user-management">
    <header class="list-header">
      <div class="header-content">
        <div class="header-icon">
          <i class="fa-solid fa-users"></i>
        </div>
        <div class="header-text">
          <h2>用户管理</h2>
          <p class="header-subtitle">管理系统用户和权限配置</p>
        </div>
      </div>
      <el-button type="primary" class="create-btn" @click="showCreateDialog">
        <i class="fa-solid fa-user-plus"></i>
        <span>创建用户</span>
      </el-button>
    </header>

    <div class="toolbar">
      <div class="search-wrapper">
        <i class="fa-solid fa-search search-icon-prefix"></i>
        <el-input
          v-model="keyword"
          placeholder="搜索用户名"
          clearable
          class="search-input"
          @keyup.enter="loadUsers"
          @clear="loadUsers"
        />
      </div>

      <el-select
        v-model="filterActive"
        placeholder="状态"
        clearable
        class="status-select"
        @change="loadUsers"
      >
        <el-option v-for="opt in activeOptions" :key="opt.value" :label="opt.label" :value="opt.value" />
      </el-select>

      <el-button type="primary" class="search-btn" @click="loadUsers">
        <i class="fa-solid fa-search"></i> 搜索
      </el-button>
    </div>

    <div class="content-panel" v-loading="loading">
      <el-table
        :data="users"
        style="width: 100%"
        :header-cell-style="{ background: '#fafbfc' }"
        :row-class-name="tableRowClass"
        @row-mouse-enter="handleRowEnter"
        @row-mouse-leave="handleRowLeave"
      >
        <el-table-column prop="id" label="ID" width="70" align="center" />
        <el-table-column prop="username" label="用户名" width="140">
          <template #default="{ row }">
            <span class="user-name">{{ row.username }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="display_name" label="显示名称" width="140">
          <template #default="{ row }">
            <span class="display-name">{{ row.display_name || '-' }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="email" label="邮箱" min-width="180">
          <template #default="{ row }">
            <span class="email-text">{{ row.email || '-' }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="is_active" label="状态" width="90" align="center">
          <template #default="{ row }">
            <el-tag :class="['status-tag', row.is_active ? 'status-tag--active' : 'status-tag--disabled']" size="small">
              <span class="status-dot"></span>
              {{ row.is_active ? '启用' : '禁用' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="created_at" label="创建时间" width="170" align="center">
          <template #default="{ row }">
            <span class="time-text">{{ formatDate(row.created_at) }}</span>
          </template>
        </el-table-column>
        <el-table-column label="角色" min-width="180">
          <template #default="{ row }">
            <div class="role-tags">
              <el-tag v-for="role in row.roles" :key="role.id" size="small" class="role-tag">{{ role.name }}</el-tag>
              <span v-if="!row.roles || row.roles.length === 0" class="no-roles">未分配</span>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="260" align="center">
          <template #default="{ row }">
            <div class="operation-buttons">
              <el-button class="btn-role" size="small" @click="showRoleDialog(row)">
                <i class="fa-solid fa-user-shield"></i> 角色
              </el-button>
              <el-button class="btn-password" size="small" @click="showPasswordDialog(row)">
                <i class="fa-solid fa-key"></i>
              </el-button>
              <el-button class="btn-toggle" size="small" type="text" @click="toggleActive(row)">
                {{ row.is_active ? '禁用' : '启用' }}
              </el-button>
            </div>
          </template>
        </el-table-column>
      </el-table>

      <div class="list-footer" v-if="total > 0">
        <div class="footer-info">
          <span class="total-badge">{{ total }}</span>
          <span class="total-label">个用户</span>
        </div>
        <el-pagination
          v-model:current-page="page"
          :page-size="pageSize"
          :total="total"
          :page-sizes="[20, 50, 100]"
          layout="sizes, prev, pager, next"
          @current-change="handlePageChange"
        />
      </div>
    </div>

    <el-dialog v-model="createVisible" title="创建用户" width="500px" class="create-dialog">
      <el-form :model="createForm" label-width="80px">
        <el-form-item label="用户名">
          <el-input v-model="createForm.username" placeholder="请输入用户名" />
        </el-form-item>
        <el-form-item label="密码">
          <el-input v-model="createForm.password" type="password" show-password placeholder="请输入密码" />
        </el-form-item>
        <el-form-item label="显示名称">
          <el-input v-model="createForm.display_name" placeholder="请输入显示名称" />
        </el-form-item>
        <el-form-item label="邮箱">
          <el-input v-model="createForm.email" placeholder="请输入邮箱" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="createVisible = false">取消</el-button>
        <el-button type="primary" @click="createUser">确定</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="roleVisible" title="分配角色" width="500px" class="role-dialog">
      <div class="dialog-header">
        <i class="fa-solid fa-user-shield"></i>
        <span>用户：<strong>{{ currentUser?.username }}</strong></span>
      </div>
      <el-form label-width="80px">
        <el-form-item label="角色">
          <el-checkbox-group v-model="selectedRoles" class="role-checkbox-group">
            <el-checkbox v-for="role in allRoles" :key="role.id" :value="role.id">
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

    <el-dialog v-model="passwordVisible" title="重置密码" width="500px" class="password-dialog">
      <div class="dialog-header">
        <i class="fa-solid fa-key"></i>
        <span>用户：<strong>{{ currentUser?.username }}</strong></span>
      </div>
      <el-form :model="passwordForm" label-width="80px">
        <el-form-item label="新密码" required>
          <el-input v-model="passwordForm.password" type="password" show-password placeholder="请输入新密码（至少6位）" />
        </el-form-item>
        <el-form-item label="确认密码" required>
          <el-input v-model="passwordForm.confirmPassword" type="password" show-password placeholder="请再次输入新密码" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="passwordVisible = false">取消</el-button>
        <el-button type="primary" @click="resetPassword">确定</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import request from '@/api/request'
import { ElMessage } from 'element-plus'

const loading = ref(false)
const users = ref<any[]>([])
const allRoles = ref<any[]>([])
const page = ref(1)
const pageSize = ref(20)
const total = ref(0)
const keyword = ref('')
const filterActive = ref<string | null>(null)
const hoveredRow = ref<number | null>(null)

const activeOptions = [
  { label: '启用', value: 'true' },
  { label: '禁用', value: 'false' },
]

const createVisible = ref(false)
const createForm = ref({ username: '', password: '', display_name: '', email: '' })

const roleVisible = ref(false)
const currentUser = ref<any>(null)
const selectedRoles = ref<number[]>([])

const passwordVisible = ref(false)
const passwordForm = ref({ password: '', confirmPassword: '' })

function tableRowClass({ rowIndex }: { rowIndex: number }) {
  return rowIndex === hoveredRow.value ? 'row-hovered' : ''
}

function handleRowEnter({ rowIndex }: { rowIndex: number }) {
  hoveredRow.value = rowIndex
}

function handleRowLeave() {
  hoveredRow.value = null
}

function formatDate(d: string): string {
  if (!d) return '-'
  return new Date(d).toLocaleString('zh-CN')
}

async function loadUsers() {
  loading.value = true
  try {
    const params: Record<string, any> = { page: page.value, page_size: pageSize.value }
    if (keyword.value) params.keyword = keyword.value
    if (filterActive.value !== null) params.is_active = filterActive.value === 'true'

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
    await request.put(`/users/${currentUser.value.id}/roles`, { role_ids: selectedRoles.value })
    ElMessage.success('角色分配成功')
    roleVisible.value = false
    loadUsers()
  } catch (e: any) {
    ElMessage.error('角色分配失败')
  }
}

async function toggleActive(user: any) {
  try {
    await request.put(`/users/${user.id}/status`, { is_active: !user.is_active })
    ElMessage.success('状态更新成功')
    loadUsers()
  } catch (e: any) {
    ElMessage.error('状态更新失败')
  }
}

function showPasswordDialog(user: any) {
  currentUser.value = user
  passwordForm.value = { password: '', confirmPassword: '' }
  passwordVisible.value = true
}

async function resetPassword() {
  if (!currentUser.value) return

  if (!passwordForm.value.password || passwordForm.value.password.length < 6) {
    ElMessage.error('密码长度至少为6位')
    return
  }

  if (passwordForm.value.password !== passwordForm.value.confirmPassword) {
    ElMessage.error('两次输入的密码不一致')
    return
  }

  try {
    await request.put(`/users/${currentUser.value.id}/password`, { password: passwordForm.value.password })
    ElMessage.success('密码重置成功')
    passwordVisible.value = false
  } catch (e: any) {
    ElMessage.error('密码重置失败')
  }
}

onMounted(() => {
  loadUsers()
  loadRoles()
})
</script>

<style scoped>
.user-management {
  min-height: 100%;
  background: linear-gradient(180deg, #f8fafc 0%, #f1f5f9 100%);
}

.list-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 20px 24px;
  background: #fff;
  border-radius: 16px;
  margin-bottom: 16px;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.04);
}

.header-content {
  display: flex;
  align-items: center;
  gap: 16px;
}

.header-icon {
  width: 48px;
  height: 48px;
  border-radius: 12px;
  background: linear-gradient(135deg, #8b5cf6 0%, #7c3aed 100%);
  display: flex;
  align-items: center;
  justify-content: center;
  color: #fff;
  font-size: 22px;
}

.header-text h2 {
  font-size: 20px;
  font-weight: 600;
  margin: 0;
  color: #1f2937;
  letter-spacing: -0.2px;
}

.header-subtitle {
  font-size: 13px;
  color: #9ca3af;
  margin: 4px 0 0;
}

.create-btn {
  height: 40px;
  padding: 0 20px;
  border-radius: 10px;
  font-weight: 500;
  font-size: 14px;
  display: flex;
  align-items: center;
  gap: 8px;
  background: linear-gradient(135deg, #2563eb 0%, #1d4ed8 100%);
  border-color: transparent;
  transition: all 0.2s ease;
}

.create-btn:hover {
  background: linear-gradient(135deg, #1d4ed8 0%, #1e40af 100%);
  transform: translateY(-1px);
  box-shadow: 0 4px 12px rgba(37, 99, 235, 0.3);
}

.toolbar {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 16px;
}

.search-wrapper {
  position: relative;
  display: flex;
  align-items: center;
}

.search-icon-prefix {
  position: absolute;
  left: 14px;
  color: #9ca3af;
  font-size: 14px;
  z-index: 1;
}

.search-input {
  width: 220px;
}

.search-input :deep(.el-input__wrapper) {
  border-radius: 10px;
  box-shadow: none;
  border: 1px solid #e5e7eb;
  padding: 0 14px 0 38px;
  height: 38px;
  transition: all 0.2s ease;
  background: #fff;
}

.search-input :deep(.el-input__wrapper:hover) {
  border-color: #d1d5db;
}

.search-input :deep(.el-input__wrapper.is-focus) {
  border-color: #2563eb;
  box-shadow: 0 0 0 3px rgba(37, 99, 235, 0.1);
}

.search-input :deep(.el-input__inner) {
  height: 36px;
  font-size: 14px;
  color: #374151;
}

.status-select {
  width: 120px;
}

.status-select :deep(.el-input__wrapper) {
  border-radius: 10px;
  box-shadow: none;
  border: 1px solid #e5e7eb;
  padding: 0 12px;
  height: 38px;
  font-size: 14px;
  background: #fff;
  transition: all 0.2s ease;
}

.status-select :deep(.el-input__wrapper:hover) {
  border-color: #d1d5db;
}

.status-select :deep(.el-input__wrapper.is-focus) {
  border-color: #2563eb;
  box-shadow: 0 0 0 3px rgba(37, 99, 235, 0.1);
}

.search-btn {
  height: 38px;
  padding: 0 16px;
  border-radius: 10px;
  font-weight: 500;
  font-size: 14px;
  background: linear-gradient(135deg, #2563eb 0%, #1d4ed8 100%);
  border-color: transparent;
  transition: all 0.2s ease;
}

.search-btn:hover {
  background: linear-gradient(135deg, #1d4ed8 0%, #1e40af 100%);
}

.content-panel {
  background: #fff;
  border-radius: 16px;
  padding: 20px;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.04);
}

:deep(.el-table .row-hovered) {
  background: #f8fafc;
}

.user-name {
  font-weight: 500;
  color: #1f2937;
}

.display-name {
  color: #374151;
}

.email-text {
  color: #6b7280;
  font-size: 13px;
}

.status-tag {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  border: none;
}

.status-tag--active {
  background: #ecfdf5;
  color: #059669;
}

.status-tag--disabled {
  background: #fef2f2;
  color: #dc2626;
}

.status-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: currentColor;
}

.time-text {
  font-size: 13px;
  color: #6b7280;
}

.role-tags {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
}

.role-tag {
  background: #f3f4f6;
  color: #374151;
  border-color: #e5e7eb;
}

.no-roles {
  color: #9ca3af;
  font-size: 13px;
}

.operation-buttons {
  display: flex;
  align-items: center;
  gap: 4px;
}

.btn-role {
  background: #f0fdf4;
  color: #15803d;
  border-color: #bbf7d0;
}

.btn-role:hover {
  background: #dcfce7;
}

.btn-password {
  background: #fff7ed;
  color: #c2410c;
  border-color: #fed7aa;
}

.btn-password:hover {
  background: #ffedd5;
}

.btn-toggle {
  color: #6366f1;
}

.btn-toggle:hover {
  background: #eef2ff;
}

.list-footer {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 16px 20px;
  border-top: 1px solid #f3f4f6;
  background: #fafafa;
  border-radius: 0 0 16px 16px;
}

.footer-info {
  display: flex;
  align-items: baseline;
  gap: 6px;
}

.total-badge {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 26px;
  height: 24px;
  padding: 0 10px;
  background: linear-gradient(135deg, #2563eb 0%, #1d4ed8 100%);
  color: #fff;
  font-size: 12px;
  font-weight: 600;
  border-radius: 6px;
}

.total-label {
  font-size: 13px;
  color: #6b7280;
}

.list-footer :deep(.el-pagination) {
  font-size: 13px;
}

.list-footer :deep(.el-pagination button) {
  border-radius: 6px;
}

.list-footer :deep(.el-pager li) {
  border-radius: 6px;
  min-width: 30px;
}

.dialog-header {
  margin-bottom: 16px;
  padding-bottom: 12px;
  border-bottom: 1px solid #f3f4f6;
  display: flex;
  align-items: center;
  gap: 8px;
  color: #6b7280;
}

.dialog-header i {
  color: #8b5cf6;
}

.role-checkbox-group {
  display: flex;
  flex-direction: column;
  gap: 8px;
}
</style>
