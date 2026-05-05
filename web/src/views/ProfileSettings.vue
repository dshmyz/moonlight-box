<template>
  <div class="profile-settings">
    <div class="page-header">
      <h2>个人设置</h2>
    </div>

    <div class="settings-grid">
      <CustomCard title="基本信息" hoverable class="settings-card">
        <el-form :model="profileForm" label-width="80px" v-loading="loading">
          <el-form-item label="用户名">
            <CustomInput :value="profile?.username" disabled />
          </el-form-item>
          <el-form-item label="显示名称">
            <CustomInput v-model="profileForm.display_name" placeholder="请输入显示名称" />
          </el-form-item>
          <el-form-item label="邮箱">
            <CustomInput v-model="profileForm.email" placeholder="请输入邮箱" />
          </el-form-item>
          <el-form-item label="角色">
            <CustomTag v-for="role in profile?.roles" :key="role" style="margin-right: 8px">{{ role }}</CustomTag>
          </el-form-item>
          <el-form-item>
            <CustomButton type="primary" @click="saveProfile">保存修改</CustomButton>
          </el-form-item>
        </el-form>
      </CustomCard>

      <CustomCard title="修改密码" hoverable class="settings-card">
        <el-form :model="passwordForm" label-width="100px">
          <el-form-item label="当前密码">
            <CustomInput v-model="passwordForm.old_password" type="password" show-password placeholder="请输入当前密码" />
          </el-form-item>
          <el-form-item label="新密码">
            <CustomInput v-model="passwordForm.new_password" type="password" show-password placeholder="请输入新密码（至少6位）" />
          </el-form-item>
          <el-form-item label="确认新密码">
            <CustomInput v-model="confirmPassword" type="password" show-password placeholder="请再次输入新密码" />
          </el-form-item>
          <el-form-item>
            <CustomButton type="primary" @click="changePassword">修改密码</CustomButton>
          </el-form-item>
        </el-form>
      </CustomCard>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { authApi, type UserProfile } from '@/api/auth'
import CustomButton from '@/components/ui/CustomButton.vue'
import CustomInput from '@/components/ui/CustomInput.vue'
import CustomCard from '@/components/ui/CustomCard.vue'
import CustomTag from '@/components/ui/CustomTag.vue'

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
  padding: var(--spacing-xl);
}

.page-header {
  margin-bottom: var(--spacing-2xl);
}

.page-header h2 {
  margin: 0;
  font-size: var(--font-size-xl);
  font-weight: var(--font-weight-semibold);
  color: var(--color-text-primary);
}

.settings-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: var(--spacing-xl);
}

.settings-card {
  margin-bottom: 0;
}
</style>
