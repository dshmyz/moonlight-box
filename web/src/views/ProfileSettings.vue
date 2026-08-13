<template>
  <div class="profile-settings">
    <header class="page-header">
      <div class="header-content">
        <div class="header-icon">
          <i class="fa-solid fa-user-circle"></i>
        </div>
        <div class="header-text">
          <h2>个人设置</h2>
          <p class="header-subtitle">管理您的个人信息和账户安全</p>
        </div>
      </div>
    </header>

    <div class="content-panel">
      <el-row :gutter="24">
        <el-col :span="12">
          <el-card class="settings-card">
            <template #header>
              <div class="card-header">
                <i class="fa-solid fa-id-card"></i>
                <span>基本信息</span>
              </div>
            </template>
            <el-form :model="profileForm" label-width="80px" v-loading="loading">
              <el-form-item label="用户名">
                <el-input :value="profile?.username" disabled class="disabled-input" />
              </el-form-item>
              <el-form-item label="显示名称">
                <el-input v-model="profileForm.display_name" placeholder="请输入显示名称" />
              </el-form-item>
              <el-form-item label="邮箱">
                <el-input v-model="profileForm.email" placeholder="请输入邮箱" />
              </el-form-item>
              <el-form-item label="角色">
                <div class="role-tags">
                  <el-tag
                    v-for="role in profile?.roles"
                    :key="role"
                    class="role-tag"
                  >
                    {{ role }}
                  </el-tag>
                </div>
              </el-form-item>
              <el-form-item>
                <el-button type="primary" class="submit-btn" @click="saveProfile">
                  <i class="fa-solid fa-check"></i> 保存修改
                </el-button>
              </el-form-item>
            </el-form>
          </el-card>
        </el-col>

        <el-col :span="12">
          <el-card class="settings-card">
            <template #header>
              <div class="card-header">
                <i class="fa-solid fa-lock"></i>
                <span>修改密码</span>
              </div>
            </template>
            <el-form :model="passwordForm" label-width="100px">
              <el-form-item label="当前密码">
                <el-input
                  v-model="passwordForm.old_password"
                  type="password"
                  show-password
                  placeholder="请输入当前密码"
                />
              </el-form-item>
              <el-form-item label="新密码">
                <el-input
                  v-model="passwordForm.new_password"
                  type="password"
                  show-password
                  placeholder="请输入新密码（至少6位）"
                />
              </el-form-item>
              <el-form-item label="确认新密码">
                <el-input
                  v-model="confirmPassword"
                  type="password"
                  show-password
                  placeholder="请再次输入新密码"
                />
              </el-form-item>
              <el-form-item>
                <el-button type="primary" class="submit-btn" @click="changePassword">
                  <i class="fa-solid fa-key"></i> 修改密码
                </el-button>
              </el-form-item>
            </el-form>
          </el-card>
        </el-col>
      </el-row>

      <!-- MCP 访问令牌 -->
      <el-card id="tokens" class="settings-card" style="margin-top: 24px">
        <template #header>
          <div class="card-header">
            <i class="fa-solid fa-key"></i>
            <span>访问令牌</span>
            <el-button type="primary" size="small" style="margin-left: auto" @click="showCreateToken = true">
              <i class="fa-solid fa-plus"></i> 生成新令牌
            </el-button>
          </div>
        </template>
        <p style="color: #909399; font-size: 13px; margin-bottom: 16px">
          用于 API 认证（CI/CD 发包、脚本自动化、MCP 客户端等）。生成后仅显示一次，请妥善保存。
        </p>
        <el-table :data="apiTokens" v-loading="tokensLoading" empty-text="暂无访问令牌">
          <el-table-column prop="name" label="名称" min-width="120" />
          <el-table-column prop="prefix" label="令牌前缀" min-width="140">
            <template #default="{ row }">
              <code style="background: #f5f7fa; padding: 2px 6px; border-radius: 4px">{{ row.prefix }}****</code>
            </template>
          </el-table-column>
          <el-table-column prop="created_at" label="创建时间" min-width="160">
            <template #default="{ row }">{{ formatTime(row.created_at) }}</template>
          </el-table-column>
          <el-table-column prop="last_used_at" label="最后使用" min-width="160">
            <template #default="{ row }">{{ row.last_used_at ? formatTime(row.last_used_at) : '未使用' }}</template>
          </el-table-column>
          <el-table-column label="操作" width="100" fixed="right">
            <template #default="{ row }">
              <el-popconfirm title="确定撤销此令牌？" @confirm="deleteToken(row.id)">
                <template #reference>
                  <el-button type="danger" size="small" link>撤销</el-button>
                </template>
              </el-popconfirm>
            </template>
          </el-table-column>
        </el-table>
      </el-card>
    </div>

    <!-- 生成令牌弹窗 -->
    <el-dialog v-model="showCreateToken" title="生成访问令牌" width="420px" @close="resetCreateToken">
      <el-form :model="createForm" label-width="80px">
        <el-form-item label="名称">
          <el-input v-model="createForm.name" placeholder="如：Claude Desktop" />
        </el-form-item>
        <el-form-item label="有效期">
          <el-select v-model="createForm.expires_in" placeholder="永久有效" clearable>
            <el-option label="30 天" value="720h" />
            <el-option label="90 天" value="2160h" />
            <el-option label="1 年" value="8760h" />
            <el-option label="永久" value="" />
          </el-select>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showCreateToken = false">取消</el-button>
        <el-button type="primary" @click="createToken" :loading="creating">生成</el-button>
      </template>
    </el-dialog>

    <!-- 显示生成的令牌 -->
    <el-dialog v-model="showTokenResult" title="令牌已生成" width="480px" :close-on-click-modal="false">
      <el-alert type="warning" :closable="false" style="margin-bottom: 16px">
        请立即复制此令牌，关闭后将无法再次查看。
      </el-alert>
      <div style="background: #f5f7fa; padding: 12px; border-radius: 8px; word-break: break-all; font-family: monospace">
        {{ generatedToken }}
      </div>
      <template #footer>
        <el-button @click="copyToken">复制令牌</el-button>
        <el-button type="primary" @click="showTokenResult = false">我已保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, nextTick } from 'vue'
import { ElMessage } from 'element-plus'
import { authApi, type UserProfile, type APIToken } from '@/api/auth'

const loading = ref(false)
const profile = ref<UserProfile | null>(null)
const profileForm = ref({ display_name: '', email: '' })
const passwordForm = ref({ old_password: '', new_password: '' })
const confirmPassword = ref('')

async function loadProfile() {
  loading.value = true
  try {
    const res = await authApi.getProfile()
    profile.value = res || null
    if (res) {
      profileForm.value = {
        display_name: res.display_name || '',
        email: res.email || '',
      }
    }
  } catch {
    console.error('Failed to load profile')
  } finally {
    loading.value = false
  }
}

async function saveProfile() {
  try {
    const res = await authApi.updateProfile(profileForm.value)
    profile.value = res || null
    ElMessage.success('个人信息更新成功')
  } catch {
    ElMessage.error('个人信息更新失败')
  }
}

async function changePassword() {
  if (!passwordForm.value.old_password) {
    ElMessage.error('请输入当前密码')
    return
  }
  if (!passwordForm.value.new_password || passwordForm.value.new_password.length < 6) {
    ElMessage.error('新密码长度至少为6位')
    return
  }
  if (passwordForm.value.new_password !== confirmPassword.value) {
    ElMessage.error('两次输入的新密码不一致')
    return
  }

  try {
    await authApi.changePassword(passwordForm.value)
    ElMessage.success('密码修改成功')
    passwordForm.value = { old_password: '', new_password: '' }
    confirmPassword.value = ''
  } catch {
    ElMessage.error('密码修改失败，请检查当前密码是否正确')
  }
}

onMounted(async () => {
  loadProfile()
  loadTokens()
  if (window.location.hash === '#tokens') {
    await nextTick()
    document.getElementById('tokens')?.scrollIntoView({ behavior: 'smooth' })
  }
})

// --- API Token 管理 ---
const apiTokens = ref<APIToken[]>([])
const tokensLoading = ref(false)
const showCreateToken = ref(false)
const showTokenResult = ref(false)
const creating = ref(false)
const generatedToken = ref('')
const createForm = ref({ name: '', expires_in: '' })

async function loadTokens() {
  tokensLoading.value = true
  try {
    const res = await authApi.listTokens()
    apiTokens.value = res || []
  } catch {
    console.error('Failed to load tokens')
  } finally {
    tokensLoading.value = false
  }
}

async function createToken() {
  if (!createForm.value.name) {
    ElMessage.error('请输入令牌名称')
    return
  }
  creating.value = true
  try {
    const res = await authApi.createToken(createForm.value)
    generatedToken.value = res.token
    showCreateToken.value = false
    showTokenResult.value = true
    loadTokens()
  } catch {
    ElMessage.error('生成令牌失败')
  } finally {
    creating.value = false
  }
}

async function deleteToken(id: number) {
  try {
    await authApi.deleteToken(id)
    ElMessage.success('令牌已撤销')
    loadTokens()
  } catch {
    ElMessage.error('撤销令牌失败')
  }
}

function resetCreateToken() {
  createForm.value = { name: '', expires_in: '' }
}

function copyToken() {
  navigator.clipboard.writeText(generatedToken.value)
  ElMessage.success('已复制到剪贴板')
}

function formatTime(t: string) {
  return new Date(t).toLocaleString('zh-CN')
}
</script>

<style scoped>
.profile-settings {
  min-height: 100%;
  background: linear-gradient(180deg, #f8fafc 0%, #f1f5f9 100%);
}

.page-header {
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
  background: linear-gradient(135deg, #2563eb 0%, #1d4ed8 100%);
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

.content-panel {
  padding: 20px;
}

.settings-card {
  border: none;
  border-radius: 12px;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.04);
}

.settings-card :deep(.el-card__header) {
  padding: 16px 20px;
  border-bottom: 1px solid #f3f4f6;
}

.card-header {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 16px;
  font-weight: 600;
  color: #374151;
}

.card-header i {
  color: #2563eb;
}

.disabled-input :deep(.el-input__wrapper) {
  background: #f9fafb;
  border-color: #e5e7eb;
}

.role-tags {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.role-tag {
  background: #eff6ff;
  color: #2563eb;
  border-color: #bfdbfe;
}

.submit-btn {
  height: 40px;
  padding: 0 24px;
  border-radius: 10px;
  font-weight: 500;
  display: flex;
  align-items: center;
  gap: 8px;
  background: linear-gradient(135deg, #2563eb 0%, #1d4ed8 100%);
  border-color: transparent;
}

.submit-btn:hover {
  background: linear-gradient(135deg, #1d4ed8 0%, #1e40af 100%);
}
</style>
