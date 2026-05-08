<template>
  <el-card class="usage-guide-card">
    <template #header>
      <div class="guide-header">
        <span class="card-title">使用指南</span>
        <el-tag v-if="selectedVersion" type="primary" size="small" effect="plain" class="version-tag">
          {{ selectedVersion }}
        </el-tag>
      </div>
    </template>

    <div class="guide-section">
      <h4 class="section-title">安装</h4>
      <div v-for="(cmd, index) in installCommands" :key="'install-' + index" class="command-block">
        <div class="command-label">{{ cmd.label }}</div>
        <div class="command-row">
          <code>{{ cmd.command }}</code>
          <el-button size="small" text @click="copyCommand(cmd.command)">
            <el-icon><CopyDocument /></el-icon>
          </el-button>
        </div>
      </div>
    </div>

    <div v-if="configSnippets.length" class="guide-section">
      <h4 class="section-title">配置</h4>
      <div v-for="(snippet, index) in configSnippets" :key="'config-' + index" class="command-block">
        <div class="command-label">{{ snippet.label }}</div>
        <div class="code-block">
          <pre><code>{{ snippet.code }}</code></pre>
          <el-button class="copy-btn" size="small" text @click="copyCommand(snippet.code)">
            <el-icon><CopyDocument /></el-icon>
          </el-button>
        </div>
      </div>
    </div>

    <div v-if="usageExamples.length" class="guide-section">
      <h4 class="section-title">使用示例</h4>
      <div v-for="(example, index) in usageExamples" :key="'usage-' + index" class="command-block">
        <div class="command-label">{{ example.label }}</div>
        <div class="code-block">
          <pre><code>{{ example.code }}</code></pre>
          <el-button class="copy-btn" size="small" text @click="copyCommand(example.code)">
            <el-icon><CopyDocument /></el-icon>
          </el-button>
        </div>
      </div>
    </div>
  </el-card>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { CopyDocument } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import { normalizePackageType } from '@/constants/package'

interface CommandItem {
  label: string
  command: string
}

interface CodeItem {
  label: string
  code: string
}

const props = defineProps<{
  pkg: {
    name: string
    type: string
    latest_version?: string
  }
  selectedVersion: string
}>()

const activeVersion = computed(() => props.selectedVersion || props.pkg.latest_version || 'latest')

function parseMavenCoords(name: string): { groupId: string; artifactId: string } {
  const separator = name.includes(':') ? ':' : '/'
  const parts = name.split(separator)
  return {
    groupId: parts.length >= 2 ? parts[0] : name,
    artifactId: parts.length >= 2 ? parts[1] : name,
  }
}

const installCommands = computed<CommandItem[]>(() => {
  const { name, type } = props.pkg
  const version = activeVersion.value
  const t = normalizePackageType(type)

  switch (t) {
    case 'npm':
      return [
        { label: 'npm', command: `npm install ${name}@${version}` },
        { label: 'Yarn', command: `yarn add ${name}@${version}` },
        { label: 'pnpm', command: `pnpm add ${name}@${version}` },
      ]
    case 'pypi':
      return [
        { label: 'pip', command: `pip install ${name}==${version}` },
        { label: 'pip (最新版)', command: `pip install ${name}` },
        { label: 'poetry', command: `poetry add ${name}==${version}` },
      ]
    case 'maven': {
      const { groupId, artifactId } = parseMavenCoords(name)
      const mavenCoord = `${groupId}:${artifactId}`
      return [
        { label: 'Maven CLI', command: `mvn dependency:get -Dartifact=${mavenCoord}:${version}` },
        { label: 'Gradle (Groovy)', command: `implementation '${mavenCoord}:${version}'` },
        { label: 'Gradle (Kotlin)', command: `implementation("${mavenCoord}:${version}")` },
      ]
    }
    case 'go':
      return [
        { label: 'go get', command: `go get ${name}@${version}` },
        { label: 'go mod tidy', command: `go mod tidy` },
      ]
    case 'yum':
      return [
        { label: 'yum', command: `yum install ${name}-${version}` },
      ]
    case 'apt':
      return [
        { label: 'apt', command: `apt-get install ${name}=${version}` },
      ]
    default:
      return []
  }
})

const configSnippets = computed<CodeItem[]>(() => {
  const { name, type } = props.pkg
  const version = activeVersion.value
  const t = normalizePackageType(type)

  switch (t) {
    case 'maven': {
      const { groupId, artifactId } = parseMavenCoords(name)
      return [
        {
          label: 'pom.xml',
          code: `<dependency>
    <groupId>${groupId}</groupId>
    <artifactId>${artifactId}</artifactId>
    <version>${version}</version>
</dependency>`,
        },
        {
          label: 'Gradle (Groovy)',
          code: `implementation '${groupId}:${artifactId}:${version}'`,
        },
        {
          label: 'Gradle (Kotlin DSL)',
          code: `implementation("${groupId}:${artifactId}:${version}")`,
        },
      ]
    }
    case 'go':
      return [
        {
          label: 'go.mod',
          code: `require ${name} ${version}`,
        },
      ]
    default:
      return []
  }
})

const usageExamples = computed<CodeItem[]>(() => {
  const { name, type } = props.pkg
  const t = normalizePackageType(type)

  switch (t) {
    case 'npm':
      return [
        {
          label: 'ES Module',
          code: `import {} from '${name}'`,
        },
        {
          label: 'CommonJS',
          code: `const _ = require('${name}')`,
        },
      ]
    case 'pypi':
      return [
        {
          label: 'Python',
          code: `import ${name.replace(/[-@/.]/g, '_')}`,
        },
      ]
    case 'go': {
      return [
        {
          label: 'Go',
          code: `import "${name}"`,
        },
      ]
    }
    case 'maven': {
      const { groupId } = parseMavenCoords(name)
      return [
        {
          label: 'Java',
          code: `import ${groupId}.*;`,
        },
      ]
    }
    default:
      return []
  }
})

async function copyCommand(text: string) {
  try {
    await navigator.clipboard.writeText(text)
    ElMessage.success('已复制到剪贴板')
  } catch {
    ElMessage.error('复制失败')
  }
}
</script>

<style scoped>
.usage-guide-card {
  margin-bottom: 20px;
}

.guide-header {
  display: flex;
  align-items: center;
  gap: 8px;
}

.card-title {
  font-size: 16px;
  font-weight: 600;
  color: #303133;
}

.version-tag {
  font-family: 'SF Mono', Monaco, 'Cascadia Code', monospace;
}

.guide-section {
  margin-bottom: 20px;
}

.guide-section:last-child {
  margin-bottom: 0;
}

.section-title {
  font-size: 14px;
  font-weight: 600;
  color: #303133;
  margin: 0 0 12px;
}

.command-block {
  margin-bottom: 10px;
}

.command-block:last-child {
  margin-bottom: 0;
}

.command-label {
  font-size: 12px;
  color: #909399;
  margin-bottom: 4px;
}

.command-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  background: #f5f7fa;
  border-radius: 6px;
  padding: 8px 12px;
  gap: 8px;
}

.command-row code {
  font-family: 'SF Mono', Monaco, 'Cascadia Code', monospace;
  font-size: 13px;
  color: #303133;
  word-break: break-all;
  flex: 1;
}

.code-block {
  position: relative;
  background: #1e1e1e;
  border-radius: 6px;
  padding: 12px 40px 12px 16px;
  overflow-x: auto;
}

.code-block pre {
  margin: 0;
}

.code-block code {
  font-family: 'SF Mono', Monaco, 'Cascadia Code', monospace;
  font-size: 13px;
  color: #d4d4d4;
  line-height: 1.6;
  white-space: pre;
}

.copy-btn {
  position: absolute;
  top: 8px;
  right: 8px;
  color: #909399 !important;
}

.copy-btn:hover {
  color: #d4d4d4 !important;
}
</style>
