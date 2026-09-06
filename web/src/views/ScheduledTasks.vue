<template>
  <div class="scheduled-tasks">
    <header class="list-header">
      <div class="header-content">
        <h2>定时任务</h2>
        <p class="subtitle">
          统一管理后台周期任务。每个任务可使用 cron 表达式（如 <code>0 3 * * *</code>）或
          <code>@every</code> 间隔（如 <code>@every 24h</code>）自定义调度；留空则使用全局默认间隔（当前
          <code>{{ globalInterval }}</code>）。
        </p>
      </div>
      <div class="header-actions">
        <el-button :loading="loading" @click="load()">刷新</el-button>
        <el-button type="primary" plain @click="openInterval">设置全局间隔</el-button>
      </div>
    </header>

    <el-table v-loading="loading" :data="tasks" style="width: 100%">
      <el-table-column prop="name" label="任务" width="260">
        <template #default="{ row }">
          <div class="task-name">{{ row.name }}</div>
          <div class="task-desc">{{ describe(row.name) }}</div>
        </template>
      </el-table-column>

      <el-table-column label="调度表达式" min-width="260">
        <template #default="{ row }">
          <el-tag :type="row.custom ? 'primary' : 'info'" size="small" effect="plain">
            {{ row.custom ? '专属' : '全局默认' }}
          </el-tag>
          <code class="schedule">{{ row.schedule }}</code>
        </template>
      </el-table-column>

      <el-table-column label="操作" width="220">
        <template #default="{ row }">
          <el-button size="small" @click="openEdit(row)">编辑调度</el-button>
          <el-button size="small" type="primary" plain :loading="row.running" @click="runNow(row)">
            立即运行
          </el-button>
        </template>
      </el-table-column>
    </el-table>

    <el-dialog v-model="dialogVisible" title="编辑任务" width="560">
      <el-form label-position="top">
        <el-form-item label="调度表达式">
          <el-input
            v-model="form.schedule"
            placeholder="例如 0 3 * * *  或  @every 24h；留空表示使用全局默认间隔"
          />
          <div class="form-tip">
            支持标准 cron 5 段表达式（分 时 日 月 星期），或 <code>@every &lt;间隔&gt;</code> 与
            <code>@daily</code>/<code>@hourly</code> 等预置。
          </div>
        </el-form-item>

        <el-form-item v-for="field in editing?.config ?? []" :key="field.key" :label="field.label">
          <el-switch
            v-if="field.kind === 'bool'"
            v-model="configValues[field.key]"
            :active-value="true"
            :inactive-value="false"
          />
          <el-input-number
            v-else-if="field.kind === 'int'"
            v-model="configValues[field.key]"
            :min="1"
          />
          <el-input v-else v-model="configValues[field.key]" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="save">保存</el-button>
      </template>
    </el-dialog>
    <el-dialog v-model="intervalDialog" title="全局默认间隔" width="420">
      <el-form label-position="top">
        <el-form-item label="间隔">
          <el-input v-model="intervalForm" placeholder="如 24h、12h" />
          <div class="form-tip">所有未设置专属调度的任务使用此默认间隔。</div>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="intervalDialog = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="saveInterval">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { schedulerApi, type ScheduledTaskItem } from '@/api/scheduler'

const loading = ref(false)
const saving = ref(false)
const dialogVisible = ref(false)
const intervalDialog = ref(false)
const intervalForm = ref('')
const globalInterval = ref('24h')
const tasks = ref<(ScheduledTaskItem & { running?: boolean })[]>([])
const editing = ref<ScheduledTaskItem | null>(null)
const form = reactive({ schedule: '' })
const configValues = reactive<Record<string, unknown>>({})

const TASK_DESC: Record<string, string> = {
  maven_snapshot: '清理过期的 Maven SNAPSHOT 构建',
  proxy_metadata_cache_gc: '清理代理仓库中已缓存但未下载的过期元数据',
  log_cleanup: '清理过期的下载日志与聚合数据',
}

const describe = (name: string) => TASK_DESC[name] ?? ''

const load = async () => {
  loading.value = true
  try {
    const res = await schedulerApi.list()
    tasks.value = res.tasks || []
    globalInterval.value = res.interval || '24h'
  } catch {
    ElMessage.error('加载定时任务失败')
  } finally {
    loading.value = false
  }
}

const openEdit = (row: ScheduledTaskItem) => {
  editing.value = row
  form.schedule = row.schedule.startsWith('@every') && !row.custom ? '' : row.schedule
  for (const key of Object.keys(configValues)) delete configValues[key]
  for (const field of row.config ?? []) configValues[field.key] = field.value
  dialogVisible.value = true
}

const save = async () => {
  if (!editing.value) return
  saving.value = true
  try {
    const payload: { schedule: string; config?: Record<string, unknown> } = { schedule: form.schedule.trim() }
    if ((editing.value.config?.length ?? 0) > 0) {
      payload.config = { ...configValues }
    }
    await schedulerApi.update(editing.value.name, payload)
    ElMessage.success('任务已更新')
    dialogVisible.value = false
    await load()
  } catch {
    ElMessage.error('更新任务失败')
  } finally {
    saving.value = false
  }
}

const openInterval = () => {
  intervalForm.value = globalInterval.value
  intervalDialog.value = true
}

const saveInterval = async () => {
  saving.value = true
  try {
    const res = await schedulerApi.updateInterval(intervalForm.value.trim())
    globalInterval.value = res.interval
    ElMessage.success('全局间隔已更新')
    intervalDialog.value = false
    await load()
  } catch {
    ElMessage.error('更新全局间隔失败')
  } finally {
    saving.value = false
  }
}

const runNow = async (row: ScheduledTaskItem & { running?: boolean }) => {
  const ok = await ElMessageBox.confirm(`立即执行任务 "${row.name}"？`, '提示', {
    confirmButtonText: '执行',
    cancelButtonText: '取消',
    type: 'warning',
  }).catch(() => false)
  if (!ok) return
  row.running = true
  try {
    const res = await schedulerApi.run(row.name)
    ElMessage.success(`执行完成（处理 ${res.processed} 项）`)
  } catch {
    ElMessage.error('执行失败')
  } finally {
    row.running = false
  }
}

onMounted(load)
</script>

<style scoped>
.list-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  margin-bottom: 16px;
}
.subtitle {
  color: var(--el-text-color-secondary);
  font-size: 13px;
  margin: 6px 0 0;
  line-height: 1.6;
}
.task-name {
  font-weight: 600;
}
.task-desc {
  color: var(--el-text-color-secondary);
  font-size: 12px;
}
.schedule {
  margin-left: 8px;
  font-family: monospace;
}
.form-tip {
  color: var(--el-text-color-secondary);
  font-size: 12px;
  margin-top: 6px;
  line-height: 1.6;
}
</style>
