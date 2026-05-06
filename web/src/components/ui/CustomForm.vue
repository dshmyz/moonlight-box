<template>
  <el-form
    :model="model"
    :rules="rules"
    :label-width="labelWidth"
    :label-position="labelPosition"
    class="custom-form"
  >
    <slot />
  </el-form>
</template>

<script setup lang="ts">
interface Props {
  model: object
  rules?: object
  labelWidth?: string
  labelPosition?: 'left' | 'right' | 'top'
}

withDefaults(defineProps<Props>(), {
  labelWidth: '100px',
  labelPosition: 'right'
})
</script>

<style scoped>
.custom-form {
  width: 100%;
}
</style>
