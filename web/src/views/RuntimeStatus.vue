<template>
  <div class="runtime-status">
    <header class="list-header">
      <div class="header-content">
        <div class="header-icon">
          <i class="fa-solid fa-wave-square"></i>
        </div>
        <div class="header-text">
          <h2>运行状态</h2>
          <p class="header-subtitle">查看系统运行信息与实时连接</p>
        </div>
      </div>
    </header>

    <div class="content-panel">
      <el-tabs v-model="activeTab" class="status-tabs">
        <el-tab-pane label="系统信息" name="info" lazy>
          <SystemInfoPanel />
        </el-tab-pane>
        <el-tab-pane v-if="canViewConnections" label="活跃连接" name="connections" lazy>
          <ConnectionsPanel />
        </el-tab-pane>
      </el-tabs>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import SystemInfoPanel from '@/components/ops/SystemInfoPanel.vue'
import ConnectionsPanel from '@/components/ops/ConnectionsPanel.vue'

const route = useRoute()
const router = useRouter()
const authStore = useAuthStore()

const canViewConnections = computed(() => authStore.hasPermission('system', 'admin'))

const requestedTab = String(route.query.tab || '')
const activeTab = ref(
  requestedTab === 'connections' && canViewConnections.value ? 'connections' : 'info',
)

watch(activeTab, (tab) => {
  // 保留 tab 在 URL 上，便于从旧路由重定向和分享链接
  router.replace({ query: { ...route.query, tab } })
})
</script>

<style scoped>
.runtime-status {
  min-height: 100%;
  background: linear-gradient(180deg, #f8fafc 0%, #f1f5f9 100%);
}

.list-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 16px;
  flex-wrap: wrap;
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
  width: 48px;
  height: 48px;
  border-radius: 12px;
  background: linear-gradient(135deg, #0ea5e9 0%, #0284c7 100%);
  display: flex;
  align-items: center;
  justify-content: center;
  color: #fff;
  font-size: 22px;
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

.content-panel {
  background: #fff;
  border-radius: 16px;
  padding: 8px 20px 20px;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.04);
}

.status-tabs {
  --el-color-primary: #0ea5e9;
}
</style>
