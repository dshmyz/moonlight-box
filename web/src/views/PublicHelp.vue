<template>
  <div class="public-help-page">
    <div class="help-hero">
      <h1>📖 帮助中心</h1>
      <p>快速配置您的客户端，开始使用 Moonlight Registry</p>
    </div>

    <div class="help-content">
      <el-tabs v-model="activeTab" type="border-card">
        <el-tab-pane label="🚀 快速开始" name="quickstart">
          <div class="tab-content">
            <el-alert type="info" :closable="false" style="margin-bottom: var(--spacing-5)">
              <template #title>
                <strong>首次使用？</strong> 请按照以下步骤配置您的客户端
              </template>
            </el-alert>

            <el-steps :active="activeStep" finish-status="success" align-center>
              <el-step title="获取令牌" description="登录后获取访问令牌" />
              <el-step title="配置客户端" description="选择包管理器并配置" />
              <el-step title="开始使用" description="验证配置并使用" />
            </el-steps>

            <div class="step-content" v-if="activeStep === 0">
              <h3>步骤 1：获取访问令牌</h3>
              <ol>
                <li>点击右上角的"登录"按钮</li>
                <li>使用您的账号登录系统</li>
                <li>进入"个人设置" → "访问令牌"</li>
                <li>点击"生成新令牌"</li>
                <li>复制并妥善保管您的令牌</li>
              </ol>
              <el-button type="primary" @click="activeStep = 1">下一步</el-button>
            </div>

            <div class="step-content" v-if="activeStep === 1">
              <h3>步骤 2：配置客户端</h3>
              <p>选择您的包管理器：</p>
              <el-radio-group v-model="selectedManager" size="large">
                <el-radio-button label="npm">NPM</el-radio-button>
                <el-radio-button label="maven">Maven</el-radio-button>
                <el-radio-button label="pypi">PyPI</el-radio-button>
                <el-radio-button label="go">Go</el-radio-button>
                <el-radio-button label="nuget">NuGet</el-radio-button>
              </el-radio-group>

              <div class="config-example" v-if="selectedManager">
                <h4>{{ managerTitle }} 配置示例</h4>
                <div v-html="managerConfig"></div>
              </div>

              <el-button @click="activeStep = 0">上一步</el-button>
              <el-button type="primary" @click="activeStep = 2">下一步</el-button>
            </div>

            <div class="step-content" v-if="activeStep === 2">
              <h3>步骤 3：验证配置</h3>
              <p>运行以下命令验证您的配置：</p>
              <code-block :code="verifyCommand" />
              <el-result
                icon="success"
                title="配置完成！"
                sub-title="您现在可以开始使用仓库了"
              >
                <template #extra>
                  <el-button type="primary" @click="$router.push('/')">
                    浏览仓库
                  </el-button>
                </template>
              </el-result>
            </div>
          </div>
        </el-tab-pane>

        <el-tab-pane label="📖 配置指南" name="guide">
          <div class="tab-content">
            <el-alert type="info" :closable="false" style="margin-bottom: var(--spacing-5)">
              详细的配置说明请查看
              <a href="/docs/client-configuration.md" target="_blank">完整配置文档</a>
            </el-alert>

            <el-collapse>
              <el-collapse-item title="NPM 配置" name="npm">
                <p>创建或编辑 <code>~/.npmrc</code> 文件：</p>
                <code-block :code="npmConfig" />
                <el-button size="small" @click="downloadTemplate('.npmrc')" style="margin-top: 10px">
                  下载 .npmrc 模板
                </el-button>
              </el-collapse-item>

              <el-collapse-item title="Maven 配置" name="maven">
                <p>编辑 <code>~/.m2/settings.xml</code> 文件：</p>
                <el-button size="small" @click="downloadTemplate('settings.xml')">
                  下载 settings.xml 模板
                </el-button>
              </el-collapse-item>

              <el-collapse-item title="PyPI 配置" name="pypi">
                <p>创建 <code>~/.pip/pip.conf</code> 文件：</p>
                <el-button size="small" @click="downloadTemplate('pip.conf')">
                  下载 pip.conf 模板
                </el-button>
              </el-collapse-item>

              <el-collapse-item title="Go 配置" name="go">
                <p>设置环境变量：</p>
                <el-button size="small" @click="downloadTemplate('go-env.sh')">
                  下载环境变量脚本
                </el-button>
              </el-collapse-item>

              <el-collapse-item title="NuGet 配置" name="nuget">
                <p>配置 NuGet 包源：</p>
                <el-button size="small" @click="downloadTemplate('NuGet.Config')">
                  下载 NuGet.Config 模板
                </el-button>
              </el-collapse-item>
            </el-collapse>
          </div>
        </el-tab-pane>

        <el-tab-pane label="❓ 常见问题" name="faq">
          <div class="tab-content">
            <el-input
              v-model="searchQuery"
              placeholder="搜索问题..."
              prefix-icon="Search"
              clearable
              style="margin-bottom: var(--spacing-5)"
            />

            <el-collapse>
              <el-collapse-item title="如何获取访问令牌？" name="token">
                <p>登录后，在"个人设置" → "访问令牌"中生成新的访问令牌。</p>
              </el-collapse-item>

              <el-collapse-item title="npm adduser 不工作怎么办？" name="npm-adduser">
                <p>当前版本暂不支持 <code>npm adduser</code> 命令。</p>
                <p>请通过 Web UI 获取令牌后，手动配置 <code>.npmrc</code> 文件。</p>
              </el-collapse-item>

              <el-collapse-item title="如何发布包？" name="publish">
                <p>NPM: <code>npm publish --registry=http://your-registry/repo/npm-local/</code></p>
                <p>Maven: <code>mvn clean deploy</code></p>
                <p>PyPI: <code>twine upload --repository-url http://your-registry/pypi/upload/ dist/*</code></p>
              </el-collapse-item>

              <el-collapse-item title="Go get 报错 checksum mismatch？" name="go-checksum">
                <p>当前版本不支持校验和数据库，请禁用：</p>
                <code-block code="export GOSUMDB=off" />
              </el-collapse-item>
            </el-collapse>

            <el-alert type="info" :closable="false" style="margin-top: 20px">
              更多问题请查看 <a href="/docs/faq.md" target="_blank">完整 FAQ</a>
              或联系管理员：admin@company.com
            </el-alert>
          </div>
        </el-tab-pane>
      </el-tabs>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import CodeBlock from '@/components/help/CodeBlock.vue'

const activeTab = ref('quickstart')
const activeStep = ref(0)
const selectedManager = ref('npm')
const searchQuery = ref('')

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

const registry = computed(() => window.location.origin)
const host = computed(() => window.location.host)

const managerConfig = computed(() => {
  const configs: Record<string, string> = {
    npm: `
      <pre><code>registry=${registry.value}/repo/npm-virtual/
//${host.value}/repo/npm-virtual/:_authToken=YOUR_TOKEN_HERE</code></pre>
    `,
    maven: `
      <pre><code>&lt;mirror&gt;
  &lt;id&gt;moonlight&lt;/id&gt;
  &lt;mirrorOf&gt;central&lt;/mirrorOf&gt;
  &lt;url&gt;${registry.value}/repo/maven-virtual/&lt;/url&gt;
&lt;/mirror&gt;</code></pre>
    `,
    pypi: `
      <pre><code>[global]
index-url = ${registry.value}/repo/pypi-virtual/simple/
trusted-host = ${host.value}</code></pre>
    `,
    go: `
      <pre><code>export GOPROXY=${registry.value}/go,https://proxy.golang.org,direct
export GOSUMDB=off</code></pre>
    `,
    nuget: `
      <pre><code>nuget sources add -name moonlight \\
  -source ${registry.value}/nuget/v3/index.json</code></pre>
    `
  }
  return configs[selectedManager.value]
})

const verifyCommand = computed(() => {
  const commands: Record<string, string> = {
    npm: 'npm config list',
    maven: 'mvn help:effective-settings',
    pypi: 'pip config list',
    go: 'go env GOPROXY',
    nuget: 'nuget sources list'
  }
  return commands[selectedManager.value]
})

const npmConfig = computed(() => {
  return `registry=${registry.value}/repo/npm-virtual/
//${host.value}/repo/npm-virtual/:_authToken=YOUR_TOKEN_HERE`
})

const downloadTemplate = (filename: string) => {
  window.open(`/docs/templates/${filename}`, '_blank')
}
</script>

<style scoped>
.public-help-page {
  max-width: 1000px;
  margin: 0 auto;
  padding: var(--spacing-5);
}

.help-hero {
  text-align: center;
  padding: 40px 20px;
  background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
  border-radius: 8px;
  color: white;
  margin-bottom: 30px;
}

.help-hero h1 {
  margin: 0 0 10px 0;
  font-size: 32px;
}

.help-hero p {
  margin: 0;
  font-size: 16px;
  opacity: 0.9;
}

.help-content {
  background: white;
  border-radius: 8px;
  box-shadow: 0 2px 12px rgba(0, 0, 0, 0.1);
}

.tab-content {
  padding: var(--spacing-5);
}

.step-content {
  margin-top: 30px;
  padding: var(--spacing-5);
  background: #f5f7fa;
  border-radius: 4px;
}

.step-content h3 {
  margin-top: 0;
  margin-bottom: 15px;
}

.step-content ol {
  padding-left: 20px;
}

.step-content li {
  margin: 8px 0;
  line-height: 1.6;
}

.config-example {
  margin-top: 20px;
  padding: 15px;
  background: white;
  border-radius: 4px;
}

.config-example h4 {
  margin-top: 0;
  margin-bottom: 10px;
}

.config-example pre {
  background: #f5f7fa;
  padding: 10px;
  border-radius: 4px;
  overflow-x: auto;
}

.config-example code {
  font-family: 'Courier New', monospace;
  font-size: 13px;
}

a {
  color: #409eff;
  text-decoration: none;
}

a:hover {
  text-decoration: underline;
}
</style>
