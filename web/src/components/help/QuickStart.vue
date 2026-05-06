<template>
  <div class="quick-start">
    <div class="section-header">
      <h2>🚀 快速开始</h2>
      <p class="section-desc">按照以下步骤配置您的客户端，开始使用 Moonlight Registry</p>
    </div>

    <el-steps :active="activeStep" finish-status="success" align-center class="steps-container">
      <el-step title="配置认证" description="设置用户名密码" />
      <el-step title="选择包管理器" description="配置客户端工具" />
      <el-step title="开始使用" description="验证配置并使用" />
    </el-steps>

    <div class="step-content-wrapper" v-if="activeStep === 0">
      <div class="step-card">
        <div class="step-icon">🔐</div>
        <h3>配置认证信息</h3>
        <p>根据您使用的包管理器，配置相应的认证方式：</p>
        <el-timeline>
          <el-timeline-item>
            <template #dot><i class="fa-solid fa-key"></i></template>
            <p><strong>NPM/PyPI/NuGet：</strong>使用您的账号密码进行认证</p>
          </el-timeline-item>
          <el-timeline-item>
            <template #dot><i class="fa-solid fa-file-code"></i></template>
            <p><strong>Maven：</strong>在 settings.xml 中配置 server 信息</p>
          </el-timeline-item>
          <el-timeline-item>
            <template #dot><i class="fa-solid fa-globe"></i></template>
            <p><strong>Go：</strong>通过 GOPROXY 配置，无需额外认证</p>
          </el-timeline-item>
        </el-timeline>
        <el-alert type="info" :closable="false" style="margin-top: 20px">
          <template #title>提示</template>
          <p>访问令牌功能即将推出，当前版本请使用用户名密码进行认证</p>
        </el-alert>
        <div class="step-actions">
          <el-button type="primary" size="large" @click="activeStep = 1">
            下一步
            <i class="fa-solid fa-arrow-right"></i>
          </el-button>
        </div>
      </div>
    </div>

    <div class="step-content-wrapper" v-if="activeStep === 1">
      <div class="step-card">
        <div class="step-icon">📦</div>
        <h3>选择您的包管理器</h3>
        <el-radio-group v-model="selectedManager" size="large" class="manager-selector">
          <el-radio-button label="npm">
            <i class="fa-brands fa-npm"></i>
            <span>NPM</span>
          </el-radio-button>
          <el-radio-button label="maven">
            <i class="fa-brands fa-java"></i>
            <span>Maven</span>
          </el-radio-button>
          <el-radio-button label="pypi">
            <i class="fa-brands fa-python"></i>
            <span>PyPI</span>
          </el-radio-button>
          <el-radio-button label="go">
            <i class="fa-brands fa-golang"></i>
            <span>Go</span>
          </el-radio-button>
          <el-radio-button label="nuget">
            <i class="fa-solid fa-box"></i>
            <span>NuGet</span>
          </el-radio-button>
        </el-radio-group>

        <div class="manager-config" v-if="selectedManager">
          <h4>{{ managerTitle }} 配置</h4>
          <div v-html="managerConfig"></div>
        </div>
        <div class="step-actions">
          <el-button size="large" @click="activeStep = 0">
            <i class="fa-solid fa-arrow-left"></i>
            上一步
          </el-button>
          <el-button type="primary" size="large" @click="activeStep = 2">
            下一步
            <i class="fa-solid fa-arrow-right"></i>
          </el-button>
        </div>
      </div>
    </div>

    <div class="step-content-wrapper" v-if="activeStep === 2">
      <div class="step-card final-step">
        <div class="success-icon">🎉</div>
        <h3>配置完成！</h3>
        <p class="success-desc">您现在可以开始使用 Moonlight Registry 仓库了</p>
        
        <div class="next-steps">
          <h4>下一步操作</h4>
          <ul>
            <li><i class="fa-solid fa-arrow-right"></i> 浏览仓库查找可用的软件包</li>
            <li><i class="fa-solid fa-arrow-right"></i> 发布您的第一个包</li>
            <li><i class="fa-solid fa-arrow-right"></i> 配置 CI/CD 集成</li>
          </ul>
        </div>

        <div class="action-buttons">
          <el-button type="primary" size="large" @click="$router.push('/')">
            <i class="fa-solid fa-search"></i>
            浏览仓库
          </el-button>
          <el-button size="large" @click="downloadConfig">
            <i class="fa-solid fa-download"></i>
            下载配置文件
          </el-button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'

const activeStep = ref(0)
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
  padding: 48px 56px;
  min-height: 100%;
  max-width: 1200px;
  margin: 0 auto;
}

.section-header {
  text-align: center;
  margin-bottom: 48px;
}

.section-header h2 {
  font-size: 28px;
  font-weight: 700;
  color: #0f172a;
  margin-bottom: 12px;
  letter-spacing: -0.5px;
}

.section-desc {
  color: #64748b;
  font-size: 16px;
  line-height: 1.7;
  max-width: 600px;
  margin: 0 auto;
}

.steps-container {
  margin-bottom: 48px;
  padding: 32px 40px;
  background: linear-gradient(135deg, #f8fafc 0%, #f1f5f9 100%);
  border-radius: 16px;
  border: 1px solid #e2e8f0;
}

.step-content-wrapper {
  animation: fadeIn 0.3s ease;
}

@keyframes fadeIn {
  from {
    opacity: 0;
    transform: translateY(10px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}

.step-card {
  background: #fff;
  border-radius: 16px;
  padding: 40px;
  box-shadow: 0 4px 16px rgba(0, 0, 0, 0.06);
  border: 1px solid #e2e8f0;
}

.step-icon {
  font-size: 48px;
  margin-bottom: 20px;
}

.step-card h3 {
  font-size: 22px;
  font-weight: 700;
  color: #0f172a;
  margin-bottom: 16px;
  letter-spacing: -0.3px;
}

.step-card p {
  color: #475569;
  font-size: 15px;
  line-height: 1.7;
  margin: 0 0 20px;
}

.step-card :deep(.el-timeline) {
  padding-left: 8px;
  margin: 24px 0;
}

.step-card :deep(.el-timeline-item__node) {
  width: 32px;
  height: 32px;
  left: -4px;
}

.step-card :deep(.el-timeline-item__node i) {
  font-size: 14px;
}

.step-card :deep(.el-timeline-item__content) {
  color: #334155;
  font-size: 15px;
  line-height: 1.7;
  padding-bottom: 8px;
}

.step-card :deep(.el-timeline-item__content p) {
  margin: 0;
}

.step-card :deep(.el-timeline-item__content strong) {
  color: #0f172a;
  font-weight: 600;
}

.step-card :deep(.el-alert) {
  border-radius: 10px;
  padding: 16px 20px;
}

.step-card :deep(.el-alert__title) {
  font-size: 14px;
  font-weight: 600;
}

.step-actions {
  display: flex;
  gap: 16px;
  justify-content: center;
  margin-top: 32px;
}

.step-actions :deep(.el-button) {
  padding: 12px 28px;
  font-size: 15px;
  font-weight: 500;
  border-radius: 10px;
}

.manager-selector {
  display: flex;
  flex-wrap: wrap;
  gap: 16px;
  margin: 32px 0;
  padding: 0;
  background: transparent;
  border-radius: 0;
}

.manager-selector :deep(.el-radio-button__inner) {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 14px 24px;
  border-radius: 10px;
  font-size: 15px;
  font-weight: 500;
  border: 1px solid #e2e8f0;
  transition: all 0.2s ease;
  background: #fff;
}

.manager-selector :deep(.el-radio-button__inner i) {
  font-size: 18px;
}

.manager-selector :deep(.el-radio-button__inner:hover) {
  border-color: #2563eb;
  color: #2563eb;
  background: #eff6ff;
}

.manager-selector :deep(.el-radio-button__original-radio:checked + .el-radio-button__inner) {
  background: linear-gradient(135deg, #2563eb 0%, #1d4ed8 100%);
  border-color: #2563eb;
  color: #fff;
  box-shadow: 0 4px 12px rgba(37, 99, 235, 0.25);
}

.manager-config {
  margin-top: 32px;
  padding: 32px;
  background: linear-gradient(135deg, #f8fafc 0%, #f1f5f9 100%);
  border-radius: 12px;
  border: 1px solid #e2e8f0;
}

.manager-config h4 {
  font-size: 17px;
  font-weight: 700;
  color: #0f172a;
  margin: 0 0 20px;
}

.manager-config p {
  margin: 16px 0;
  color: #475569;
  font-size: 15px;
  line-height: 1.7;
}

.manager-config pre {
  background: #0f172a;
  color: #e2e8f0;
  padding: 20px;
  border-radius: 10px;
  overflow-x: auto;
  font-family: 'SF Mono', 'Fira Code', 'Consolas', monospace;
  font-size: 14px;
  line-height: 1.7;
  margin: 16px 0;
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.1);
}

.manager-config code {
  font-family: 'SF Mono', 'Fira Code', 'Consolas', monospace;
}

.manager-config :deep(.el-button) {
  margin-top: 12px;
  border-radius: 8px;
  padding: 10px 20px;
  font-weight: 500;
}

.final-step {
  text-align: center;
}

.success-icon {
  font-size: 72px;
  margin-bottom: 20px;
}

.success-desc {
  font-size: 17px;
  color: #475569;
  margin-bottom: 32px;
  line-height: 1.6;
}

.next-steps {
  background: linear-gradient(135deg, #f0fdf4 0%, #dcfce7 100%);
  border-radius: 12px;
  padding: 28px 32px;
  margin-bottom: 32px;
  text-align: left;
  border: 1px solid #bbf7d0;
}

.next-steps h4 {
  font-size: 17px;
  font-weight: 700;
  color: #166534;
  margin: 0 0 16px;
}

.next-steps ul {
  list-style: none;
  padding: 0;
  margin: 0;
}

.next-steps li {
  padding: 10px 0;
  color: #15803d;
  font-size: 15px;
  line-height: 1.7;
}

.next-steps li i {
  margin-right: 10px;
  font-size: 14px;
}

.action-buttons {
  display: flex;
  gap: 16px;
  justify-content: center;
}

.action-buttons :deep(.el-button) {
  padding: 14px 32px;
  font-size: 15px;
  font-weight: 600;
  border-radius: 10px;
  transition: all 0.2s ease;
}

.action-buttons :deep(.el-button--primary) {
  background: linear-gradient(135deg, #2563eb 0%, #1d4ed8 100%);
  border: none;
  box-shadow: 0 4px 12px rgba(37, 99, 235, 0.25);
}

.action-buttons :deep(.el-button--primary:hover) {
  transform: translateY(-2px);
  box-shadow: 0 6px 16px rgba(37, 99, 235, 0.3);
}

@media (max-width: 768px) {
  .quick-start {
    padding: 24px;
  }
  
  .step-card {
    padding: 24px;
  }
  
  .manager-selector {
    flex-direction: column;
  }
  
  .action-buttons {
    flex-direction: column;
  }
}
</style>
