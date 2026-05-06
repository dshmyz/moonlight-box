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
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { authApi, type UserProfile } from '@/api/auth'

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

onMounted(() => {
  loadProfile()
})
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
