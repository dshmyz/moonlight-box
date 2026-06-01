<template>
  <el-dialog v-model="dialogVisible" title="批量导入" width="800px" @close="resetImport">
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
      <el-button @click="dialogVisible = false">取消</el-button>
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
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import { Download } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import * as XLSX from 'xlsx'
import type { UploadFile } from 'element-plus'
import { blockRuleApi, type BlockRuleCreateParams } from '@/api/blockRule'

const props = defineProps<{
  visible: boolean
}>()

const emit = defineEmits<{
  'update:visible': [value: boolean]
  'imported': []
}>()

const dialogVisible = computed({
  get: () => props.visible,
  set: (val) => emit('update:visible', val),
})

const importTab = ref('text')
const importText = ref('')
const parsedPreview = ref<BlockRuleCreateParams[]>([])
const pasteAreaRef = ref<HTMLElement | null>(null)
const importing = ref(false)

const resetImport = () => {
  importTab.value = 'text'
  importText.value = ''
  parsedPreview.value = []
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

const isHeaderLine = (line: string): boolean => {
  const lower = line.toLowerCase()
  return lower.includes('包名') || lower.includes('package_name') || lower.includes('版本') || lower.includes('version')
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
    dialogVisible.value = false
    emit('imported')
  } catch {
    ElMessage.error('批量导入失败')
  } finally {
    importing.value = false
  }
}
</script>

<style scoped>
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
  font-family: var(--font-family-mono);
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