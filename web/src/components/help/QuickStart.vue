<template>
  <div class="quick-start">
    <h2>🚀 快速开始</h2>

    <el-alert
      type="info"
      :closable="false"
      show-icon
      style="margin-bottom: 20px"
    >
      <template #title>
        <strong>首次使用？</strong> 请按照以下步骤配置您的客户端
      </template>
    </el-alert>

    <el-collapse v-model="activeSteps">
      <el-collapse-item title="1️⃣ 获取访问令牌" name="token">
        <div class="step-content">
          <p>在开始之前，您需要获取访问令牌：</p>
          <ol>
            <li>点击右上角的用户头像</li>
            <li>选择"个人设置"</li>
            <li>点击"访问令牌"标签</li>
            <li>点击"生成新令牌"按钮</li>
            <li>复制生成的令牌（仅显示一次）</li>
          </ol>
          <el-alert type="warning" :closable="false" style="margin-top: 10px">
            ⚠️ 请妥善保管您的令牌，不要分享给他人
          </el-alert>
        </div>
      </el-collapse-item>

      <el-collapse-item title="2️⃣ 选择您的包管理器" name="package-manager">
        <div class="step-content">
          <el-radio-group v-model="selectedManager" size="large">
            <el-radio-button label="npm">NPM</el-radio-button>
            <el-radio-button label="maven">Maven</el-radio-button>
            <el-radio-button label="pypi">PyPI</el-radio-button>
            <el-radio-button label="go">Go</el-radio-button>
            <el-radio-button label="nuget">NuGet</el-radio-button>
          </el-radio-group>

          <div class="manager-config" v-if="selectedManager">
            <h4>{{ managerTitle }} 配置</h4>
            <div v-html="managerConfig"></div>
          </div>
        </div>
      </el-collapse-item>

      <el-collapse-item title="3️⃣ 验证配置" name="verify">
        <div class="step-content">
          <p>配置完成后，运行以下命令验证：</p>
          <div v-if="selectedManager === 'npm'">
            <code-block code="npm config list" />
            <code-block code="npm install test-package" />
          </div>
          <div v-else-if="selectedManager === 'maven'">
            <code-block code="mvn help:effective-settings" />
          </div>
          <div v-else-if="selectedManager === 'pypi'">
            <code-block code="pip config list" />
            <code-block code="pip install test-package" />
          </div>
          <div v-else-if="selectedManager === 'go'">
            <code-block code="go env GOPROXY" />
          </div>
          <div v-else-if="selectedManager === 'nuget'">
            <code-block code="nuget sources list" />
          </div>
        </div>
      </el-collapse-item>

      <el-collapse-item title="4️⃣ 开始使用" name="start">
        <div class="step-content">
          <el-result
            icon="success"
            title="配置完成！"
            sub-title="您现在可以开始使用仓库了"
          >
            <template #extra>
              <el-button type="primary" @click="$router.push('/')">
                浏览仓库
              </el-button>
              <el-button @click="downloadConfig">
                下载配置文件
              </el-button>
            </template>
          </el-result>
        </div>
      </el-collapse-item>
    </el-collapse>
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import CodeBlock from '@/components/help/CodeBlock.vue'

const activeSteps = ref(['token'])
const selectedManager = ref('npm')

const managerTitle = computed(() => {
  const titles: Record<string, string> = {
    npm: 'NPM',
    maven: 'Maven',
    pypi: 'PyPI',
    go: 'Go',
    nuget: 'NuGet'
  }
  return titles[selectedManager.value]
})

const managerConfig = computed(() => {
  const registry = window.location.origin

  const configs: Record<string, string> = {
    npm: `
      <p>创建或编辑 <code>~/.npmrc</code> 文件：</p>
      <pre><code>registry=${registry}/repo/npm-virtual/
//${window.location.host}/repo/npm-virtual/:_authToken=YOUR_TOKEN_HERE</code></pre>
      <p style="margin-top: 10px">
        <el-button size="small" @click="downloadTemplate('npmrc')">
          下载 .npmrc 模板
        </el-button>
      </p>
    `,
    maven: `
      <p>编辑 <code>~/.m2/settings.xml</code> 文件：</p>
      <pre><code>&lt;settings&gt;
  &lt;servers&gt;
    &lt;server&gt;
      &lt;id&gt;moonlight&lt;/id&gt;
      &lt;username&gt;YOUR_USERNAME&lt;/username&gt;
      &lt;password&gt;YOUR_PASSWORD&lt;/password&gt;
    &lt;/server&gt;
  &lt;/servers&gt;
  &lt;mirrors&gt;
    &lt;mirror&gt;
      &lt;id&gt;moonlight&lt;/id&gt;
      &lt;mirrorOf&gt;central&lt;/mirrorOf&gt;
      &lt;url&gt;${registry}/repo/maven-virtual/&lt;/url&gt;
    &lt;/mirror&gt;
  &lt;/mirrors&gt;
&lt;/settings&gt;</code></pre>
      <p style="margin-top: 10px">
        <el-button size="small" @click="downloadTemplate('settings.xml')">
          下载 settings.xml 模板
        </el-button>
      </p>
    `,
    pypi: `
      <p>创建 <code>~/.pip/pip.conf</code> 文件：</p>
      <pre><code>[global]
index-url = ${registry}/repo/pypi-virtual/simple/
trusted-host = ${window.location.host}</code></pre>
      <p style="margin-top: 10px">
        <el-button size="small" @click="downloadTemplate('pip.conf')">
          下载 pip.conf 模板
        </el-button>
      </p>
    `,
    go: `
      <p>设置环境变量：</p>
      <pre><code>export GOPROXY=${registry}/go,https://proxy.golang.org,direct
export GOPRIVATE=${window.location.host}
export GOSUMDB=off</code></pre>
      <p style="margin-top: 10px">
        <el-button size="small" @click="downloadTemplate('go-env.sh')">
          下载环境变量脚本
        </el-button>
      </p>
    `,
    nuget: `
      <p>运行以下命令：</p>
      <pre><code>nuget sources add -name moonlight -source ${registry}/nuget/v3/index.json
nuget sources update -name moonlight -username YOUR_USERNAME -password YOUR_PASSWORD</code></pre>
      <p style="margin-top: 10px">
        <el-button size="small" @click="downloadTemplate('NuGet.Config')">
          下载 NuGet.Config 模板
        </el-button>
      </p>
    `
  }

  return configs[selectedManager.value]
})

const downloadTemplate = (filename: string) => {
  window.open(`/docs/templates/${filename}`, '_blank')
}

const downloadConfig = () => {
  const filenames: Record<string, string> = {
    npm: '.npmrc',
    maven: 'settings.xml',
    pypi: 'pip.conf',
    go: 'go-env.sh',
    nuget: 'NuGet.Config'
  }
  downloadTemplate(filenames[selectedManager.value])
}
</script>

<style scoped>
.quick-start {
  padding: 20px;
}

.quick-start h2 {
  margin-bottom: 20px;
}

.step-content {
  padding: 10px 0;
}

.step-content ol {
  padding-left: 20px;
}

.step-content li {
  margin: 8px 0;
}

.manager-config {
  margin-top: 20px;
  padding: 15px;
  background: #f5f7fa;
  border-radius: 4px;
}

.manager-config h4 {
  margin-bottom: 15px;
}

.manager-config pre {
  background: #fff;
  padding: 10px;
  border-radius: 4px;
  overflow-x: auto;
}

.manager-config code {
  font-family: 'Courier New', monospace;
  font-size: 13px;
}
</style>
