<template>
  <div class="metadata-sync-config-form">
    <el-form-item label="启用元数据同步">
      <el-switch v-model="syncConfig.metadata_sync_enabled" />
      <div class="form-help">
        启用后将定期从远程仓库同步包的元数据（名称、版本列表等）
      </div>
    </el-form-item>

    <el-form-item
      v-if="syncConfig.metadata_sync_enabled"
      label="同步间隔"
    >
      <el-select v-model="syncConfig.metadata_sync_interval" style="width: 100%">
        <el-option label="每30分钟" :value="1800" />
        <el-option label="每1小时" :value="3600" />
        <el-option label="每6小时" :value="21600" />
        <el-option label="每12小时" :value="43200" />
        <el-option label="每天" :value="86400" />
      </el-select>
    </el-form-item>

    <el-form-item
      v-if="syncConfig.metadata_sync_enabled"
      label="同步模式"
    >
      <el-radio-group v-model="syncConfig.sync_mode">
        <el-radio value="metadata_only">仅元数据</el-radio>
        <el-radio value="full">完整同步</el-radio>
      </el-radio-group>
      <div class="form-help">
        <strong>仅元数据：</strong>只同步包的索引信息，下载时再从远程拉取文件<br/>
        <strong>完整同步：</strong>同步元数据的同时下载所有包文件
      </div>
    </el-form-item>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'

interface SyncConfigModel {
  metadata_sync_enabled: boolean
  metadata_sync_interval: number
  sync_mode: 'metadata_only' | 'full'
}

interface Props {
  form: {
    metadata_sync_enabled: boolean
    metadata_sync_interval: number
    sync_mode: 'metadata_only' | 'full'
  }
}

interface Emits {
  (e: 'update:form', value: SyncConfigModel): void
}

const props = defineProps<Props>()
const emit = defineEmits<Emits>()

const syncConfig = computed({
  get: () => props.form,
  set: (val: SyncConfigModel) => emit('update:form', val),
})
</script>

<style scoped>
.metadata-sync-config-form {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.form-help {
  font-size: 12px;
  color: #909399;
  margin-top: 5px;
  line-height: 1.5;
}
</style>
