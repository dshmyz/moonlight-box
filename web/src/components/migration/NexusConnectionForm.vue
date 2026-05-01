<template>
  <div class="nexus-connection-form">
    <h3>连接 Nexus</h3>
    <el-form :model="form" label-width="100px">
      <el-form-item label="URL">
        <el-input v-model="form.url" placeholder="https://nexus.example.com" />
      </el-form-item>
      <el-form-item label="用户名">
        <el-input v-model="form.username" placeholder="admin" />
      </el-form-item>
      <el-form-item label="密码">
        <el-input v-model="form.password" type="password" show-password />
      </el-form-item>
      <el-form-item>
        <el-button type="primary" :loading="testing" @click="testConnection">
          测试连接
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
