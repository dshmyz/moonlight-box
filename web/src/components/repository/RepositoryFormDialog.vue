<template>
  <el-dialog
    v-model="visible"
    :title="isEditMode ? '编辑仓库' : '创建仓库'"
    width="720px"
    :close-on-click-modal="false"
    class="repo-form-dialog"
    @close="handleClose"
  >
    <el-form
      ref="formRef"
      :model="formData"
      :rules="formRules"
      label-width="100px"
      label-position="right"
    >
      <el-tabs v-model="activeTab" class="repo-form-tabs">
        <el-tab-pane name="basic">
          <template #label>
            <span class="tab-label">
              <el-icon><Document /></el-icon>
              基础信息
            </span>
          </template>
          <BasicInfoForm
            :form="formData"
            :disabled="isEditMode"
            v-model:storage-backend-id="storageBackendId"
          />
        </el-tab-pane>

        <el-tab-pane v-if="formData.type === 'proxy'" name="proxy">
          <template #label>
            <span class="tab-label">
              <el-icon><Link /></el-icon>
              代理与认证
            </span>
          </template>
          <AuthConfigForm
            :form="formData"
            :auth-config="authConfig"
          />
        </el-tab-pane>

        <el-tab-pane v-if="formData.type === 'proxy'" name="advanced">
          <template #label>
            <span class="tab-label">
              <el-icon><Setting /></el-icon>
              高级设置
            </span>
          </template>
          <TimeoutConfigForm :form="formData" />
        </el-tab-pane>

        <el-tab-pane name="cache">
          <template #label>
            <span class="tab-label">
              <el-icon><Coin /></el-icon>
              缓存配置
            </span>
          </template>
          <CacheConfigForm
            :form="formData"
            @update:failure-rules="handleFailureRulesUpdate"
          />
        </el-tab-pane>

        <el-tab-pane
          v-if="formData.type === 'local' || formData.type === 'proxy'"
          name="permissions"
        >
          <template #label>
            <span class="tab-label">
              <el-icon><Lock /></el-icon>
              权限控制
            </span>
          </template>
          <PermissionsConfigForm :form="formData" />
        </el-tab-pane>

        <el-tab-pane v-if="formData.type === 'virtual'" name="members">
          <template #label>
            <span class="tab-label">
              <el-icon><Connection /></el-icon>
              成员仓库
            </span>
          </template>
          <VirtualMembersForm
            :members-text="membersText"
            :package-type="formData.package_type"
            @update:members-text="handleMembersUpdate"
          />
        </el-tab-pane>
      </el-tabs>
    </el-form>

    <template #footer>
      <div class="dialog-footer">
        <el-button @click="visible = false">取消</el-button>
        <el-button type="primary" :loading="submitting" @click="handleSubmit">
          {{ isEditMode ? '保存' : '创建' }}
        </el-button>
      </div>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import type { FormInstance, FormRules } from 'element-plus'
import { ElMessage } from 'element-plus'
import { Document, Link, Setting, Coin, Lock, Connection } from '@element-plus/icons-vue'
import { repositoryApi, type Repository, type ProxyAuthConfig } from '@/api/repository'
import BasicInfoForm from './BasicInfoForm.vue'
import AuthConfigForm from './AuthConfigForm.vue'
import TimeoutConfigForm from './TimeoutConfigForm.vue'
import CacheConfigForm from './CacheConfigForm.vue'
import PermissionsConfigForm from './PermissionsConfigForm.vue'
import VirtualMembersForm from './VirtualMembersForm.vue'

interface Props {
  modelValue: boolean
  editData?: Repository | null
}

const props = withDefaults(defineProps<Props>(), {
  editData: null,
})

const emit = defineEmits<{
  'update:modelValue': [value: boolean]
  submit: []
}>()

const visible = computed({
  get: () => props.modelValue,
  set: (val) => emit('update:modelValue', val),
})

const isEditMode = computed(() => !!props.editData)
const formRef = ref<FormInstance>()
const submitting = ref(false)
const activeTab = ref('basic')

interface RepoFormData {
  name: string
  display_name: string
  description: string
  type: 'local' | 'proxy' | 'virtual'
  package_type: string
  remote_url: string
  auth_type: string
  auth_config: string
  proxy_priority: number
  timeout_seconds: number
  max_redirects: number
  insecure_skip_verify: boolean
  cache_enabled: boolean
  cache_ttl_seconds: number
  cache_negative_ttl: number
  cache_max_size_gb: number
  failure_cache_rules: string
  allow_overwrite: boolean
  allow_delete: boolean
}

const defaultFormData = (): RepoFormData => ({
  name: '',
  display_name: '',
  description: '',
  type: 'local',
  package_type: 'npm',
  remote_url: '',
  auth_type: 'none',
  auth_config: '',
  proxy_priority: 0,
  timeout_seconds: 0,
  max_redirects: 0,
  insecure_skip_verify: false,
  cache_enabled: true,
  cache_ttl_seconds: 86400,
  cache_negative_ttl: 300,
  cache_max_size_gb: 10,
  failure_cache_rules: '',
  allow_overwrite: false,
  allow_delete: false,
})

const formData = ref<RepoFormData>(defaultFormData())

const authConfig = ref({
  username: '',
  password: '',
  token: '',
  header_name: '',
  key_value: '',
})

const membersText = ref('')
const storageBackendId = ref<number | null>(null)

const formRules = computed<FormRules>(() => ({
  name: [{ required: true, message: '请输入仓库名称', trigger: 'blur' }],
  type: [{ required: true, message: '请选择仓库类型', trigger: 'change' }],
  package_type: [{ required: true, message: '请选择包类型', trigger: 'change' }],
  remote_url: [
    {
      required: formData.value.type === 'proxy',
      message: '代理仓库必须填写远程地址',
      trigger: 'blur',
    },
  ],
}))

const resetForm = () => {
  formData.value = defaultFormData()
  authConfig.value = {
    username: '',
    password: '',
    token: '',
    header_name: '',
    key_value: '',
  }
  membersText.value = ''
  storageBackendId.value = null
  activeTab.value = 'basic'
  formRef.value?.clearValidate()
}

watch(
  () => props.editData,
  (repo) => {
    if (repo) {
      const cfg = repo.config || {}

      formData.value = {
        name: repo.name,
        display_name: repo.display_name || '',
        description: repo.description || '',
        type: repo.type,
        package_type: repo.package_type || 'npm',
        remote_url: cfg.remote_url || '',
        auth_type: cfg.auth_type || 'none',
        auth_config: '',
        proxy_priority: cfg.proxy_priority ?? 0,
        timeout_seconds: cfg.timeout_seconds ?? 0,
        max_redirects: cfg.max_redirects ?? 0,
        insecure_skip_verify: cfg.insecure_skip_verify ?? false,
        cache_enabled: cfg.cache_enabled ?? true,
        cache_ttl_seconds: cfg.cache_ttl_seconds ?? 86400,
        cache_negative_ttl: cfg.cache_negative_ttl ?? 300,
        cache_max_size_gb: cfg.cache_max_size_gb ?? 10,
        failure_cache_rules: cfg.failure_cache_rules || '',
        allow_overwrite: repo.allow_overwrite ?? false,
        allow_delete: repo.allow_delete ?? false,
      }

      storageBackendId.value = repo.storage_backend_id || null

      if (cfg.auth) {
        const auth = cfg.auth
        if (auth.type === 'basic' && auth.basic) {
          authConfig.value = {
            username: auth.basic.username || '',
            password: auth.basic.password || '',
            token: '',
            header_name: '',
            key_value: '',
          }
        } else if (auth.type === 'bearer' && auth.bearer) {
          authConfig.value = {
            username: '',
            password: '',
            token: auth.bearer.token || '',
            header_name: '',
            key_value: '',
          }
        } else if (auth.type === 'api_key' && auth.api_key) {
          authConfig.value = {
            username: '',
            password: '',
            token: '',
            header_name: auth.api_key.header_name || '',
            key_value: auth.api_key.key_value || '',
          }
        } else {
          authConfig.value = {
            username: '',
            password: '',
            token: '',
            header_name: '',
            key_value: '',
          }
        }
      } else {
        authConfig.value = {
          username: '',
          password: '',
          token: '',
          header_name: '',
          key_value: '',
        }
      }

      if (repo.members && repo.members.length > 0) {
        membersText.value = repo.members
          .map(m => typeof m === 'string' ? m : m.member_repo?.name || '')
          .filter(Boolean)
          .join('\n')
      }
    } else {
      resetForm()
    }
  },
  { immediate: true }
)

const handleFailureRulesUpdate = (val: string) => {
  formData.value.failure_cache_rules = val
}

const handleMembersUpdate = (val: string) => {
  membersText.value = val
}

const buildProxyAuthConfig = (): ProxyAuthConfig | undefined => {
  if (!formData.value.auth_type || formData.value.auth_type === 'none') {
    return undefined
  }

  if (formData.value.auth_type === 'basic') {
    return {
      type: 'basic',
      basic: {
        username: authConfig.value.username,
        password: authConfig.value.password,
      },
    }
  }

  if (formData.value.auth_type === 'bearer') {
    return {
      type: 'bearer',
      bearer: {
        token: authConfig.value.token,
      },
    }
  }

  if (formData.value.auth_type === 'api_key') {
    return {
      type: 'api_key',
      api_key: {
        header_name: authConfig.value.header_name,
        key_value: authConfig.value.key_value,
      },
    }
  }

  return undefined
}

const buildSubmitData = (): Partial<Repository> => {
  const memberNames = formData.value.type === 'virtual'
    ? membersText.value
        .split('\n')
        .map(name => name.trim())
        .filter(Boolean)
    : undefined

  const data: Partial<Repository> = {
    name: formData.value.name,
    display_name: formData.value.display_name,
    description: formData.value.description,
    type: formData.value.type,
    package_type: formData.value.package_type,
    storage_backend_id: storageBackendId.value || undefined,
  }

  if (formData.value.type === 'proxy') {
    data.config = {
      remote_url: formData.value.remote_url,
      auth_type: formData.value.auth_type,
      auth: buildProxyAuthConfig(),
      proxy_priority: formData.value.proxy_priority,
      cache_enabled: formData.value.cache_enabled,
      cache_ttl_seconds: formData.value.cache_ttl_seconds,
      cache_negative_ttl: formData.value.cache_negative_ttl,
      cache_max_size_gb: formData.value.cache_max_size_gb,
      timeout_seconds: formData.value.timeout_seconds,
      max_redirects: formData.value.max_redirects,
      insecure_skip_verify: formData.value.insecure_skip_verify,
      failure_cache_rules: formData.value.failure_cache_rules || '',
    }
  } else {
    data.config = {
      cache_enabled: formData.value.cache_enabled,
      cache_ttl_seconds: formData.value.cache_ttl_seconds,
      cache_max_size_gb: formData.value.cache_max_size_gb,
    }
  }

  if (formData.value.type === 'local' || formData.value.type === 'proxy') {
    data.allow_overwrite = formData.value.allow_overwrite
    data.allow_delete = formData.value.allow_delete
  }

  if (formData.value.type === 'virtual') {
    data.members = memberNames
  }

  return data
}

const handleSubmit = async () => {
  if (!formRef.value) return

  try {
    await formRef.value.validate()
  } catch {
    ElMessage.error('请检查表单填写是否正确')
    return
  }

  submitting.value = true
  try {
    const data = buildSubmitData()

    if (isEditMode.value) {
      await repositoryApi.update(formData.value.name, data)
      ElMessage.success('更新成功')
    } else {
      await repositoryApi.create(data)
      ElMessage.success('创建成功')
    }

    visible.value = false
    emit('submit')
  } catch (err) {
    ElMessage.error(isEditMode.value ? '更新失败' : '创建失败')
  } finally {
    submitting.value = false
  }
}

const handleClose = () => {
  resetForm()
}
</script>

<style scoped>
.repo-form-tabs :deep(.el-tabs__content) {
  padding: 16px 0;
  min-height: 200px;
}

.repo-form-tabs :deep(.el-tabs__item) {
  font-size: 14px;
  font-weight: 500;
  padding: 0 12px;
}

.repo-form-tabs :deep(.el-tabs__header) {
  border-bottom: 1px solid #e5e6eb;
}

.tab-label {
  display: flex;
  align-items: center;
  gap: 4px;
}

.tab-label .el-icon {
  font-size: 14px;
}

.dialog-footer {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}
</style>
