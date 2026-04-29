<template>
  <div class="permissions-config-form">
    <el-form-item label="允许覆盖">
      <el-switch v-model="form.allow_overwrite" />
    </el-form-item>
    <el-form-item label="允许删除">
      <el-switch v-model="form.allow_delete" />
    </el-form-item>
  </div>
</template>

<script setup lang="ts">
interface FormModel {
  allow_overwrite: boolean
  allow_delete: boolean
}

interface Props {
  form: FormModel
}

defineProps<Props>()
</script>
