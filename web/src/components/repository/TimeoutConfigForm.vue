<template>
  <div class="timeout-config-form">
    <el-form-item label="超时时间（秒）">
      <el-input-number
        v-model="form.timeout_seconds"
        :min="0"
        :step="1"
        controls-position="right"
      />
      <span class="form-hint">0 表示使用全局默认值（30s）</span>
    </el-form-item>
    <el-form-item label="最大重定向次数">
      <el-input-number
        v-model="form.max_redirects"
        :min="-1"
        :step="1"
        controls-position="right"
      />
      <span class="form-hint">0 表示使用全局默认值（10），-1 表示不跟随重定向</span>
    </el-form-item>
    <el-form-item label="跳过证书校验">
      <el-switch v-model="form.insecure_skip_verify" />
    </el-form-item>
  </div>
</template>

<script setup lang="ts">
interface FormModel {
  timeout_seconds: number
  max_redirects: number
  insecure_skip_verify: boolean
}

interface Props {
  form: FormModel
}

defineProps<Props>()
</script>

<style scoped>
.timeout-config-form {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.form-hint {
  display: block;
  color: #86909c;
  font-size: 12px;
  line-height: 1.5;
  margin-top: 4px;
}
</style>
