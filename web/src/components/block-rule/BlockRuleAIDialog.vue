<template>
  <el-dialog
    v-model="visible"
    :title="mode === 'generator' ? 'AI 生成阻断规则' : 'AI 优化阻断规则'"
    width="720px"
    @close="handleClose"
  >
    <!-- 输入区 -->
    <div class="ai-input-section">
      <template v-if="mode === 'generator'">
        <el-radio-group v-model="genSource" class="source-group">
          <el-radio-button value="vulnerability">按 CVE 生成</el-radio-button>
          <el-radio-button value="description">按描述生成</el-radio-button>
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

        <div v-else class="description-form">
          <el-input
            v-model="descForm.package_name"
            placeholder="包名，如 log4j-core"
            class="desc-input"
          />
          <el-input
            v-model="descForm.version"
            placeholder='版本约束，如 <2.17.1 或 *'
            class="desc-input"
          />
          <el-select v-model="descForm.match_type" placeholder="匹配类型" class="desc-select">
            <el-option label="精确" value="exact" />
            <el-option label="通配符" value="wildcard" />
            <el-option label="版本范围" value="range" />
          </el-select>
          <el-input
            v-model="descForm.reason"
            placeholder="阻断原因（可选）"
            class="desc-input"
          />
          <el-button type="primary" :loading="loading" @click="handleGenerate">生成草案</el-button>
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
import { MagicStick } from '@element-plus/icons-vue'
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
const descForm = ref({
  package_name: '',
  version: '',
  match_type: 'range' as 'exact' | 'wildcard' | 'range',
  reason: '',
})

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

  // description 模式
  const f = descForm.value
  if (!f.package_name || !f.version || !f.match_type) {
    errorMsg.value = '请填写包名、版本和匹配类型'
    return null
  }
  const reasonPart = f.reason ? `，reason=${f.reason}` : ''
  return `请调用 block_rule_generator 工具，source=description，package_name=${f.package_name}，version=${f.version}，match_type=${f.match_type}${reasonPart}，生成阻断规则草案。`
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

.description-form {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 8px;
  align-items: center;
}

.desc-input,
.desc-select {
  width: 100%;
}

.description-form > .el-button {
  grid-column: span 2;
  justify-self: start;
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
