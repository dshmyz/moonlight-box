<template>
  <div class="role-management">
    <header class="list-header">
      <div class="header-content">
        <div class="header-icon">
          <i class="fa-solid fa-user-shield"></i>
        </div>
        <div class="header-text">
          <h2>角色管理</h2>
          <p class="header-subtitle">管理系统角色和权限配置</p>
        </div>
      </div>
      <el-button type="primary" class="create-btn" @click="showCreateDialog">
        <i class="fa-solid fa-plus"></i>
        <span>新建角色</span>
      </el-button>
    </header>

    <div class="content-panel" v-loading="loading">
      <el-table
        :data="roles"
        style="width: 100%"
        :header-cell-style="{ background: '#fafbfc' }"
        :row-class-name="tableRowClass"
        @row-mouse-enter="handleRowEnter"
        @row-mouse-leave="handleRowLeave"
      >
        <el-table-column prop="id" label="ID" width="70" align="center" />
        <el-table-column prop="name" label="角色名称" width="150">
          <template #default="{ row }">
            <span class="role-name">{{ row.name }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="description" label="描述" min-width="200" show-overflow-tooltip />
        <el-table-column prop="is_system_role" label="系统角色" width="100" align="center">
          <template #default="{ row }">
            <el-tag v-if="row.is_system_role" type="info" size="small">
              <i class="fa-solid fa-lock"></i> 是
            </el-tag>
            <span v-else class="text-no">否</span>
          </template>
        </el-table-column>
        <el-table-column prop="permissions" label="权限数量" width="100" align="center">
          <template #default="{ row }">
            <span class="perm-count">{{ row.permissions?.length || 0 }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="created_at" label="创建时间" width="180" align="center">
          <template #default="{ row }">
            <span class="time-text">{{ formatDate(row.created_at) }}</span>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="240" align="center">
          <template #default="{ row }">
            <div class="operation-buttons">
              <el-button class="btn-perm" size="small" @click="showPermissionDialog(row)">
                <i class="fa-solid fa-key"></i> 权限
              </el-button>
              <el-button class="btn-clone" size="small" @click="showCloneDialog(row)">
                <i class="fa-solid fa-clone"></i> 克隆
              </el-button>
              <el-button class="btn-delete" size="small" link @click="deleteRole(row)" :disabled="row.is_system_role">
                <i class="fa-solid fa-trash"></i>
              </el-button>
            </div>
          </template>
        </el-table-column>
      </el-table>
    </div>

    <el-dialog v-model="createVisible" title="新建角色" width="500px" class="create-dialog">
      <el-form :model="createForm" label-width="80px">
        <el-form-item label="角色名称" required>
          <el-input v-model="createForm.name" placeholder="请输入角色名称（英文）" />
        </el-form-item>
        <el-form-item label="描述">
          <el-input v-model="createForm.description" type="textarea" :rows="3" placeholder="请输入角色描述" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="createVisible = false">取消</el-button>
        <el-button type="primary" @click="createRole">确定</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="permissionVisible" title="配置权限" width="600px" class="permission-dialog">
      <div class="permission-header">
        <i class="fa-solid fa-user-shield"></i>
        <span>角色：<strong>{{ currentRole?.name }}</strong></span>
      </div>
      <el-table :data="groupedPermissions" style="width: 100%" max-height="400">
        <el-table-column prop="resource" label="资源" width="150">
          <template #default="{ row }">
            <span class="resource-name">{{ resourceLabel(row.resource) }}</span>
          </template>
        </el-table-column>
        <el-table-column label="权限">
          <template #default="{ row }">
            <el-checkbox-group v-model="selectedPermissions">
              <el-checkbox v-for="action in row.actions" :key="action.id" :value="action.id">
                {{ actionLabel(action.action) }}
              </el-checkbox>
            </el-checkbox-group>
          </template>
        </el-table-column>
      </el-table>
      <template #footer>
        <el-button @click="permissionVisible = false">取消</el-button>
        <el-button type="primary" @click="updatePermissions">保存</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="cloneVisible" title="克隆角色" width="500px" class="clone-dialog">
      <div class="clone-source-info">
        <i class="fa-solid fa-clone"></i>
        <span>基于角色：<strong>{{ cloneSourceRole?.name }}</strong> 创建新角色</span>
      </div>
      <el-form :model="cloneForm" label-width="80px">
        <el-form-item label="角色名称" required>
          <el-input v-model="cloneForm.name" placeholder="请输入新角色名称（英文）" />
        </el-form-item>
        <el-form-item label="描述">
          <el-input v-model="cloneForm.description" type="textarea" :rows="3" placeholder="请输入新角色描述" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="cloneVisible = false">取消</el-button>
        <el-button type="primary" @click="cloneRole">确定</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { roleApi, type Role, type Permission } from '@/api/role'

const loading = ref(false)
const roles = ref<Role[]>([])
const allPermissions = ref<Permission[]>([])
const hoveredRow = ref<number | null>(null)

const createVisible = ref(false)
const createForm = ref({ name: '', description: '' })

const cloneVisible = ref(false)
const cloneSourceRole = ref<Role | null>(null)
const cloneForm = ref({ name: '', description: '' })

const permissionVisible = ref(false)
const currentRole = ref<Role | null>(null)
const selectedPermissions = ref<number[]>([])

interface GroupedPermission {
  resource: string
  actions: { id: number; action: string }[]
}

const groupedPermissions = computed<GroupedPermission[]>(() => {
  const groups: Record<string, { id: number; action: string }[]> = {}
  for (const perm of allPermissions.value) {
    if (!groups[perm.resource]) {
      groups[perm.resource] = []
    }
    groups[perm.resource].push({ id: perm.id, action: perm.action })
  }
  const result: GroupedPermission[] = []
  for (const [resource, actions] of Object.entries(groups)) {
    result.push({ resource, actions })
  }
  return result
})

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
  if (!d || d === '') return '-'
  const date = new Date(d)
  if (isNaN(date.getTime())) return '-'
  return date.toLocaleString('zh-CN')
}

function actionLabel(action: string): string {
  const map: Record<string, string> = {
    read: '读取',
    write: '写入',
    delete: '删除',
    admin: '管理',
    delete_own: '删除自己',
  }
  return map[action] || action
}

function resourceLabel(resource: string): string {
  const map: Record<string, string> = {
    system: '系统管理',
    users: '用户管理',
    audit: '审计日志',
    repositories: '仓库管理',
    cache: '缓存管理',
    'block-rules': '阻断规则',
    'storage-backends': '存储后端',
    security: '安全扫描',
    webhooks: 'Webhook',
    package: '包管理',
  }
  return map[resource] || resource
}

async function loadRoles() {
  loading.value = true
  try {
    const res = await roleApi.list()
    roles.value = res || []
  } catch {
    console.error('Failed to load roles')
  } finally {
    loading.value = false
  }
}

async function loadPermissions() {
  try {
    const res = await roleApi.listPermissions()
    allPermissions.value = res || []
  } catch {
    console.error('Failed to load permissions')
  }
}

function showCreateDialog() {
  createForm.value = { name: '', description: '' }
  createVisible.value = true
}

async function createRole() {
  if (!createForm.value.name) {
    ElMessage.error('请输入角色名称')
    return
  }

  try {
    await roleApi.create(createForm.value)
    ElMessage.success('角色创建成功')
    createVisible.value = false
    loadRoles()
  } catch {
    ElMessage.error('角色创建失败')
  }
}

function showPermissionDialog(role: Role) {
  currentRole.value = role
  selectedPermissions.value = (role.permissions || []).map((p) => p.id)
  permissionVisible.value = true
}

async function updatePermissions() {
  if (!currentRole.value) return

  try {
    await roleApi.updatePermissions(currentRole.value.id, { permission_ids: selectedPermissions.value })
    ElMessage.success('权限更新成功')
    permissionVisible.value = false
    loadRoles()
  } catch {
    ElMessage.error('权限更新失败')
  }
}

async function deleteRole(role: Role) {
  if (role.is_system_role) {
    ElMessage.warning('系统角色不能删除')
    return
  }

  try {
    await ElMessageBox.confirm('确定要删除该角色吗？删除后无法恢复。', '删除确认', {
      confirmButtonText: '确定',
      cancelButtonText: '取消',
      type: 'warning',
    })

    await roleApi.delete(role.id)
    ElMessage.success('角色删除成功')
    loadRoles()
  } catch {
  }
}

function showCloneDialog(role: Role) {
  cloneSourceRole.value = role
  cloneForm.value = { name: `${role.name}-copy`, description: role.description }
  cloneVisible.value = true
}

async function cloneRole() {
  if (!cloneForm.value.name) {
    ElMessage.error('请输入角色名称')
    return
  }

  if (!cloneSourceRole.value) return

  try {
    await roleApi.clone(cloneSourceRole.value.id, cloneForm.value)
    ElMessage.success('角色克隆成功')
    cloneVisible.value = false
    loadRoles()
  } catch {
    ElMessage.error('角色克隆失败')
  }
}

onMounted(() => {
  loadRoles()
  loadPermissions()
})
</script>

<style scoped>
.role-management {
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
  background: linear-gradient(135deg, #ec4899 0%, #db2777 100%);
  display: flex;
  align-items: center;
  justify-content: center;
  color: #fff;
  font-size: 24px;
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

.content-panel {
  background: #fff;
  border-radius: 16px;
  padding: 20px;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.04);
}

:deep(.el-table .row-hovered) {
  background: #f8fafc;
}

.role-name {
  font-weight: 500;
  color: #1f2937;
}

.text-no {
  color: #9ca3af;
  font-size: 13px;
}

.perm-count {
  font-weight: 600;
  color: #6366f1;
}

.time-text {
  font-size: 13px;
  color: #6b7280;
}

.operation-buttons {
  display: flex;
  align-items: center;
  gap: 4px;
}

.btn-perm {
  background: #f0f9ff;
  color: #0369a1;
  border-color: #bae6fd;
}

.btn-perm:hover {
  background: #e0f2fe;
}

.btn-clone {
  background: #f0fdf4;
  color: #166534;
  border-color: #bbf7d0;
}

.btn-clone:hover {
  background: #dcfce7;
}

.btn-delete {
  color: #ef4444;
}

.btn-delete:hover {
  background: #fef2f2;
}

.permission-header {
  margin-bottom: 16px;
  padding-bottom: 12px;
  border-bottom: 1px solid #f3f4f6;
  display: flex;
  align-items: center;
  gap: 8px;
  color: #6b7280;
}

.permission-header i {
  color: #ec4899;
}

.resource-name {
  font-weight: 500;
  color: #374151;
}

.clone-source-info {
  margin-bottom: 16px;
  padding: 12px;
  background: #f0fdf4;
  border-radius: 8px;
  display: flex;
  align-items: center;
  gap: 8px;
  color: #166534;
}

.clone-source-info i {
  color: #22c55e;
}
</style>
