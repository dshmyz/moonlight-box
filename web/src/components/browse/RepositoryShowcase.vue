<template>
  <div class="repo-showcase">
    <div v-loading="loading">
      <el-empty v-if="!loading && repos.length === 0" description="暂无可用仓库" :image-size="64" />
      
      <div v-else class="repo-showcase-content">
        <TypeSidebar
          v-model:active-type="activeType"
          :groups="sidebarGroups"
        />

        <div class="repo-panel">
          <template v-for="group in groupedRepos" :key="group.type">
            <div v-show="activeType === group.type" class="repo-group">
              <div class="group-header">
                <span class="group-icon" :style="{ background: group.color }">{{ group.icon }}</span>
                <span class="group-name">{{ group.label }} 仓库</span>
                <span class="group-count">{{ group.repos.length }} 个</span>
              </div>

              <div class="repo-list">
                <RepoCard
                  v-for="repo in group.repos"
                  :key="repo.name"
                  :repo="repo"
                  :config-command="getConfigCommand(repo)"
                  @copy="copyText"
                />
              </div>
            </div>
          </template>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { repositoryApi, type Repository } from '@/api/repository'
import { ElMessage } from 'element-plus'
import TypeSidebar from './TypeSidebar.vue'
import RepoCard from './RepoCard.vue'

const loading = ref(false)
const repos = ref<Repository[]>([])
const activeType = ref('')

interface RepoGroup {
  type: string
  label: string
  icon: string
  color: string
  repos: Repository[]
}

const groupedRepos = computed<RepoGroup[]>(() => {
  const groupConfig: Record<string, { label: string; icon: string; color: string }> = {
    npm: { label: 'npm', icon: '⬢', color: '#cb3837' },
    maven2: { label: 'Maven', icon: 'M', color: '#e65100' },
    pypi: { label: 'PyPI', icon: '🐍', color: '#3775a9' },
    go: { label: 'Go', icon: 'Go', color: '#00add8' },
    nuget: { label: 'NuGet', icon: 'N', color: '#004880' },
    yum: { label: 'Yum', icon: 'Y', color: '#2e6da4' },
    apt: { label: 'Apt', icon: 'A', color: '#d70a53' },
    generic: { label: 'Generic', icon: 'G', color: '#606266' },
  }

  const groups: Record<string, Repository[]> = {}
  for (const repo of repos.value) {
    const key = repo.package_type || 'generic'
    if (!groups[key]) groups[key] = []
    groups[key].push(repo)
  }

  return Object.entries(groups)
    .sort(([a], [b]) => {
      const order = ['npm', 'maven2', 'pypi', 'go', 'nuget']
      return order.indexOf(a) - order.indexOf(b)
    })
    .map(([type, reposList]) => {
      const cfg = groupConfig[type] || groupConfig.generic
      return { type, label: cfg.label, icon: cfg.icon, color: cfg.color, repos: reposList }
    })
})

const sidebarGroups = computed(() => {
  return groupedRepos.value.map(group => ({
    type: group.type,
    label: group.label,
    icon: group.icon,
    color: group.color,
    count: group.repos.length
  }))
})

onMounted(async () => {
  loading.value = true
  try {
    const res = await repositoryApi.list()
    const data = res.data as any
    const list = data?.list ?? []
    repos.value = list.length > 0 ? list : getMockRepos()
  } catch {
    repos.value = getMockRepos()
  } finally {
    if (repos.value.length > 0 && !activeType.value) {
      activeType.value = groupedRepos.value[0]?.type || ''
    }
    loading.value = false
  }
})

function getMockRepos(): Repository[] {
  return [
    { id: 1, name: 'npm-virtual', display_name: 'npm-virtual', description: 'npm 虚拟仓库，聚合本地与代理', type: 'virtual', package_type: 'npm', enabled: true, created_at: '2024-01-01', updated_at: '2024-06-01' },
    { id: 2, name: 'npm-local', display_name: 'npm-local', description: 'npm 本地发布仓库', type: 'local', package_type: 'npm', enabled: true, created_at: '2024-01-01', updated_at: '2024-06-01' },
    { id: 3, name: 'npm-proxy', display_name: 'npm-proxy', description: 'npm 官方仓库代理', type: 'proxy', package_type: 'npm', enabled: true, remote_url: 'https://registry.npmjs.org', created_at: '2024-01-01', updated_at: '2024-06-01' },
    { id: 4, name: 'maven-virtual', display_name: 'maven-virtual', description: 'Maven 虚拟仓库', type: 'virtual', package_type: 'maven2', enabled: true, created_at: '2024-01-01', updated_at: '2024-06-01' },
    { id: 5, name: 'maven-proxy', display_name: 'maven-proxy', description: 'Maven Central 代理', type: 'proxy', package_type: 'maven2', enabled: true, remote_url: 'https://repo.maven.apache.org/maven2', created_at: '2024-01-01', updated_at: '2024-06-01' },
    { id: 6, name: 'pypi-virtual', display_name: 'pypi-virtual', description: 'PyPI 虚拟仓库', type: 'virtual', package_type: 'pypi', enabled: true, created_at: '2024-01-01', updated_at: '2024-06-01' },
    { id: 7, name: 'pypi-proxy', display_name: 'pypi-proxy', description: 'PyPI 官方代理', type: 'proxy', package_type: 'pypi', enabled: true, remote_url: 'https://pypi.org/simple', created_at: '2024-01-01', updated_at: '2024-06-01' },
    { id: 8, name: 'go-proxy', display_name: 'go-proxy', description: 'Go 模块代理', type: 'proxy', package_type: 'go', enabled: true, remote_url: 'https://proxy.golang.org', created_at: '2024-01-01', updated_at: '2024-06-01' },
    { id: 9, name: 'nuget-proxy', display_name: 'nuget-proxy', description: 'NuGet 官方代理', type: 'proxy', package_type: 'nuget', enabled: false, remote_url: 'https://api.nuget.org/v3/index.json', created_at: '2024-01-01', updated_at: '2024-06-01' },
  ]
}

function getRegistryUrl(repo: Repository): string {
  const base = `${window.location.origin}/api/v1`
  switch (repo.package_type) {
    case 'npm':
      return `${base}/repository/${repo.name}/`
    case 'pypi':
      return `${base}/repository/${repo.name}/simple`
    case 'nuget':
      return `${base}/repository/${repo.name}/v3/index.json`
    default:
      return `${base}/repository/${repo.name}/`
  }
}

function getConfigCommand(repo: Repository): string {
  const url = getRegistryUrl(repo)
  switch (repo.package_type) {
    case 'npm':
      return `npm config set registry ${url}`
    case 'pypi':
      return `pip config set global.index-url ${url}`
    case 'maven2':
      return url
    case 'go':
      return `GOPROXY=${url} go mod tidy`
    case 'nuget':
      return `dotnet nuget add source ${url} -n ${repo.name}`
    default:
      return url
  }
}

async function copyText(text: string) {
  try {
    await navigator.clipboard.writeText(text)
    ElMessage.success('已复制到剪贴板')
  } catch {
    ElMessage.error('复制失败')
  }
}
</script>

<style scoped>
.repo-showcase {
  margin-top: 24px;
  margin-bottom: 24px;
}

.repo-showcase-content {
  display: flex;
  gap: 24px;
  min-height: 300px;
}

.repo-panel {
  flex: 1;
  min-width: 0;
}

.repo-group {
  margin-bottom: 16px;
}

.repo-group:last-child {
  margin-bottom: 0;
}

.group-header {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 12px;
  padding-bottom: 8px;
  border-bottom: 1px solid #ebeef5;
}

.group-icon {
  width: 22px;
  height: 22px;
  border-radius: 4px;
  display: flex;
  align-items: center;
  justify-content: center;
  color: #fff;
  font-size: 12px;
  font-weight: 700;
  font-family: 'SF Mono', Monaco, 'Cascadia Code', monospace;
  flex-shrink: 0;
}

.group-name {
  font-size: 14px;
  font-weight: 600;
  color: #303133;
}

.group-count {
  font-size: 12px;
  color: #909399;
}

.repo-list {
  display: flex;
  flex-direction: column;
  gap: 12px;
}
</style>
