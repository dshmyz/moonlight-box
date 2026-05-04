<template>
  <div class="security-center">
    <div class="page-header">
      <h2>安全中心</h2>
      <CustomButton type="primary" @click="triggerFullScan">
        <el-icon><Refresh /></el-icon> 全量扫描
      </CustomButton>
    </div>

    <el-row :gutter="16" class="stat-cards">
      <el-col :span="6">
        <el-card shadow="hover" class="stat-card">
          <div class="stat-value">{{ stats.total_scans || 0 }}</div>
          <div class="stat-label">已扫描包</div>
        </el-card>
      </el-col>
      <el-col :span="6">
        <el-card shadow="hover" class="stat-card critical">
          <div class="stat-value">{{ stats.critical || 0 }}</div>
          <div class="stat-label">严重漏洞</div>
        </el-card>
      </el-col>
      <el-col :span="6">
        <el-card shadow="hover" class="stat-card high">
          <div class="stat-value">{{ stats.high || 0 }}</div>
          <div class="stat-label">高危漏洞</div>
        </el-card>
      </el-col>
      <el-col :span="6">
        <el-card shadow="hover" class="stat-card medium">
          <div class="stat-value">{{ (stats.medium || 0) + (stats.low || 0) }}</div>
          <div class="stat-label">中低危漏洞</div>
        </el-card>
      </el-col>
    </el-row>

    <el-tabs v-model="activeTab">
      <el-tab-pane label="漏洞列表" name="vulnerabilities">
        <div class="filter-bar">
          <CustomSelect 
            v-model="filterSeverity" 
            placeholder="严重等级" 
            clearable 
            style="width: 140px" 
            @change="loadVulnerabilities"
            :options="[
              { label: '严重', value: 'critical' },
              { label: '高危', value: 'high' },
              { label: '中危', value: 'medium' },
              { label: '低危', value: 'low' }
            ]"
          />
          <CustomSelect 
            v-model="filterPkgType" 
            placeholder="包类型" 
            clearable 
            style="width: 120px" 
            @change="loadVulnerabilities"
            :options="[
              { label: 'npm', value: 'npm' },
              { label: 'maven', value: 'maven' },
              { label: 'pypi', value: 'pypi' },
              { label: 'go', value: 'go' },
              { label: 'nuget', value: 'nuget' }
            ]"
          />
          <CustomButton type="primary" @click="loadVulnerabilities">搜索</CustomButton>
        </div>

        <el-table :data="vulnerabilities" v-loading="loading" style="width: 100%">
          <el-table-column prop="cve_id" label="CVE" width="160">
            <template #default="{ row }">
              <el-link type="primary" :href="row.references" target="_blank">{{ row.cve_id }}</el-link>
            </template>
          </el-table-column>
          <el-table-column prop="severity" label="严重等级" width="100">
            <template #default="{ row }">
              <el-tag :type="severityTagType(row.severity)" size="small">
                {{ severityLabel(row.severity) }}
              </el-tag>
            </template>
          </el-table-column>
          <el-table-column prop="cvss_score" label="CVSS" width="80">
            <template #default="{ row }">
              <span :class="`cvss-${row.severity}`">{{ row.cvss_score }}</span>
            </template>
          </el-table-column>
          <el-table-column prop="title" label="漏洞名称" min-width="220" show-overflow-tooltip />
          <el-table-column prop="dependency_name" label="依赖包" min-width="150" />
          <el-table-column prop="current_version" label="当前版本" width="100" />
          <el-table-column prop="fixed_version" label="修复版本" width="100">
            <template #default="{ row }">
              <el-tag v-if="row.fixed_version" type="success" size="small">{{ row.fixed_version }}</el-tag>
              <span v-else>-</span>
            </template>
          </el-table-column>
          <el-table-column label="操作" width="100" fixed="right">
            <template #default="{ row }">
              <el-button size="small" type="danger" @click="blockCVE(row.cve_id)">阻断</el-button>
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
      </el-tab-pane>

      <el-tab-pane label="扫描结果" name="scanResults">
        <el-table :data="scanResults" v-loading="loading" style="width: 100%">
          <el-table-column prop="id" label="ID" width="60" />
          <el-table-column prop="version_id" label="版本 ID" width="100" />
          <el-table-column prop="scan_status" label="状态" width="100">
            <template #default="{ row }">
              <el-tag :type="scanStatusType(row.scan_status)" size="small">
                {{ scanStatusLabel(row.scan_status) }}
              </el-tag>
            </template>
          </el-table-column>
          <el-table-column prop="total_vulnerabilities" label="漏洞数" width="90" />
          <el-table-column prop="critical_count" label="严重" width="80">
            <template #default="{ row }">
              <span v-if="row.critical_count > 0" class="text-danger">{{ row.critical_count }}</span>
              <span v-else>{{ row.critical_count }}</span>
            </template>
          </el-table-column>
          <el-table-column prop="high_count" label="高危" width="80" />
          <el-table-column prop="medium_count" label="中危" width="80" />
          <el-table-column prop="low_count" label="低危" width="80" />
          <el-table-column prop="scanned_at" label="扫描时间" width="180">
            <template #default="{ row }">
              {{ formatDate(row.scanned_at) }}
            </template>
          </el-table-column>
        </el-table>
      </el-tab-pane>
    </el-tabs>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { Refresh } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { securityApi, type SecurityStats, type Vulnerability, type ScanResult } from '@/api/security'
import CustomButton from '@/components/ui/CustomButton.vue'
import CustomSelect from '@/components/ui/CustomSelect.vue'

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

function severityLabel(s: string): string {
  const map: Record<string, string> = { critical: '严重', high: '高危', medium: '中危', low: '低危' }
  return map[s] || s
}

function severityTagType(s: string): string {
  const map: Record<string, string> = { critical: 'danger', high: 'warning', medium: 'info', low: 'success' }
  return map[s] || 'info'
}

function scanStatusType(s: string): string {
  const map: Record<string, string> = { completed: 'success', scanning: 'warning', failed: 'danger', pending: 'info' }
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
  padding: 20px;
}

.page-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 24px;
}

.page-header h2 {
  margin: 0;
  font-size: 22px;
  font-weight: 600;
}

.stat-cards {
  margin-bottom: 24px;
}

.stat-card {
  text-align: center;
  padding: 8px 0;
}

.stat-value {
  font-size: 32px;
  font-weight: 700;
  color: #303133;
}

.stat-label {
  font-size: 14px;
  color: #909399;
  margin-top: 4px;
}

.stat-card.critical .stat-value {
  color: #f56c6c;
}

.stat-card.high .stat-value {
  color: #e6a23c;
}

.stat-card.medium .stat-value {
  color: #909399;
}

.filter-bar {
  display: flex;
  gap: 12px;
  margin-bottom: 16px;
}

.pagination {
  margin-top: 16px;
  display: flex;
  justify-content: flex-end;
}

.cvss-critical {
  color: #f56c6c;
  font-weight: 700;
}

.cvss-high {
  color: #e6a23c;
  font-weight: 600;
}

.text-danger {
  color: #f56c6c;
  font-weight: 600;
}
</style>
