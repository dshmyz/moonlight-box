<template>
  <div class="vuln-rule-management">
    <el-tabs v-model="activeTab">
      <el-tab-pane label="规则管理" name="rules">
        <div class="rules-panel">
          <div class="toolbar">
            <div class="toolbar-left">
              <el-select v-model="filterSource" placeholder="规则来源" clearable class="filter-select" @change="loadRules">
                <el-option label="内置" value="builtin" />
                <el-option label="自定义" value="custom" />
                <el-option label="同步" value="synced" />
              </el-select>
              <el-select v-model="filterSeverity" placeholder="严重等级" clearable class="filter-select" @change="loadRules">
                <el-option label="严重" value="critical" />
                <el-option label="高危" value="high" />
                <el-option label="中危" value="medium" />
                <el-option label="低危" value="low" />
              </el-select>
              <el-select v-model="filterPkgType" placeholder="包类型" clearable class="filter-select" @change="loadRules">
                <el-option label="npm" value="npm" />
                <el-option label="maven" value="maven" />
                <el-option label="pypi" value="pypi" />
                <el-option label="go" value="go" />
              </el-select>
              <el-input v-model="keyword" placeholder="搜索规则..." clearable class="search-input" @keyup.enter="loadRules" />
              <el-button type="primary" @click="loadRules">搜索</el-button>
            </div>
            <div class="toolbar-right">
              <el-button type="primary" @click="showAddDialog">新增规则</el-button>
              <el-button type="success" @click="showImportDialog">批量导入</el-button>
            </div>
          </div>

          <el-table :data="rules" v-loading="loading" style="width: 100%">
            <el-table-column prop="cve" label="CVE" width="160">
              <template #default="{ row }">
                <el-link type="primary" :href="row.references" target="_blank">{{ row.cve }}</el-link>
              </template>
            </el-table-column>
            <el-table-column prop="title" label="漏洞名称" min-width="200" show-overflow-tooltip />
            <el-table-column prop="package_pattern" label="包名模式" width="160" show-overflow-tooltip />
            <el-table-column prop="package_type" label="包类型" width="90" align="center" />
            <el-table-column prop="max_version" label="最大版本" width="100" align="center" />
            <el-table-column prop="severity" label="严重等级" width="90" align="center">
              <template #default="{ row }">
                <el-tag :class="['severity-tag', `severity-tag--${row.severity}`]" size="small">
                  {{ severityLabel(row.severity) }}
                </el-tag>
              </template>
            </el-table-column>
            <el-table-column prop="cvss" label="CVSS" width="80" align="center" />
            <el-table-column prop="source" label="来源" width="80" align="center">
              <template #default="{ row }">
                <el-tag :type="row.source === 'builtin' ? 'info' : row.source === 'custom' ? 'success' : 'warning'" size="small">
                  {{ sourceLabel(row.source) }}
                </el-tag>
              </template>
            </el-table-column>
            <el-table-column prop="enabled" label="状态" width="80" align="center">
              <template #default="{ row }">
                <el-tag :type="row.enabled ? 'success' : 'danger'" size="small">
                  {{ row.enabled ? '启用' : '禁用' }}
                </el-tag>
              </template>
            </el-table-column>
            <el-table-column label="操作" width="140" align="center">
              <template #default="{ row }">
                <el-button size="small" type="primary" @click="showEditDialog(row)" :disabled="row.source === 'builtin'">编辑</el-button>
                <el-button size="small" type="danger" @click="deleteRule(row.id)" :disabled="row.source === 'builtin'">删除</el-button>
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

      <el-tab-pane label="数据源管理" name="sources">
        <div class="sources-panel">
          <div class="toolbar">
            <div class="toolbar-left">
              <el-button type="info" @click="syncAll">同步全部</el-button>
            </div>
            <div class="toolbar-right">
              <el-button type="primary" @click="showAddSourceDialog">新增数据源</el-button>
            </div>
          </div>

          <el-table :data="dataSources" v-loading="loading" style="width: 100%">
            <el-table-column prop="name" label="名称" width="150" />
            <el-table-column prop="type" label="类型" width="100" align="center">
              <template #default="{ row }">
                <el-tag size="small">{{ row.type === 'http' ? 'HTTP API' : row.type }}</el-tag>
              </template>
            </el-table-column>
            <el-table-column prop="url" label="地址" min-width="300" show-overflow-tooltip />
            <el-table-column prop="enabled" label="状态" width="80" align="center">
              <template #default="{ row }">
                <el-tag :type="row.enabled ? 'success' : 'danger'" size="small">
                  {{ row.enabled ? '启用' : '禁用' }}
                </el-tag>
              </template>
            </el-table-column>
            <el-table-column prop="last_status" label="上次同步" width="100" align="center">
              <template #default="{ row }">
                <el-tag :type="row.last_status === 'success' ? 'success' : row.last_status === 'failed' ? 'danger' : 'info'" size="small" v-if="row.last_status">
                  {{ row.last_status === 'success' ? '成功' : '失败' }}
                </el-tag>
                <span v-else>-</span>
              </template>
            </el-table-column>
            <el-table-column prop="last_sync_at" label="同步时间" width="170" align="center">
              <template #default="{ row }">
                {{ row.last_sync_at ? formatDate(row.last_sync_at) : '-' }}
              </template>
            </el-table-column>
            <el-table-column label="操作" width="220" align="center">
              <template #default="{ row }">
                <el-button size="small" type="success" @click="syncDataSource(row.id)">同步</el-button>
                <el-button size="small" type="warning" @click="testDataSource(row)">测试</el-button>
                <el-button size="small" @click="showEditSourceDialog(row)">编辑</el-button>
                <el-button size="small" type="danger" @click="deleteDataSource(row.id)">删除</el-button>
              </template>
            </el-table-column>
          </el-table>
        </div>
      </el-tab-pane>
    </el-tabs>

    <el-dialog v-model="ruleDialogVisible" :title="editingRule.id ? '编辑规则' : '新增规则'" width="600px">
      <el-form :model="editingRule" label-width="100px">
        <el-form-item label="CVE编号">
          <el-input v-model="editingRule.cve" placeholder="CVE-2023-XXXXX" />
        </el-form-item>
        <el-form-item label="漏洞名称">
          <el-input v-model="editingRule.title" placeholder="漏洞名称" />
        </el-form-item>
        <el-form-item label="包名模式">
          <el-input v-model="editingRule.package_pattern" placeholder="正则表达式，如 ^lodash$" />
        </el-form-item>
        <el-form-item label="包类型">
          <el-select v-model="editingRule.package_type" placeholder="选择包类型" style="width: 100%">
            <el-option label="npm" value="npm" />
            <el-option label="maven" value="maven" />
            <el-option label="pypi" value="pypi" />
            <el-option label="go" value="go" />
          </el-select>
        </el-form-item>
        <el-row :gutter="16">
          <el-col :span="12">
            <el-form-item label="最大版本">
              <el-input v-model="editingRule.max_version" placeholder="如 4.17.21" />
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="最小版本">
              <el-input v-model="editingRule.min_version" placeholder="如 1.0.0" />
            </el-form-item>
          </el-col>
        </el-row>
        <el-row :gutter="16">
          <el-col :span="12">
            <el-form-item label="严重等级">
              <el-select v-model="editingRule.severity" style="width: 100%">
                <el-option label="严重" value="critical" />
                <el-option label="高危" value="high" />
                <el-option label="中危" value="medium" />
                <el-option label="低危" value="low" />
              </el-select>
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="CVSS分数">
              <el-input-number v-model="editingRule.cvss" :min="0" :max="10" :step="0.1" style="width: 100%" />
            </el-form-item>
          </el-col>
        </el-row>
        <el-form-item label="修复版本">
          <el-input v-model="editingRule.fixed_version" placeholder="如 4.17.21" />
        </el-form-item>
        <el-form-item label="描述">
          <el-input v-model="editingRule.description" type="textarea" :rows="3" />
        </el-form-item>
        <el-form-item label="参考链接">
          <el-input v-model="editingRule.references" placeholder="https://nvd.nist.gov/..." />
        </el-form-item>
        <el-form-item label="启用">
          <el-switch v-model="editingRule.enabled" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="ruleDialogVisible = false">取消</el-button>
        <el-button type="primary" @click="saveRule">保存</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="sourceDialogVisible" :title="editingSource.id ? '编辑数据源' : '新增数据源'" width="600px">
      <el-form :model="editingSource" label-width="100px">
        <el-form-item label="名称">
          <el-input v-model="editingSource.name" placeholder="如 内部漏洞库" />
        </el-form-item>
        <el-form-item label="类型">
          <el-select v-model="editingSource.type" style="width: 100%">
            <el-option label="HTTP API" value="http" />
          </el-select>
        </el-form-item>
        <el-form-item label="URL">
          <el-input v-model="editingSource.url" placeholder="https://internal-vuln-db.example.com/api/vulns" />
        </el-form-item>
        <el-form-item label="认证类型">
          <el-select v-model="editingSource.auth_type" style="width: 100%">
            <el-option label="无" value="" />
            <el-option label="Bearer Token" value="bearer" />
          </el-select>
        </el-form-item>
        <el-form-item label="Token" v-if="editingSource.auth_type">
          <el-input v-model="editingSource.auth_token" type="password" show-password placeholder="认证Token" />
        </el-form-item>
        <el-form-item label="启用">
          <el-switch v-model="editingSource.enabled" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="sourceDialogVisible = false">取消</el-button>
        <el-button type="primary" @click="saveDataSource">保存</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="importDialogVisible" title="批量导入规则" width="700px">
      <p class="import-hint">请粘贴 JSON 格式的规则数组，支持字段：cve, package_pattern, package_type, max_version, min_version, severity, cvss, title, description, fixed_version, references</p>
      <el-input v-model="importText" type="textarea" :rows="15" placeholder='[{"cve":"CVE-2023-XXXX","package_pattern":"^example$","severity":"high",...}]' />
      <template #footer>
        <el-button @click="importDialogVisible = false">取消</el-button>
        <el-button type="primary" @click="importRules">导入</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { vulnRuleApi, type VulnRule, type VulnDataSource } from '@/api/vulnRule'

const activeTab = ref('rules')
const loading = ref(false)

const rules = ref<VulnRule[]>([])
const page = ref(1)
const pageSize = ref(20)
const total = ref(0)
const filterSource = ref('')
const filterSeverity = ref('')
const filterPkgType = ref('')
const keyword = ref('')

const dataSources = ref<VulnDataSource[]>([])

const ruleDialogVisible = ref(false)
const editingRule = ref<VulnRule>({
  package_pattern: '', package_type: '', max_version: '', min_version: '',
  cve: '', severity: 'medium', cvss: 0, title: '', description: '',
  fixed_version: '', references: '', enabled: true,
})

const sourceDialogVisible = ref(false)
const editingSource = ref<VulnDataSource>({
  name: '', type: 'http', url: '', auth_type: '', auth_token: '', enabled: true, sync_cron: '',
})

const importDialogVisible = ref(false)
const importText = ref('')

function severityLabel(s: string): string {
  const map: Record<string, string> = { critical: '严重', high: '高危', medium: '中危', low: '低危' }
  return map[s] || s
}

function sourceLabel(s: string): string {
  const map: Record<string, string> = { builtin: '内置', custom: '自定义', synced: '同步' }
  return map[s] || s
}

function formatDate(d: string): string {
  if (!d || d === '') return '-'
  const date = new Date(d)
  if (isNaN(date.getTime())) return '-'
  return date.toLocaleString('zh-CN')
}

async function loadRules() {
  loading.value = true
  try {
    const params: Record<string, any> = { page: page.value, page_size: pageSize.value }
    if (filterSource.value) params.source = filterSource.value
    if (filterSeverity.value) params.severity = filterSeverity.value
    if (filterPkgType.value) params.pkg_type = filterPkgType.value
    if (keyword.value) params.keyword = keyword.value

    const res = await vulnRuleApi.listRules(params)
    rules.value = res?.items || []
    total.value = res?.pagination?.total || 0
  } catch {
    console.error('Failed to load rules')
  } finally {
    loading.value = false
  }
}

async function loadDataSources() {
  loading.value = true
  try {
    dataSources.value = await vulnRuleApi.listDataSources() || []
  } catch {
    console.error('Failed to load data sources')
  } finally {
    loading.value = false
  }
}

function handlePageChange(p: number) {
  page.value = p
  loadRules()
}

function showAddDialog() {
  editingRule.value = {
    package_pattern: '', package_type: '', max_version: '', min_version: '',
    cve: '', severity: 'medium', cvss: 0, title: '', description: '',
    fixed_version: '', references: '', enabled: true,
  }
  ruleDialogVisible.value = true
}

function showEditDialog(rule: VulnRule) {
  editingRule.value = { ...rule }
  ruleDialogVisible.value = true
}

async function saveRule() {
  try {
    if (editingRule.value.id) {
      await vulnRuleApi.updateRule(editingRule.value.id, editingRule.value)
      ElMessage.success('规则已更新')
    } else {
      await vulnRuleApi.createRule(editingRule.value)
      ElMessage.success('规则已创建')
    }
    ruleDialogVisible.value = false
    loadRules()
  } catch {
    ElMessage.error('保存失败')
  }
}

async function deleteRule(id: number) {
  try {
    await ElMessageBox.confirm('确定要删除此规则吗？', '确认', { type: 'warning' })
    await vulnRuleApi.deleteRule(id)
    ElMessage.success('规则已删除')
    loadRules()
  } catch {
  }
}

function showImportDialog() {
  importText.value = ''
  importDialogVisible.value = true
}

async function importRules() {
  try {
    const parsed = JSON.parse(importText.value)
    const res = await vulnRuleApi.importRules(parsed)
    ElMessage.success(`成功导入 ${res?.count || 0} 条规则`)
    importDialogVisible.value = false
    loadRules()
  } catch (e: any) {
    ElMessage.error('JSON 解析失败: ' + e.message)
  }
}

function showAddSourceDialog() {
  editingSource.value = { name: '', type: 'http', url: '', auth_type: '', auth_token: '', enabled: true, sync_cron: '' }
  sourceDialogVisible.value = true
}

function showEditSourceDialog(ds: VulnDataSource) {
  editingSource.value = { ...ds }
  sourceDialogVisible.value = true
}

async function saveDataSource() {
  try {
    if (editingSource.value.id) {
      await vulnRuleApi.updateDataSource(editingSource.value.id, editingSource.value)
      ElMessage.success('数据源已更新')
    } else {
      await vulnRuleApi.createDataSource(editingSource.value)
      ElMessage.success('数据源已创建')
    }
    sourceDialogVisible.value = false
    loadDataSources()
  } catch {
    ElMessage.error('保存失败')
  }
}

async function deleteDataSource(id: number) {
  try {
    await ElMessageBox.confirm('确定要删除此数据源吗？', '确认', { type: 'warning' })
    await vulnRuleApi.deleteDataSource(id)
    ElMessage.success('数据源已删除')
    loadDataSources()
  } catch {
  }
}

async function syncDataSource(id: number) {
  try {
    await vulnRuleApi.syncDataSource(id)
    ElMessage.success('同步完成')
    loadDataSources()
  } catch {
    ElMessage.error('同步失败')
  }
}

async function syncAll() {
  try {
    await vulnRuleApi.syncAllDataSources()
    ElMessage.success('全部同步完成')
    loadDataSources()
  } catch {
    ElMessage.error('同步失败')
  }
}

async function testDataSource(ds: VulnDataSource) {
  try {
    await vulnRuleApi.testDataSource(ds)
    ElMessage.success('数据源连接成功')
  } catch {
    ElMessage.error('数据源连接失败')
  }
}

onMounted(() => {
  loadRules()
  loadDataSources()
})
</script>

<style scoped>
.vuln-rule-management {
  padding: 0;
}

.toolbar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 16px;
  flex-wrap: wrap;
  gap: 12px;
}

.toolbar-left {
  display: flex;
  gap: 8px;
  align-items: center;
  flex-wrap: wrap;
}

.toolbar-right {
  display: flex;
  gap: 8px;
}

.filter-select {
  width: 120px;
}

.search-input {
  width: 200px;
}

.pagination {
  margin-top: 16px;
  display: flex;
  justify-content: flex-end;
}

.import-hint {
  font-size: 13px;
  color: #64748b;
  margin-bottom: 12px;
}
</style>
