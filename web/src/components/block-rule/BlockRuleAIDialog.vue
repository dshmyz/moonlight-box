<template>
  <el-dialog
    v-model="visible"
    :title="mode === 'generator' ? 'AI 生成阻断规则' : 'AI 优化阻断规则'"
    width="820px"
    @close="handleClose"
  >
    <!-- 输入区 -->
    <div class="ai-input-section">
      <template v-if="mode === 'generator'">
        <el-radio-group v-model="genSource" class="source-group">
          <el-radio-button value="vulnerability">按 CVE 生成</el-radio-button>
          <el-radio-button value="description">自然语言描述</el-radio-button>
        </el-radio-group>

        <div v-if="genSource === 'vulnerability'" class="input-row">
          <el-input
            v-model="cveInput"
            placeholder="输入 CVE 编号，多个用逗号分隔，如 CVE-2021-44228, CVE-2022-22965"
            clearable
            @keyup.enter="handleGenerate"
          />
          <el-button type="primary" :loading="loading" @click="handleGenerate">生成草案</el-button>
        </div>

        <div v-else class="description-input">
          <!-- 文本框 + 工具栏 -->
          <div class="textarea-wrapper">
            <el-input
              v-model="descInput"
              type="textarea"
              :rows="4"
              placeholder="描述阻断需求，如：阻断所有 log4j 1.x 版本"
              @keyup.enter.ctrl="handleGenerate"
            />
            <div class="textarea-toolbar">
              <el-upload
                v-if="!fileContent"
                :auto-upload="false"
                :show-file-list="false"
                accept=".txt,.csv,.json,.md"
                :on-change="handleFileChange"
              >
                <el-button text :icon="UploadFilled" size="small">导入文件</el-button>
              </el-upload>
              <template v-else>
                <span class="file-chip">
                  <el-icon><Document /></el-icon>
                  <span class="file-chip-name">{{ fileName }}</span>
                  <span class="file-chip-size">{{ filePreview }}</span>
                  <el-button text :icon="Close" size="small" @click="clearFile" />
                </span>
              </template>
            </div>
          </div>

          <!-- 文件内容预览（导入后展示） -->
          <div v-if="fileContent" class="file-content-preview">
            {{ fileContent.slice(0, 500) }}{{ fileContent.length > 500 ? '...' : '' }}
          </div>

          <!-- 快捷示例：紧凑文字链接 -->
          <div class="quick-links">
            <span class="quick-links-label">常用</span>
            <a
              v-for="(p, idx) in quickPrompts"
              :key="idx"
              class="quick-link"
              @click="useQuickPrompt(p.text)"
            >{{ p.label }}</a>
          </div>

          <!-- 操作行 -->
          <div class="action-row">
            <span class="action-tip">{{ fileContent ? 'AI 将解析文件内容生成草案' : 'Ctrl+Enter 快速生成' }}</span>
            <el-button type="primary" :loading="loading" :disabled="!descInput && !fileContent" @click="handleGenerate">生成草案</el-button>
          </div>
        </div>
      </template>

      <template v-else>
        <el-button type="primary" :loading="loading" @click="handleAnalyze">
          <el-icon><MagicStick /></el-icon>
          <span>分析现有规则</span>
        </el-button>
        <span class="hint-text">AI 将扫描所有启用的阻断规则，检测过宽、过期、冗余问题</span>
      </template>
    </div>

    <!-- AI 文本回退（工具未调用时） -->
    <div v-if="aiMessage && drafts.length === 0 && suggestions.length === 0" class="ai-message">
      <el-alert type="info" :closable="false">
        <pre>{{ aiMessage }}</pre>
      </el-alert>
    </div>

    <!-- 错误提示 -->
    <el-alert v-if="errorMsg" type="error" :closable="false" class="error-alert">
      {{ errorMsg }}
    </el-alert>

    <!-- 生成草案结果 -->
    <div v-if="drafts.length > 0" class="draft-section">
      <div class="section-header">
        <span class="section-title">规则草案（共 {{ drafts.length }} 条）</span>
        <el-checkbox v-model="selectAll" @change="toggleSelectAll">全选</el-checkbox>
      </div>

      <el-table :data="drafts" style="width: 100%" max-height="360">
        <el-table-column type="selection" width="45" :selectable="canSelect" />
        <el-table-column prop="package_name" label="包名" min-width="120" show-overflow-tooltip />
        <el-table-column prop="version" label="版本" min-width="100" />
        <el-table-column prop="match_type" label="匹配类型" min-width="90" align="center">
          <template #default="{ row }">
            <el-tag size="small" :type="matchTagType(row.match_type)">
              {{ matchTypeLabel(row.match_type) }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="affected_count" label="影响版本数" min-width="90" align="center">
          <template #default="{ row }">
            <span :class="{ 'affected-zero': row.affected_count === 0 }">{{ row.affected_count }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="severity" label="严重程度" min-width="80" align="center">
          <template #default="{ row }">
            <el-tag v-if="row.severity" size="small" :type="severityTagType(row.severity)">
              {{ row.severity }}
            </el-tag>
            <span v-else>—</span>
          </template>
        </el-table-column>
        <el-table-column label="重复" min-width="80" align="center">
          <template #default="{ row }">
            <el-tooltip v-if="row.duplicate_of_id" :content="row.duplicate_of_desc" placement="top">
              <el-tag type="warning" size="small">已存在</el-tag>
            </el-tooltip>
            <span v-else>—</span>
          </template>
        </el-table-column>
        <el-table-column prop="reason" label="原因" min-width="150" show-overflow-tooltip />
      </el-table>

      <div class="action-bar">
        <span class="selected-count">已选 {{ selectedCount }} 条</span>
        <el-button
          type="primary"
          :loading="creating"
          :disabled="selectedCount === 0"
          @click="handleCreateSelected"
        >
          创建选中规则
        </el-button>
      </div>
    </div>

    <!-- 优化建议结果 -->
    <div v-if="suggestions.length > 0" class="suggestion-section">
      <div class="section-header">
        <span class="section-title">优化建议（共 {{ suggestions.length }} 条，总规则 {{ totalRules }} 条）</span>
      </div>

      <div class="suggestion-list">
        <div v-for="(s, idx) in suggestions" :key="idx" :class="['suggestion-card', `suggestion-${s.type}`]">
          <div class="suggestion-header">
            <el-tag :type="suggestionTagType(s.type)" size="small">{{ suggestionTypeLabel(s.type) }}</el-tag>
            <span class="suggestion-pkg">{{ s.package_name }} <span class="suggestion-version">{{ s.version }}</span></span>
            <span class="suggestion-rule-id">规则 ID: {{ s.rule_id }}</span>
          </div>
          <div class="suggestion-detail">{{ s.detail }}</div>
          <div class="suggestion-action">{{ s.suggestion }}</div>
        </div>
      </div>
    </div>
  </el-dialog>
</template>

<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { MagicStick, UploadFilled, Document, Close } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import { aiApi } from '@/api/ai'
import { blockRuleApi, type BlockRuleCreateParams } from '@/api/blockRule'

interface Props {
  visible: boolean
  mode: 'generator' | 'optimizer'
}

const props = defineProps<Props>()
const emit = defineEmits<{
  'update:visible': [val: boolean]
  created: []
}>()

const visible = computed({
  get: () => props.visible,
  set: (val) => emit('update:visible', val),
})

// === generator 模式状态 ===
const genSource = ref<'vulnerability' | 'description'>('vulnerability')
const cveInput = ref('')
const descInput = ref('')

// === 文件导入状态 ===
const fileName = ref('')
const fileContent = ref('')

// === 快捷提示词 ===
interface QuickPrompt {
  label: string
  text: string
  type: '' | 'success' | 'warning' | 'info' | 'danger'
}

const quickPrompts: QuickPrompt[] = [
  { label: '阻断 log4j 1.x', text: '阻断所有 log4j-core 1.x 版本', type: 'danger' },
  { label: '阻断低版本 lodash', text: '禁止使用 lodash 4.17.20 以下的版本', type: 'warning' },
  { label: '屏蔽 GPL-3.0', text: '屏蔽所有含有 GPL-3.0 许可证的包', type: 'info' },
  { label: '阻断高危漏洞包', text: '阻断所有存在 critical 级别漏洞的包', type: 'danger' },
  { label: '阻断旧版本 Spring', text: '阻断 spring-core 5.3.0 以下的版本', type: 'warning' },
  { label: '屏蔽废弃包', text: '屏蔽已标记为 deprecated 的包', type: 'info' },
]

const useQuickPrompt = (text: string) => {
  descInput.value = text
}

const filePreview = computed(() => {
  if (!fileContent.value) return ''
  const bytes = new Blob([fileContent.value]).size
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / 1024 / 1024).toFixed(1)} MB`
})

// 限制文件大小 1MB，防止超长内容撑爆 LLM 上下文
const MAX_FILE_SIZE = 1024 * 1024

const handleFileChange = (file: any) => {
  const raw = file.raw || file
  if (!raw) return
  if (raw.size > MAX_FILE_SIZE) {
    ElMessage.error('文件过大，请限制在 1MB 以内')
    return
  }
  const reader = new FileReader()
  reader.onload = (e) => {
    const text = e.target?.result as string
    if (!text || !text.trim()) {
      ElMessage.error('文件内容为空')
      return
    }
    fileName.value = raw.name
    fileContent.value = text
  }
  reader.onerror = () => {
    ElMessage.error('读取文件失败')
  }
  reader.readAsText(raw)
}

const clearFile = () => {
  fileName.value = ''
  fileContent.value = ''
}

// === 共享状态 ===
const loading = ref(false)
const creating = ref(false)
const errorMsg = ref('')
const aiMessage = ref('')

// === 草案数据 ===
interface RuleDraft {
  package_name: string
  version: string
  match_type: string
  package_type: string
  reason: string
  severity?: string
  affected_count: number
  affected_versions?: string[]
  duplicate_of_id?: number
  duplicate_of_desc?: string
  _selected?: boolean
}

const drafts = ref<RuleDraft[]>([])
const selectAll = ref(false)

const selectedCount = computed(() => drafts.value.filter(d => d._selected).length)

const canSelect = (row: RuleDraft) => !row.duplicate_of_id

const toggleSelectAll = (val: boolean) => {
  drafts.value.forEach(d => {
    if (canSelect(d)) d._selected = val
  })
}

// === 优化建议数据 ===
interface OptimizationSuggestion {
  type: string
  rule_id: number
  package_name: string
  version: string
  match_type: string
  detail: string
  suggestion: string
}

const suggestions = ref<OptimizationSuggestion[]>([])
const totalRules = ref(0)

// === 标签映射 ===
const MATCH_TYPE_LABELS: Record<string, string> = {
  exact: '精确',
  wildcard: '通配符',
  range: '版本范围',
}

const matchTypeLabel = (t: string) => MATCH_TYPE_LABELS[t] || t

const matchTagType = (t: string): '' | 'success' | 'warning' | 'info' => {
  if (t === 'range') return 'success'
  if (t === 'wildcard') return 'warning'
  return 'info'
}

const severityTagType = (s: string): 'danger' | 'warning' | 'info' => {
  if (s === 'critical') return 'danger'
  if (s === 'high') return 'warning'
  return 'info'
}

const SUGGESTION_TYPE_LABELS: Record<string, string> = {
  over_broad: '过宽',
  stale: '过期',
  redundant: '冗余',
}

const suggestionTypeLabel = (t: string) => SUGGESTION_TYPE_LABELS[t] || t

const suggestionTagType = (t: string): 'danger' | 'warning' | 'info' => {
  if (t === 'over_broad') return 'danger'
  if (t === 'stale') return 'warning'
  return 'info'
}

// === 生成草案 ===
const handleGenerate = async () => {
  errorMsg.value = ''
  aiMessage.value = ''
  drafts.value = []

  const prompt = buildGeneratorPrompt()
  if (!prompt) return

  loading.value = true
  try {
    const res = await aiApi.chat({ message: prompt })
    extractDrafts(res)
  } catch (e: any) {
    errorMsg.value = e?.message || 'AI 请求失败'
  } finally {
    loading.value = false
  }
}

const buildGeneratorPrompt = (): string | null => {
  if (genSource.value === 'vulnerability') {
    const cves = cveInput.value.trim()
    if (!cves) {
      errorMsg.value = '请输入 CVE 编号'
      return null
    }
    const ids = cves.split(',').map(s => s.trim()).filter(Boolean)
    if (ids.length === 1) {
      return `请调用 block_rule_generator 工具，source=vulnerability，cve_id=${ids[0]}，生成阻断规则草案。`
    }
    const idsJson = JSON.stringify(ids)
    return `请调用 block_rule_generator 工具，source=vulnerability，cve_ids=${idsJson}，批量生成阻断规则草案。`
  }

  // description 模式：自然语言描述 / 文件导入
  const desc = descInput.value.trim()
  const file = fileContent.value.trim()
  if (!desc && !file) {
    errorMsg.value = '请输入阻断需求描述或导入文件'
    return null
  }

  // 同时有描述和文件：合并
  if (desc && file) {
    return `请调用 block_rule_generator 工具，source=description，根据以下描述和文件内容生成阻断规则草案。\n描述：${desc}\n文件内容（${fileName.value}）：\n${file}`
  }
  // 只有文件
  if (file) {
    return `请调用 block_rule_generator 工具，source=description，根据以下文件内容（${fileName.value}）解析并生成阻断规则草案。文件可能是 CSV/JSON/TXT 格式，请自动识别格式，提取每条规则的包名、版本和匹配类型：\n${file}`
  }
  // 只有描述
  return `请调用 block_rule_generator 工具，source=description，根据以下描述生成阻断规则草案：${desc}`
}

const extractDrafts = (res: any) => {
  if (res?.tool_calls && res.tool_calls.length > 0) {
    const toolCall = res.tool_calls.find((tc: any) => tc.name === 'block_rule_generator')
    if (toolCall) {
      try {
        const parsed = JSON.parse(toolCall.result)
        if (parsed.rules && Array.isArray(parsed.rules)) {
          drafts.value = parsed.rules.map((r: any) => ({ ...r, _selected: false }))
          return
        }
      } catch {
        // JSON 解析失败，回退到文本展示
      }
    }
  }
  // 回退：展示 AI 文本回复
  aiMessage.value = res?.message || 'AI 未返回规则草案，请尝试更明确的描述'
}

// === 创建选中规则 ===
const handleCreateSelected = async () => {
  const selected = drafts.value.filter(d => d._selected && !d.duplicate_of_id)
  if (selected.length === 0) {
    ElMessage.warning('请至少选择一条草案')
    return
  }

  creating.value = true
  let successCount = 0
  let failCount = 0
  for (const draft of selected) {
    try {
      const params: BlockRuleCreateParams = {
        package_name: draft.package_name,
        version: draft.version,
        match_type: draft.match_type as 'exact' | 'wildcard' | 'range',
        package_type: draft.package_type || '*',
        reason: draft.reason || '',
        enabled: true,
      }
      await blockRuleApi.create(params)
      successCount++
      draft._selected = false
    } catch {
      failCount++
    }
  }
  creating.value = false

  if (successCount > 0) {
    ElMessage.success(`成功创建 ${successCount} 条规则${failCount > 0 ? `，${failCount} 条失败` : ''}`)
    emit('created')
  } else {
    ElMessage.error('创建失败')
  }

  // 全部创建成功后关闭对话框
  if (failCount === 0 && successCount > 0) {
    visible.value = false
  }
}

// === 优化分析 ===
const handleAnalyze = async () => {
  errorMsg.value = ''
  aiMessage.value = ''
  suggestions.value = []

  loading.value = true
  try {
    const res = await aiApi.chat({ message: '请调用 block_rule_optimizer 工具，operation=analyze，分析现有阻断规则并给出优化建议。' })
    extractSuggestions(res)
  } catch (e: any) {
    errorMsg.value = e?.message || 'AI 请求失败'
  } finally {
    loading.value = false
  }
}

const extractSuggestions = (res: any) => {
  if (res?.tool_calls && res.tool_calls.length > 0) {
    const toolCall = res.tool_calls.find((tc: any) => tc.name === 'block_rule_optimizer')
    if (toolCall) {
      try {
        const parsed = JSON.parse(toolCall.result)
        totalRules.value = parsed.total_rules || 0
        if (parsed.suggestions && Array.isArray(parsed.suggestions)) {
          suggestions.value = parsed.suggestions
          return
        }
      } catch {
        // JSON 解析失败，回退
      }
    }
  }
  aiMessage.value = res?.message || 'AI 未返回优化建议，请稍后重试'
}

// === 对话框重置 ===
const handleClose = () => {
  errorMsg.value = ''
  aiMessage.value = ''
  drafts.value = []
  suggestions.value = []
  totalRules.value = 0
  selectAll.value = false
  clearFile()
}

// 切换模式时重置
watch(() => props.mode, () => {
  handleClose()
})
</script>

<style scoped>
.ai-input-section {
  margin-bottom: 16px;
}

.source-group {
  margin-bottom: 12px;
}

.input-row {
  display: flex;
  gap: 8px;
  align-items: center;
}

.input-row .el-input {
  flex: 1;
}

.description-input {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

/* 文本框 + 工具栏 */
.textarea-wrapper {
  position: relative;
}

.textarea-wrapper :deep(.el-textarea__inner) {
  padding-bottom: 32px;
}

.textarea-toolbar {
  position: absolute;
  left: 8px;
  bottom: 4px;
  display: flex;
  align-items: center;
  gap: 4px;
}

.file-chip {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 2px 8px;
  border-radius: 4px;
  background: var(--el-fill-color);
  font-size: 12px;
  color: var(--el-text-color-regular);
}

.file-chip .el-icon {
  color: var(--el-color-primary);
  font-size: 14px;
}

.file-chip-name {
  max-width: 200px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.file-chip-size {
  color: var(--el-text-color-secondary);
}

/* 文件内容预览 */
.file-content-preview {
  font-size: 12px;
  color: var(--el-text-color-regular);
  font-family: monospace;
  white-space: pre-wrap;
  word-break: break-all;
  max-height: 120px;
  overflow-y: auto;
  line-height: 1.5;
  padding: 8px 12px;
  background: var(--el-fill-color-light);
  border-radius: 6px;
  border: 1px solid var(--el-border-color-lighter);
}

/* 快捷示例：紧凑文字链接 */
.quick-links {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 4px 0;
  font-size: 13px;
}

.quick-links-label {
  color: var(--el-text-color-secondary);
  margin-right: 8px;
}

.quick-link {
  color: var(--el-color-primary);
  cursor: pointer;
  padding: 2px 8px;
  border-radius: 4px;
  transition: background 0.15s;
}

.quick-link:hover {
  background: var(--el-color-primary-light-9);
}

.quick-link:not(:last-child)::after {
  content: '·';
  color: var(--el-text-color-placeholder);
  margin-left: 8px;
}

/* 操作行 */
.action-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding-top: 4px;
}

.action-tip {
  font-size: 12px;
  color: var(--el-text-color-secondary);
}

.hint-text {
  margin-left: 12px;
  font-size: 13px;
  color: var(--el-text-color-secondary);
}

.ai-message {
  margin: 12px 0;
}

.ai-message pre {
  white-space: pre-wrap;
  word-break: break-word;
  font-family: inherit;
  margin: 0;
}

.error-alert {
  margin: 12px 0;
}

.section-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 12px;
}

.section-title {
  font-weight: 600;
  font-size: 14px;
  color: var(--el-text-color-primary);
}

.draft-section,
.suggestion-section {
  margin-top: 16px;
}

.affected-zero {
  color: var(--el-text-color-secondary);
}

.action-bar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-top: 12px;
  padding: 8px 0;
}

.selected-count {
  font-size: 13px;
  color: var(--el-text-color-secondary);
}

.suggestion-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
  max-height: 360px;
  overflow-y: auto;
}

.suggestion-card {
  padding: 12px;
  border-radius: 8px;
  border-left: 3px solid;
}

.suggestion-over_broad {
  border-left-color: var(--el-color-danger);
  background: var(--el-color-danger-light-9);
}

.suggestion-stale {
  border-left-color: var(--el-color-warning);
  background: var(--el-color-warning-light-9);
}

.suggestion-redundant {
  border-left-color: var(--el-color-info);
  background: var(--el-color-info-light-9);
}

.suggestion-header {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 6px;
}

.suggestion-pkg {
  font-weight: 600;
  font-size: 13px;
}

.suggestion-version {
  color: var(--el-text-color-secondary);
  font-weight: normal;
}

.suggestion-rule-id {
  margin-left: auto;
  font-size: 12px;
  color: var(--el-text-color-secondary);
}

.suggestion-detail {
  font-size: 13px;
  color: var(--el-text-color-primary);
  margin-bottom: 4px;
}

.suggestion-action {
  font-size: 12px;
  color: var(--el-text-color-secondary);
}
</style>
