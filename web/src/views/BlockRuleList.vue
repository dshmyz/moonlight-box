<template>
  <div class="block-rule-list">
    <header class="list-header">
      <div class="header-content">
        <div class="header-icon">
          <i class="fa-solid fa-ban"></i>
        </div>
        <div class="header-text">
          <h2>阻断规则</h2>
          <p class="header-subtitle">管理和配置包下载阻断规则</p>
        </div>
      </div>
      <div class="header-actions">
        <el-button class="import-btn" @click="showImportDialog = true">
          <el-icon><Upload /></el-icon>
          <span>批量导入</span>
        </el-button>
        <el-button type="primary" class="create-btn" @click="openCreateDialog">
          <el-icon><Plus /></el-icon>
          <span>创建规则</span>
        </el-button>
      </div>
    </header>

    <div class="content-panel" v-loading="loading">
      <el-tabs v-model="activeTab" class="type-tabs">
        <el-tab-pane label="规则列表" name="rules">
          <div class="tab-content">
            <div class="filter-bar">
              <div class="search-wrapper">
                <el-input
                  v-model="searchName"
                  placeholder="搜索包名"
                  clearable
                  class="search-input"
                  @clear="loadRules"
                  @keyup.enter="loadRules"
                >
                  <template #prefix>
                    <i class="fa-solid fa-magnifying-glass search-icon"></i>
                  </template>
                </el-input>
              </div>

              <el-select
                v-model="filterPkgType"
                placeholder="包类型"
                clearable
                class="filter-select"
                @change="loadRules"
              >
                <el-option v-for="opt in pkgTypeOptions" :key="opt.value" :label="opt.label" :value="opt.value" />
              </el-select>

              <el-button type="primary" class="search-btn" @click="loadRules">搜索</el-button>
            </div>

            <el-table
              :data="rules"
              style="width: 100%"
              :header-cell-style="{ background: '#fafbfc' }"
              :row-class-name="tableRowClass"
              @row-mouse-enter="handleRowEnter"
              @row-mouse-leave="handleRowLeave"
              fit
            >
              <el-table-column prop="package_name" label="包名" min-width="120" show-overflow-tooltip />
              <el-table-column prop="version" label="版本" min-width="100" align="center" />
              <el-table-column prop="match_type" label="匹配类型" min-width="90" align="center">
                <template #default="{ row }">
                  <el-tag :class="['match-tag', row.match_type === 'exact' ? 'match-tag--exact' : 'match-tag--wildcard']" size="small">
                    {{ row.match_type === 'exact' ? '精确' : '通配符' }}
                  </el-tag>
                </template>
              </el-table-column>
              <el-table-column prop="package_type" label="包类型" min-width="85" align="center">
                <template #default="{ row }">
                  <span class="pkg-type">{{ row.package_type }}</span>
                </template>
              </el-table-column>
              <el-table-column prop="reason" label="阻断原因" min-width="150" show-overflow-tooltip />
              <el-table-column prop="enabled" label="状态" min-width="85" align="center">
                <template #default="{ row }">
                  <el-switch
                    :model-value="row.enabled"
                    @change="(val: boolean) => toggleEnabled(row, val)"
                  />
                </template>
              </el-table-column>
              <el-table-column prop="created_at" label="创建时间" min-width="180" align="center">
                <template #default="{ row }">
                  {{ formatTime(row.created_at) }}
                </template>
              </el-table-column>
              <el-table-column label="操作" min-width="120" align="center">
                <template #default="{ row }">
                  <div class="operation-buttons">
                    <el-button class="btn-edit" size="small" @click="openEditDialog(row)">编辑</el-button>
                    <el-popconfirm title="确定删除此规则?" @confirm="deleteRule(row.id)">
                      <template #reference>
                        <el-button class="btn-delete" size="small" type="text">删除</el-button>
                      </template>
                    </el-popconfirm>
                  </div>
                </template>
              </el-table-column>
            </el-table>
          </div>
        </el-tab-pane>

        <el-tab-pane label="阻断日志" name="logs">
          <div class="tab-content">
            <BlockLogTable ref="logTableRef" />
          </div>
        </el-tab-pane>
      </el-tabs>
    </div>

    <el-dialog v-model="showDialog" :title="isEdit ? '编辑规则' : '创建规则'" width="520px" @close="resetForm">
      <BlockRuleForm
        v-if="showDialog"
        ref="ruleFormRef"
        v-model="formData"
      />
      <template #footer>
        <el-button @click="showDialog = false">取消</el-button>
        <el-button type="primary" @click="handleSubmit" :loading="submitting">确定</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="showImportDialog" title="批量导入" width="800px" @close="resetImport">
      <el-tabs v-model="importTab">
        <el-tab-pane label="粘贴文本" name="text">
          <div class="import-desc">
            <p>按以下 JSON 格式输入阻断规则：</p>
            <el-alert type="info" :closable="false" show-icon>
              <template #title>
                <pre class="format-example">
[
  { "package_name": "lodash", "version": "4.17.20", "package_type": "npm", "reason": "安全漏洞" }
]
                </pre>
              </template>
            </el-alert>
          </div>
          <el-input
            v-model="importText"
            type="textarea"
            :rows="12"
            placeholder='[{"package_name":"lodash","version":"4.17.20","package_type":"npm","reason":"安全漏洞"}]'
          />
        </el-tab-pane>

        <el-tab-pane label="粘贴表格" name="paste">
          <div class="import-desc">
            <p>从 Excel / Google Sheets 复制表格后直接粘贴到下方：</p>
            <el-alert type="info" :closable="false" show-icon>
              <template #title>
                表头顺序：包名 | 版本 | 包类型(npm/maven) | 匹配类型(exact/wildcard) | 阻断原因
              </template>
            </el-alert>
          </div>
          <div
            class="paste-area"
            @paste="handlePaste"
            contenteditable="true"
            ref="pasteAreaRef"
          >
            点击此处并按 Ctrl+V 粘贴表格数据
          </div>
          <el-table v-if="parsedPreview.length > 0" :data="parsedPreview" size="small" style="margin-top: 12px" max-height="260">
            <el-table-column prop="package_name" label="包名" width="150" />
            <el-table-column prop="version" label="版本" width="100" />
            <el-table-column prop="package_type" label="包类型" width="80" />
            <el-table-column prop="match_type" label="匹配类型" width="100" />
            <el-table-column prop="reason" label="阻断原因" min-width="180" show-overflow-tooltip />
          </el-table>
        </el-tab-pane>

        <el-tab-pane label="上传文件" name="file">
          <div class="import-desc">
            <p>上传 Excel (.xlsx/.xls) 文件：</p>
            <div class="upload-actions">
              <el-button type="primary" size="small" @click="handleDownloadTemplate">
                <el-icon><Download /></el-icon> 下载模板
              </el-button>
            </div>
            <el-alert type="info" :closable="false" show-icon style="margin-top: 8px">
              <template #title>
                表格第一行为表头，列顺序：包名 | 版本 | 包类型(npm/maven) | 匹配类型(exact/wildcard) | 阻断原因
              </template>
            </el-alert>
          </div>
          <el-upload
            class="upload-area"
            drag
            :auto-upload="false"
            :on-change="handleFileChange"
            accept=".xlsx,.xls"
            :limit="1"
          >
            <div class="el-upload__text">拖拽文件到此处，或 <em>点击上传</em></div>
          </el-upload>
          <el-table v-if="parsedPreview.length > 0" :data="parsedPreview" size="small" style="margin-top: 12px" max-height="260">
            <el-table-column prop="package_name" label="包名" width="150" />
            <el-table-column prop="version" label="版本" width="100" />
            <el-table-column prop="package_type" label="包类型" width="80" />
            <el-table-column prop="match_type" label="匹配类型" width="100" />
            <el-table-column prop="reason" label="阻断原因" min-width="180" show-overflow-tooltip />
          </el-table>
        </el-tab-pane>
      </el-tabs>

      <template #footer>
        <el-button @click="showImportDialog = false">取消</el-button>
        <el-button
          type="primary"
          @click="handleBatchImport"
          :loading="importing"
          :disabled="importTab === 'file' && parsedPreview.length === 0 && importText.length === 0"
        >
          导入
        </el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { Plus, Upload, Download } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import * as XLSX from 'xlsx'
import type { UploadFile } from 'element-plus'
import { blockRuleApi, type BlockRule, type BlockRuleCreateParams } from '@/api/blockRule'
import BlockRuleForm from '@/components/block-rule/BlockRuleForm.vue'
import BlockLogTable from '@/components/block-rule/BlockLogTable.vue'

const loading = ref(false)
const submitting = ref(false)
const importing = ref(false)
const activeTab = ref('rules')
const rules = ref<BlockRule[]>([])
const searchName = ref('')
const filterPkgType = ref('')
const hoveredRow = ref<number | null>(null)

const pkgTypeOptions = [
  { label: 'npm', value: 'npm' },
  { label: 'Maven', value: 'maven' },
  { label: 'PyPI', value: 'pypi' },
  { label: 'Go', value: 'go' },
  { label: 'NuGet', value: 'nuget' },
  { label: 'Yum', value: 'yum' },
  { label: 'Apt', value: 'apt' },
  { label: 'Generic', value: 'generic' },
]

const showDialog = ref(false)
const showImportDialog = ref(false)
const importTab = ref('text')
const importText = ref('')
const parsedPreview = ref<BlockRuleCreateParams[]>([])
const pasteAreaRef = ref<HTMLElement | null>(null)
const isEdit = ref(false)
const editId = ref<number | null>(null)
const formData = ref<BlockRuleCreateParams>({
  package_name: '',
  version: '',
  match_type: 'exact',
  package_type: 'npm',
  reason: '',
  enabled: true,
})

const ruleFormRef = ref<InstanceType<typeof BlockRuleForm> | null>(null)
const logTableRef = ref<InstanceType<typeof BlockLogTable> | null>(null)

const formatTime = (t: string) => {
  if (!t) return ''
  return new Date(t).toLocaleString('zh-CN')
}

const tableRowClass = ({ rowIndex }: { rowIndex: number }) => {
  return hoveredRow.value === rowIndex ? 'row-hovered' : ''
}

const handleRowEnter = ({ rowIndex }: { rowIndex: number }) => {
  hoveredRow.value = rowIndex
}

const handleRowLeave = () => {
  hoveredRow.value = null
}

const loadRules = async () => {
  loading.value = true
  try {
    const params: Record<string, string> = {}
    if (searchName.value) params.package_name = searchName.value
    if (filterPkgType.value) params.package_type = filterPkgType.value
    const res = await blockRuleApi.list(params)
    rules.value = res || []
  } catch {
    ElMessage.error('加载阻断规则失败')
  } finally {
    loading.value = false
  }
}

const openCreateDialog = () => {
  isEdit.value = false
  editId.value = null
  formData.value = {
    package_name: '',
    version: '',
    match_type: 'exact',
    package_type: 'npm',
    reason: '',
    enabled: true,
  }
  showDialog.value = true
}

const openEditDialog = (row: BlockRule) => {
  isEdit.value = true
  editId.value = row.id
  formData.value = {
    package_name: row.package_name,
    version: row.version,
    match_type: row.match_type,
    package_type: row.package_type,
    reason: row.reason,
    enabled: row.enabled,
  }
  showDialog.value = true
}

const handleSubmit = async () => {
  if (ruleFormRef.value?.formRef) {
    try {
      await ruleFormRef.value.formRef.validate()
    } catch {
      return
    }
  }

  submitting.value = true
  try {
    if (isEdit.value && editId.value !== null) {
      await blockRuleApi.update(editId.value, formData.value)
      ElMessage.success('更新成功')
    } else {
      await blockRuleApi.create(formData.value)
      ElMessage.success('创建成功')
    }
    showDialog.value = false
    loadRules()
  } catch {
    ElMessage.error(isEdit.value ? '更新失败' : '创建失败')
  } finally {
    submitting.value = false
  }
}

const deleteRule = async (id: number) => {
  try {
    await blockRuleApi.delete(id)
    ElMessage.success('删除成功')
    loadRules()
  } catch {
    ElMessage.error('删除失败')
  }
}

const toggleEnabled = async (row: BlockRule, val: boolean) => {
  try {
    await blockRuleApi.update(row.id, { enabled: val })
    row.enabled = val
    ElMessage.success(val ? '已启用' : '已禁用')
  } catch {
    ElMessage.error('操作失败')
  }
}

const handleBatchImport = async () => {
  let parsedRules: BlockRuleCreateParams[] = []

  if (importTab.value === 'text') {
    if (!importText.value.trim()) {
      ElMessage.warning('请输入导入数据')
      return
    }
    try {
      parsedRules = JSON.parse(importText.value)
    } catch {
      ElMessage.error('JSON 格式错误，请检查输入')
      return
    }
    if (!Array.isArray(parsedRules) || parsedRules.length === 0) {
      ElMessage.warning('请输入有效的规则数组')
      return
    }
  } else if (importTab.value === 'paste' || importTab.value === 'file') {
    if (parsedPreview.value.length === 0) {
      ElMessage.warning('请先解析数据')
      return
    }
    parsedRules = parsedPreview.value
  }

  importing.value = true
  try {
    const res = await blockRuleApi.batchImport({ rules: parsedRules })
    const { success, failed, total } = res || {}
    if (failed === 0) {
      ElMessage.success(`成功导入 ${success} 条规则`)
    } else {
      ElMessage.warning(`共 ${total} 条，成功 ${success} 条，失败 ${failed} 条`)
    }
    showImportDialog.value = false
    loadRules()
  } catch {
    ElMessage.error('批量导入失败')
  } finally {
    importing.value = false
  }
}

const parseRowToRule = (cells: string[]): BlockRuleCreateParams | null => {
  const [packageName, version, packageType, matchType, reason] = cells
  if (!packageName?.trim() || !version?.trim()) return null
  const pt = (packageType || 'npm').trim().toLowerCase()
  if (!['npm', 'maven'].includes(pt)) return null
  const mt = (matchType || 'exact').trim().toLowerCase()
  return {
    package_name: packageName.trim(),
    version: version.trim(),
    package_type: pt,
    match_type: mt === 'wildcard' ? 'wildcard' : 'exact',
    reason: (reason || '').trim(),
    enabled: true,
  }
}

const parseTsvText = (text: string): BlockRuleCreateParams[] => {
  const lines = text.trim().split('\n')
  if (lines.length === 0) return []
  const startIdx = isHeaderLine(lines[0]) ? 1 : 0
  const rules: BlockRuleCreateParams[] = []
  for (let i = startIdx; i < lines.length; i++) {
    const cells = lines[i].split('\t')
    const rule = parseRowToRule(cells)
    if (rule) rules.push(rule)
  }
  return rules
}

const isHeaderLine = (line: string): boolean => {
  const lower = line.toLowerCase()
  return lower.includes('包名') || lower.includes('package_name') || lower.includes('版本') || lower.includes('version')
}

const handlePaste = (e: ClipboardEvent) => {
  e.preventDefault()
  const text = e.clipboardData?.getData('text/plain')
  if (!text) return
  const rules = parseTsvText(text)
  if (rules.length === 0) {
    ElMessage.warning('未能解析到有效数据')
    return
  }
  parsedPreview.value = rules
  ElMessage.success(`解析到 ${rules.length} 条规则，请检查预览后点击导入`)
}

const handleFileChange = (file: UploadFile) => {
  const raw = file.raw
  if (!raw) return
  const reader = new FileReader()
  reader.onload = (e) => {
    try {
      const data = new Uint8Array(e.target?.result as ArrayBuffer)
      const workbook = XLSX.read(data, { type: 'array' })
      const sheetName = workbook.SheetNames[0]
      const worksheet = workbook.Sheets[sheetName]
      const tsv = XLSX.utils.sheet_to_csv(worksheet, { FS: '\t' })
      const rules = parseTsvText(tsv)
      if (rules.length === 0) {
        ElMessage.warning('未能从文件中解析到有效数据')
        return
      }
      parsedPreview.value = rules
      ElMessage.success(`从文件解析到 ${rules.length} 条规则，请检查预览后点击导入`)
    } catch {
      ElMessage.error('文件解析失败')
    }
  }
  reader.readAsArrayBuffer(raw)
}

const handleDownloadTemplate = async () => {
  try {
    const res = await blockRuleApi.downloadTemplate()
    const blob = new Blob([res as BlobPart], { type: 'text/csv;charset=utf-8' })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = 'block_rule_template.csv'
    a.click()
    URL.revokeObjectURL(url)
  } catch {
    ElMessage.error('下载模板失败')
  }
}

const resetImport = () => {
  importTab.value = 'text'
  importText.value = ''
  parsedPreview.value = []
}

const resetForm = () => {
  isEdit.value = false
  editId.value = null
}

onMounted(loadRules)
</script>

<style scoped>
.block-rule-list {
  padding: var(--spacing-5);
  min-height: 100%;
  background: linear-gradient(180deg, #f8fafc 0%, #f1f5f9 100%);
}

.list-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 20px 24px;
  background: #fff;
  border-radius: 16px;
  margin-bottom: 16px;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.04);
}

.header-content {
  display: flex;
  align-items: center;
  gap: 16px;
}

.header-icon {
  width: 52px;
  height: 52px;
  border-radius: 14px;
  background: linear-gradient(135deg, #fee2e2 0%, #fecaca 100%);
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 22px;
  color: #dc2626;
}

.header-text h2 {
  margin: 0;
  font-size: 20px;
  font-weight: 600;
  color: #1e293b;
}

.header-subtitle {
  margin: 4px 0 0;
  font-size: 13px;
  color: #94a3b8;
}

.header-actions {
  display: flex;
  gap: 12px;
}

.import-btn {
  border-radius: 10px;
  padding: 10px 18px;
  font-weight: 500;
  font-size: 13px;
  display: flex;
  align-items: center;
  gap: 8px;
  background: #f8fafc;
  border: 1px solid #e2e8f0;
  color: #64748b;
  transition: all 0.2s ease;
}

.import-btn:hover {
  background: #f1f5f9;
  border-color: #cbd5e1;
  color: #475569;
}

.create-btn {
  border-radius: 10px;
  padding: 10px 18px;
  font-weight: 500;
  font-size: 13px;
  display: flex;
  align-items: center;
  gap: 8px;
  background: linear-gradient(135deg, #6366f1 0%, #4f46e5 100%);
  border: none;
  box-shadow: 0 4px 12px rgba(99, 102, 241, 0.3);
  transition: all 0.2s ease;
}

.create-btn:hover {
  transform: translateY(-1px);
  box-shadow: 0 6px 16px rgba(99, 102, 241, 0.4);
}

.content-panel {
  background: #fff;
  border-radius: 16px;
  padding: 20px;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.04);
}

.type-tabs {
  --el-color-primary: #6366f1;
}

:deep(.el-tabs__item) {
  font-size: 14px;
  font-weight: 500;
  color: #64748b;
  padding: 0 20px;
}

:deep(.el-tabs__item.is-active) {
  color: #6366f1;
  font-weight: 600;
}

:deep(.el-tabs__active-bar) {
  height: 3px;
  border-radius: 3px;
  background: linear-gradient(90deg, #6366f1 0%, #818cf8 100%);
}

:deep(.el-tabs__nav-wrap::after) {
  height: 1px;
  background: #e2e8f0;
}

.tab-content {
  padding-top: 16px;
}

.filter-bar {
  display: flex;
  gap: 12px;
  margin-bottom: 16px;
  align-items: center;
}

.search-input {
  width: 200px;
}

:deep(.search-input .el-input__wrapper) {
  border-radius: 8px;
  box-shadow: none;
  border: 1px solid #e2e8f0;
  padding: 0 12px;
  height: 36px;
  font-size: 13px;
  background: #f8fafc;
  transition: all 0.25s cubic-bezier(0.4, 0, 0.2, 1);
}

:deep(.search-input .el-input__wrapper:hover) {
  border-color: #cbd5e1;
  background: #fff;
}

:deep(.search-input .el-input__wrapper.is-focus) {
  border-color: #6366f1;
  box-shadow: 0 0 0 3px rgba(99, 102, 241, 0.1);
  background: #fff;
}

.search-icon {
  font-size: 14px;
  color: #94a3b8;
}

.filter-select {
  width: 130px;
}

:deep(.filter-select .el-input__wrapper) {
  border-radius: 8px;
  box-shadow: none;
  border: 1px solid #e2e8f0;
  padding: 0 12px;
  height: 36px;
  font-size: 13px;
  background: #f8fafc;
  transition: all 0.25s cubic-bezier(0.4, 0, 0.2, 1);
}

:deep(.filter-select .el-input__wrapper:hover) {
  border-color: #cbd5e1;
}

:deep(.filter-select .el-input__wrapper.is-focus) {
  border-color: #6366f1;
  box-shadow: 0 0 0 3px rgba(99, 102, 241, 0.1);
}

.search-btn {
  border-radius: 8px;
  padding: 8px 16px;
  font-size: 13px;
  font-weight: 500;
  background: linear-gradient(135deg, #6366f1 0%, #4f46e5 100%);
  border: none;
  box-shadow: 0 2px 8px rgba(99, 102, 241, 0.25);
}

:deep(.el-table) {
  border-radius: 12px;
  overflow: hidden;
}

:deep(.el-table th) {
  font-weight: 600;
  color: #475569;
  font-size: 13px;
}

:deep(.el-table td) {
  padding: 14px 12px;
  border-bottom: 1px solid rgba(0, 0, 0, 0.03);
}

:deep(.el-table .row-hovered td) {
  background: #f8fafc;
}

.match-tag {
  font-size: 11px;
  padding: 4px 10px;
  border-radius: 6px;
  font-weight: 500;
  border: none;
}

.match-tag--exact {
  background: linear-gradient(135deg, #e0e7ff 0%, #c7d2fe 100%);
  color: #4f46e5;
}

.match-tag--wildcard {
  background: linear-gradient(135deg, #fef3c7 0%, #fde68a 100%);
  color: #d97706;
}

.pkg-type {
  font-size: 12px;
  font-weight: 500;
  color: #64748b;
  background: #f1f5f9;
  padding: 3px 8px;
  border-radius: 4px;
}

.operation-buttons {
  display: flex;
  gap: 8px;
  align-items: center;
  justify-content: center;
}

.btn-edit {
  border-radius: 8px;
  padding: 6px 12px;
  font-size: 12px;
  font-weight: 500;
  color: #64748b;
  border: 1px solid #e2e8f0;
  background: #f8fafc;
  transition: all 0.2s ease;
}

.btn-edit:hover {
  background: #f1f5f9;
  border-color: #cbd5e1;
  color: #475569;
}

.btn-delete {
  border-radius: 8px;
  padding: 6px 10px;
  font-size: 12px;
  font-weight: 500;
  color: #dc2626;
  transition: all 0.2s ease;
}

.btn-delete:hover {
  background: #fef2f2;
}

.import-desc {
  margin-bottom: 16px;
}

.import-desc p {
  margin: 0 0 8px;
  font-size: 14px;
  color: #475569;
}

.format-example {
  margin: 0;
  font-size: 12px;
  color: #64748b;
  font-family: 'Monaco', 'Menlo', monospace;
}

.paste-area {
  border: 2px dashed #e2e8f0;
  border-radius: 8px;
  padding: 32px;
  text-align: center;
  color: #94a3b8;
  font-size: 14px;
  cursor: text;
  transition: all 0.2s ease;
}

.paste-area:hover {
  border-color: #6366f1;
  background: #f8fafc;
}

.upload-area {
  margin-top: 16px;
}

:deep(.upload-area .el-upload-dragger) {
  border-radius: 12px;
  border: 2px dashed #e2e8f0;
  background: #fafbfc;
  padding: 40px 20px;
  transition: all 0.2s ease;
}

:deep(.upload-area .el-upload-dragger:hover) {
  border-color: #6366f1;
  background: #f8fafc;
}

:deep(.upload-area .el-upload__text) {
  color: #64748b;
  font-size: 14px;
}

:deep(.upload-area .el-upload__text em) {
  color: #6366f1;
  font-style: normal;
}

.upload-actions {
  margin-bottom: 8px;
}
</style>
