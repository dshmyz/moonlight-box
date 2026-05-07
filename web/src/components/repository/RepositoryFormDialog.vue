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
            v-model:selected-package-types="selectedPackageTypes"
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

        <el-tab-pane v-if="formData.type === 'proxy'" name="sync">
          <template #label>
            <span class="tab-label">
              <el-icon><Refresh /></el-icon>
              元数据同步
            </span>
          </template>
          <MetadataSyncConfigForm :form="formData" />
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
import { Document, Link, Setting, Coin, Lock, Connection, Refresh } from '@element-plus/icons-vue'
import { repositoryApi, type Repository, type FailureCacheRule } from '@/api/repository'
import BasicInfoForm from './BasicInfoForm.vue'
import AuthConfigForm from './AuthConfigForm.vue'
import TimeoutConfigForm from './TimeoutConfigForm.vue'
import CacheConfigForm from './CacheConfigForm.vue'
import PermissionsConfigForm from './PermissionsConfigForm.vue'
import VirtualMembersForm from './VirtualMembersForm.vue'
import MetadataSyncConfigForm from './MetadataSyncConfigForm.vue'

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
  metadata_sync_enabled: boolean
  metadata_sync_interval: number
  sync_mode: 'metadata_only' | 'full'
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
  metadata_sync_enabled: false,
  metadata_sync_interval: 3600,
  sync_mode: 'metadata_only',
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
const selectedPackageTypes = ref<string[]>([])
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
  selectedPackageTypes.value = []
  storageBackendId.value = null
  activeTab.value = 'basic'
  formRef.value?.clearValidate()
}

watch(
  () => props.editData,
  (repo) => {
    if (repo) {
      formData.value = {
        name: repo.name,
        display_name: repo.display_name || '',
        description: repo.description || '',
        type: repo.type,
        package_type: repo.package_type || 'npm',
        remote_url: repo.remote_url || '',
        auth_type: repo.auth_type || 'none',
        auth_config: repo.auth_config || '',
        proxy_priority: repo.proxy_priority ?? 0,
        timeout_seconds: repo.timeout_seconds ?? 0,
        max_redirects: repo.max_redirects ?? 0,
        insecure_skip_verify: repo.insecure_skip_verify ?? false,
        cache_enabled: repo.cache_enabled ?? true,
        cache_ttl_seconds: repo.cache_ttl_seconds ?? 86400,
        cache_negative_ttl: repo.cache_negative_ttl ?? 300,
        cache_max_size_gb: repo.cache_max_size_gb ?? 10,
        failure_cache_rules: repo.failure_cache_rules
          ? JSON.stringify(repo.failure_cache_rules, null, 2)
          : '',
        allow_overwrite: repo.allow_overwrite ?? false,
        allow_delete: repo.allow_delete ?? false,
        metadata_sync_enabled: repo.metadata_sync_enabled ?? false,
        metadata_sync_interval: repo.metadata_sync_interval ?? 3600,
        sync_mode: repo.sync_mode ?? 'metadata_only',
      }

      storageBackendId.value = repo.storage_backend_id || null

      if (repo.auth_config) {
        try {
          const parsed = JSON.parse(repo.auth_config)
          authConfig.value = {
            username: parsed.username || '',
            password: parsed.password || '',
            token: parsed.token || '',
            header_name: parsed.header_name || '',
            key_value: parsed.key_value || '',
          }
        } catch {
          // ignore
        }
      }

      if (repo.members && repo.members.length > 0) {
        membersText.value = repo.members
          .map(m => m.member_repo?.name || '')
          .filter(Boolean)
          .join('\n')
      }

      if (repo.package_types) {
        try {
          selectedPackageTypes.value = repo.package_types.split(',').filter(Boolean)
        } catch {
          selectedPackageTypes.value = []
        }
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

const buildSubmitData = (): Partial<Repository> => {
  let authConfigJson = ''
  if (formData.value.auth_type && formData.value.auth_type !== 'none') {
    const config: Record<string, string> = {}
    if (formData.value.auth_type === 'basic') {
      config.username = authConfig.value.username
      config.password = authConfig.value.password
    } else if (formData.value.auth_type === 'bearer') {
      config.token = authConfig.value.token
    } else if (formData.value.auth_type === 'api_key') {
      config.header_name = authConfig.value.header_name
      config.key_value = authConfig.value.key_value
    }
    authConfigJson = JSON.stringify(config)
  }

  let failureCacheRules: FailureCacheRule[] | undefined
  if (formData.value.type === 'proxy' && formData.value.failure_cache_rules.trim()) {
    try {
      failureCacheRules = JSON.parse(formData.value.failure_cache_rules)
    } catch {
      failureCacheRules = undefined
    }
  }

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

  if (formData.value.type === 'virtual' && selectedPackageTypes.value.length > 0) {
    data.package_types = selectedPackageTypes.value.join(',')
    data.package_type = selectedPackageTypes.value[0]
  }

  if (formData.value.type === 'proxy') {
    data.remote_url = formData.value.remote_url
    data.auth_type = formData.value.auth_type
    data.auth_config = authConfigJson
    data.proxy_priority = formData.value.proxy_priority
    data.timeout_seconds = formData.value.timeout_seconds
    data.max_redirects = formData.value.max_redirects
    data.insecure_skip_verify = formData.value.insecure_skip_verify
    data.failure_cache_rules = failureCacheRules
    data.metadata_sync_enabled = formData.value.metadata_sync_enabled
    data.metadata_sync_interval = formData.value.metadata_sync_interval
    data.sync_mode = formData.value.sync_mode
  }

  if (formData.value.type === 'local' || formData.value.type === 'proxy') {
    data.allow_overwrite = formData.value.allow_overwrite
    data.allow_delete = formData.value.allow_delete
  }

  if (formData.value.type === 'virtual') {
    data.members = memberNames
  }

  data.cache_enabled = formData.value.cache_enabled
  data.cache_ttl_seconds = formData.value.cache_ttl_seconds
  data.cache_max_size_gb = formData.value.cache_max_size_gb

  if (formData.value.type === 'proxy') {
    data.cache_negative_ttl = formData.value.cache_negative_ttl
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
