<template>
  <div class="markdown-content" v-html="renderedContent"></div>
</template>

<script setup lang="ts">
import { computed } from 'vue'

interface Props {
  content: string
}

const props = defineProps<Props>()

const renderedContent = computed(() => {
  if (!props.content) return ''
  
  let html = props.content
  
  html = escapeHtml(html)
  
  html = html.replace(/```(\w+)?\n([\s\S]*?)```/g, (match, lang, code) => {
    return `<pre class="code-block" data-lang="${lang || ''}"><code>${code.trim()}</code></pre>`
  })
  
  html = html.replace(/`([^`]+)`/g, '<code class="inline-code">$1</code>')
  
  html = html.replace(/^### (.*$)/gm, '<h3>$1</h3>')
  html = html.replace(/^## (.*$)/gm, '<h2>$1</h2>')
  html = html.replace(/^# (.*$)/gm, '<h1>$1</h1>')
  
  html = html.replace(/\*\*([^*]+)\*\*/g, '<strong>$1</strong>')
  html = html.replace(/\*([^*]+)\*/g, '<em>$1</em>')
  
  html = html.replace(/^\- (.*$)/gm, '<li>$1</li>')
  html = html.replace(/(<li>.*<\/li>)/s, '<ul>$1</ul>')
  
  html = html.replace(/^\d+\. (.*$)/gm, '<li>$1</li>')
  
  html = html.replace(/\[([^\]]+)\]\(([^)]+)\)/g, '<a href="$2" target="_blank">$1</a>')
  
  html = html.replace(/\n/g, '<br>')
  
  return html
})

const escapeHtml = (text: string): string => {
  const map: Record<string, string> = {
    '&': '&amp;',
    '<': '&lt;',
    '>': '&gt;',
    '"': '&quot;',
    "'": '&#039;'
  }
  return text.replace(/[&<>"']/g, m => map[m])
}
</script>

<style scoped>
.markdown-content {
  line-height: 1.6;
  word-break: break-word;
}

.markdown-content :deep(h1),
.markdown-content :deep(h2),
.markdown-content :deep(h3) {
  margin: 16px 0 8px 0;
  font-weight: 600;
  color: var(--el-text-color-primary);
}

.markdown-content :deep(h1) {
  font-size: 20px;
}

.markdown-content :deep(h2) {
  font-size: 18px;
}

.markdown-content :deep(h3) {
  font-size: 16px;
}

.markdown-content :deep(code.inline-code) {
  padding: 2px 6px;
  background: var(--el-fill-color-light);
  border-radius: 3px;
  font-family: 'Courier New', Consolas, monospace;
  font-size: 0.9em;
  color: var(--el-color-primary);
}

.markdown-content :deep(.code-block) {
  padding: 12px;
  background: var(--el-fill-color-light);
  border-radius: 4px;
  overflow-x: auto;
  margin: 8px 0;
}

.markdown-content :deep(.code-block code) {
  font-family: 'Courier New', Consolas, monospace;
  font-size: 13px;
  line-height: 1.5;
  color: var(--el-text-color-primary);
}

.markdown-content :deep(ul),
.markdown-content :deep(ol) {
  padding-left: 24px;
  margin: 8px 0;
}

.markdown-content :deep(li) {
  margin: 4px 0;
}

.markdown-content :deep(strong) {
  font-weight: 600;
  color: var(--el-text-color-primary);
}

.markdown-content :deep(em) {
  font-style: italic;
}

.markdown-content :deep(a) {
  color: var(--el-color-primary);
  text-decoration: none;
}

.markdown-content :deep(a:hover) {
  text-decoration: underline;
}
</style>
