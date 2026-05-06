<template>
  <div class="basic-info-form">
    <el-form-item label="仓库名称" prop="name">
      <el-input
        v-model="form.name"
        placeholder="例如：npm-local"
        :disabled="disabled"
      />
      <span class="form-hint">唯一标识，创建后不可修改</span>
    </el-form-item>
    
    <el-form-item label="仓库类型" prop="type">
      <el-select v-model="form.type" :disabled="disabled" style="width: 100%">
        <el-option label="Local（本地仓库）" value="local" />
        <el-option label="Proxy（代理仓库）" value="proxy" />
        <el-option label="Virtual（虚拟仓库）" value="virtual" />
      </el-select>
    </el-form-item>
    
    <el-form-item
      label="包类型"
      prop="package_type"
      v-if="form.type !== 'virtual'"
    >
      <el-select v-model="form.package_type" :disabled="disabled" style="width: 100%">
        <el-option label="npm" value="npm" />
        <el-option label="Maven" value="maven" />
        <el-option label="PyPI" value="pypi" />
        <el-option label="Go" value="go" />
        <el-option label="NuGet" value="nuget" />
        <el-option label="Yum" value="yum" />
        <el-option label="Apt" value="apt" />
        <el-option label="Generic" value="generic" />
      </el-select>
    </el-form-item>

    <el-form-item
      label="包类型"
      v-if="form.type === 'virtual'"
    >
      <el-select
        v-model="selectedPackageTypes"
        :disabled="disabled"
        multiple
        placeholder="选择包类型"
        style="width: 100%"
      >
        <el-option label="npm" value="npm" />
        <el-option label="Maven" value="maven" />
        <el-option label="PyPI" value="pypi" />
        <el-option label="Go" value="go" />
        <el-option label="NuGet" value="nuget" />
        <el-option label="Yum" value="yum" />
        <el-option label="Apt" value="apt" />
        <el-option label="Generic" value="generic" />
      </el-select>
      <span class="form-hint">虚拟仓库可聚合多种包类型的仓库</span>
    </el-form-item>
    
    <el-form-item label="显示名称" prop="display_name">
      <el-input v-model="form.display_name" placeholder="例如：NPM 内部仓库" />
    </el-form-item>
    
    <el-form-item label="描述">
      <el-input
        v-model="form.description"
        type="textarea"
        :rows="2"
        placeholder="仓库用途说明..."
      />
    </el-form-item>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'

interface FormModel {
  name: string
  display_name: string
  description: string
  type: 'local' | 'proxy' | 'virtual'
  package_type: string
}

interface Props {
  form: FormModel
  disabled?: boolean
  selectedPackageTypes?: string[]
}

interface Emits {
  (e: 'update:selectedPackageTypes', value: string[]): void
}

const props = withDefaults(defineProps<Props>(), {
  disabled: false,
  selectedPackageTypes: () => [],
})

const emit = defineEmits<Emits>()

const selectedPackageTypes = computed({
  get: () => props.selectedPackageTypes,
  set: (val: string[]) => emit('update:selectedPackageTypes', val),
})
</script>

<style scoped>
.basic-info-form {
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
