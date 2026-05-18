<template>
  <div ref="containerRef" class="mermaid-container">
    <div v-if="loading" class="mermaid-loading">
      <el-icon class="is-loading"><Loading /></el-icon>
      <span>正在渲染依赖图...</span>
    </div>
    <div v-else-if="error" class="mermaid-error">
      <el-icon><Warning /></el-icon>
      <span>{{ error }}</span>
      <el-button link type="primary" size="small" @click="retry">重试</el-button>
    </div>
    <div v-else class="mermaid-content" v-html="svgContent" />
    
    <!-- 包详情弹窗 -->
    <PackageNodeDetail
      v-if="selectedNode"
      v-model:visible="showDetail"
      :package-name="selectedNode.name"
      :package-type="selectedNode.type"
      @optimize="handleOptimize"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, watch, nextTick, onBeforeUnmount } from 'vue'
import { ElMessage } from 'element-plus'
import { Loading, Warning } from '@element-plus/icons-vue'
import PackageNodeDetail from './PackageNodeDetail.vue'

interface Props {
  content: string
  theme?: 'default' | 'dark'
  interactive?: boolean
}

interface Emits {
  (e: 'node-click', node: { name: string; type: string }): void
  (e: 'optimize', pkgName: string, pkgType?: string): void
}

const props = withDefaults(defineProps<Props>(), {
  theme: 'default',
  interactive: true
})

const emit = defineEmits<Emits>()

const containerRef = ref<HTMLElement>()
const loading = ref(false)
const error = ref<string>()
const svgContent = ref<string>('')

// 交互状态
const showDetail = ref(false)
const selectedNode = ref<{ name: string; type: string } | null>(null)
let clickHandler: ((e: MouseEvent) => void) | null = null

declare const mermaid: {
  initialize: (config: any) => void
  render: (id: string, text: string) => Promise<{ svg: string }>
}

let mermaidLoaded = false

onMounted(async () => {
  await loadMermaid()
})

onBeforeUnmount(() => {
  if (containerRef.value && clickHandler) {
    containerRef.value.removeEventListener('click', clickHandler)
  }
})

const loadMermaid = async () => {
  if (mermaidLoaded) return
  
  try {
    const mermaidModule: any = await import('mermaid')
    const mermaidLib = mermaidModule.default || mermaidModule
    
    mermaidLib.initialize({
      startOnLoad: false,
      theme: props.theme === 'dark' ? 'dark' : 'default',
      securityLevel: 'loose' as const,
      fontFamily: 'var(--el-font-family, -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto)',
      flowchart: {
        useMaxWidth: true,
        htmlLabels: true,
        curve: 'basis',
        padding: 20,
        nodeSpacing: 50,
        rankSpacing: 50
      },
      themeVariables: {
        primaryColor: '#6366f1',
        primaryBorderColor: '#4f46e5',
        lineColor: '#94a3b8',
        textColor: '#0f172a'
      }
    })
    
    ;(window as any).__mermaid = mermaidLib
    mermaidLoaded = true
  } catch (e: any) {
    error.value = 'Mermaid 加载失败，请执行: npm install mermaid'
    console.error('Failed to load mermaid:', e)
    ElMessage.error('请安装 mermaid 依赖: npm install mermaid')
  }
}

const getMermaid = () => (window as any).__mermaid

const render = async () => {
  if (!props.content?.trim() || !containerRef.value) return
  
  const mermaidLib = getMermaid()
  if (!mermaidLib) {
    await loadMermaid()
    if (!getMermaid()) return
  }
  
  loading.value = true
  error.value = undefined
  
  try {
    const id = `mermaid-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`
    const { svg } = await mermaidLib.render(id, props.content.trim())
    svgContent.value = svg
    
    await nextTick()
    applyCustomStyles()
    
    if (props.interactive) {
      await nextTick()
      bindNodeClickEvents()
    }
  } catch (e: any) {
    console.error('Mermaid render error:', e)
    error.value = e.message?.includes('Parse error') 
      ? '依赖图语法解析失败，请检查格式'
      : `渲染失败: ${e.message || '未知错误'}`
    ElMessage.warning(error.value)
  } finally {
    loading.value = false
  }
}

const applyCustomStyles = () => {
  if (!containerRef.value) return
  const svg = containerRef.value.querySelector('svg')
  if (!svg) return
  
  svg.setAttribute('width', '100%')
  svg.setAttribute('height', 'auto')
  svg.style.maxWidth = '100%'
  svg.style.cursor = props.interactive ? 'pointer' : 'default'
  
  svg.querySelectorAll('g.node').forEach((node: Element) => {
    const rect = node.querySelector('rect')
    if (rect) {
      rect.setAttribute('style', 'transition: all 0.2s ease; cursor: pointer;')
      rect.addEventListener('mouseenter', function(this: Element) {
        this.setAttribute('stroke-width', '3')
        this.setAttribute('stroke', '#6366f1')
      })
      rect.addEventListener('mouseleave', function(this: Element) {
        this.setAttribute('stroke-width', '1')
      })
    }
  })
  
  if (document.documentElement.classList.contains('dark')) {
    svg.querySelectorAll('g.node rect').forEach((el: any) => {
      el.setAttribute('fill', '#1e293b')
      el.setAttribute('stroke', '#475569')
    })
    svg.querySelectorAll('g.node text').forEach((el: any) => {
      el.setAttribute('fill', '#f1f5f9')
    })
  }
}

const bindNodeClickEvents = () => {
  if (!containerRef.value) return
  
  if (clickHandler) {
    containerRef.value.removeEventListener('click', clickHandler)
  }
  
  clickHandler = (e: MouseEvent) => {
    const target = e.target as Element
    const nodeGroup = target.closest('g.node')
    
    if (!nodeGroup) return
    
    const textEl = nodeGroup.querySelector('text')
    if (!textEl) return
    
    const label = textEl.textContent || ''
    const [namePart, metaPart] = label.split('\n')
    const name = namePart?.trim()
    
    if (!name) return
    
    let type = props.content.includes('npm') ? 'npm' : 'generic'
    if (metaPart?.includes('npm')) type = 'npm'
    else if (metaPart?.includes('maven')) type = 'maven'
    else if (metaPart?.includes('pypi')) type = 'pypi'
    else if (metaPart?.includes('go')) type = 'go'
    else if (metaPart?.includes('nuget')) type = 'nuget'
    
    const nodeInfo = { name, type }
    emit('node-click', nodeInfo)
    
    selectedNode.value = nodeInfo
    showDetail.value = true
    
    const rect = nodeGroup.querySelector('rect')
    if (rect) {
      rect.setAttribute('stroke', '#22c55e')
      rect.setAttribute('stroke-width', '3')
      setTimeout(() => {
        rect.setAttribute('stroke-width', '1')
      }, 300)
    }
  }
  
  containerRef.value.addEventListener('click', clickHandler)
}

const handleOptimize = (pkgName: string, pkgType?: string) => {
  emit('optimize', pkgName, pkgType)
  showDetail.value = false
}

const retry = () => {
  render()
}

watch(() => props.content, (newVal) => {
  if (newVal?.trim()) {
    render()
  }
}, { immediate: true })

watch(() => props.theme, () => {
  const mermaidLib = getMermaid()
  if (mermaidLib && mermaidLoaded) {
    mermaidLib.initialize({ 
      theme: props.theme === 'dark' ? 'dark' : 'default',
      startOnLoad: false,
      securityLevel: 'loose'
    })
    render()
  }
})

watch(() => props.interactive, (val) => {
  if (containerRef.value) {
    const svg = containerRef.value.querySelector('svg')
    if (svg) {
      svg.style.cursor = val ? 'pointer' : 'default'
    }
  }
  if (val) {
    bindNodeClickEvents()
  } else if (clickHandler && containerRef.value) {
    containerRef.value.removeEventListener('click', clickHandler)
  }
})
</script>

<style scoped>
.mermaid-container {
  margin: 16px 0;
  padding: 16px;
  background: var(--el-bg-color);
  border: 1px solid var(--el-border-color-light);
  border-radius: 8px;
  overflow-x: auto;
  position: relative;
}

.mermaid-loading,
.mermaid-error {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  padding: 24px;
  color: var(--el-text-color-secondary);
}

.mermaid-error {
  color: var(--el-color-warning);
  flex-direction: column;
  text-align: center;
}

.mermaid-error :deep(.el-button) {
  margin-top: 8px;
}

:deep(.mermaid-content svg) {
  max-width: 100%;
  height: auto;
  display: block;
}

:deep(.mermaid-content .node) {
  transition: transform 0.2s ease;
}

:deep(.mermaid-content .node:hover) {
  transform: scale(1.02);
}

:deep(.mermaid-content .edgeLabel) {
  background: var(--el-bg-color);
  padding: 2px 6px;
  border-radius: 4px;
  font-size: 12px;
  pointer-events: none;
}

:deep(.dark .mermaid-content .edgeLabel) {
  background: #1e293b;
  color: #f1f5f9;
}

:deep(.mermaid-content .node) {
  position: relative;
}

:deep(.mermaid-content .node:hover::after) {
  content: '点击查看详情';
  position: absolute;
  bottom: -24px;
  left: 50%;
  transform: translateX(-50%);
  background: var(--el-color-info);
  color: #fff;
  padding: 4px 8px;
  border-radius: 4px;
  font-size: 11px;
  white-space: nowrap;
  pointer-events: none;
  opacity: 0;
  animation: fadeIn 0.2s ease forwards;
}

@keyframes fadeIn {
  to { opacity: 1; }
}
</style>
