<template>
  <div class="cache-config-form">
    <el-form-item label="启用缓存">
      <el-switch v-model="form.cache_enabled" />
    </el-form-item>

    <template v-if="form.cache_enabled">
      <el-form-item label="缓存 TTL（秒）">
        <el-input-number
          v-model="form.cache_ttl_seconds"
          :min="0"
          :step="100"
          controls-position="right"
        />
      </el-form-item>

      <el-form-item v-if="isProxy" label="负向缓存 TTL（秒）">
        <el-input-number
          v-model="form.cache_negative_ttl"
          :min="0"
          :step="10"
          controls-position="right"
        />
        <span class="form-hint">404 响应的缓存时间</span>
      </el-form-item>

      <el-form-item label="缓存最大大小（GB）">
        <el-input-number
          v-model="form.cache_max_size_gb"
          :min="0"
          :step="1"
          controls-position="right"
        />
      </el-form-item>

      <el-form-item v-if="isProxy" label="失败缓存规则">
        <el-input
          v-model="failureRulesText"
          type="textarea"
          :rows="4"
          :placeholder="failureCachePlaceholder"
          @input="handleRulesChange"
        />
        <span class="form-hint">JSON 格式，例如：[{"status_code": 404, "ttl_seconds": 60}]</span>
      </el-form-item>
    </template>
  </div>
</template>

<script setup lang="ts">
import { ref, watch, computed } from 'vue'

interface FormModel {
  type: 'local' | 'proxy' | 'virtual'
  cache_enabled: boolean
  cache_ttl_seconds: number
  cache_negative_ttl: number
  cache_max_size_gb: number
  failure_cache_rules: string
}

interface Props {
  form: FormModel
}

const props = defineProps<Props>()
const emit = defineEmits<{
  'update:failureRules': [value: string]
}>()

const isProxy = computed(() => props.form.type === 'proxy')

const failureRulesText = ref(props.form.failure_cache_rules)

const failureCachePlaceholder = JSON.stringify(
  [
    { status_code: 404, ttl_seconds: 60 },
    { status_code_range: [500, 599], ttl_seconds: 30 },
  ],
  null,
  2,
)

watch(
  () => props.form.failure_cache_rules,
  (val) => {
    failureRulesText.value = val
  }
)

const handleRulesChange = () => {
  emit('update:failureRules', failureRulesText.value)
}
</script>

<style scoped>
.cache-config-form {
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
