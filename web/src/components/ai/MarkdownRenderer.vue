<template>
  <div class="markdown-content">
    <template v-for="(block, index) in parsedBlocks" :key="index">
      <MermaidRenderer 
        v-if="block.type === 'mermaid'" 
        :content="block.content" 
        :theme="theme"
        :interactive="true"
        @node-click="handleNodeClick"
        @optimize="handleOptimize"
      />
      <div v-else v-html="block.html" />
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import MermaidRenderer from './MermaidRenderer.vue'

interface Props {
  content: string
  theme?: 'default' | 'dark'
}

const props = withDefaults(defineProps<Props>(), {
  theme: 'default'
})

const emit = defineEmits({
  'node-click': (_node: { name: string; type: string }) => true,
  optimize: (_pkgName: string, _pkgType?: string) => true
})

type Block = 
  | { type: 'mermaid'; content: string }
  | { type: 'html'; html: string }

const parsedBlocks = computed((): Block[] => {
  if (!props.content) return []
  
  const blocks: Block[] = []
  const content = props.content
  
  const mermaidRegex = /```mermaid\s*\n([\s\S]*?)```/g
  let lastIndex = 0
  let match: RegExpExecArray | null
  
  while ((match = mermaidRegex.exec(content)) !== null) {
    if (match.index > lastIndex) {
      const before = content.slice(lastIndex, match.index)
      const rendered = renderBasicMarkdown(before)
      if (rendered.trim()) {
        blocks.push({ type: 'html', html: rendered })
      }
    }
    
    const mermaidCode = match[1]?.trim()
    if (mermaidCode) {
      blocks.push({ type: 'mermaid', content: mermaidCode })
    }
    
    lastIndex = mermaidRegex.lastIndex
  }
  
  if (lastIndex < content.length) {
    const after = content.slice(lastIndex)
    const rendered = renderBasicMarkdown(after)
    if (rendered.trim()) {
      blocks.push({ type: 'html', html: rendered })
    }
  }
  
  if (blocks.length === 0) {
    return [{ type: 'html', html: renderBasicMarkdown(content) }]
  }
  
  return blocks
})

const renderBasicMarkdown = (text: string): string => {
  if (!text) return ''
  
  let html = escapeHtml(text)
  
  html = html.replace(/```(\w+)?\n([\s\S]*?)```/g, (_match, lang, code) => {
    if (lang === 'mermaid') return ''
    return `<pre class="code-block" data-lang="${lang || ''}"><code>${escapeHtml(code.trim())}</code></pre>`
  })
  
  html = html.replace(/`([^`]+)`/g, '<code class="inline-code">$1</code>')
  html = html.replace(/^### (.*$)/gm, '<h3>$1</h3>')
  html = html.replace(/^## (.*$)/gm, '<h2>$1</h2>')
  html = html.replace(/^# (.*$)/gm, '<h1>$1</h1>')
  html = html.replace(/\*\*([^*]+)\*\*/g, '<strong>$1</strong>')
  html = html.replace(/\*([^*]+)\*/g, '<em>$1</em>')
  html = html.replace(/^\s*[-*+]\s+(.*$)/gm, '<li>$1</li>')
  html = html.replace(/(<li>.*<\/li>)/gs, '<ul>$1</ul>')
  html = html.replace(/^\s*\d+\.\s+(.*$)/gm, '<li>$1</li>')
  html = html.replace(/\[([^\]]+)\]\(([^)]+)\)/g, '<a href="$2" target="_blank" rel="noopener">$1</a>')
  html = html.replace(/\n/g, '<br>')
  
  return html
}

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

const handleNodeClick = (node: { name: string; type: string }) => {
  emit('node-click', node)
}

const handleOptimize = (pkgName: string, pkgType?: string) => {
  emit('optimize', pkgName, pkgType)
}
</script>

<style scoped>
.markdown-content {
  line-height: 1.6;
  word-break: break-word;
  color: var(--el-text-color-primary);
}

.markdown-content :deep(h1),
.markdown-content :deep(h2),
.markdown-content :deep(h3) {
  margin: 16px 0 8px 0;
  font-weight: 600;
  color: var(--el-text-color-primary);
  line-height: 1.4;
}

.markdown-content :deep(h1) { font-size: 20px; }
.markdown-content :deep(h2) { font-size: 18px; }
.markdown-content :deep(h3) { font-size: 16px; }

.markdown-content :deep(code.inline-code) {
  padding: 2px 6px;
  background: var(--el-fill-color-light);
  border-radius: 4px;
  font-family: 'SF Mono', 'Courier New', Consolas, monospace;
  font-size: 0.9em;
  color: var(--el-color-primary);
}

.markdown-content :deep(.code-block) {
  padding: 12px 16px;
  background: var(--el-fill-color-light);
  border-radius: 6px;
  overflow-x: auto;
  margin: 12px 0;
  border: 1px solid var(--el-border-color-light);
}

.markdown-content :deep(.code-block code) {
  font-family: 'SF Mono', 'Courier New', Consolas, monospace;
  font-size: 13px;
  line-height: 1.5;
  color: var(--el-text-color-primary);
  background: transparent;
  padding: 0;
}

.markdown-content :deep(ul),
.markdown-content :deep(ol) {
  padding-left: 24px;
  margin: 12px 0;
}

.markdown-content :deep(li) {
  margin: 4px 0;
  line-height: 1.5;
}

.markdown-content :deep(strong) {
  font-weight: 600;
}

.markdown-content :deep(em) {
  font-style: italic;
}

.markdown-content :deep(a) {
  color: var(--el-color-primary);
  text-decoration: none;
  transition: color 0.2s;
}

.markdown-content :deep(a:hover) {
  text-decoration: underline;
  color: var(--el-color-primary-light-3);
}

:deep(.dark .markdown-content) {
  color: #f1f5f9;
}

:deep(.dark .markdown-content :deep(h1)),
:deep(.dark .markdown-content :deep(h2)),
:deep(.dark .markdown-content :deep(h3)) {
  color: #f1f5f9;
}

:deep(.dark .markdown-content :deep(.code-block)) {
  background: #1e293b;
  border-color: #334155;
}

:deep(.dark .markdown-content :deep(.code-block code)) {
  color: #e2e8f0;
}
</style>
