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
        <el-button class="ai-optimize-btn" @click="openOptimizerDialog">
          <el-icon><MagicStick /></el-icon>
          <span>AI 优化建议</span>
        </el-button>
        <el-button class="ai-generate-btn" @click="openGeneratorDialog">
          <el-icon><MagicStick /></el-icon>
          <span>AI 生成规则</span>
        </el-button>
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
                <el-option v-for="opt in PACKAGE_TYPE_OPTIONS" :key="opt.value" :label="opt.label" :value="opt.value" />
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
                  <el-tag :class="['match-tag', matchTypeClass(row.match_type)]" size="small">
                    {{ matchTypeLabel(row.match_type) }}
                  </el-tag>
                </template>
              </el-table-column>
              <el-table-column prop="package_type" label="包类型" min-width="85" align="center">
                <template #default="{ row }">
                  <span class="pkg-type">{{ row.package_type }}</span>
                </template>
              </el-table-column>
              <el-table-column prop="reason" label="阻断原因" min-width="150" show-overflow-tooltip />
              <el-table-column label="条件" min-width="140" align="center">
                <template #default="{ row }">
                  <span v-if="!row.condition_type">—</span>
                  <span v-else class="condition-summary">{{ formatCondition(row) }}</span>
                </template>
              </el-table-column>
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
                  {{ formatDate(row.created_at) }}
                </template>
              </el-table-column>
              <el-table-column label="操作" min-width="120" align="center">
                <template #default="{ row }">
                  <div class="operation-buttons">
                    <el-button class="btn-edit" size="small" @click="openEditDialog(row)">编辑</el-button>
                    <el-button class="btn-delete" size="small" link @click="confirmDeleteRule(row)">删除</el-button>
                  </div>
                </template>
              </el-table-column>
            </el-table>

            <el-pagination
              v-if="total > pageSize"
              :current-page="page"
              :page-size="pageSize"
              :total="total"
              layout="total, prev, pager, next"
              class="pagination"
              @current-change="handlePageChange"
            />
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

    <BlockRuleImportDialog v-model:visible="showImportDialog" @imported="loadRules" />

    <BlockRuleAIDialog
      v-model:visible="showAIDialog"
      :mode="aiDialogMode"
      @created="loadRules"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { Plus, Upload, MagicStick } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import { blockRuleApi, type BlockRule, type BlockRuleCreateParams } from '@/api/blockRule'
import BlockRuleForm from '@/components/block-rule/BlockRuleForm.vue'
import BlockLogTable from '@/components/block-rule/BlockLogTable.vue'
import BlockRuleImportDialog from '@/components/block-rule/BlockRuleImportDialog.vue'
import BlockRuleAIDialog from '@/components/block-rule/BlockRuleAIDialog.vue'
import { confirm, success, error } from '@/utils/message'
import { formatDate } from '@/utils/format'
import { PACKAGE_TYPE_OPTIONS } from '@/constants/package'
import { useTableRowHover } from '@/composables/useTableRowHover'

const { tableRowClass, handleRowEnter, handleRowLeave } = useTableRowHover()

const loading = ref(false)
const submitting = ref(false)
const activeTab = ref('rules')
const rules = ref<BlockRule[]>([])
const searchName = ref('')
const filterPkgType = ref('')
const page = ref(1)
const pageSize = ref(20)
const total = ref(0)

const showDialog = ref(false)
const showImportDialog = ref(false)
const showAIDialog = ref(false)
const aiDialogMode = ref<'generator' | 'optimizer'>('generator')
const isEdit = ref(false)
const editId = ref<number | null>(null)
const formData = ref<BlockRuleCreateParams>({
  package_name: '',
  version: '',
  match_type: 'exact',
  package_type: 'npm',
  reason: '',
  enabled: true,
  condition_type: '',
  condition_op: '',
  condition_value: '',
})

const ruleFormRef = ref<InstanceType<typeof BlockRuleForm> | null>(null)
const logTableRef = ref<InstanceType<typeof BlockLogTable> | null>(null)

// 操作符的中文映射
const OPERATOR_LABELS: Record<string, string> = {
  equals: '等于',
  contains: '包含',
  before: '早于',
  after: '晚于',
}

// 条件类型的中文映射
const CONDITION_TYPE_LABELS: Record<string, string> = {
  license: 'License',
  publish_time: '发布时间',
}

const MATCH_TYPE_LABELS: Record<string, string> = {
  exact: '精确',
  wildcard: '通配符',
  range: '版本范围',
}

const matchTypeLabel = (matchType: string) => MATCH_TYPE_LABELS[matchType] || matchType

const matchTypeClass = (matchType: string) => {
  if (matchType === 'range') return 'match-tag--range'
  if (matchType === 'wildcard') return 'match-tag--wildcard'
  return 'match-tag--exact'
}

// 生成条件摘要文案，如 "License 等于 GPL-3.0"
const formatCondition = (row: BlockRule) => {
  const typeLabel = CONDITION_TYPE_LABELS[row.condition_type] || row.condition_type
  const opLabel = OPERATOR_LABELS[row.condition_op] || row.condition_op
  return `${typeLabel} ${opLabel} ${row.condition_value}`
}

const loadRules = async () => {
  loading.value = true
  try {
    const params: Record<string, any> = { page: page.value, page_size: pageSize.value }
    if (searchName.value) params.package_name = searchName.value
    if (filterPkgType.value) params.package_type = filterPkgType.value
    const res = await blockRuleApi.list(params)
    rules.value = res?.items || []
    total.value = res?.pagination?.total || 0
  } catch {
    ElMessage.error('加载阻断规则失败')
  } finally {
    loading.value = false
  }
}

const handlePageChange = (p: number) => {
  page.value = p
  loadRules()
}

const openGeneratorDialog = () => {
  aiDialogMode.value = 'generator'
  showAIDialog.value = true
}

const openOptimizerDialog = () => {
  aiDialogMode.value = 'optimizer'
  showAIDialog.value = true
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
    condition_type: '',
    condition_op: '',
    condition_value: '',
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
    condition_type: row.condition_type ?? '',
    condition_op: row.condition_op ?? '',
    condition_value: row.condition_value ?? '',
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

const confirmDeleteRule = async (row: BlockRule) => {
  const ok = await confirm({
    title: '删除确认',
    message: `确定要删除规则 "${row.package_name}" 吗？`,
    type: 'warning',
  })
  if (ok) {
    await deleteRule(row.id)
  }
}

const deleteRule = async (id: number) => {
  try {
    await blockRuleApi.delete(id)
    success('删除成功')
    loadRules()
  } catch {
    error('删除失败')
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

.ai-generate-btn,
.ai-optimize-btn {
  border-radius: 10px;
  padding: 10px 18px;
  font-weight: 500;
  font-size: 13px;
  display: flex;
  align-items: center;
  gap: 8px;
  background: linear-gradient(135deg, #f0fdf4 0%, #dcfce7 100%);
  border: 1px solid #86efac;
  color: #15803d;
  transition: all 0.2s ease;
}

.ai-generate-btn:hover,
.ai-optimize-btn:hover {
  background: linear-gradient(135deg, #dcfce7 0%, #bbf7d0 100%);
  border-color: #4ade80;
  color: #166534;
}

.ai-optimize-btn {
  background: linear-gradient(135deg, #fefce8 0%, #fef9c3 100%);
  border-color: #fde047;
  color: #a16207;
}

.ai-optimize-btn:hover {
  background: linear-gradient(135deg, #fef9c3 0%, #fef08a 100%);
  border-color: #facc15;
  color: #854d0e;
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

.match-tag--range {
  background: linear-gradient(135deg, #dcfce7 0%, #bbf7d0 100%);
  color: #16a34a;
}

.pkg-type {
  font-size: 12px;
  font-weight: 500;
  color: #64748b;
  background: #f1f5f9;
  padding: 3px 8px;
  border-radius: 4px;
}

.condition-summary {
  font-size: 12px;
  color: #475569;
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

.pagination {
  margin-top: 16px;
  display: flex;
  justify-content: flex-end;
}

:deep(.el-pagination) {
  font-weight: 500;
}
</style>
