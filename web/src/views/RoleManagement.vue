<template>
  <div class="role-management">
    <div class="page-header">
      <h2>角色管理</h2>
      <CustomButton type="primary" @click="showCreateDialog">
        <el-icon><Plus /></el-icon> 新建角色
      </CustomButton>
    </div>

    <CustomTable :columns="roleColumns" :data="roles" :loading="loading" row-key="id">
      <template #is_system_role="{ row }">
        <CustomTag v-if="row.is_system_role" type="info" size="small">是</CustomTag>
        <span v-else>否</span>
      </template>
      <template #permissions="{ row }">
        {{ row.permissions?.length || 0 }}
      </template>
      <template #created_at="{ row }">
        {{ formatDate(row.created_at) }}
      </template>
      <template #actions="{ row }">
        <div class="action-buttons">
          <CustomButton size="small" @click="showPermissionDialog(row)">配置权限</CustomButton>
          <CustomButton size="small" type="outline" @click="deleteRole(row)" :disabled="row.is_system_role">删除</CustomButton>
        </div>
      </template>
    </CustomTable>

    <CustomDialog v-model="createVisible" title="新建角色" width="500px">
      <el-form :model="createForm" label-width="80px">
        <el-form-item label="角色名称" required>
          <CustomInput v-model="createForm.name" placeholder="请输入角色名称（英文）" />
        </el-form-item>
        <el-form-item label="描述">
          <textarea
            v-model="createForm.description"
            class="custom-textarea"
            rows="3"
            placeholder="请输入角色描述"
          />
        </el-form-item>
      </el-form>
      <template #footer>
        <CustomButton @click="createVisible = false">取消</CustomButton>
        <CustomButton type="primary" @click="createRole">确定</CustomButton>
      </template>
    </CustomDialog>

    <CustomDialog v-model="permissionVisible" title="配置权限" width="600px">
      <div class="permission-header">
        <span>角色：<strong>{{ currentRole?.name }}</strong></span>
      </div>
      <CustomTable :columns="permissionColumns" :data="groupedPermissions" row-key="resource">
        <template #actions="{ row }">
          <el-checkbox-group v-model="selectedPermissions">
            <el-checkbox v-for="action in row.actions" :key="action.id" :label="action.id">
              {{ actionLabel(action.action) }}
            </el-checkbox>
          </el-checkbox-group>
        </template>
      </CustomTable>
      <template #footer>
        <CustomButton @click="permissionVisible = false">取消</CustomButton>
        <CustomButton type="primary" @click="updatePermissions">保存</CustomButton>
      </template>
    </CustomDialog>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { Plus } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { roleApi, type Role, type Permission } from '@/api/role'
import CustomButton from '@/components/ui/CustomButton.vue'
import CustomInput from '@/components/ui/CustomInput.vue'
import CustomTable from '@/components/ui/CustomTable.vue'
import CustomTag from '@/components/ui/CustomTag.vue'
import CustomDialog from '@/components/ui/CustomDialog.vue'

const loading = ref(false)
const roles = ref<Role[]>([])
const allPermissions = ref<Permission[]>([])

const createVisible = ref(false)
const createForm = ref({ name: '', description: '' })

const permissionVisible = ref(false)
const currentRole = ref<Role | null>(null)
const selectedPermissions = ref<number[]>([])

const roleColumns = [
  { prop: 'id', label: 'ID', width: '60px' },
  { prop: 'name', label: '角色名称', width: '150px' },
  { prop: 'description', label: '描述' },
  { prop: 'is_system_role', label: '系统角色', width: '100px' },
  { prop: 'permissions', label: '权限数量', width: '100px' },
  { prop: 'created_at', label: '创建时间', width: '180px' },
  { prop: 'actions', label: '操作', width: '200px' },
]

const permissionColumns = [
  { prop: 'resource', label: '资源', width: '150px' },
  { prop: 'actions', label: '权限' },
]

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

function formatDate(d: string): string {
  if (!d) return '-'
  return new Date(d).toLocaleString('zh-CN')
}

function actionLabel(action: string): string {
  const map: Record<string, string> = {
    read: '读取',
    write: '写入',
    delete: '删除',
  }
  return map[action] || action
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
    // 用户取消或删除失败
  }
}

onMounted(() => {
  loadRoles()
  loadPermissions()
})
</script>

<style scoped>
.role-management {
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
  font-size: var(--font-size-2xl);
  font-weight: var(--font-weight-semibold);
  color: var(--color-text-primary);
}

.action-buttons {
  display: flex;
  gap: var(--spacing-sm);
}

.permission-header {
  margin-bottom: var(--spacing-lg);
  padding-bottom: var(--spacing-md);
  border-bottom: 1px solid var(--color-border);
  font-size: var(--font-size-base);
  color: var(--color-text-primary);
}

.permission-header strong {
  font-weight: var(--font-weight-semibold);
}

.custom-textarea {
  width: 100%;
  padding: 10px 14px;
  font-size: var(--font-size-base);
  font-family: inherit;
  color: var(--color-text-primary);
  background: #fafbfc;
  border: 2px solid #e2e8f0;
  border-radius: var(--radius-lg);
  outline: none;
  resize: vertical;
  transition: all var(--transition-base);
  line-height: 1.5;
}

.custom-textarea::placeholder {
  color: #94a3b8;
}

.custom-textarea:hover:not(:focus) {
  border-color: #0f172a;
  background: #ffffff;
}

.custom-textarea:focus {
  border-color: #0f172a;
  background: #ffffff;
  box-shadow: 0 0 0 3px rgba(15, 23, 42, 0.08);
}
</style>
