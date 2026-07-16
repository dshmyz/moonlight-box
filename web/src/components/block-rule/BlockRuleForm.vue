<template>
  <el-form :model="form" :rules="rules" ref="formRef" label-width="100px">
    <el-form-item label="包名" prop="package_name">
      <el-input v-model="form.package_name" placeholder="lodash 或 *@scope/*" />
    </el-form-item>

    <el-form-item label="版本" prop="version">
      <el-input v-model="form.version" :placeholder="versionPlaceholder" />
      <div v-if="form.match_type === 'wildcard'" class="form-tip">
        支持 * 通配符，如 4.* 匹配所有 4.x 版本
      </div>
      <div v-else-if="form.match_type === 'range'" class="form-tip">
        支持 SemVer 范围，如 &gt;=1.2.0 &lt;2.0.0、^1.2.0、2.1.10-2.1.101
      </div>
    </el-form-item>

    <el-form-item label="匹配类型" prop="match_type">
      <el-select v-model="form.match_type" style="width: 100%">
        <el-option label="精确匹配" value="exact" />
        <el-option label="通配符匹配" value="wildcard" />
        <el-option label="版本范围" value="range" />
      </el-select>
    </el-form-item>

    <el-form-item label="包类型" prop="package_type">
      <el-select v-model="form.package_type" style="width: 100%">
        <el-option label="全部" value="all" />
        <el-option label="npm" value="npm" />
        <el-option label="Maven" value="maven" />
        <el-option label="PyPI" value="pypi" />
        <el-option label="Go" value="go" />
        <el-option label="Yum" value="yum" />
        <el-option label="Apt" value="apt" />
        <el-option label="Generic" value="generic" />
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

    <el-form-item label="条件类型" prop="condition_type">
      <el-select v-model="form.condition_type" style="width: 100%" @change="handleConditionTypeChange">
        <el-option label="无" value="" />
        <el-option label="按 License" value="license" />
        <el-option label="按发布时间" value="publish_time" />
      </el-select>
    </el-form-item>

    <el-form-item v-if="form.condition_type" label="操作符" prop="condition_op">
      <el-select v-model="form.condition_op" style="width: 100%" @change="handleConditionOpChange">
        <el-option
          v-for="op in operatorOptions"
          :key="op.value"
          :label="op.label"
          :value="op.value"
        />
      </el-select>
    </el-form-item>

    <el-form-item v-if="form.condition_type" label="条件值" prop="condition_value">
      <el-input
        v-if="form.condition_type === 'license'"
        v-model="form.condition_value"
        placeholder="如 GPL-3.0"
      />
      <el-input
        v-else-if="form.condition_type === 'publish_time' && form.condition_op === 'within_last'"
        v-model="form.condition_value"
        placeholder="输入天数，如 15"
        type="number"
      />
      <el-date-picker
        v-else-if="form.condition_type === 'publish_time'"
        v-model="form.condition_value"
        type="datetime"
        placeholder="选择日期时间"
        format="YYYY-MM-DD HH:mm:ss"
        value-format="YYYY-MM-DDTHH:mm:ss[Z]"
        style="width: 100%"
      />
    </el-form-item>

    <el-form-item label="启用">
      <el-switch v-model="form.enabled" />
    </el-form-item>
  </el-form>
</template>

<script setup lang="ts">
import { ref, watch, computed } from 'vue'
import type { FormInstance, FormRules } from 'element-plus'
import type { BlockRuleCreateParams } from '@/api/blockRule'

const props = defineProps<{
  modelValue: BlockRuleCreateParams
}>()

const emit = defineEmits<{
  'update:modelValue': [value: BlockRuleCreateParams]
}>()

const formRef = ref<FormInstance>()

// 条件类型默认为"无"
const form = ref<BlockRuleCreateParams>({
  ...props.modelValue,
  condition_type: props.modelValue.condition_type ?? '',
  condition_op: props.modelValue.condition_op ?? '',
  condition_value: props.modelValue.condition_value ?? '',
})

watch(
  () => props.modelValue,
  (val) => {
    // 值未变时跳过，避免与下方 watch(form) 形成回环更新
    if (
      val.package_name === form.value.package_name &&
      val.version === form.value.version &&
      val.match_type === form.value.match_type &&
      val.package_type === form.value.package_type &&
      val.reason === form.value.reason &&
      val.enabled === form.value.enabled &&
      (val.condition_type ?? '') === form.value.condition_type &&
      (val.condition_op ?? '') === form.value.condition_op &&
      (val.condition_value ?? '') === form.value.condition_value
    ) {
      return
    }
    form.value = {
      ...val,
      condition_type: val.condition_type ?? '',
      condition_op: val.condition_op ?? '',
      condition_value: val.condition_value ?? '',
    }
  },
  { deep: true }
)

watch(
  form,
  (val) => {
    emit('update:modelValue', { ...val })
  },
  { deep: true, flush: 'post' }
)

// 各条件类型对应的操作符选项
const operatorOptions = computed(() => {
  switch (form.value.condition_type) {
    case 'license':
      return [
        { label: '等于', value: 'equals' },
        { label: '包含', value: 'contains' },
      ]
    case 'publish_time':
      return [
        { label: '早于', value: 'before' },
        { label: '晚于', value: 'after' },
        { label: '最近 N 天内', value: 'within_last' },
      ]
    default:
      return []
  }
})

const versionPlaceholder = computed(() => {
  switch (form.value.match_type) {
    case 'wildcard':
      return '4.*'
    case 'range':
      return '>=1.2.0 <2.0.0 或 2.1.10-2.1.101'
    default:
      return '4.17.20'
  }
})

// 条件类型变化时：重置操作符与条件值，避免脏数据
const handleConditionTypeChange = () => {
  form.value.condition_op = ''
  form.value.condition_value = ''
}

// 操作符变化时：publish_time 下 before/after 与 within_last 的值格式不同，需清空
const handleConditionOpChange = () => {
  form.value.condition_value = ''
}

// 动态校验规则：选了条件类型时，操作符与条件值必填
const validateConditionOp = (_rule: any, value: string, callback: (err?: Error) => void) => {
  if (form.value.condition_type && !value) {
    callback(new Error('请选择操作符'))
  } else {
    callback()
  }
}

const validateConditionValue = (_rule: any, value: string, callback: (err?: Error) => void) => {
  if (form.value.condition_type && !value) {
    callback(new Error('请输入条件值'))
  } else {
    callback()
  }
}

const rules: FormRules = {
  package_name: [{ required: true, message: '请输入包名', trigger: 'blur' }],
  version: [{ required: true, message: '请输入版本号', trigger: 'blur' }],
  match_type: [{ required: true, message: '请选择匹配类型', trigger: 'change' }],
  package_type: [{ required: true, message: '请选择包类型', trigger: 'change' }],
  condition_op: [{ validator: validateConditionOp, trigger: 'change' }],
  condition_value: [{ validator: validateConditionValue, trigger: ['change', 'blur'] }],
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
