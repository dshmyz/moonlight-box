<template>
  <div class="migration-wizard">
    <header class="page-header">
      <div class="header-content">
        <div class="header-icon"><i class="fa-solid fa-arrow-right-arrow-left"></i></div>
        <div class="header-text">
          <h2>数据迁移 V2</h2>
          <p class="header-subtitle">从 Nexus 迁移仓库配置、安全策略和制品数据</p>
        </div>
      </div>
    </header>

    <div class="wizard-container">
      <el-steps :active="step" finish-status="success" class="step-bar">
        <el-step title="连接源" />
        <el-step title="选择范围" />
        <el-step title="扫描" />
        <el-step title="处理冲突" />
        <el-step title="执行" />
        <el-step title="结果" />
      </el-steps>

      <div class="step-content">
        <!-- Step 0: Connect Source -->
        <div v-if="step === 0" class="step-panel">
          <el-form :model="connForm" label-width="100px">
            <el-form-item label="源类型"><el-tag>Nexus</el-tag></el-form-item>
            <el-form-item label="URL"><el-input v-model="connForm.url" placeholder="https://nexus.example.com" /></el-form-item>
            <el-form-item label="用户名"><el-input v-model="connForm.username" /></el-form-item>
            <el-form-item label="密码"><el-input v-model="connForm.password" type="password" show-password /></el-form-item>
            <el-form-item>
              <el-button type="primary" @click="testConnection" :loading="testing">测试连接</el-button>
              <el-tag v-if="connectionSuccess" type="success">✓ 连接成功</el-tag>
            </el-form-item>
            <el-form-item>
              <el-button type="primary" @click="nextStep" :disabled="!connectionSuccess">下一步</el-button>
            </el-form-item>
          </el-form>
        </div>

        <!-- Step 1: Select Scope -->
        <div v-if="step === 1" class="step-panel">
          <h3>选择迁移范围</h3>
          <el-checkbox v-model="scope.repo_config" @change="onRepoConfigChange">仓库配置</el-checkbox>
          <div v-if="scope.repo_config" class="sub-options">
            <el-checkbox v-model="scope.hosted_repos">hosted/local 仓库</el-checkbox>
            <el-checkbox v-model="scope.proxy_repos">proxy 仓库</el-checkbox>
            <el-checkbox v-model="scope.group_repos">group/virtual 仓库</el-checkbox>
            <el-checkbox v-model="scope.group_memberships">group 成员关系</el-checkbox>
          </div>
          <el-divider />
          <el-checkbox v-model="scope.roles">角色</el-checkbox>
          <el-checkbox v-model="scope.users" style="margin-left:16px">用户</el-checkbox>
          <el-divider />
          <el-checkbox v-model="scope.artifacts" @change="onArtifactsChange">制品数据</el-checkbox>
          <div v-if="scope.artifacts" class="sub-options">
            <el-form-item label="目标仓库">
              <el-select v-model="scope.target_strategy" placeholder="选择目标策略">
                <el-option label="保持源仓库结构" value="keep_structure" />
                <el-option label="映射到指定仓库" value="map_target" />
              </el-select>
            </el-form-item>
            <el-form-item v-if="scope.target_strategy === 'map_target'" label="目标仓库名">
              <el-input v-model="scope.target_repo_name" placeholder="输入目标仓库名" />
            </el-form-item>
          </div>
          <div class="actions">
            <el-button @click="prevStep">上一步</el-button>
            <el-button type="primary" @click="createAndScan">创建计划并扫描</el-button>
          </div>
        </div>

        <!-- Step 2: Scan Progress -->
        <div v-if="step === 2" class="step-panel">
          <h3>{{ isScanning ? '正在扫描源数据...' : (scanFailed ? '扫描失败' : '扫描完成') }}</h3>

          <!-- scan in progress / summary -->
          <div v-if="currentPlan?.stats" class="stats-grid">
            <div class="stat-card" v-if="currentPlan.stats.total_repos > 0">
              <div class="stat-icon repos"><i class="fa-solid fa-database"></i></div>
              <div class="stat-num">{{ currentPlan.stats.total_repos }}</div>
              <div class="stat-label">仓库配置</div>
            </div>
            <div class="stat-card" v-if="currentPlan.stats.total_roles > 0">
              <div class="stat-icon roles"><i class="fa-solid fa-shield-halved"></i></div>
              <div class="stat-num">{{ currentPlan.stats.total_roles }}</div>
              <div class="stat-label">角色</div>
            </div>
            <div class="stat-card" v-if="currentPlan.stats.total_users > 0">
              <div class="stat-icon users"><i class="fa-solid fa-user-group"></i></div>
              <div class="stat-num">{{ currentPlan.stats.total_users }}</div>
              <div class="stat-label">用户</div>
            </div>
            <div class="stat-card" v-if="currentPlan.stats.total_artifacts > 0">
              <div class="stat-icon artifacts"><i class="fa-solid fa-box-archive"></i></div>
              <div class="stat-num">{{ currentPlan.stats.total_artifacts }}</div>
              <div class="stat-label">制品</div>
            </div>
          </div>

          <!-- scan events -->
          <div v-if="events.length" class="event-log">
            <div v-for="e in recentEvents" :key="e.id" class="event-line" :class="'event-' + e.level">
              <span class="event-time">{{ formatTime(e.created_at) }}</span>
              <span class="event-msg">{{ e.message }}</span>
            </div>
          </div>

          <el-alert v-if="scanFailed" type="error" title="扫描失败" :description="scanError" show-icon closable style="margin-top:16px" />

          <div class="actions">
            <el-button @click="prevStep" :disabled="isScanning">上一步</el-button>
            <el-button v-if="!isScanning && !scanFailed" type="primary" @click="runPrecheck">执行预检</el-button>
            <el-button v-if="scanFailed" type="primary" @click="retryScan" :loading="isScanning">重新扫描</el-button>
          </div>
        </div>

        <!-- Step 3: Conflicts -->
        <div v-if="step === 3" class="step-panel">
          <h3>冲突处理</h3>
          <p v-if="conflicts.length === 0">没有发现冲突</p>
          <div v-for="c in conflicts" :key="c.id" class="conflict-item">
            <el-alert :type="c.severity === 'blocking' ? 'error' : 'warning'" :title="c.kind" :description="c.message" show-icon />
            <el-radio-group v-model="conflictPolicies[c.id]" style="margin-top:8px">
              <el-radio :value="c.suggested_policy">{{ c.suggested_policy }} (推荐)</el-radio>
              <el-radio value="skip">skip</el-radio>
              <el-radio value="rename">rename</el-radio>
              <el-radio value="map_existing">map_existing</el-radio>
            </el-radio-group>
          </div>
          <div class="actions">
            <el-button @click="prevStep">上一步</el-button>
            <el-button type="primary" @click="applyConflicts">应用策略并开始执行</el-button>
          </div>
        </div>

        <!-- Step 4: Execution -->
        <div v-if="step === 4" class="step-panel">
          <h3>{{ execTitle }}</h3>

          <!-- overall progress -->
          <div class="overall-progress">
            <div class="progress-header">
              <span>总体进度</span>
              <span class="progress-text">{{ overallProgress }}% ({{ totalCompleted }}/{{ totalJobs }})</span>
            </div>
            <el-progress :percentage="overallProgress" :status="execProgressStatus" :stroke-width="12" />
          </div>

          <!-- category progress bars -->
          <div class="category-progress" v-if="jobCategories.length">
            <div v-for="cat in jobCategories" :key="cat.key" class="cat-row">
              <div class="cat-header">
                <span class="cat-name">{{ cat.label }}</span>
                <span class="cat-count">{{ cat.completed }}/{{ cat.total }}</span>
              </div>
              <el-progress :percentage="cat.pct" :status="cat.pct === 100 ? 'success' : ''" :stroke-width="8"
                :color="cat.failed > 0 ? '#f56c6c' : '#409eff'" />
            </div>
          </div>

          <!-- recent events -->
          <div v-if="recentEvents.length" class="event-log">
            <div v-for="e in recentEvents" :key="e.id" class="event-line" :class="'event-' + e.level">
              <span class="event-time">{{ formatTime(e.created_at) }}</span>
              <el-tag v-if="e.level === 'error'" type="danger" size="small">错误</el-tag>
              <el-tag v-else-if="e.level === 'warn'" type="warning" size="small">警告</el-tag>
              <span class="event-msg">{{ e.message }}</span>
            </div>
          </div>

          <div class="actions">
            <el-button v-if="currentPlan?.status === 'paused'" type="primary" @click="resumePlan">继续执行</el-button>
            <el-button v-else @click="pausePlan" :disabled="!isRunning">暂停</el-button>
            <el-button @click="cancelPlan" :disabled="!isRunning && currentPlan?.status !== 'paused'">取消</el-button>
          </div>
        </div>

        <!-- Step 5: Result -->
        <div v-if="step === 5" class="step-panel">
          <h3>迁移结果</h3>
          <div class="result-summary">
            <el-tag :type="resultTagType" size="large">{{ planStatusLabel(currentPlan?.status || '') }}</el-tag>
          </div>

          <div v-if="currentPlan?.stats" class="stats-grid">
            <div class="stat-card">
              <div class="stat-icon repos"><i class="fa-solid fa-database"></i></div>
              <div class="stat-num">{{ currentPlan.stats.synced_repos }}/{{ currentPlan.stats.total_repos }}</div>
              <div class="stat-label">仓库配置</div>
            </div>
            <div class="stat-card" v-if="currentPlan.stats.total_roles > 0">
              <div class="stat-icon roles"><i class="fa-solid fa-shield-halved"></i></div>
              <div class="stat-num">{{ currentPlan.stats.synced_roles }}/{{ currentPlan.stats.total_roles }}</div>
              <div class="stat-label">角色</div>
            </div>
            <div class="stat-card" v-if="currentPlan.stats.total_users > 0">
              <div class="stat-icon users"><i class="fa-solid fa-user-group"></i></div>
              <div class="stat-num">{{ currentPlan.stats.synced_users }}/{{ currentPlan.stats.total_users }}</div>
              <div class="stat-label">用户</div>
            </div>
            <div class="stat-card" v-if="currentPlan.stats.total_artifacts > 0">
              <div class="stat-icon artifacts"><i class="fa-solid fa-box-archive"></i></div>
              <div class="stat-num">{{ currentPlan.stats.synced_artifacts }}/{{ currentPlan.stats.total_artifacts }}</div>
              <div class="stat-label">制品</div>
            </div>
          </div>

          <div class="actions">
            <el-button @click="prevStep">上一步</el-button>
            <el-button type="primary" @click="resetWizard">重新开始</el-button>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { ElMessage } from 'element-plus'
import { migrationV2Api, type MigrationPlan, type MigrationJob, type MigrationConflict, type MigrationEvent, type ConflictPolicy, type ScopeSelection, type JobKind } from '@/api/migrationV2'

const step = ref(0)
const testing = ref(false)
const scanError = ref('')
const pollTimer = ref<number | null>(null)
const scanPollTimer = ref<number | null>(null)
const connectionSuccess = ref(false)

const connForm = ref({ url: '', username: 'admin', password: '' })
const currentPlan = ref<MigrationPlan | null>(null)
const jobs = ref<MigrationJob[]>([])
const conflicts = ref<MigrationConflict[]>([])
const events = ref<MigrationEvent[]>([])
const conflictPolicies = ref<Record<number, ConflictPolicy>>({})

const scope = ref<ScopeSelection>({
  repo_config: true, hosted_repos: true, proxy_repos: true, group_repos: true,
  group_memberships: false, privileges: false, roles: true, users: true,
  user_roles: false, artifacts: false, artifact_repos: [],
  target_strategy: 'keep_structure', target_repo_id: 0, target_repo_name: '',
})

// --- computed ---

const isScanning = computed(() => currentPlan.value?.status === 'scanning')
const scanFailed = computed(() => currentPlan.value?.status === 'failed' && currentPlan.value?.current_stage === 'scan')
const isRunning = computed(() => currentPlan.value?.status === 'running' || currentPlan.value?.status === 'verifying')

const execTitle = computed(() => {
  const s = currentPlan.value?.status
  if (s === 'running') return '正在执行迁移...'
  if (s === 'verifying') return '正在验证迁移结果...'
  if (s === 'paused') return '迁移已暂停'
  return '执行中'
})

const totalJobs = computed(() => {
  const kinds = ['repo_config', 'role', 'user', 'artifact_copy'] as JobKind[]
  return jobs.value.filter(j => kinds.includes(j.kind)).length
})

const totalCompleted = computed(() =>
  jobs.value.filter(j => j.status === 'completed' && ['repo_config', 'role', 'user', 'artifact_copy'].includes(j.kind)).length
)

const overallProgress = computed(() => {
  if (totalJobs.value === 0) return 0
  return Math.round((totalCompleted.value / totalJobs.value) * 100)
})

const execProgressStatus = computed(() => {
  if (currentPlan.value?.status === 'completed') return 'success'
  if (currentPlan.value?.status === 'failed') return 'exception'
  return ''
})

const jobCategories = computed(() => {
  const cats: { key: JobKind; label: string }[] = [
    { key: 'repo_config', label: '仓库配置' },
    { key: 'role', label: '角色' },
    { key: 'user', label: '用户' },
    { key: 'artifact_copy', label: '制品复制' },
  ]
  return cats.map(cat => {
    const list = jobs.value.filter(j => j.kind === cat.key)
    const total = list.length
    const completed = list.filter(j => j.status === 'completed').length
    const failed = list.filter(j => j.status === 'failed').length
    const skipped = list.filter(j => j.status === 'skipped').length
    const done = completed + skipped
    return {
      key: cat.key,
      label: cat.label,
      total,
      completed,
      skipped,
      failed,
      done,
      pct: total === 0 ? 0 : Math.round((done / total) * 100),
    }
  }).filter(c => c.total > 0)
})

const resultTagType = computed(() => {
  const s = currentPlan.value?.status
  if (s === 'completed') return 'success'
  if (s === 'failed') return 'danger'
  if (s === 'cancelled') return 'info'
  return ''
})

const recentEvents = computed(() => events.value.slice(0, 20))

// --- helpers ---

function planStatusLabel(s: string): string {
  const m: Record<string, string> = {
    draft: '草稿',
    scanning: '扫描中',
    prechecking: '预检中',
    precheck_failed: '预检失败',
    ready: '就绪',
    running: '执行中',
    paused: '已暂停',
    verifying: '验证中',
    completed: '已完成',
    failed: '失败',
    cancelling: '取消中',
    cancelled: '已取消',
  }
  return m[s] || s
}

function formatTime(t: string): string {
  if (!t) return ''
  const d = new Date(t)
  return d.toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit', second: '2-digit' })
}

// --- actions ---

async function testConnection() {
  testing.value = true
  try {
    await migrationV2Api.testSource({ source_type: 'nexus', ...connForm.value })
    ElMessage.success('连接成功')
    connectionSuccess.value = true
  } catch (e: any) {
    ElMessage.error('连接失败: ' + e.message)
    connectionSuccess.value = false
  }
  finally { testing.value = false }
}

function nextStep() { if (step.value < 5) step.value++ }
function prevStep() {
  if (step.value > 0) step.value--
}

function resetWizard() {
  step.value = 0
  connectionSuccess.value = false
  currentPlan.value = null
  jobs.value = []
  conflicts.value = []
  events.value = []
  conflictPolicies.value = {}
  connForm.value = { url: '', username: 'admin', password: '' }
  stopAllPolling()
}

function onRepoConfigChange(v: boolean) { if (!v) { scope.value.hosted_repos = scope.value.proxy_repos = scope.value.group_repos = scope.value.group_memberships = false } }
function onArtifactsChange(v: boolean) { if (!v) scope.value.artifact_repos = [] }

// --- scan ---

async function createAndScan() {
  try {
    const plan = await migrationV2Api.createPlan({
      name: connForm.value.url,
      source_url: connForm.value.url,
      username: connForm.value.username,
      password: connForm.value.password,
      scope: scope.value,
    }) as MigrationPlan
    currentPlan.value = plan
    step.value = 2
    scanError.value = ''
    await migrationV2Api.scanPlan(currentPlan.value.id)
    startScanPolling()
  } catch (e: any) { ElMessage.error('创建失败: ' + e.message) }
}

async function retryScan() {
  if (!currentPlan.value) return
  scanError.value = ''
  try {
    await migrationV2Api.scanPlan(currentPlan.value.id)
    events.value = []
    startScanPolling()
  } catch (e: any) { ElMessage.error('扫描失败: ' + e.message) }
}

function startScanPolling() {
  stopScanPolling()
  scanPollTimer.value = window.setInterval(async () => {
    if (!currentPlan.value) return
    await refreshPlan()
    try { events.value = (await migrationV2Api.getEvents(currentPlan.value.id, 50) as MigrationEvent[]).reverse() }
    catch { /* ignore */ }
    try { jobs.value = (await migrationV2Api.getJobs(currentPlan.value.id) as MigrationJob[]) || [] }
    catch { /* ignore */ }

    const s = currentPlan.value?.status
    if (s === 'prechecking' || s === 'failed') {
      stopScanPolling()
      if (s === 'failed') {
        scanError.value = '扫描失败，请查看上方事件日志了解详情'
      }
    }
  }, 1500)
}

function stopScanPolling() {
  if (scanPollTimer.value) { clearInterval(scanPollTimer.value); scanPollTimer.value = null }
}

// --- precheck ---

async function runPrecheck() {
  if (!currentPlan.value) return
  try {
    await migrationV2Api.precheckPlan(currentPlan.value.id)
    const res: MigrationConflict[] = await migrationV2Api.getConflicts(currentPlan.value.id) as any
    conflicts.value = res || []
    conflicts.value.forEach(c => { conflictPolicies.value[c.id] = c.suggested_policy })
    if (conflicts.value.length === 0) {
      step.value = 4
      await migrationV2Api.startPlan(currentPlan.value.id)
      startExecPolling()
    } else {
      step.value = 3
    }
  } catch (e: any) { ElMessage.error('预检失败: ' + e.message) }
}

async function applyConflicts() {
  if (!currentPlan.value) return
  try {
    if (conflicts.value.length > 0) {
      const resolutions = Object.entries(conflictPolicies.value).map(([id, policy]) => ({
        conflict_id: Number(id), policy,
      }))
      await migrationV2Api.applyConflicts(currentPlan.value.id, resolutions)
    }
    await migrationV2Api.startPlan(currentPlan.value.id)
    step.value = 4
    startExecPolling()
  } catch (e: any) { ElMessage.error('执行失败: ' + e.message) }
}

// --- execution ---

async function pausePlan() {
  if (!currentPlan.value) return
  await migrationV2Api.pausePlan(currentPlan.value.id)
  await refreshPlan()
}

async function resumePlan() {
  if (!currentPlan.value) return
  try {
    await migrationV2Api.resumePlan(currentPlan.value.id)
    startExecPolling()
  } catch (e: any) { ElMessage.error('恢复失败: ' + e.message) }
}

async function cancelPlan() {
  if (!currentPlan.value) return
  await migrationV2Api.cancelPlan(currentPlan.value.id)
  stopExecPolling()
  await refreshPlan()
  step.value = 5
}

async function refreshPlan() {
  if (!currentPlan.value) return
  try { currentPlan.value = (await migrationV2Api.getPlan(currentPlan.value.id)) as any }
  catch { /* ignore */ }
}

function startExecPolling() {
  stopExecPolling()
  pollTimer.value = window.setInterval(async () => {
    if (!currentPlan.value) return
    await refreshPlan()
    try { jobs.value = (await migrationV2Api.getJobs(currentPlan.value.id)) as any }
    catch { /* ignore */ }
    try { events.value = (await migrationV2Api.getEvents(currentPlan.value.id, 30) as MigrationEvent[]).reverse() }
    catch { /* ignore */ }
    const s = currentPlan.value?.status
    if (s === 'completed' || s === 'failed' || s === 'cancelled') {
      stopExecPolling()
      step.value = 5
    }
  }, 3000)
}

function stopExecPolling() {
  if (pollTimer.value) { clearInterval(pollTimer.value); pollTimer.value = null }
}

function stopAllPolling() {
  stopScanPolling()
  stopExecPolling()
}

// --- lifecycle ---

async function loadRecentPlan() {
  try {
    const res = await migrationV2Api.listPlans()
    if (!res || !Array.isArray(res) || res.length === 0) return

    const recentPlan = res[0]
    currentPlan.value = recentPlan

    switch (recentPlan.status) {
      case 'draft':
        step.value = 1
        break
      case 'scanning':
        step.value = 2
        events.value = (await migrationV2Api.getEvents(recentPlan.id, 50) as MigrationEvent[]).reverse() || []
        jobs.value = (await migrationV2Api.getJobs(recentPlan.id) as MigrationJob[]) || []
        startScanPolling()
        break
      case 'prechecking':
        step.value = 2
        await loadPlanData(recentPlan.id)
        events.value = events.value.reverse()
        break
      case 'precheck_failed':
        step.value = 3
        conflicts.value = (await migrationV2Api.getConflicts(recentPlan.id) as MigrationConflict[]) || []
        conflicts.value.forEach(c => { conflictPolicies.value[c.id] = c.suggested_policy })
        break
      case 'ready':
        conflicts.value = (await migrationV2Api.getConflicts(recentPlan.id) as MigrationConflict[]) || []
        conflicts.value.forEach(c => { conflictPolicies.value[c.id] = c.suggested_policy })
        if (conflicts.value.length === 0) {
          step.value = 4
          await migrationV2Api.startPlan(recentPlan.id)
          startExecPolling()
        } else {
          step.value = 3
        }
        break
      case 'running':
      case 'verifying':
        step.value = 4
        await loadPlanData(recentPlan.id)
        events.value = events.value.reverse()
        startExecPolling()
        break
      case 'paused':
        step.value = 4
        await loadPlanData(recentPlan.id)
        events.value = events.value.reverse()
        break
      case 'completed':
      case 'failed':
      case 'cancelled':
        if (recentPlan.current_stage === 'scan') {
          step.value = 2
          await loadPlanData(recentPlan.id)
          events.value = events.value.reverse()
        } else {
          step.value = 5
          await loadPlanData(recentPlan.id)
        }
        break
    }
  } catch (e) {
    console.error('Failed to load recent plan:', e)
  }
}

async function loadPlanData(planID: number) {
  try {
    jobs.value = (await migrationV2Api.getJobs(planID) as MigrationJob[]) || []
    events.value = (await migrationV2Api.getEvents(planID, 50) as MigrationEvent[]) || []
    conflicts.value = (await migrationV2Api.getConflicts(planID) as MigrationConflict[]) || []
    conflicts.value.forEach(c => { conflictPolicies.value[c.id] = c.suggested_policy })
  } catch (e) {
    console.error('Failed to load plan data:', e)
  }
}

onMounted(() => {
  loadRecentPlan()
})

onUnmounted(() => stopAllPolling())
</script>

<style scoped>
.migration-wizard { min-height: 100%; background: linear-gradient(180deg, #f8fafc 0%, #f1f5f9 100%); padding-bottom: 32px; }
.page-header { padding: 20px 24px; background: #fff; border-radius: 16px; margin-bottom: 24px; box-shadow: 0 1px 3px rgba(0,0,0,0.04); }
.header-content { display: flex; align-items: center; gap: 16px; }
.header-icon { width: 48px; height: 48px; border-radius: 12px; background: linear-gradient(135deg, #8b5cf6, #7c3aed); display: flex; align-items: center; justify-content: center; color: #fff; font-size: 22px; }
.header-text h2 { font-size: 20px; font-weight: 600; margin: 0; color: #1f2937; }
.header-subtitle { font-size: 13px; color: #9ca3af; margin: 4px 0 0; }
.wizard-container { background: #fff; border-radius: 16px; padding: 24px; box-shadow: 0 1px 3px rgba(0,0,0,0.04); }
.step-bar { margin-bottom: 32px; }
.step-panel { min-height: 300px; }
.sub-options { margin-left: 24px; padding: 8px 0; }
.actions { margin-top: 24px; display: flex; gap: 12px; }
.conflict-item { margin-bottom: 16px; }

/* stats grid */
.stats-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(140px, 1fr)); gap: 16px; margin: 20px 0; }
.stat-card { text-align: center; padding: 20px 12px; border-radius: 12px; background: #f8fafc; border: 1px solid #e5e7eb; }
.stat-icon { width: 40px; height: 40px; border-radius: 10px; display: flex; align-items: center; justify-content: center; margin: 0 auto 10px; font-size: 18px; }
.stat-icon.repos { background: #eff6ff; color: #3b82f6; }
.stat-icon.roles { background: #fef3c7; color: #f59e0b; }
.stat-icon.users { background: #ecfdf5; color: #10b981; }
.stat-icon.artifacts { background: #ede9fe; color: #8b5cf6; }
.stat-num { font-size: 28px; font-weight: 700; color: #1f2937; }
.stat-label { font-size: 13px; color: #6b7280; margin-top: 4px; }

/* progress */
.overall-progress { margin-bottom: 24px; }
.progress-header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 8px; }
.progress-text { font-size: 13px; color: #6b7280; }

.category-progress { margin-bottom: 20px; }
.cat-row { margin-bottom: 12px; }
.cat-header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 4px; }
.cat-name { font-size: 13px; font-weight: 500; color: #374151; }
.cat-count { font-size: 12px; color: #9ca3af; }

.result-summary { margin-bottom: 20px; }

/* event log */
.event-log { margin-top: 16px; max-height: 280px; overflow-y: auto; background: #f9fafb; border-radius: 8px; padding: 12px; }
.event-line { font-size: 12px; padding: 3px 0; display: flex; align-items: center; gap: 8px; border-bottom: 1px solid #f0f0f0; }
.event-line:last-child { border-bottom: none; }
.event-line.event-warn { color: #e6a23c; }
.event-line.event-error { color: #f56c6c; }
.event-time { color: #9ca3af; white-space: nowrap; font-family: monospace; }
.event-msg { flex: 1; }
</style>
