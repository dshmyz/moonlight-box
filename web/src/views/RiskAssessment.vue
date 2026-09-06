<template>
  <div class="risk-assessment">
    <el-tabs v-model="activeTab">
      <el-tab-pane label="研判中心" name="assessment">
        <div class="upload-panel">
          <el-upload
            drag
            accept=".xlsx,.csv"
            :auto-upload="false"
            :limit="1"
            :on-change="handleFileChange"
            :on-remove="handleFileRemove"
            :file-list="fileList"
          >
            <el-icon class="el-icon--upload"><upload-filled /></el-icon>
            <div class="el-upload__text">将 Excel/CSV 拖到此处，或<em>点击上传</em></div>
            <template #tip>
              <div class="el-upload__tip">
                支持 .xlsx / .csv。必填"组件名/名称"和"版本"列，可选"格式(npm/pypi/maven…)/风险等级/原因/CVE"列。
              </div>
            </template>
          </el-upload>
          <div class="upload-actions">
            <el-button type="primary" :loading="analyzing" :disabled="!selectedFile" @click="runAssessment">
              开始研判
            </el-button>
            <el-button :loading="aiParsing" :disabled="!selectedFile" @click="aiParseFile">AI 智能解析</el-button>
            <el-button :disabled="!currentResult" @click="exportCurrent">导出结果</el-button>
            <el-button @click="downloadTemplate">下载模板</el-button>
          </div>
        </div>

        <template v-if="currentResult">
          <div class="stat-row">
            <el-card class="stat-card" shadow="never">
              <div class="stat-value">{{ currentResult.total_items }}</div>
              <div class="stat-label">组件总数</div>
            </el-card>
            <el-card class="stat-card" shadow="never">
              <div class="stat-value stat-value--hit">{{ currentResult.matched_items }}</div>
              <div class="stat-label">命中组件</div>
            </el-card>
            <el-card class="stat-card" shadow="never">
              <div class="stat-value">{{ currentResult.artifact_hits }}</div>
              <div class="stat-label">制品命中</div>
            </el-card>
            <el-card class="stat-card" shadow="never">
              <div class="stat-value">{{ currentResult.dependency_hits }}</div>
              <div class="stat-label">依赖命中</div>
            </el-card>
          </div>

          <el-card class="result-card" shadow="never">
            <template #header>
              <div class="result-header">
                <span class="result-title">
                  研判结果：{{ currentResult.file_name }}
                  <el-tag v-if="currentResult.matched_items === 0" type="success" size="small" class="result-tag">未发现风险命中</el-tag>
                  <el-tag v-else type="danger" size="small" class="result-tag">存在 {{ currentResult.matched_items }} 个风险命中</el-tag>
                </span>
                <span class="result-actions">
                  <el-button v-if="currentResult.matched_items > 0" type="danger" size="small" @click="oneClickBlock">
                    一键生成阻断规则
                  </el-button>
                  <el-button size="small" @click="openAIReport">AI 研判报告</el-button>
                </span>
              </div>
            </template>
            <el-table :data="currentResult.items || []" size="small" style="width: 100%">
              <el-table-column prop="name" label="组件名" min-width="180" show-overflow-tooltip />
              <el-table-column prop="version" label="风险版本" width="130" />
              <el-table-column prop="format" label="格式" width="90" align="center">
                <template #default="{ row }">
                  <el-tag v-if="row.format" size="small" type="info" effect="plain">{{ row.format }}</el-tag>
                  <span v-else>-</span>
                </template>
              </el-table-column>
              <el-table-column prop="severity" label="风险等级" width="100" align="center">
                <template #default="{ row }">
                  <el-tag v-if="row.severity" size="small">
                    {{ row.severity }}
                  </el-tag>
                  <span v-else>-</span>
                </template>
              </el-table-column>
              <el-table-column prop="reason" label="原因" width="160" show-overflow-tooltip>
                <template #default="{ row }">{{ row.reason || '-' }}</template>
              </el-table-column>
              <el-table-column prop="cve" label="CVE" width="140" show-overflow-tooltip>
                <template #default="{ row }">{{ row.cve || '-' }}</template>
              </el-table-column>
              <el-table-column label="是否命中" width="90" align="center">
                <template #default="{ row }">
                  <el-tag :type="row.matched ? 'danger' : 'info'" size="small">{{ row.matched ? '是' : '否' }}</el-tag>
                </template>
              </el-table-column>
              <el-table-column label="制品命中" min-width="220">
                <template #default="{ row }">
                  <div v-if="row.artifact_hit && row.artifact_hit.length" class="hit-cell">
                    <el-tag
                      v-for="(h, i) in visibleHits(row.artifact_hit)"
                      :key="i"
                      type="warning"
                      size="small"
                      class="hit-tag"
                      :title="`${h.repo}@${h.version}(${h.format})`"
                    >
                      {{ h.repo }}@{{ h.version }}
                    </el-tag>
                    <el-button link type="primary" size="small" class="hit-more" @click="openHits('artifact', row)">
                      全部 {{ row.artifact_hit.length }} 条
                    </el-button>
                  </div>
                  <span v-else>-</span>
                </template>
              </el-table-column>
              <el-table-column label="依赖命中" min-width="240">
                <template #default="{ row }">
                  <div v-if="row.dependency_hit && row.dependency_hit.length" class="hit-cell">
                    <el-tag
                      v-for="(h, i) in visibleHits(row.dependency_hit)"
                      :key="i"
                      type="danger"
                      size="small"
                      class="hit-tag"
                      :title="`${h.dependent_name}@${h.version}(${h.format}:${h.repo})`"
                    >
                      {{ h.dependent_name }}@{{ h.version }}
                    </el-tag>
                    <el-button link type="primary" size="small" class="hit-more" @click="openHits('dependency', row)">
                      全部 {{ row.dependency_hit.length }} 条
                    </el-button>
                  </div>
                  <span v-else>-</span>
                </template>
              </el-table-column>
              <el-table-column prop="suggestion" label="处置建议" min-width="300" show-overflow-tooltip>
                <template #default="{ row }">{{ row.suggestion || '-' }}</template>
              </el-table-column>
              <el-table-column label="处置状态" width="110" align="center">
                <template #default="{ row }">
                  <el-tooltip v-if="row.disposition" :content="dispositionTip(row)" placement="top">
                    <el-tag :type="dispositionTagType(row.disposition)" size="small">
                      {{ dispositionLabel(row.disposition) }}
                    </el-tag>
                  </el-tooltip>
                  <span v-else class="disposition-none">待处置</span>
                </template>
              </el-table-column>
              <el-table-column label="快速处置" width="200" align="center" fixed="right">
                <template #default="{ row }">
                  <template v-if="row.disposition">
                    <el-button link type="primary" size="small" @click="dispose(row, 'reset')">撤销</el-button>
                  </template>
                  <template v-else-if="row.artifact_hit && row.artifact_hit.length">
                    <el-button link type="danger" size="small" @click="dispose(row, 'remove')">移除制品</el-button>
                    <el-button link type="warning" size="small" @click="dispose(row, 'block')">仅阻断</el-button>
                    <el-button link type="info" size="small" @click="dispose(row, 'ignore')">忽略</el-button>
                  </template>
                  <template v-else-if="row.dependency_hit && row.dependency_hit.length">
                    <el-button link type="warning" size="small" @click="dispose(row, 'block')">仅阻断</el-button>
                    <el-button link type="info" size="small" @click="dispose(row, 'ignore')">忽略</el-button>
                  </template>
                  <span v-else>-</span>
                </template>
              </el-table-column>
            </el-table>
          </el-card>
        </template>
      </el-tab-pane>

      <el-tab-pane label="历史记录" name="history">
        <el-table :data="historyList" v-loading="historyLoading" style="width: 100%">
          <el-table-column prop="id" label="ID" width="70" align="center" />
          <el-table-column prop="file_name" label="来源文件" min-width="220" show-overflow-tooltip />
          <el-table-column prop="total_items" label="组件数" width="90" align="center" />
          <el-table-column prop="matched_items" label="命中数" width="90" align="center">
            <template #default="{ row }">
              <el-tag :type="row.matched_items > 0 ? 'danger' : 'success'" size="small">{{ row.matched_items }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column prop="artifact_hits" label="制品命中" width="90" align="center" />
          <el-table-column prop="dependency_hits" label="依赖命中" width="90" align="center" />
          <el-table-column label="研判时间" width="180">
            <template #default="{ row }">{{ formatDate(row.created_at) }}</template>
          </el-table-column>
          <el-table-column label="操作" width="200" align="center">
            <template #default="{ row }">
              <el-button type="primary" link size="small" @click="viewDetail(row.id)">查看</el-button>
              <el-button type="success" link size="small" @click="exportHistory(row.id)">导出</el-button>
              <el-button type="danger" link size="small" @click="removeHistory(row)">删除</el-button>
            </template>
          </el-table-column>
        </el-table>
        <div class="pagination-row">
          <el-pagination
            v-model:current-page="historyPage"
            v-model:page-size="historyPageSize"
            :total="historyTotal"
            :page-sizes="[10, 20, 50]"
            layout="total, sizes, prev, pager, next"
            @change="loadHistory"
          />
        </div>
      </el-tab-pane>
    </el-tabs>

    <el-dialog v-model="aiReportVisible" title="AI 研判报告" width="640px" top="6vh">
      <div v-if="aiReportLoading" v-loading="true" class="ai-report-loading">正在生成报告...</div>
      <pre v-else class="ai-report-content">{{ aiReport }}</pre>
    </el-dialog>

    <el-dialog v-model="hitDialogVisible" :title="hitDialogTitle" width="720px" top="6vh">
      <el-input v-model="hitFilter" placeholder="筛选组件 / 版本 / 仓库" clearable class="hit-filter" />
      <el-table :data="filteredHits" size="small" max-height="50vh" style="width: 100%">
        <el-table-column v-if="hitDialogType === 'dependency'" prop="dependent_name" label="依赖方" min-width="200" show-overflow-tooltip />
        <el-table-column prop="repo" label="仓库" min-width="170" show-overflow-tooltip />
        <el-table-column prop="version" label="版本" min-width="140" show-overflow-tooltip />
        <el-table-column prop="format" label="格式" width="90" />
      </el-table>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { UploadFilled } from '@element-plus/icons-vue'
import type { UploadFile } from 'element-plus'
import { riskAssessmentApi, type RiskAssessment, type RiskAssessmentItem, type RiskHit } from '@/api/riskAssessment'
import { formatDate } from '@/utils/format'

const DISPLAY_HIT_LIMIT = 8

const visibleHits = (hits: RiskHit[] | undefined) => (hits || []).slice(0, DISPLAY_HIT_LIMIT)

const activeTab = ref('assessment')

const fileList = ref<UploadFile[]>([])
const selectedFile = ref<File | null>(null)
const analyzing = ref(false)
const currentResult = ref<RiskAssessment | null>(null)

const handleFileChange = (file: UploadFile) => {
  if (!file.raw) return
  const name = file.name.toLowerCase()
  if (!name.endsWith('.xlsx') && !name.endsWith('.csv')) {
    ElMessage.error('仅支持 .xlsx / .csv 文件')
    fileList.value = []
    selectedFile.value = null
    return
  }
  selectedFile.value = file.raw
}

const handleFileRemove = () => {
  selectedFile.value = null
  fileList.value = []
}

const runAssessment = async () => {
  if (!selectedFile.value) return
  analyzing.value = true
  try {
    const result = await riskAssessmentApi.upload(selectedFile.value)
    currentResult.value = result
    ElMessage.success(`研判完成：${result.matched_items}/${result.total_items} 个组件命中风险`)
    activeTab.value = 'assessment'
    loadHistory()
  } catch {
    // request 层已提示错误
  } finally {
    analyzing.value = false
  }
}

const aiParsing = ref(false)
const aiParseFile = async () => {
  if (!selectedFile.value) return
  aiParsing.value = true
  try {
    const res = await riskAssessmentApi.aiParse(selectedFile.value)
    const preview = res.components
      .map((c) => `${c.name}@${c.version}${c.format ? `(${c.format})` : ''}${c.severity ? ` [${c.severity}]` : ''}`)
      .join('\n')
    try {
      await ElMessageBox.confirm(
        `AI 解析出 ${res.count} 个风险组件：\n\n${preview}\n\n确认执行研判？`,
        'AI 智能解析结果',
        { type: 'info', confirmButtonText: '确认研判', cancelButtonText: '取消' },
      )
    } catch {
      return
    }
    currentResult.value = await riskAssessmentApi.analyze(res.file_name, res.components)
    ElMessage.success(`研判完成：${currentResult.value.matched_items}/${currentResult.value.total_items} 个组件命中风险`)
    loadHistory()
  } catch {
    // request 层已提示错误（如 AI 未启用）
  } finally {
    aiParsing.value = false
  }
}

const downloadTemplate = async () => {
  try {
    const res = await riskAssessmentApi.downloadTemplate()
    downloadBlob(res.data as Blob, 'risk-assessment-template.xlsx')
  } catch {
    ElMessage.error('下载模板失败')
  }
}

const oneClickBlock = async () => {
  if (!currentResult.value) return
  const matched = (currentResult.value.items || []).filter((i) => i.matched)
  const desc = matched
    .map((i) => `${i.name}@${i.version}`)
    .join('、')
  try {
    await ElMessageBox.confirm(
      `将为以下 ${matched.length} 个命中组件生成阻断规则：\n\n${desc}\n\n确认创建？`,
      '一键生成阻断规则',
      { type: 'warning', confirmButtonText: '确认创建', cancelButtonText: '取消' },
    )
  } catch {
    return
  }
  try {
    const res = await riskAssessmentApi.createBlockRules(currentResult.value.id, matched.map((i) => i.id))
    ElMessage.success(`已生成 ${res.count} 条阻断规则`)
  } catch {
    ElMessage.error('生成阻断规则失败')
  }
}

const aiReportVisible = ref(false)
const aiReportLoading = ref(false)
const aiReport = ref('')
const openAIReport = async () => {
  if (!currentResult.value) return
  aiReportVisible.value = true
  aiReportLoading.value = true
  aiReport.value = ''
  try {
    const res = await riskAssessmentApi.aiReport(currentResult.value.id)
    aiReport.value = res.report
  } catch {
    aiReport.value = '（生成失败：AI 服务未启用或调用出错）'
  } finally {
    aiReportLoading.value = false
  }
}

const hitDialogVisible = ref(false)
const hitDialogType = ref<'artifact' | 'dependency'>('artifact')
const hitDialogRows = ref<RiskHit[]>([])
const hitFilter = ref('')

const hitDialogTitle = computed(() => {
  const label = hitDialogType.value === 'dependency' ? '依赖命中明细' : '制品命中明细'
  return `${label}（共 ${hitDialogRows.value.length} 条）`
})

const filteredHits = computed(() => {
  const kw = hitFilter.value.trim().toLowerCase()
  if (!kw) return hitDialogRows.value
  return hitDialogRows.value.filter((h) => {
    const fields = [h.dependent_name, h.repo, h.version, h.format].filter(Boolean)
    const label = `${h.dependent_name || h.repo || ''}@${h.version || ''}`
    return fields.some((v) => String(v).toLowerCase().includes(kw)) || label.toLowerCase().includes(kw)
  })
})

const openHits = (type: 'artifact' | 'dependency', row: RiskAssessmentItem) => {
  hitDialogType.value = type
  hitDialogRows.value = type === 'dependency' ? row.dependency_hit || [] : row.artifact_hit || []
  hitFilter.value = ''
  hitDialogVisible.value = true
}

const dispositionLabel = (d: string) =>
  ({ removed: '已移除', blocked: '已阻断', ignored: '已忽略' })[d] || d
const dispositionTagType = (d: string) =>
  ({ removed: 'danger', blocked: 'warning', ignored: 'info' })[d] || 'info'
const dispositionTip = (row: RiskAssessmentItem) => {
  const parts = []
  if (row.disposition_note) parts.push(row.disposition_note)
  if (row.disposed_at) parts.push(`处置时间：${formatDate(row.disposed_at)}`)
  return parts.join('；') || dispositionLabel(row.disposition || '')
}

const dispose = async (row: RiskAssessmentItem, action: 'remove' | 'block' | 'ignore' | 'reset') => {
  const actionDesc: Record<string, string> = {
    remove: `将删除 ${row.name}@${row.version} 在所有命中仓库中的制品，并自动生成阻断规则防止再次引入。此操作不可恢复！`,
    block: `将为 ${row.name}@${row.version} 生成阻断规则（不删除已有制品）。`,
    ignore: `将忽略 ${row.name}@${row.version} 的处置（仅记录状态）。`,
    reset: `将撤销 ${row.name}@${row.version} 的处置状态。`,
  }
  let note = ''
  try {
    const { value } = await ElMessageBox.prompt(
      actionDesc[action] + '\n可填写备注（如上级要求、工单号）：',
      '快速处置确认',
      { type: action === 'remove' ? 'warning' : 'info', confirmButtonText: '确认', cancelButtonText: '取消', inputPlaceholder: '备注（可选）' },
    )
    note = value || ''
  } catch {
    return
  }
  try {
    const updated = await riskAssessmentApi.dispose(row.id, action, note)
    const idx = (currentResult.value?.items || []).findIndex((i) => i.id === row.id)
    if (idx >= 0 && currentResult.value?.items) {
      currentResult.value.items[idx] = { ...currentResult.value.items[idx], ...updated }
    }
    ElMessage.success(`处置完成：${dispositionLabel(updated.disposition || '')}`)
    loadHistory()
  } catch {
    // request 层已提示错误
  }
}

const downloadBlob = (blob: Blob, filename: string) => {
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  a.click()
  URL.revokeObjectURL(url)
}

const exportCurrent = async () => {
  if (!currentResult.value) return
  try {
    const res = await riskAssessmentApi.exportFile(currentResult.value.id)
    downloadBlob(res.data as Blob, `risk-assessment-${currentResult.value.id}.xlsx`)
  } catch {
    ElMessage.error('导出失败')
  }
}

const exportHistory = async (id: number) => {
  try {
    const res = await riskAssessmentApi.exportFile(id)
    downloadBlob(res.data as Blob, `risk-assessment-${id}.xlsx`)
  } catch {
    ElMessage.error('导出失败')
  }
}

const historyList = ref<RiskAssessment[]>([])
const historyLoading = ref(false)
const historyPage = ref(1)
const historyPageSize = ref(10)
const historyTotal = ref(0)

const loadHistory = async () => {
  historyLoading.value = true
  try {
    const res = await riskAssessmentApi.list({ page: historyPage.value, page_size: historyPageSize.value })
    historyList.value = res.items
    historyTotal.value = res.pagination.total
  } catch {
    // request 层已提示错误
  } finally {
    historyLoading.value = false
  }
}

const viewDetail = async (id: number) => {
  try {
    const detail = await riskAssessmentApi.get(id)
    currentResult.value = detail
    activeTab.value = 'assessment'
    fileList.value = []
    selectedFile.value = null
  } catch {
    ElMessage.error('加载研判详情失败')
  }
}

const removeHistory = async (row: RiskAssessment) => {
  try {
    await ElMessageBox.confirm(`确定删除研判记录「${row.file_name}」？`, '删除确认', { type: 'warning' })
  } catch {
    return
  }
  try {
    await riskAssessmentApi.remove(row.id)
    ElMessage.success('已删除')
    if (currentResult.value?.id === row.id) {
      currentResult.value = null
    }
    loadHistory()
  } catch {
    ElMessage.error('删除失败')
  }
}

loadHistory()
</script>

<style scoped>
.risk-assessment {
  padding: 4px;
}

.upload-panel {
  margin-bottom: 16px;
}

.upload-actions {
  margin-top: 12px;
  display: flex;
  gap: 8px;
}

.stat-row {
  display: flex;
  gap: 12px;
  margin-bottom: 16px;
}

.stat-card {
  flex: 1;
  text-align: center;
}

.stat-value {
  font-size: 24px;
  font-weight: 600;
  color: var(--el-text-color-primary);
}

.stat-value--hit {
  color: var(--el-color-danger);
}

.stat-label {
  margin-top: 4px;
  font-size: 13px;
  color: var(--el-text-color-secondary);
}

.result-card :deep(.el-card__header) {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.result-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  width: 100%;
}

.result-title {
  display: inline-flex;
  align-items: center;
}

.result-actions {
  display: inline-flex;
  gap: 8px;
}

.ai-report-loading {
  height: 200px;
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--el-text-color-secondary);
}

.ai-report-content {
  white-space: pre-wrap;
  word-break: break-word;
  max-height: 60vh;
  overflow: auto;
  margin: 0;
  line-height: 1.7;
  font-family: inherit;
  color: var(--el-text-color-primary);
}

.result-tag {
  margin-left: 8px;
}

.hit-cell {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
}

.hit-tag {
  max-width: 220px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  vertical-align: middle;
}

.hit-more {
  margin-left: 4px;
}

.hit-filter {
  margin-bottom: 12px;
  width: 280px;
}

.disposition-none {
  color: var(--el-text-color-secondary);
  font-size: 12px;
}

.pagination-row {
  display: flex;
  justify-content: flex-end;
  margin-top: 12px;
}
</style>
