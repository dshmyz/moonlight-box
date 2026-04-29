<template>
  <el-form :model="form" label-width="140px" style="max-width: 600px;">
    <el-form-item label="启用 CAS 登录">
      <el-switch v-model="form.enabled" />
    </el-form-item>

    <el-form-item label="CAS 服务器地址">
      <el-input v-model="form.server_url" placeholder="https://cas.example.com" />
    </el-form-item>

    <el-form-item label="Service URL">
      <el-input v-model="form.service_url" placeholder="https://your-app.com/api/v1/auth/cas/callback" />
    </el-form-item>

    <el-form-item label="登录路径">
      <el-input v-model="form.login_path" placeholder="/cas/login" />
    </el-form-item>

    <el-form-item label="验证路径">
      <el-input v-model="form.validate_path" placeholder="/cas/serviceValidate" />
    </el-form-item>
  </el-form>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import type { CASConfig } from '@/api/casConfig'

interface Props {
  modelValue: CASConfig
}

interface Emits {
  (e: 'update:modelValue', value: CASConfig): void
}

const props = defineProps<Props>()
const emit = defineEmits<Emits>()

const form = ref<CASConfig>({ ...props.modelValue })

watch(
  () => props.modelValue,
  (newVal) => {
    form.value = { ...newVal }
  },
  { deep: true }
)

watch(
  form,
  (newVal) => {
    emit('update:modelValue', { ...newVal })
  },
  { deep: true }
)
</script>
