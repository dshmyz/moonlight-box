<template>
  <div class="auth-config-form">
    <el-form-item label="远程地址" prop="remote_url">
      <el-input v-model="form.remote_url" placeholder="https://registry.npmjs.org" />
    </el-form-item>

    <el-form-item label="认证类型">
      <el-select v-model="form.auth_type" clearable style="width: 100%">
        <el-option label="无" value="none" />
        <el-option label="Basic Auth" value="basic" />
        <el-option label="Bearer Token" value="bearer" />
        <el-option label="API Key" value="api_key" />
      </el-select>
    </el-form-item>

    <!-- Basic Auth 配置 -->
    <template v-if="form.auth_type === 'basic'">
      <el-form-item label="用户名">
        <el-input v-model="authConfig.username" placeholder="用户名" />
      </el-form-item>
      <el-form-item label="密码">
        <el-input
          v-model="authConfig.password"
          type="password"
          show-password
          placeholder="密码"
        />
      </el-form-item>
    </template>

    <!-- Bearer Token 配置 -->
    <el-form-item v-if="form.auth_type === 'bearer'" label="Token">
      <el-input
        v-model="authConfig.token"
        type="password"
        show-password
        placeholder="Bearer Token"
      />
    </el-form-item>

    <!-- API Key 配置 -->
    <template v-if="form.auth_type === 'api_key'">
      <el-form-item label="Header Name">
        <el-input v-model="authConfig.header_name" placeholder="X-API-Key" />
      </el-form-item>
      <el-form-item label="Key Value">
        <el-input
          v-model="authConfig.key_value"
          type="password"
          show-password
          placeholder="API Key 值"
        />
      </el-form-item>
    </template>

    <el-form-item label="优先级">
      <el-input-number v-model="form.proxy_priority" :min="0" :max="100" :step="1" />
      <span class="form-hint">数字越小优先级越高</span>
    </el-form-item>
  </div>
</template>

<script setup lang="ts">
interface AuthConfig {
  username: string
  password: string
  token: string
  header_name: string
  key_value: string
}

interface FormModel {
  remote_url: string
  auth_type: string
  auth_config: string
  proxy_priority: number
}

interface Props {
  form: FormModel
  authConfig: AuthConfig
}

defineProps<Props>()
</script>

<style scoped>
.auth-config-form {
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
