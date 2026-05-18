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
                <span class="group-icon" :style="{ background: group.color }"><i :class="group.icon"></i></span>
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
import { normalizePackageType } from '@/constants/package'
import TypeSidebar from './TypeSidebar.vue'
import RepoCard from './RepoCard.vue'
import { copyToClipboard } from '@/utils/clipboard'

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

const GROUP_CONFIG: Record<string, { label: string; icon: string; color: string }> = {
  npm: { label: 'npm', icon: 'fa-solid fa-cube', color: '#cb3837' },
  maven: { label: 'Maven', icon: 'fa-solid fa-cubes', color: '#e65100' },
  pypi: { label: 'PyPI', icon: 'fa-brands fa-python', color: '#3775a9' },
  go: { label: 'Go', icon: 'fa-brands fa-golang', color: '#00add8' },
  yum: { label: 'Yum', icon: 'fa-solid fa-server', color: '#2e6da4' },
  apt: { label: 'Apt', icon: 'fa-solid fa-box', color: '#d70a53' },
  generic: { label: 'Generic', icon: 'fa-solid fa-folder', color: '#606266' },
}

const GROUP_ORDER = ['npm', 'maven', 'pypi', 'go', 'yum', 'apt', 'generic']

const groupedRepos = computed<RepoGroup[]>(() => {
  const groups: Record<string, Repository[]> = {}
  for (const repo of repos.value) {
    const key = normalizePackageType(repo.package_type) || 'generic'
    if (!groups[key]) groups[key] = []
    groups[key].push(repo)
  }

  return Object.entries(groups)
    .sort(([a], [b]) => GROUP_ORDER.indexOf(a) - GROUP_ORDER.indexOf(b))
    .map(([type, reposList]) => {
      const cfg = GROUP_CONFIG[type] || GROUP_CONFIG.generic
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
    const list = res || []
    repos.value = list
  } catch {
    repos.value = []
  } finally {
    if (repos.value.length > 0 && !activeType.value) {
      activeType.value = groupedRepos.value[0]?.type || ''
    }
    loading.value = false
  }
})

function getRegistryUrl(repo: Repository): string {
  const base = `${window.location.origin}/repo/${repo.name}`
  const type = normalizePackageType(repo.package_type)
  switch (type) {
    case 'npm':
      return `${base}/`
    case 'pypi':
      return `${base}/simple`
    default:
      return `${base}/`
  }
}

function getConfigCommand(repo: Repository): string {
  const url = getRegistryUrl(repo)
  const type = normalizePackageType(repo.package_type)
  switch (type) {
    case 'npm':
      return `npm config set registry ${url}`
    case 'pypi':
      return `pip config set global.index-url ${url}`
    case 'maven':
      return url
    case 'go':
      return `GOPROXY=${url} go mod tidy`
    case 'yum':
      return `sudo yum-config-manager --add-repo ${url}repodata/repomd.xml`
    case 'apt':
      return `deb [trusted=yes] ${url} ./`
    default:
      return url
  }
}

function copyText(text: string) {
  copyToClipboard(text)
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
