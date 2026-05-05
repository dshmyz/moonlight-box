<template>
  <div class="block-rule-list">
    <div class="page-header">
      <h2>阻断规则</h2>
      <div class="header-actions">
        <CustomButton type="secondary" :icon="Upload" @click="showImportDialog = true">
          批量导入
        </CustomButton>
        <CustomButton type="primary" :icon="Plus" @click="openCreateDialog">
          创建规则
        </CustomButton>
      </div>
    </div>

    <CustomTabs v-model="activeTab" :tabs="mainTabs" />

    <div v-if="activeTab === 'rules'" class="tab-content">
      <div class="filter-bar">
        <CustomInput
          v-model="searchName"
          placeholder="搜索包名"
          clearable
          style="width: 200px"
          @clear="loadRules"
          @enter="loadRules"
        />
        <CustomSelect
          v-model="filterPkgType"
          :options="pkgTypeOptions"
          placeholder="包类型"
          style="width: 140px"
          @change="loadRules"
        />
        <CustomButton type="primary" @click="loadRules">搜索</CustomButton>
      </div>

      <CustomTable :columns="ruleColumns" :data="rules" :loading="loading" row-key="id">
        <template #match_type="{ row }">
          <CustomTag :type="row.match_type === 'exact' ? 'primary' : 'warning'" size="small">
            {{ row.match_type === 'exact' ? '精确' : '通配符' }}
          </CustomTag>
        </template>
        <template #enabled="{ row }">
          <el-switch
            :model-value="row.enabled"
            @change="(val: boolean) => toggleEnabled(row, val)"
          />
        </template>
        <template #created_at="{ row }">
          {{ formatTime(row.created_at) }}
        </template>
        <template #actions="{ row }">
          <div class="action-buttons">
            <CustomButton size="small" type="secondary" @click="openEditDialog(row)">编辑</CustomButton>
            <el-popconfirm title="确定删除此规则?" @confirm="deleteRule(row.id)">
              <template #reference>
                <CustomButton size="small" type="outline">删除</CustomButton>
              </template>
            </el-popconfirm>
          </div>
        </template>
      </CustomTable>
    </div>

    <div v-if="activeTab === 'logs'" class="tab-content">
      <BlockLogTable ref="logTableRef" />
    </div>

    <CustomDialog v-model="showDialog" :title="isEdit ? '编辑规则' : '创建规则'" width="520px">
      <BlockRuleForm
        v-if="showDialog"
        ref="ruleFormRef"
        v-model="formData"
      />
      <template #footer>
        <CustomButton type="secondary" @click="showDialog = false">取消</CustomButton>
        <CustomButton type="primary" @click="handleSubmit" :loading="submitting">确定</CustomButton>
      </template>
    </CustomDialog>

    <CustomDialog v-model="showImportDialog" title="批量导入" width="800px">
      <CustomTabs v-model="importTab" :tabs="importTabs" />

      <div v-if="importTab === 'text'" class="tab-content">
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
        <textarea
          v-model="importText"
          class="custom-textarea"
          :rows="12"
          placeholder='[{"package_name":"lodash","version":"4.17.20","package_type":"npm","reason":"安全漏洞"}]'
        />
      </div>

      <div v-if="importTab === 'paste'" class="tab-content">
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
        <CustomTable
          v-if="parsedPreview.length > 0"
          :columns="previewColumns"
          :data="parsedPreview"
          style="margin-top: var(--spacing-md)"
        />
      </div>

      <div v-if="importTab === 'file'" class="tab-content">
        <div class="import-desc">
          <p>上传 Excel (.xlsx/.xls) 文件：</p>
          <div class="upload-actions">
            <CustomButton type="primary" size="small" :icon="Download" @click="handleDownloadTemplate">
              下载模板
            </CustomButton>
          </div>
          <el-alert type="info" :closable="false" show-icon style="margin-top: var(--spacing-sm)">
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
        <CustomTable
          v-if="parsedPreview.length > 0"
          :columns="previewColumns"
          :data="parsedPreview"
          style="margin-top: var(--spacing-md)"
        />
      </div>

      <template #footer>
        <CustomButton type="secondary" @click="showImportDialog = false">取消</CustomButton>
        <CustomButton
          type="primary"
          @click="handleBatchImport"
          :loading="importing"
          :disabled="importTab === 'file' && parsedPreview.length === 0 && importText.length === 0"
        >
          导入
        </CustomButton>
      </template>
    </CustomDialog>
  </div>
</template>

<script setup lang="ts">
import { ref, watch, onMounted } from 'vue'
import { Plus, Upload, Download } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import * as XLSX from 'xlsx'
import type { UploadFile } from 'element-plus'
import { blockRuleApi, type BlockRule, type BlockRuleCreateParams } from '@/api/blockRule'
import BlockRuleForm from '@/components/block-rule/BlockRuleForm.vue'
import BlockLogTable from '@/components/block-rule/BlockLogTable.vue'
import CustomButton from '@/components/ui/CustomButton.vue'
import CustomTabs from '@/components/ui/CustomTabs.vue'
import CustomInput from '@/components/ui/CustomInput.vue'
import CustomSelect from '@/components/ui/CustomSelect.vue'
import CustomTable from '@/components/ui/CustomTable.vue'
import CustomTag from '@/components/ui/CustomTag.vue'
import CustomDialog from '@/components/ui/CustomDialog.vue'

// --- 数据定义 ---
const mainTabs = [
  { name: 'rules', label: '规则列表' },
  { name: 'logs', label: '阻断日志' },
]

const importTabs = [
  { name: 'text', label: '粘贴文本' },
  { name: 'paste', label: '粘贴表格' },
  { name: 'file', label: '上传文件' },
]

const pkgTypeOptions = [
  { label: '全部类型', value: '' },
  { label: 'npm', value: 'npm' },
  { label: 'maven', value: 'maven' },
]

const ruleColumns = [
  { prop: 'package_name', label: '包名', width: '180px' },
  { prop: 'version', label: '版本', width: '120px' },
  { prop: 'match_type', label: '匹配类型', width: '110px' },
  { prop: 'package_type', label: '包类型', width: '100px' },
  { prop: 'reason', label: '阻断原因' },
  { prop: 'enabled', label: '状态', width: '80px', align: 'center' as const },
  { prop: 'created_at', label: '创建时间', width: '170px' },
  { prop: 'actions', label: '操作', width: '150px', align: 'center' as const },
]

const previewColumns = [
  { prop: 'package_name', label: '包名', width: '150px' },
  { prop: 'version', label: '版本', width: '100px' },
  { prop: 'package_type', label: '包类型', width: '80px' },
  { prop: 'match_type', label: '匹配类型', width: '100px' },
  { prop: 'reason', label: '阻断原因' },
]

// --- 状态 ---
const loading = ref(false)
const submitting = ref(false)
const importing = ref(false)
const activeTab = ref('rules')
const rules = ref<BlockRule[]>([])
const searchName = ref('')
const filterPkgType = ref('')

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

// --- 对话框关闭时重置 ---
watch(showDialog, (val) => {
  if (!val) resetForm()
})

watch(showImportDialog, (val) => {
  if (!val) resetImport()
})

// --- 方法 ---
const formatTime = (t: string) => {
  if (!t) return ''
  return new Date(t).toLocaleString('zh-CN')
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
  padding: var(--spacing-xl);
}

.page-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: var(--spacing-xl);
}

.page-header h2 {
  margin: 0;
  font-size: var(--font-size-xl);
  font-weight: var(--font-weight-semibold);
  color: var(--color-text-primary);
}

.header-actions {
  display: flex;
  gap: var(--spacing-sm);
}

.tab-content {
  margin-top: var(--spacing-lg);
}

.filter-bar {
  display: flex;
  gap: var(--spacing-md);
  margin-bottom: var(--spacing-lg);
  align-items: center;
}

.action-buttons {
  display: flex;
  gap: var(--spacing-sm);
  justify-content: center;
}

.import-desc {
  margin-bottom: var(--spacing-md);
}

.import-desc p {
  margin: 0 0 var(--spacing-sm);
  font-size: var(--font-size-base);
  color: var(--color-text-secondary);
}

.format-example {
  margin: 0;
  font-size: var(--font-size-xs);
  line-height: 1.6;
  white-space: pre-wrap;
  color: var(--color-primary);
}

.custom-textarea {
  width: 100%;
  padding: 10px 14px;
  font-size: var(--font-size-base);
  color: var(--color-text-primary);
  background: #fafbfc;
  border: 2px solid var(--color-border);
  border-radius: var(--radius-lg);
  outline: none;
  transition: all var(--transition-base);
  font-family: inherit;
  resize: vertical;
  box-sizing: border-box;
}

.custom-textarea::placeholder {
  color: var(--color-text-tertiary);
}

.custom-textarea:hover:not(:focus) {
  border-color: var(--color-border-dark);
  background: #ffffff;
}

.custom-textarea:focus {
  border-color: var(--color-border-dark);
  background: #ffffff;
  box-shadow: 0 0 0 3px rgba(15, 23, 42, 0.08);
}

.paste-area {
  min-height: 80px;
  padding: var(--spacing-md);
  border: 1px dashed var(--color-border);
  border-radius: var(--radius-md);
  background: var(--color-bg-page);
  color: var(--color-text-tertiary);
  font-size: var(--font-size-sm);
  cursor: text;
  white-space: pre-wrap;
  word-break: break-all;
}

.paste-area:focus {
  border-color: var(--color-primary);
  background: var(--color-bg-card);
  outline: none;
}

.upload-area {
  width: 100%;
}

.upload-actions {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
}
</style>
