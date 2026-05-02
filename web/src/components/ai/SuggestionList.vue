<template>
  <div v-if="visible" class="suggestion-list">
    <div class="suggestion-header">你可以问我：</div>
    <div class="suggestions">
      <el-tag
        v-for="(suggestion, index) in suggestions"
        :key="index"
        class="suggestion-item"
        @click="handleSelect(suggestion)"
      >
        {{ suggestion }}
      </el-tag>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'

interface Props {
  visible?: boolean
}

withDefaults(defineProps<Props>(), {
  visible: true
})

const emit = defineEmits<{
  select: [suggestion: string]
}>()

const suggestions = ref([
  '查询lodash包的信息',
  '最近有哪些错误日志？',
  '分析express包的安全问题',
  '生成vue的使用示例',
  '查询npm仓库的配置',
  '帮我排查依赖冲突'
])

const handleSelect = (suggestion: string) => {
  emit('select', suggestion)
}
</script>

<style scoped>
.suggestion-list {
  padding: 16px;
  background: var(--el-bg-color);
  border-radius: 8px;
  margin-bottom: 16px;
}

.suggestion-header {
  font-size: 14px;
  color: var(--el-text-color-secondary);
  margin-bottom: 12px;
}

.suggestions {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.suggestion-item {
  cursor: pointer;
  transition: all 0.3s;
  user-select: none;
}

.suggestion-item:hover {
  transform: translateY(-2px);
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.1);
}
</style>
