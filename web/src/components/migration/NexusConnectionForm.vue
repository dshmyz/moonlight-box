<template>
  <div class="nexus-connection-form">
    <div class="form-header">
      <div class="form-icon">
        <i class="fa-solid fa-plug"></i>
      </div>
      <div class="form-title">
        <h3>连接 Nexus</h3>
        <p class="form-desc">请输入 Nexus 服务器的连接信息</p>
      </div>
    </div>

    <el-form :model="form" label-position="top" class="connection-form">
      <el-form-item label="服务器地址" class="form-item">
        <el-input 
          v-model="form.url" 
          placeholder="https://nexus.example.com"
          prefix-icon="Link"
        />
      </el-form-item>
      
      <el-form-item label="用户名" class="form-item">
        <el-input 
          v-model="form.username" 
          placeholder="请输入用户名"
          prefix-icon="User"
        />
      </el-form-item>
      
      <el-form-item label="密码" class="form-item">
        <el-input 
          v-model="form.password" 
          type="password" 
          show-password 
          placeholder="请输入密码"
          prefix-icon="Lock"
        />
      </el-form-item>
      
      <el-form-item class="form-actions">
        <el-button type="primary" :loading="testing" @click="testConnection" class="submit-btn">
          <i class="fa-solid fa-circle-check"></i>
          测试连接并继续
        </el-button>
      </el-form-item>
    </el-form>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { testNexusConnection } from '@/api/migration'
import { ElMessage } from 'element-plus'

const emit = defineEmits<{
  connected: [data: { url: string; username: string; password: string }]
}>()

const form = ref({
  url: '',
  username: '',
  password: '',
})

const testing = ref(false)

async function testConnection() {
  if (!form.value.url) {
    ElMessage.warning('请输入服务器地址')
    return
  }
  
  testing.value = true
  try {
    await testNexusConnection(form.value)
    ElMessage.success('连接成功')
    emit('connected', { ...form.value })
  } catch (e: any) {
    ElMessage.error(e.message || '连接失败')
  } finally {
    testing.value = false
  }
}
</script>

<style scoped>
.nexus-connection-form {
  max-width: 560px;
  margin: 0 auto;
}

.form-header {
  display: flex;
  align-items: flex-start;
  gap: 16px;
  margin-bottom: 32px;
  padding-bottom: 24px;
  border-bottom: 1px solid #f0f0f0;
}

.form-icon {
  width: 48px;
  height: 48px;
  border-radius: 12px;
  background: linear-gradient(135deg, #8b5cf6 0%, #7c3aed 100%);
  display: flex;
  align-items: center;
  justify-content: center;
  color: #fff;
  font-size: 20px;
  flex-shrink: 0;
}

.form-title h3 {
  font-size: 18px;
  font-weight: 600;
  margin: 0 0 4px;
  color: #1f2937;
}

.form-desc {
  font-size: 13px;
  color: #9ca3af;
  margin: 0;
  line-height: 1.5;
}

.connection-form {
  padding: 8px 0;
}

.form-item {
  margin-bottom: 24px;
}

.form-item :deep(.el-form-item__label) {
  font-weight: 500;
  color: #374151;
  font-size: 14px;
  line-height: 1.5;
  margin-bottom: 8px;
  height: auto;
}

.form-item :deep(.el-input__wrapper) {
  border-radius: 10px;
  padding: 12px 16px;
  box-shadow: 0 1px 2px rgba(0, 0, 0, 0.05);
  transition: all 0.2s ease;
}

.form-item :deep(.el-input__wrapper:hover) {
  box-shadow: 0 2px 6px rgba(139, 92, 246, 0.15);
}

.form-item :deep(.el-input__wrapper.is-focus) {
  box-shadow: 0 0 0 3px rgba(139, 92, 246, 0.1);
}

.form-actions {
  margin-top: 32px;
  margin-bottom: 0;
}

.submit-btn {
  width: 100%;
  height: 44px;
  border-radius: 10px;
  font-weight: 500;
  font-size: 15px;
  letter-spacing: 0.2px;
  background: linear-gradient(135deg, #8b5cf6 0%, #7c3aed 100%);
  border: none;
  transition: all 0.2s ease;
}

.submit-btn:hover {
  transform: translateY(-1px);
  box-shadow: 0 4px 12px rgba(139, 92, 246, 0.35);
}

.submit-btn:active {
  transform: translateY(0);
}

.submit-btn i {
  margin-right: 6px;
}
</style>
