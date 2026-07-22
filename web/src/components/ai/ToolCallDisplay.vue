<template>
  <div :class="['tool-call', { success: toolCall.success, error: !toolCall.success }]">
    <div class="tool-header">
      <el-icon :size="16">
        <component :is="toolIcon" />
      </el-icon>
      <span class="tool-name">{{ getToolDisplayName(toolCall.name) }}</span>
      <el-tag :type="toolCall.success ? 'success' : 'danger'" size="small">
        {{ toolCall.success ? '成功' : '失败' }}
      </el-tag>
      <span class="tool-duration">{{ toolCall.duration_ms }}ms</span>
    </div>
    
    <el-collapse v-if="showDetails">
      <el-collapse-item title="参数">
        <pre class="code-block">{{ JSON.stringify(toolCall.params, null, 2) }}</pre>
      </el-collapse-item>
      <el-collapse-item :title="toolCall.success ? '结果' : '错误'">
        <pre class="code-block">{{ toolCall.result || toolCall.error }}</pre>
      </el-collapse-item>
    </el-collapse>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { Search, Document, Warning, Edit, Lock, MagicStick } from '@element-plus/icons-vue'
import type { ToolCallResult } from '@/api/ai'

interface Props {
  toolCall: ToolCallResult
  showDetails?: boolean
}

const props = withDefaults(defineProps<Props>(), {
  showDetails: true
})

const toolIcon = computed(() => {
  const iconMap: Record<string, any> = {
    query_logs: Search,
    query_database: Document,
    analyze_security: Warning,
    generate_demo_code: Edit,
    query_package_info: Document,
    block_rule_generator: Lock,
    block_rule_optimizer: MagicStick,
  }
  return iconMap[props.toolCall.name] || Document
})

const getToolDisplayName = (name: string) => {
  const nameMap: Record<string, string> = {
    query_logs: '查询日志',
    query_database: '查询数据库',
    analyze_security: '安全分析',
    generate_demo_code: '生成代码',
    query_package_info: '查询包信息',
    block_rule_generator: '生成阻断规则',
    block_rule_optimizer: '优化阻断规则',
  }
  return nameMap[name] || name
}
</script>

<style scoped>
.tool-call {
  padding: 12px;
  border: 1px solid var(--el-border-color);
  border-radius: 6px;
  background: var(--el-bg-color);
}

.tool-call.success {
  border-left: 3px solid var(--el-color-success);
}

.tool-call.error {
  border-left: 3px solid var(--el-color-danger);
}

.tool-header {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 8px;
}

.tool-name {
  font-weight: 500;
  flex: 1;
}

.tool-duration {
  font-size: 12px;
  color: var(--el-text-color-secondary);
}

.code-block {
  padding: 12px;
  background: var(--el-fill-color-light);
  border-radius: 4px;
  font-family: var(--font-family-mono);
  font-size: 12px;
  overflow-x: auto;
  white-space: pre-wrap;
  word-break: break-word;
  margin: 0;
}
</style>
