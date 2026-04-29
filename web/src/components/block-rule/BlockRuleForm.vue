<template>
  <el-form :model="form" :rules="rules" ref="formRef" label-width="100px">
    <el-form-item label="包名" prop="package_name">
      <el-input v-model="form.package_name" placeholder="lodash 或 *@scope/*" />
    </el-form-item>

    <el-form-item label="版本" prop="version">
      <el-input v-model="form.version" placeholder="4.17.20 或 4.*" />
      <div v-if="form.match_type === 'wildcard'" class="form-tip">
        支持 * 通配符，如 4.* 匹配所有 4.x 版本
      </div>
    </el-form-item>

    <el-form-item label="匹配类型" prop="match_type">
      <el-select v-model="form.match_type" style="width: 100%">
        <el-option label="精确匹配" value="exact" />
        <el-option label="通配符匹配" value="wildcard" />
      </el-select>
    </el-form-item>

    <el-form-item label="包类型" prop="package_type">
      <el-select v-model="form.package_type" style="width: 100%">
        <el-option label="npm" value="npm" />
        <el-option label="maven" value="maven" />
      </el-select>
    </el-form-item>

    <el-form-item label="阻断原因" prop="reason">
      <el-input
        v-model="form.reason"
        type="textarea"
        :rows="3"
        placeholder="请说明阻断原因，如：存在严重安全漏洞"
      />
    </el-form-item>

    <el-form-item label="启用">
      <el-switch v-model="form.enabled" />
    </el-form-item>
  </el-form>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import type { FormInstance, FormRules } from 'element-plus'
import type { BlockRuleCreateParams } from '@/api/blockRule'

const props = defineProps<{
  modelValue: BlockRuleCreateParams
}>()

const emit = defineEmits<{
  'update:modelValue': [value: BlockRuleCreateParams]
}>()

const formRef = ref<FormInstance>()

const form = ref({ ...props.modelValue })

watch(
  () => props.modelValue,
  (val) => {
    form.value = { ...val }
  },
  { deep: true }
)

watch(
  form,
  (val) => {
    emit('update:modelValue', { ...val })
  },
  { deep: true }
)

const rules: FormRules = {
  package_name: [{ required: true, message: '请输入包名', trigger: 'blur' }],
  version: [{ required: true, message: '请输入版本号', trigger: 'blur' }],
  match_type: [{ required: true, message: '请选择匹配类型', trigger: 'change' }],
  package_type: [{ required: true, message: '请选择包类型', trigger: 'change' }],
}

defineExpose({ formRef })
</script>

<style scoped>
.form-tip {
  font-size: 12px;
  color: #909399;
  margin-top: 4px;
  line-height: 1.4;
}
</style>
