<template>
  <div class="repo-config-page">
    <div v-loading="loading" class="config-container">
      <el-empty v-if="!loading && error" :description="error">
        <el-button @click="goBack">返回</el-button>
      </el-empty>

      <template v-else-if="repoConfig">
        <div class="config-header">
          <div class="header-left">
            <h1 class="repo-title">{{ repoConfig.display_name || repoConfig.name }}</h1>
            <p v-if="repoConfig.description" class="repo-desc">{{ repoConfig.description }}</p>
          </div>
          <div class="header-right">
            <el-tag :type="typeColor" size="large" effect="plain">
              {{ typeLabel }}
            </el-tag>
            <el-tag size="large" effect="plain">
              {{ packageTypeLabel }}
            </el-tag>
          </div>
        </div>

        <div class="registry-url-section">
          <div class="section-label">Registry URL</div>
          <div class="url-box">
            <code class="url-text">{{ repoConfig.registry_url }}</code>
            <el-button type="primary" size="small" @click="copyText(repoConfig.registry_url)">
              <el-icon><CopyDocument /></el-icon>
              复制
            </el-button>
          </div>
        </div>

        <div v-if="repoConfig.remote_url" class="remote-url-section">
          <div class="section-label">远程仓库地址</div>
          <div class="url-box secondary">
            <code class="url-text">{{ repoConfig.remote_url }}</code>
          </div>
        </div>

        <div class="config-guide-section">
          <h2 class="section-title">配置指南</h2>
          <div class="guide-list">
            <div v-for="(step, index) in repoConfig.config_guide" :key="index" class="guide-card">
              <div class="guide-header">
                <span class="guide-number">{{ index + 1 }}</span>
                <div class="guide-title-group">
                  <h3 class="guide-title">{{ step.title }}</h3>
                  <p class="guide-desc">{{ step.description }}</p>
                </div>
              </div>
              <div class="guide-code">
                <pre><code :class="`language-${step.language}`">{{ step.command }}</code></pre>
                <el-button size="small" text @click="copyText(step.command)">
                  <el-icon><CopyDocument /></el-icon>
                </el-button>
              </div>
            </div>
          </div>
        </div>
      </template>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { CopyDocument } from '@element-plus/icons-vue'
import { publicRepoApi, type RepoConfigResponse } from '@/api/public'
import { copyToClipboard } from '@/utils/clipboard'

const route = useRoute()
const router = useRouter()

const loading = ref(false)
const error = ref('')
const repoConfig = ref<RepoConfigResponse | null>(null)

const typeColor = computed(() => {
  if (!repoConfig.value) return 'info'
  const map: Record<string, string> = { local: '', proxy: 'success', virtual: 'warning' }
  return map[repoConfig.value.type] || 'info'
})

const typeLabel = computed(() => {
  if (!repoConfig.value) return ''
  const map: Record<string, string> = { local: '本地仓库', proxy: '代理仓库', virtual: '虚拟仓库' }
  return map[repoConfig.value.type] || repoConfig.value.type
})

const packageTypeLabel = computed(() => {
  if (!repoConfig.value) return ''
  const map: Record<string, string> = {
    npm: 'npm',
    maven: 'Maven',
    pypi: 'PyPI',
    go: 'Go',
    yum: 'Yum',
    apt: 'Apt',
    generic: 'Generic',
  }
  return map[repoConfig.value.package_type] || repoConfig.value.package_type
})

async function loadRepoConfig() {
  const name = route.params.name as string
  if (!name) {
    error.value = '仓库名称不能为空'
    return
  }

  loading.value = true
  error.value = ''

  try {
    const res = await publicRepoApi.getRepoConfig(name)
    repoConfig.value = (res as any).data
  } catch (e: any) {
    error.value = e.response?.data?.message || '加载仓库配置失败'
  } finally {
    loading.value = false
  }
}

function copyText(text: string) {
  copyToClipboard(text)
}

function goBack() {
  router.push('/')
}

onMounted(() => {
  loadRepoConfig()
})
</script>

<style scoped>
.repo-config-page {
  width: 100%;
}

.config-container {
  max-width: 900px;
  margin: 0 auto;
}

.config-header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  padding: 24px;
  background: #ffffff;
  border: 1px solid #e2e8f0;
  border-radius: 12px;
  margin-bottom: 20px;
}

.header-left {
  flex: 1;
}

.repo-title {
  font-size: 28px;
  font-weight: 800;
  color: #0f172a;
  margin: 0 0 8px;
  letter-spacing: -0.5px;
}

.repo-desc {
  font-size: 14px;
  color: #64748b;
  margin: 0;
  line-height: 1.5;
}

.header-right {
  display: flex;
  gap: 8px;
  flex-shrink: 0;
}

.registry-url-section,
.remote-url-section {
  padding: 20px 24px;
  background: #ffffff;
  border: 1px solid #e2e8f0;
  border-radius: 12px;
  margin-bottom: 16px;
}

.section-label {
  font-size: 12px;
  font-weight: 600;
  color: #64748b;
  text-transform: uppercase;
  letter-spacing: 0.5px;
  margin-bottom: 12px;
}

.url-box {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 12px 16px;
  background: #f8fafc;
  border: 1px solid #e2e8f0;
  border-radius: 8px;
}

.url-box.secondary {
  background: #fffbeb;
  border-color: #fcd34d;
}

.url-text {
  flex: 1;
  font-family: var(--font-family-mono);
  font-size: 14px;
  color: #0f172a;
  word-break: break-all;
}

.config-guide-section {
  padding: 24px;
  background: #ffffff;
  border: 1px solid #e2e8f0;
  border-radius: 12px;
}

.section-title {
  font-size: 18px;
  font-weight: 700;
  color: #0f172a;
  margin: 0 0 20px;
}

.guide-list {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.guide-card {
  padding: 16px 20px;
  background: #f8fafc;
  border: 1px solid #e2e8f0;
  border-radius: 8px;
}

.guide-header {
  display: flex;
  align-items: flex-start;
  gap: 12px;
  margin-bottom: 12px;
}

.guide-number {
  width: 24px;
  height: 24px;
  border-radius: 50%;
  background: #0f172a;
  color: #ffffff;
  font-size: 12px;
  font-weight: 700;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}

.guide-title-group {
  flex: 1;
}

.guide-title {
  font-size: 15px;
  font-weight: 600;
  color: #0f172a;
  margin: 0 0 4px;
}

.guide-desc {
  font-size: 13px;
  color: #64748b;
  margin: 0;
}

.guide-code {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  padding: 12px 16px;
  background: #1e293b;
  border-radius: 6px;
  overflow-x: auto;
}

.guide-code pre {
  margin: 0;
  flex: 1;
  min-width: 0;
}

.guide-code code {
  font-family: var(--font-family-mono);
  font-size: 13px;
  color: #e2e8f0;
  white-space: pre-wrap;
  word-break: break-all;
}

.guide-code .el-button {
  color: #94a3b8;
  flex-shrink: 0;
}

.guide-code .el-button:hover {
  color: #e2e8f0;
}
</style>
