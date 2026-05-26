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
              <el-button @click="nextStep" :disabled="!connForm.url">下一步</el-button>
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
          <div class="actions"><el-button type="primary" @click="createAndScan">创建计划并扫描</el-button></div>
        </div>

        <!-- Step 2: Scan Progress -->
        <div v-if="step === 2" class="step-panel">
          <h3>扫描中...</h3>
          <p v-if="currentPlan">Plan #{{ currentPlan.id }} - 状态: {{ currentPlan.status }}</p>
          <el-button type="primary" @click="runPrecheck" :disabled="scanRunning">执行预检</el-button>
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
          <el-button type="primary" @click="applyConflicts">应用策略</el-button>
        </div>

        <!-- Step 4: Execution -->
        <div v-if="step === 4" class="step-panel">
          <h3>执行中</h3>
          <p>状态: {{ currentPlan?.status }}</p>
          <div v-if="jobs.length">
            <p>Jobs: {{ completedJobs }}/{{ jobs.length }}</p>
            <div v-for="j in jobs" :key="j.id" class="job-row">
              <span>{{ j.kind }}/{{ j.source_key }}</span>
              <el-tag :type="jobTagType(j.status)" size="small">{{ j.status }}</el-tag>
            </div>
          </div>
          <div v-if="events.length" class="event-log">
            <p v-for="e in recentEvents" :key="e.id" class="event-line">{{ e.message }}</p>
          </div>
          <div class="actions">
            <el-button @click="pausePlan" :disabled="!isRunning">暂停</el-button>
            <el-button @click="cancelPlan" :disabled="!isRunning">取消</el-button>
          </div>
        </div>

        <!-- Step 5: Result -->
        <div v-if="step === 5" class="step-panel">
          <h3>迁移结果</h3>
          <p>状态: {{ currentPlan?.status }}</p>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onUnmounted } from 'vue'
import { ElMessage } from 'element-plus'
import { migrationV2Api, type MigrationPlan, type MigrationJob, type MigrationConflict, type MigrationEvent, type ConflictPolicy, type ScopeSelection } from '@/api/migrationV2'

const step = ref(0)
const testing = ref(false)
const scanRunning = ref(false)
const pollTimer = ref<number | null>(null)

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

const isRunning = computed(() => currentPlan.value?.status === 'running')
const completedJobs = computed(() => jobs.value.filter(j => j.status === 'completed').length)
const recentEvents = computed(() => events.value.slice(-10))

function jobTagType(s: string) {
  const m: Record<string, string> = { completed: 'success', failed: 'danger', running: 'warning', pending: 'info', skipped: '' }
  return m[s] || ''
}

async function testConnection() {
  testing.value = true
  try {
    await migrationV2Api.testSource({ source_type: 'nexus', ...connForm.value })
    ElMessage.success('连接成功')
  } catch (e: any) { ElMessage.error('连接失败: ' + e.message) }
  finally { testing.value = false }
}

function nextStep() { step.value = 1 }

function onRepoConfigChange(v: boolean) { if (!v) { scope.value.hosted_repos = scope.value.proxy_repos = scope.value.group_repos = scope.value.group_memberships = false } }
function onArtifactsChange(v: boolean) { if (!v) scope.value.artifact_repos = [] }

async function createAndScan() {
  try {
    const res = await migrationV2Api.createPlan({
      name: connForm.value.url,
      source_url: connForm.value.url,
      username: connForm.value.username,
      password: connForm.value.password,
      scope: scope.value,
    })
    currentPlan.value = res as any
    step.value = 2
    scanRunning.value = true
    try {
      await migrationV2Api.scanPlan(currentPlan.value!.id)
      ElMessage.success('扫描完成')
    } catch (e: any) { ElMessage.error('扫描失败: ' + e.message) }
    finally {
      scanRunning.value = false
      await refreshPlan()
    }
  } catch (e: any) { ElMessage.error('创建失败: ' + e.message) }
}

async function runPrecheck() {
  if (!currentPlan.value) return
  try {
    await migrationV2Api.precheckPlan(currentPlan.value.id)
    const res = await migrationV2Api.getConflicts(currentPlan.value.id)
    conflicts.value = (res as any)?.list || (res as any)?.data || res || []
    conflicts.value.forEach(c => { conflictPolicies.value[c.id] = c.suggested_policy })
    step.value = 3
  } catch (e: any) { ElMessage.error('预检失败: ' + e.message) }
}

async function applyConflicts() {
  if (!currentPlan.value) return
  try {
    const resolutions = Object.entries(conflictPolicies.value).map(([id, policy]) => ({
      conflict_id: Number(id), policy,
    }))
    await migrationV2Api.applyConflicts(currentPlan.value.id, resolutions)
    await migrationV2Api.startPlan(currentPlan.value.id)
    step.value = 4
    startPolling()
  } catch (e: any) { ElMessage.error('执行失败: ' + e.message) }
}

async function pausePlan() {
  if (!currentPlan.value) return
  await migrationV2Api.pausePlan(currentPlan.value.id)
  await refreshPlan()
}

async function cancelPlan() {
  if (!currentPlan.value) return
  await migrationV2Api.cancelPlan(currentPlan.value.id)
  stopPolling()
  step.value = 5
}

async function refreshPlan() {
  if (!currentPlan.value) return
  try { currentPlan.value = (await migrationV2Api.getPlan(currentPlan.value.id)) as any }
  catch { /* ignore */ }
}

function startPolling() {
  stopPolling()
  pollTimer.value = window.setInterval(async () => {
    if (!currentPlan.value) return
    await refreshPlan()
    try { jobs.value = (await migrationV2Api.getJobs(currentPlan.value.id)) as any }
    catch { /* ignore */ }
    try { events.value = (await migrationV2Api.getEvents(currentPlan.value.id, 20)) as any }
    catch { /* ignore */ }
    if (currentPlan.value?.status === 'completed' || currentPlan.value?.status === 'failed' || currentPlan.value?.status === 'cancelled') {
      stopPolling()
      step.value = 5
    }
  }, 3000)
}

function stopPolling() {
  if (pollTimer.value) { clearInterval(pollTimer.value); pollTimer.value = null }
}

onUnmounted(() => stopPolling())
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
.actions { margin-top: 24px; }
.conflict-item { margin-bottom: 16px; }
.job-row { display: flex; justify-content: space-between; align-items: center; padding: 8px 0; border-bottom: 1px solid #f0f0f0; }
.event-log { margin-top: 16px; max-height: 200px; overflow-y: auto; background: #f9fafb; border-radius: 8px; padding: 12px; }
.event-line { font-size: 12px; color: #6b7280; margin: 4px 0; }
</style>
