<template>
  <div class="migration-page">
    <header class="page-header">
      <div class="header-content">
        <div class="header-icon">
          <i class="fa-solid fa-arrow-right-arrow-left"></i>
        </div>
        <div class="header-text">
          <h2>数据迁移</h2>
          <p class="header-subtitle">从 Nexus 迁移仓库数据</p>
        </div>
      </div>
    </header>

    <div class="content-wrapper">
      <div class="step-container">
        <el-steps :active="currentStep" finish-status="success" class="step-bar">
          <el-step title="连接 Nexus" />
          <el-step title="选择仓库" />
          <el-step title="执行迁移" />
        </el-steps>

        <div class="step-content">
          <NexusConnectionForm
            v-if="currentStep === 0"
            @connected="onConnected"
          />

          <template v-if="currentStep === 1">
            <RepositorySelector
              :repositories="nexusRepos"
              @selected="onSelected"
            />
            <div class="target-repo-section">
              <label class="section-label">目标仓库</label>
              <el-select
                v-model="targetRepoId"
                placeholder="选择目标仓库"
                class="repo-select"
              >
                <el-option
                  v-for="repo in localRepos"
                  :key="repo.id"
                  :label="repo.name"
                  :value="repo.id"
                />
              </el-select>
              <span class="tip">迁移的包将存储到选定的目标仓库</span>
            </div>
            <div class="advanced-config">
              <el-collapse>
                <el-collapse-item title="高级配置" name="advanced">
                  <el-form label-width="120px">
                    <el-form-item label="并发数">
                      <el-input-number
                        v-model="workerCount"
                        :min="1"
                        :max="50"
                        :step="1"
                      />
                      <span class="config-tip">同时处理的组件数量（默认：10）</span>
                    </el-form-item>
                    <el-form-item label="最大重试次数">
                      <el-input-number
                        v-model="maxRetries"
                        :min="1"
                        :max="10"
                        :step="1"
                      />
                      <span class="config-tip">失败后的重试次数（默认：3）</span>
                    </el-form-item>
                    <el-form-item label="批处理大小">
                      <el-input-number
                        v-model="batchSize"
                        :min="10"
                        :max="500"
                        :step="10"
                      />
                      <span class="config-tip">每批处理的组件数量（默认：50）</span>
                    </el-form-item>
                  </el-form>
                </el-collapse-item>
              </el-collapse>
            </div>
            <div class="actions">
              <el-button type="primary" @click="startMigration" :disabled="selectedRepos.length === 0 || !targetRepoId">
                开始迁移
              </el-button>
            </div>
          </template>

          <MigrationProgress
            v-if="currentStep === 2"
            :status="migrationStatus"
            :total="totalItems"
            :processed="processedItems"
            :failed="failedItems"
            :logs="logs"
            @cancel="onCancel"
          />
        </div>
      </div>

      <div class="history-section">
        <MigrationHistory :tasks="historyTasks" />
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue'
import { ElMessage } from 'element-plus'
import NexusConnectionForm from '@/components/migration/NexusConnectionForm.vue'
import RepositorySelector from '@/components/migration/RepositorySelector.vue'
import MigrationProgress from '@/components/migration/MigrationProgress.vue'
import MigrationHistory from '@/components/migration/MigrationHistory.vue'
import {
  listNexusRepositories,
  createMigration,
  getMigrationStatus,
  cancelMigration,
  listMigrations,
  type NexusRepo,
  type MigrationTask,
} from '@/api/migration'
import { repositoryApi } from '@/api/repository'

const currentStep = ref(0)
const nexusRepos = ref<NexusRepo[]>([])
const selectedRepos = ref<string[]>([])
const migrationStatus = ref('pending')
const processedItems = ref(0)
const failedItems = ref(0)
const totalItems = ref(0)
const logs = ref<string[]>([])
const historyTasks = ref<MigrationTask[]>([])
const currentTaskId = ref(0)
const pollingTimer = ref<number | null>(null)
const localRepos = ref<any[]>([])
const targetRepoId = ref<number | undefined>(undefined)
const workerCount = ref(10)
const maxRetries = ref(3)
const batchSize = ref(50)

const nexusCredentials = ref({ url: '', username: '', password: '' })

async function loadLocalRepos() {
  try {
    const res = (await repositoryApi.list()) as any
    localRepos.value = (res?.list || res || []).filter((r: any) => r.type === 'local')
  } catch {
    // ignore errors
  }
}

async function onConnected(data: { url: string; username: string; password: string }) {
  nexusCredentials.value = data
  try {
    const res = (await listNexusRepositories(data)) as any
    nexusRepos.value = res?.items || res || []
    currentStep.value = 1
  } catch (e: any) {
    ElMessage.error('获取仓库列表失败: ' + e.message)
  }
}

function onSelected(repos: string[]) {
  selectedRepos.value = repos
}

async function startMigration() {
  try {
    const selectedRepo = localRepos.value.find((r: any) => r.id === targetRepoId.value)
    const res = (await createMigration({
      ...nexusCredentials.value,
      selected_repos: selectedRepos.value,
      target_repository_id: targetRepoId.value,
      target_repository: selectedRepo?.name || '',
      worker_count: workerCount.value,
      max_retries: maxRetries.value,
      batch_size: batchSize.value,
    })) as any
    currentTaskId.value = res?.id || res?.task?.id
    currentStep.value = 2
    migrationStatus.value = 'running'
    startPolling()
  } catch (e: any) {
    ElMessage.error('创建迁移任务失败: ' + e.message)
  }
}

function startPolling() {
  pollingTimer.value = window.setInterval(async () => {
    try {
      const res = (await getMigrationStatus(currentTaskId.value)) as any
      processedItems.value = res?.processed_items ?? 0
      failedItems.value = res?.failed_items ?? 0
      totalItems.value = res?.total_items ?? 0
      migrationStatus.value = res?.task?.status ?? 'unknown'
      if (res?.logs) {
        logs.value = res.logs
      }

      if (res?.task?.status !== 'running' && res?.task?.status !== 'pending') {
        stopPolling()
        loadHistory()
      }
    } catch {
      // ignore polling errors
    }
  }, 3000)
}

function stopPolling() {
  if (pollingTimer.value) {
    clearInterval(pollingTimer.value)
    pollingTimer.value = null
  }
}

async function onCancel() {
  try {
    await cancelMigration(currentTaskId.value)
    migrationStatus.value = 'cancelled'
    stopPolling()
    loadHistory()
  } catch (e: any) {
    ElMessage.error('取消失败: ' + e.message)
  }
}

async function loadHistory() {
  try {
    const res = (await listMigrations()) as any
    historyTasks.value = res?.list || res || []
  } catch {
    // ignore errors when loading history
  }
}

onMounted(() => {
  loadHistory()
  loadLocalRepos()
})

onUnmounted(() => {
  stopPolling()
})
</script>

<style scoped>
.migration-page {
  min-height: 100%;
  background: linear-gradient(180deg, #f8fafc 0%, #f1f5f9 100%);
  padding-bottom: 32px;
}

.page-header {
  padding: 20px 24px;
  background: #fff;
  border-radius: 16px;
  margin-bottom: 24px;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.04);
}

.header-content {
  display: flex;
  align-items: center;
  gap: 16px;
}

.header-icon {
  width: 48px;
  height: 48px;
  border-radius: 12px;
  background: linear-gradient(135deg, #8b5cf6 0%, #7c3aed 100%);
  display: flex;
  align-items: center;
  justify-content: center;
  color: #fff;
  font-size: 22px;
}

.header-text h2 {
  font-size: 20px;
  font-weight: 600;
  margin: 0;
  color: #1f2937;
  letter-spacing: -0.2px;
}

.header-subtitle {
  font-size: 13px;
  color: #9ca3af;
  margin: 4px 0 0;
}

.content-wrapper {
  display: flex;
  flex-direction: column;
  gap: 24px;
}

.step-container {
  background: #fff;
  border-radius: 16px;
  padding: 24px;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.04);
}

.step-bar {
  margin-bottom: 32px;
}

.step-content {
  min-height: 380px;
}

.target-repo-section {
  margin-top: 24px;
  padding: 20px;
  background: #f8fafc;
  border-radius: 12px;
  border: 1px solid #e2e8f0;
}

.section-label {
  display: block;
  font-size: 14px;
  font-weight: 500;
  color: #374151;
  margin-bottom: 12px;
}

.repo-select {
  width: 100%;
  max-width: 320px;
}

.tip {
  display: block;
  font-size: 12px;
  color: #9ca3af;
  margin-top: 8px;
}

.actions {
  margin-top: 24px;
}

.history-section {
  background: #fff;
  border-radius: 16px;
  padding: 24px;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.04);
}

@media (max-width: 1200px) {
  .content-wrapper {
    gap: 16px;
  }
}
</style>
