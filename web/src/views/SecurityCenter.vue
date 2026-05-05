<template>
  <div class="security-center">
    <div class="page-header">
      <h2>安全中心</h2>
      <CustomButton type="primary" @click="triggerFullScan">
        <template #icon>
          <Refresh />
        </template>
        全量扫描
      </CustomButton>
    </div>

    <div class="stat-cards">
      <CustomCard hoverable class="stat-card">
        <div class="stat-value">{{ stats.total_scans || 0 }}</div>
        <div class="stat-label">已扫描包</div>
      </CustomCard>
      <CustomCard hoverable class="stat-card critical">
        <div class="stat-value">{{ stats.critical || 0 }}</div>
        <div class="stat-label">严重漏洞</div>
      </CustomCard>
      <CustomCard hoverable class="stat-card high">
        <div class="stat-value">{{ stats.high || 0 }}</div>
        <div class="stat-label">高危漏洞</div>
      </CustomCard>
      <CustomCard hoverable class="stat-card medium">
        <div class="stat-value">{{ (stats.medium || 0) + (stats.low || 0) }}</div>
        <div class="stat-label">中低危漏洞</div>
      </CustomCard>
    </div>

    <CustomTabs v-model="activeTab" :tabs="tabOptions" />

    <template v-if="activeTab === 'vulnerabilities'">
      <div class="filter-bar">
        <CustomSelect
          v-model="filterSeverity"
          placeholder="严重等级"
          clearable
          style="width: 140px"
          @change="loadVulnerabilities"
          :options="severityOptions"
        />
        <CustomSelect
          v-model="filterPkgType"
          placeholder="包类型"
          clearable
          style="width: 120px"
          @change="loadVulnerabilities"
          :options="pkgTypeOptions"
        />
        <CustomButton type="primary" @click="loadVulnerabilities">搜索</CustomButton>
      </div>

      <CustomTable
        :columns="vulnColumns"
        :data="vulnerabilities"
        :loading="loading"
        row-key="cve_id"
      >
        <template #cve_id="{ row }">
          <a :href="row.references" target="_blank" class="cve-link">{{ row.cve_id }}</a>
        </template>

        <template #severity="{ row }">
          <CustomTag :type="severityTagType(row.severity)" size="small">
            {{ severityLabel(row.severity) }}
          </CustomTag>
        </template>

        <template #cvss_score="{ row }">
          <span :class="`cvss-${row.severity}`">{{ row.cvss_score }}</span>
        </template>

        <template #fixed_version="{ row }">
          <CustomTag v-if="row.fixed_version" type="success" size="small">{{ row.fixed_version }}</CustomTag>
          <span v-else>-</span>
        </template>

        <template #operations="{ row }">
          <CustomButton size="small" type="outline" @click="blockCVE(row.cve_id)">阻断</CustomButton>
        </template>
      </CustomTable>

      <el-pagination
        v-if="total > pageSize"
        :current-page="page"
        :page-size="pageSize"
        :total="total"
        layout="total, prev, pager, next"
        class="pagination"
        @current-change="handlePageChange"
      />
    </template>

    <template v-if="activeTab === 'scanResults'">
      <CustomTable
        :columns="scanColumns"
        :data="scanResults"
        :loading="loading"
        row-key="id"
      >
        <template #scan_status="{ row }">
          <CustomTag :type="scanStatusType(row.scan_status)" size="small">
            {{ scanStatusLabel(row.scan_status) }}
          </CustomTag>
        </template>

        <template #critical_count="{ row }">
          <span v-if="row.critical_count > 0" class="text-danger">{{ row.critical_count }}</span>
          <span v-else>{{ row.critical_count }}</span>
        </template>

        <template #scanned_at="{ row }">
          {{ formatDate(row.scanned_at) }}
        </template>
      </CustomTable>
    </template>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { Refresh } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { securityApi, type SecurityStats, type Vulnerability, type ScanResult } from '@/api/security'
import CustomButton from '@/components/ui/CustomButton.vue'
import CustomSelect from '@/components/ui/CustomSelect.vue'
import CustomTable from '@/components/ui/CustomTable.vue'
import CustomTag from '@/components/ui/CustomTag.vue'
import CustomTabs from '@/components/ui/CustomTabs.vue'
import CustomCard from '@/components/ui/CustomCard.vue'

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

const tabOptions = [
  { name: 'vulnerabilities', label: '漏洞列表' },
  { name: 'scanResults', label: '扫描结果' },
]

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

const vulnColumns = [
  { prop: 'cve_id', label: 'CVE', width: '160px' },
  { prop: 'severity', label: '严重等级', width: '100px' },
  { prop: 'cvss_score', label: 'CVSS', width: '80px' },
  { prop: 'title', label: '漏洞名称' },
  { prop: 'dependency_name', label: '依赖包' },
  { prop: 'current_version', label: '当前版本', width: '100px' },
  { prop: 'fixed_version', label: '修复版本', width: '100px' },
  { prop: 'operations', label: '操作', width: '100px' },
]

const scanColumns = [
  { prop: 'id', label: 'ID', width: '60px' },
  { prop: 'version_id', label: '版本 ID', width: '100px' },
  { prop: 'scan_status', label: '状态', width: '100px' },
  { prop: 'total_vulnerabilities', label: '漏洞数', width: '90px' },
  { prop: 'critical_count', label: '严重', width: '80px' },
  { prop: 'high_count', label: '高危', width: '80px' },
  { prop: 'medium_count', label: '中危', width: '80px' },
  { prop: 'low_count', label: '低危', width: '80px' },
  { prop: 'scanned_at', label: '扫描时间', width: '180px' },
]

function severityLabel(s: string): string {
  const map: Record<string, string> = { critical: '严重', high: '高危', medium: '中危', low: '低危' }
  return map[s] || s
}

function severityTagType(s: string): 'default' | 'primary' | 'success' | 'warning' | 'danger' | 'info' {
  const map: Record<string, 'default' | 'primary' | 'success' | 'warning' | 'danger' | 'info'> = { critical: 'danger', high: 'warning', medium: 'info', low: 'success' }
  return map[s] || 'info'
}

function scanStatusType(s: string): 'default' | 'primary' | 'success' | 'warning' | 'danger' | 'info' {
  const map: Record<string, 'default' | 'primary' | 'success' | 'warning' | 'danger' | 'info'> = { completed: 'success', scanning: 'warning', failed: 'danger', pending: 'info' }
  return map[s] || 'info'
}

function scanStatusLabel(s: string): string {
  const map: Record<string, string> = { completed: '已完成', scanning: '扫描中', failed: '失败', pending: '待扫描' }
  return map[s] || s
}

function formatDate(d: string): string {
  if (!d) return '-'
  return new Date(d).toLocaleString('zh-CN')
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

async function loadScanResults() {
  loading.value = true
  try {
    const res = await securityApi.getDashboard()
    const data = res as any
    scanResults.value = data?.recent_vulnerabilities || []
  } catch {
    console.error('Failed to load scan results')
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
  padding: var(--spacing-xl);
}

.page-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: var(--spacing-2xl);
}

.page-header h2 {
  margin: 0;
  font-size: var(--font-size-xl);
  font-weight: var(--font-weight-semibold);
}

.stat-cards {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: var(--spacing-lg);
  margin-bottom: var(--spacing-2xl);
}

.stat-card {
  text-align: center;
  padding: var(--spacing-lg) var(--spacing-xl);
}

.stat-value {
  font-size: var(--font-size-3xl);
  font-weight: var(--font-weight-bold);
  color: var(--color-text-primary);
}

.stat-label {
  font-size: var(--font-size-sm);
  color: var(--color-text-tertiary);
  margin-top: var(--spacing-xs);
}

.stat-card.critical .stat-value {
  color: var(--color-danger);
}

.stat-card.high .stat-value {
  color: var(--color-warning);
}

.stat-card.medium .stat-value {
  color: var(--color-text-tertiary);
}

.filter-bar {
  display: flex;
  gap: var(--spacing-md);
  margin-bottom: var(--spacing-xl);
}

.pagination {
  margin-top: var(--spacing-xl);
  display: flex;
  justify-content: flex-end;
}

.cvss-critical {
  color: var(--color-danger);
  font-weight: var(--font-weight-bold);
}

.cvss-high {
  color: var(--color-warning);
  font-weight: var(--font-weight-semibold);
}

.text-danger {
  color: var(--color-danger);
  font-weight: var(--font-weight-semibold);
}

.cve-link {
  color: var(--color-primary);
  text-decoration: none;
}

.cve-link:hover {
  text-decoration: underline;
}
</style>
