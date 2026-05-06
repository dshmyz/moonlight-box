<template>
  <div class="docs-viewer">
    <div class="docs-header">
      <el-button text @click="$router.back()">
        <i class="fa-solid fa-arrow-left"></i>
        返回
      </el-button>
      <h2>{{ docTitle }}</h2>
    </div>
    <div class="docs-content" v-loading="loading">
      <div v-if="error" class="error-state">
        <i class="fa-solid fa-circle-exclamation"></i>
        <p>文档加载失败</p>
      </div>
      <div v-else class="markdown-body" v-html="renderedContent"></div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useRoute } from 'vue-router'

const route = useRoute()
const loading = ref(true)
const error = ref(false)
const markdownContent = ref('')

const docTitle = computed(() => {
  const titles: Record<string, string> = {
    'client-configuration': '客户端配置指南'
  }
  const docName = (route.params.doc as string)?.replace('.md', '')
  return titles[docName] || '文档'
})

const renderedContent = computed(() => {
  if (!markdownContent.value) return ''
  return renderMarkdown(markdownContent.value)
})

const renderMarkdown = (md: string): string => {
  let html = md
  
  const codeBlocks: string[] = []
  html = html.replace(/```(\w*)\n([\s\S]*?)```/g, (_match, lang, code) => {
    const index = codeBlocks.length
    codeBlocks.push(`<pre class="code-block" data-lang="${lang}"><code>${escapeHtml(code.trim())}</code></pre>`)
    return `%%CODEBLOCK_${index}%%`
  })
  
  html = html.replace(/`([^`]+)`/g, '<code class="inline-code">$1</code>')
  
  html = html.replace(/^#### (.*$)/gm, '<h4>$1</h4>')
  html = html.replace(/^### (.*$)/gm, '<h3>$1</h3>')
  html = html.replace(/^## (.*$)/gm, '<h2>$1</h2>')
  html = html.replace(/^# (.*$)/gm, '<h1>$1</h1>')
  
  html = html.replace(/\*\*([^*]+)\*\*/g, '<strong>$1</strong>')
  html = html.replace(/\*([^*]+)\*/g, '<em>$1</em>')
  
  html = html.replace(/^\- (.*$)/gm, '<li>$1</li>')
  html = html.replace(/((?:<li>.*<\/li>\n?)+)/g, '<ul>$1</ul>')
  
  html = html.replace(/^\d+\. (.*$)/gm, '<li>$1</li>')
  
  html = html.replace(/\[([^\]]+)\]\(([^)]+)\)/g, '<a href="$2" target="_blank" rel="noopener">$1</a>')
  
  html = html.replace(/\n\n/g, '</p><p>')
  html = html.replace(/\n/g, '<br>')
  
  html = `<p>${html}</p>`
  
  codeBlocks.forEach((block, index) => {
    html = html.replace(`%%CODEBLOCK_${index}%%`, block)
  })
  
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

const loadDoc = async () => {
  loading.value = true
  error.value = false
  try {
    const docName = route.params.doc
    console.log('Loading doc:', docName)
    const response = await fetch(`/docs/${docName}`)
    console.log('Response status:', response.status)
    if (!response.ok) {
      throw new Error(`Document not found: ${response.status}`)
    }
    markdownContent.value = await response.text()
    console.log('Content loaded, length:', markdownContent.value.length)
  } catch (err) {
    console.error('Failed to load document:', err)
    error.value = true
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  loadDoc()
})
</script>

<style scoped>
.docs-viewer {
  min-height: 100vh;
  background: #fff;
}

.docs-header {
  display: flex;
  align-items: center;
  gap: 16px;
  padding: 24px 40px;
  border-bottom: 1px solid #e5e7eb;
  background: #fafbfc;
}

.docs-header h2 {
  margin: 0;
  font-size: 20px;
  font-weight: 600;
  color: #0f172a;
}

.docs-content {
  padding: 32px 40px;
  max-width: 900px;
  margin: 0 auto;
}

.error-state {
  text-align: center;
  padding: 80px 20px;
  color: #6b7280;
}

.error-state i {
  font-size: 48px;
  color: #ef4444;
  margin-bottom: 16px;
}

.markdown-body {
  color: #334155;
  line-height: 1.7;
}

.markdown-body :deep(h1) {
  font-size: 28px;
  font-weight: 700;
  color: #0f172a;
  margin: 32px 0 16px;
  padding-bottom: 8px;
  border-bottom: 1px solid #e5e7eb;
}

.markdown-body :deep(h2) {
  font-size: 22px;
  font-weight: 600;
  color: #0f172a;
  margin: 28px 0 12px;
}

.markdown-body :deep(h3) {
  font-size: 18px;
  font-weight: 600;
  color: #1e293b;
  margin: 24px 0 10px;
}

.markdown-body :deep(h4) {
  font-size: 16px;
  font-weight: 600;
  color: #334155;
  margin: 20px 0 8px;
}

.markdown-body :deep(p) {
  margin: 12px 0;
}

.markdown-body :deep(ul) {
  padding-left: 24px;
  margin: 12px 0;
}

.markdown-body :deep(li) {
  margin: 6px 0;
}

.markdown-body :deep(a) {
  color: #2563eb;
  text-decoration: none;
}

.markdown-body :deep(a:hover) {
  text-decoration: underline;
}

.markdown-body :deep(.code-block) {
  background: #0f172a;
  color: #e2e8f0;
  padding: 20px;
  border-radius: 8px;
  overflow-x: auto;
  font-family: 'SF Mono', 'Fira Code', 'Consolas', monospace;
  font-size: 14px;
  line-height: 1.7;
  margin: 16px 0;
}

.markdown-body :deep(.inline-code) {
  background: #f1f5f9;
  color: #0f172a;
  padding: 2px 6px;
  border-radius: 4px;
  font-family: 'SF Mono', 'Fira Code', 'Consolas', monospace;
  font-size: 13px;
}

.markdown-body :deep(strong) {
  color: #0f172a;
  font-weight: 600;
}
</style>
