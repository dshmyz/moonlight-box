<template>
  <div class="migration-page">
    <h2>数据迁移</h2>

    <el-steps :active="currentStep" finish-status="success" class="step-bar">
      <el-step title="连接 Nexus" />
      <el-step title="选择仓库" />
      <el-step title="执行迁移" />
    </el-steps>

    <NexusConnectionForm
      v-if="currentStep === 0"
      @connected="onConnected"
    />

    <template v-if="currentStep === 1">
      <RepositorySelector
        :repositories="nexusRepos"
        @selected="onSelected"
      />
      <div class="actions">
        <el-button type="primary" @click="startMigration" :disabled="selectedRepos.length === 0">
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

    <MigrationHistory :tasks="historyTasks" />
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

const nexusCredentials = ref({ url: '', username: '', password: '' })

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
    const res = (await createMigration({
      ...nexusCredentials.value,
      selected_repos: selectedRepos.value,
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
})

onUnmounted(() => {
  stopPolling()
})
</script>

<style scoped>
.step-bar {
  margin: 20px 0;
}
.actions {
  margin-top: 20px;
}
</style>
