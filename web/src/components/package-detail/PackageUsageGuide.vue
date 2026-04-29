<template>
  <el-card class="usage-guide-card">
    <template #header>
      <div class="guide-header">
        <span class="card-title">使用指南</span>
        <el-tag v-if="selectedVersion" type="primary" size="small" effect="plain" class="version-tag">
          v{{ selectedVersion }}
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

function normalizeType(type: string) {
  return type === 'maven' ? 'maven2' : type
}

const installCommands = computed<CommandItem[]>(() => {
  const { name, type } = props.pkg
  const version = activeVersion.value
  const t = normalizeType(type)

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
    case 'maven2':
      return [
        { label: 'Maven CLI', command: `mvn dependency:get -Dartifact=${name}:${version}` },
        { label: 'Gradle (Groovy)', command: `implementation '${name}:${version}'` },
        { label: 'Gradle (Kotlin)', command: `implementation("${name}:${version}")` },
      ]
    case 'go':
      return [
        { label: 'go get', command: `go get ${name}@v${version}` },
        { label: 'go mod tidy', command: `go mod tidy` },
      ]
    case 'nuget':
      return [
        { label: '.NET CLI', command: `dotnet add package ${name} -v ${version}` },
        { label: 'NuGet CLI', command: `nuget install ${name} -Version ${version}` },
        { label: 'PackageReference', command: `<PackageReference Include="${name}" Version="${version}" />` },
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
  const t = normalizeType(type)

  switch (t) {
    case 'maven2': {
      const parts = name.split(':')
      const groupId = parts[0] || name
      const artifactId = parts[1] || name
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
          code: `require ${name} v${version}`,
        },
      ]
    case 'nuget':
      return [
        {
          label: 'csproj',
          code: `<PackageReference Include="${name}" Version="${version}" />`,
        },
      ]
    default:
      return []
  }
})

const usageExamples = computed<CodeItem[]>(() => {
  const { name, type } = props.pkg
  const t = normalizeType(type)

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
    case 'maven2': {
      const parts = name.split(':')
      return [
        {
          label: 'Java',
          code: `import ${parts[0] || name}.*;`,
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
