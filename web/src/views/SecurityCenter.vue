<template>
  <div class="security-center">
    <header class="list-header">
      <div class="header-content">
        <div class="header-icon">
          <i class="fa-solid fa-shield-halved"></i>
        </div>
        <div class="header-text">
          <h2>安全中心</h2>
          <p class="header-subtitle">漏洞扫描与风险管理</p>
        </div>
      </div>
      <el-button type="primary" class="scan-btn" @click="triggerFullScan">
        <el-icon><Refresh /></el-icon>
        <span>全量扫描</span>
      </el-button>
    </header>

    <div class="stats-bar">
      <div class="stat-card stat-card--total">
        <div class="stat-icon">
          <i class="fa-solid fa-box"></i>
        </div>
        <div class="stat-info">
          <span class="stat-value">{{ stats.total_scans || 0 }}</span>
          <span class="stat-label">已扫描包</span>
        </div>
      </div>
      <div class="stat-card stat-card--critical">
        <div class="stat-icon stat-icon--critical">
          <i class="fa-solid fa-circle-exclamation"></i>
        </div>
        <div class="stat-info">
          <span class="stat-value">{{ stats.critical || 0 }}</span>
          <span class="stat-label">严重漏洞</span>
        </div>
      </div>
      <div class="stat-card stat-card--high">
        <div class="stat-icon stat-icon--high">
          <i class="fa-solid fa-triangle-exclamation"></i>
        </div>
        <div class="stat-info">
          <span class="stat-value">{{ stats.high || 0 }}</span>
          <span class="stat-label">高危漏洞</span>
        </div>
      </div>
      <div class="stat-card stat-card--medium">
        <div class="stat-icon stat-icon--medium">
          <i class="fa-solid fa-circle-minus"></i>
        </div>
        <div class="stat-info">
          <span class="stat-value">{{ (stats.medium || 0) + (stats.low || 0) }}</span>
          <span class="stat-label">中低危漏洞</span>
        </div>
      </div>
    </div>

    <div class="content-panel" v-loading="loading">
      <el-tabs v-model="activeTab" class="type-tabs">
        <el-tab-pane label="漏洞列表" name="vulnerabilities">
          <div class="tab-content">
            <div class="filter-bar">
              <el-select
                v-model="filterSeverity"
                placeholder="严重等级"
                clearable
                class="filter-select"
                @change="loadVulnerabilities"
              >
                <el-option v-for="opt in severityOptions" :key="opt.value" :label="opt.label" :value="opt.value" />
              </el-select>
              <el-select
                v-model="filterPkgType"
                placeholder="包类型"
                clearable
                class="filter-select"
                @change="loadVulnerabilities"
              >
                <el-option v-for="opt in pkgTypeOptions" :key="opt.value" :label="opt.label" :value="opt.value" />
              </el-select>
              <el-button type="primary" class="search-btn" @click="loadVulnerabilities">搜索</el-button>
            </div>

            <el-table
              :data="vulnerabilities"
              style="width: 100%"
              :header-cell-style="{ background: '#fafbfc' }"
              :row-class-name="tableRowClass"
              @row-mouse-enter="handleRowEnter"
              @row-mouse-leave="handleRowLeave"
            >
              <el-table-column prop="cve_id" label="CVE" width="160">
                <template #default="{ row }">
                  <el-link type="primary" :href="row.references" target="_blank">{{ row.cve_id }}</el-link>
                </template>
              </el-table-column>
              <el-table-column prop="severity" label="严重等级" width="100" align="center">
                <template #default="{ row }">
                  <el-tag :class="['severity-tag', `severity-tag--${row.severity}`]" size="small">
                    {{ severityLabel(row.severity) }}
                  </el-tag>
                </template>
              </el-table-column>
              <el-table-column prop="cvss_score" label="CVSS" width="80" align="center">
                <template #default="{ row }">
                  <span :class="['cvss-score', `cvss-${row.severity}`]">{{ row.cvss_score }}</span>
                </template>
              </el-table-column>
              <el-table-column prop="title" label="漏洞名称" min-width="200" show-overflow-tooltip />
              <el-table-column prop="dependency_name" label="依赖包" min-width="150" show-overflow-tooltip />
              <el-table-column prop="current_version" label="当前版本" width="110" align="center" />
              <el-table-column prop="fixed_version" label="修复版本" width="110" align="center">
                <template #default="{ row }">
                  <el-tag v-if="row.fixed_version" type="success" size="small">{{ row.fixed_version }}</el-tag>
                  <span v-else class="no-fix">-</span>
                </template>
              </el-table-column>
              <el-table-column label="操作" width="80" align="center">
                <template #default="{ row }">
                  <el-button size="small" class="btn-block" @click="blockCVE(row.cve_id)">阻断</el-button>
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

        <el-tab-pane label="扫描结果" name="scanResults">
          <div class="tab-content">
            <el-table
              :data="scanResults"
              style="width: 100%"
              :header-cell-style="{ background: '#fafbfc' }"
            >
              <el-table-column prop="id" label="ID" width="60" align="center" />
              <el-table-column prop="version_id" label="版本 ID" width="100" align="center" />
              <el-table-column prop="scan_status" label="状态" width="100" align="center">
                <template #default="{ row }">
                  <el-tag :class="['status-tag', `status-tag--${row.scan_status}`]" size="small">
                    {{ scanStatusLabel(row.scan_status) }}
                  </el-tag>
                </template>
              </el-table-column>
              <el-table-column prop="total_vulnerabilities" label="漏洞数" width="90" align="center" />
              <el-table-column label="严重" width="70" align="center">
                <template #default="{ row }">
                  <span v-if="row.critical_count > 0" class="count-critical">{{ row.critical_count }}</span>
                  <span v-else class="count-zero">{{ row.critical_count }}</span>
                </template>
              </el-table-column>
              <el-table-column label="高危" width="70" align="center">
                <template #default="{ row }">
                  <span v-if="row.high_count > 0" class="count-high">{{ row.high_count }}</span>
                  <span v-else class="count-zero">{{ row.high_count }}</span>
                </template>
              </el-table-column>
              <el-table-column label="中危" width="70" align="center">
                <template #default="{ row }">
                  <span class="count-medium">{{ row.medium_count }}</span>
                </template>
              </el-table-column>
              <el-table-column label="低危" width="70" align="center">
                <template #default="{ row }">
                  <span class="count-low">{{ row.low_count }}</span>
                </template>
              </el-table-column>
              <el-table-column prop="scanned_at" label="扫描时间" width="170" align="center">
                <template #default="{ row }">
                  {{ formatDate(row.scanned_at) }}
                </template>
              </el-table-column>
            </el-table>
          </div>
        </el-tab-pane>
      </el-tabs>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { Refresh } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { securityApi, type SecurityStats, type Vulnerability, type ScanResult } from '@/api/security'

const activeTab = ref('vulnerabilities')
const loading = ref(false)
const stats = ref<SecurityStats>({} as SecurityStats)
const vulnerabilities = ref<Vulnerability[]>([])
const scanResults = ref<ScanResult[]>([])
const page = ref(1)
const pageSize = ref(20)
const total = ref(0)
const filterSeverity = ref('')
const filterPkgType = ref('')
const hoveredRow = ref<number | null>(null)

const severityOptions = [
  { label: '严重', value: 'critical' },
  { label: '高危', value: 'high' },
  { label: '中危', value: 'medium' },
  { label: '低危', value: 'low' },
]

const pkgTypeOptions = [
  { label: 'npm', value: 'npm' },
  { label: 'maven', value: 'maven' },
  { label: 'pypi', value: 'pypi' },
  { label: 'go', value: 'go' },
  { label: 'nuget', value: 'nuget' },
]

function severityLabel(s: string): string {
  const map: Record<string, string> = { critical: '严重', high: '高危', medium: '中危', low: '低危' }
  return map[s] || s
}

function scanStatusLabel(s: string): string {
  const map: Record<string, string> = { completed: '已完成', scanning: '扫描中', failed: '失败', pending: '待扫描' }
  return map[s] || s
}

function formatDate(d: string): string {
  if (!d) return '-'
  return new Date(d).toLocaleString('zh-CN')
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

async function loadStats() {
  try {
    const res = await securityApi.getStatistics()
    stats.value = res || ({} as SecurityStats)
  } catch {
    console.error('Failed to load security stats')
  }
}

async function loadVulnerabilities() {
  loading.value = true
  try {
    const params: Record<string, any> = { page: page.value, page_size: pageSize.value }
    if (filterSeverity.value) params.severity = filterSeverity.value
    if (filterPkgType.value) params.pkg_type = filterPkgType.value

    const res = await securityApi.listVulnerabilities(params)
    vulnerabilities.value = res?.items || []
    total.value = res?.pagination?.total || 0
  } catch {
    console.error('Failed to load vulnerabilities')
  } finally {
    loading.value = false
  }
}

function handlePageChange(p: number) {
  page.value = p
  loadVulnerabilities()
}

async function triggerFullScan() {
  try {
    await ElMessageBox.confirm('确定要触发全量扫描吗？这可能需要较长时间。', '确认', {
      type: 'warning',
    })
    await securityApi.triggerFullScan()
    ElMessage.success('全量扫描已触发')
    loadStats()
  } catch (e: any) {
    if (e !== 'cancel') {
      ElMessage.error('触发扫描失败')
    }
  }
}

async function blockCVE(cve: string) {
  try {
    await ElMessageBox.confirm(`确定要阻断 ${cve} 相关的包吗？`, '确认', { type: 'warning' })
    await securityApi.blockByCVE(cve)
    ElMessage.success('阻断规则已创建')
  } catch (e: any) {
    if (e !== 'cancel') {
      ElMessage.error('创建阻断规则失败')
    }
  }
}

onMounted(() => {
  loadStats()
  loadVulnerabilities()
})
</script>

<style scoped>
.security-center {
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
  background: linear-gradient(135deg, #fef3c7 0%, #fde68a 100%);
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 24px;
  color: #d97706;
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

.scan-btn {
  border-radius: 10px;
  padding: 10px 20px;
  font-weight: 500;
  background: linear-gradient(135deg, #6366f1 0%, #4f46e5 100%);
  border: none;
  box-shadow: 0 4px 12px rgba(99, 102, 241, 0.3);
  display: flex;
  align-items: center;
  gap: 8px;
  transition: all 0.2s ease;
}

.scan-btn:hover {
  transform: translateY(-1px);
  box-shadow: 0 6px 16px rgba(99, 102, 241, 0.4);
}

.stats-bar {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 16px;
  margin-bottom: 16px;
}

.stat-card {
  background: #fff;
  border-radius: 16px;
  padding: 20px;
  display: flex;
  align-items: center;
  gap: 16px;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.04);
  transition: all 0.2s ease;
}

.stat-card:hover {
  transform: translateY(-2px);
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.08);
}

.stat-icon {
  width: 48px;
  height: 48px;
  border-radius: 12px;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 20px;
}

.stat-icon i {
  font-size: 20px;
}

.stat-card--total .stat-icon {
  background: linear-gradient(135deg, #e0e7ff 0%, #c7d2fe 100%);
  color: #6366f1;
}

.stat-card--critical .stat-icon {
  background: linear-gradient(135deg, #fee2e2 0%, #fecaca 100%);
  color: #dc2626;
}

.stat-card--high .stat-icon {
  background: linear-gradient(135deg, #fef3c7 0%, #fde68a 100%);
  color: #d97706;
}

.stat-card--medium .stat-icon {
  background: linear-gradient(135deg, #fef9c3 0%, #fef08a 100%);
  color: #ca8a04;
}

.stat-info {
  display: flex;
  flex-direction: column;
}

.stat-value {
  font-size: 28px;
  font-weight: 700;
  color: #1e293b;
  line-height: 1.2;
}

.stat-label {
  font-size: 13px;
  color: #94a3b8;
  margin-top: 4px;
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

.filter-select {
  width: 140px;
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

.severity-tag {
  font-size: 11px;
  padding: 4px 10px;
  border-radius: 6px;
  font-weight: 500;
  border: none;
}

.severity-tag--critical {
  background: linear-gradient(135deg, #fee2e2 0%, #fecaca 100%);
  color: #dc2626;
}

.severity-tag--high {
  background: linear-gradient(135deg, #fef3c7 0%, #fde68a 100%);
  color: #d97706;
}

.severity-tag--medium {
  background: linear-gradient(135deg, #fef9c3 0%, #fef08a 100%);
  color: #ca8a04;
}

.severity-tag--low {
  background: linear-gradient(135deg, #dcfce7 0%, #bbf7d0 100%);
  color: #16a34a;
}

.cvss-score {
  font-weight: 600;
  font-size: 13px;
}

.cvss-critical {
  color: #dc2626;
}

.cvss-high {
  color: #d97706;
}

.cvss-medium {
  color: #ca8a04;
}

.cvss-low {
  color: #16a34a;
}

.no-fix {
  color: #cbd5e1;
}

.btn-block {
  border-radius: 8px;
  padding: 6px 12px;
  font-size: 12px;
  font-weight: 500;
  color: #64748b;
  border: 1px solid #e2e8f0;
  background: #f8fafc;
  transition: all 0.2s ease;
}

.btn-block:hover {
  background: #fef2f2;
  border-color: #fecaca;
  color: #dc2626;
}

.status-tag {
  font-size: 11px;
  padding: 4px 10px;
  border-radius: 6px;
  font-weight: 500;
  border: none;
}

.status-tag--completed {
  background: linear-gradient(135deg, #dcfce7 0%, #bbf7d0 100%);
  color: #059669;
}

.status-tag--scanning {
  background: linear-gradient(135deg, #fef3c7 0%, #fde68a 100%);
  color: #d97706;
}

.status-tag--failed {
  background: linear-gradient(135deg, #fee2e2 0%, #fecaca 100%);
  color: #dc2626;
}

.status-tag--pending {
  background: linear-gradient(135deg, #f1f5f9 0%, #e2e8f0 100%);
  color: #64748b;
}

.count-critical {
  color: #dc2626;
  font-weight: 600;
}

.count-high {
  color: #d97706;
  font-weight: 600;
}

.count-medium,
.count-low {
  color: #64748b;
}

.count-zero {
  color: #cbd5e1;
}

.pagination {
  margin-top: 20px;
  display: flex;
  justify-content: flex-end;
}

:deep(.el-pagination) {
  font-weight: 500;
}

:deep(.el-pagination button) {
  border-radius: 8px;
}

:deep(.el-pager li) {
  border-radius: 8px;
  margin: 0 2px;
}

:deep(.el-pager li.is-active) {
  background: linear-gradient(135deg, #6366f1 0%, #4f46e5 100%);
}
</style>
