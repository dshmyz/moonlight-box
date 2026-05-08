<template>
  <div class="public-help-page">
    <div class="help-hero">
      <div class="hero-content">
        <div class="hero-icon">
          <i class="fa-solid fa-book-open"></i>
        </div>
        <h1>帮助中心</h1>
        <p>快速配置您的客户端，开始使用 Moonlight Registry</p>
      </div>
    </div>

    <div class="help-content">
      <el-tabs v-model="activeTab" type="card" class="help-tabs">
        <el-tab-pane label="🚀 快速开始" name="quickstart">
          <div class="tab-content">
            <div class="steps-container">
              <el-steps :active="activeStep" finish-status="success" align-center class="steps-wrapper">
                <el-step title="配置认证" description="设置用户名密码" />
                <el-step title="配置客户端" description="选择包管理器并配置" />
                <el-step title="开始使用" description="验证配置并使用" />
              </el-steps>
            </div>

            <div class="step-card" v-if="activeStep === 0">
              <div class="step-header">
                <div class="step-number">01</div>
                <div>
                  <h3>配置认证信息</h3>
                  <p class="step-desc">根据您使用的包管理器，配置相应的认证方式</p>
                </div>
              </div>
              <div class="auth-timeline">
                <div class="timeline-item">
                  <div class="timeline-icon">
                    <i class="fa-solid fa-key"></i>
                  </div>
                  <div class="timeline-content">
                    <strong>NPM/PyPI</strong>
                    <p>使用您的账号密码进行认证</p>
                  </div>
                </div>
                <div class="timeline-item">
                  <div class="timeline-icon">
                    <i class="fa-solid fa-file-code"></i>
                  </div>
                  <div class="timeline-content">
                    <strong>Maven</strong>
                    <p>在 settings.xml 中配置 server 信息</p>
                  </div>
                </div>
                <div class="timeline-item">
                  <div class="timeline-icon">
                    <i class="fa-solid fa-globe"></i>
                  </div>
                  <div class="timeline-content">
                    <strong>Go</strong>
                    <p>通过 GOPROXY 配置，无需额外认证</p>
                  </div>
                </div>
              </div>
              <div class="info-alert">
                <i class="fa-solid fa-info-circle"></i>
                <span>访问令牌功能即将推出，当前版本请使用用户名密码进行认证</span>
              </div>
              <div class="step-actions">
                <el-button type="primary" size="large" @click="activeStep = 1">
                  <i class="fa-solid fa-arrow-right"></i>
                  下一步
                </el-button>
              </div>
            </div>

            <div class="step-card" v-if="activeStep === 1">
              <div class="step-header">
                <div class="step-number">02</div>
                <div>
                  <h3>配置客户端</h3>
                  <p class="step-desc">选择您的包管理器并配置</p>
                </div>
              </div>
              
              <div class="manager-selector">
                <el-radio-group v-model="selectedManager" class="manager-group">
                  <el-radio-button label="npm" class="manager-option">
                    <i class="fa-solid fa-box"></i>
                    <span>NPM</span>
                  </el-radio-button>
                  <el-radio-button label="maven" class="manager-option">
                    <i class="fa-solid fa-box"></i>
                    <span>Maven</span>
                  </el-radio-button>
                  <el-radio-button label="pypi" class="manager-option">
                    <i class="fa-solid fa-code"></i>
                    <span>PyPI</span>
                  </el-radio-button>
                  <el-radio-button label="go" class="manager-option">
                    <i class="fa-brands fa-golang"></i>
                    <span>Go</span>
                  </el-radio-button>
                  <el-radio-button label="yum" class="manager-option">
                    <i class="fa-solid fa-server"></i>
                    <span>Yum/APT</span>
                  </el-radio-button>
                </el-radio-group>
              </div>

              <div class="config-card" v-if="selectedManager">
                <h4 class="config-title">
                  <i class="fa-solid fa-code"></i>
                  {{ managerTitle }} 配置示例
                </h4>
                <div class="config-content" v-html="managerConfig"></div>
              </div>

              <div class="step-actions">
                <el-button size="large" @click="activeStep = 0">
                  <i class="fa-solid fa-arrow-left"></i>
                  上一步
                </el-button>
                <el-button type="primary" size="large" @click="activeStep = 2">
                  <i class="fa-solid fa-arrow-right"></i>
                  下一步
                </el-button>
              </div>
            </div>

            <div class="step-card success-card" v-if="activeStep === 2">
              <div class="step-header">
                <div class="step-number success">03</div>
                <div>
                  <h3>验证配置</h3>
                  <p class="step-desc">运行以下命令验证您的配置</p>
                </div>
              </div>
              
              <div class="verify-command">
                <code-block :code="verifyCommand" />
              </div>
              
              <div class="success-result">
                <div class="success-icon">
                  <i class="fa-solid fa-check-circle"></i>
                </div>
                <h4>配置完成！</h4>
                <p>您现在可以开始使用仓库了</p>
                <el-button type="primary" size="large" @click="$router.push('/')">
                  <i class="fa-solid fa-rocket"></i>
                  浏览仓库
                </el-button>
              </div>
            </div>
          </div>
        </el-tab-pane>

        <el-tab-pane label="📖 配置指南" name="guide">
          <div class="tab-content">
            <div class="guide-intro">
              <i class="fa-solid fa-file-text"></i>
              <div>
                <h3>详细配置说明</h3>
                <p>根据您使用的包管理器，查看对应的配置指南</p>
              </div>
            </div>

            <div class="guide-cards">
              <div class="guide-card" v-for="guide in guides" :key="guide.name">
                <div class="guide-icon" :style="{ background: guide.color }">
                  <i :class="guide.icon"></i>
                </div>
                <div class="guide-info">
                  <h4>{{ guide.title }}</h4>
                  <p class="guide-desc">{{ guide.description }}</p>
                  <code>{{ guide.file }}</code>
                </div>
                <div class="guide-actions">
                  <el-button size="small" type="primary" @click="downloadTemplate(guide.file)">
                    <i class="fa-solid fa-download"></i>
                    下载模板
                  </el-button>
                </div>
              </div>
            </div>
          </div>
        </el-tab-pane>

        <el-tab-pane label="❓ 常见问题" name="faq">
          <div class="tab-content">
            <div class="search-wrapper">
              <el-input
                v-model="searchQuery"
                placeholder="搜索问题..."
                prefix-icon="Search"
                clearable
                size="large"
                class="faq-search"
              />
            </div>

            <el-collapse v-model="activeFaq" accordion class="faq-collapse">
              <el-collapse-item
                v-for="item in filteredFaqs"
                :key="item.name"
                :title="item.title"
                :name="item.name"
                class="faq-item"
              >
                <div class="faq-content">
                  <div v-html="item.content"></div>
                </div>
              </el-collapse-item>
            </el-collapse>

            <div class="contact-section">
              <i class="fa-solid fa-message-circle"></i>
              <div>
                <p>没有找到答案？</p>
                <a href="mailto:admin@company.com" class="contact-link">
                  联系管理员：admin@company.com
                </a>
              </div>
            </div>
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
const activeFaq = ref('')

const managerTitle = computed(() => {
  const titles: Record<string, string> = {
    npm: 'NPM',
    maven: 'Maven',
    pypi: 'PyPI',
    go: 'Go',
    yum: 'Yum/APT'
  }
  return titles[selectedManager.value]
})

const registry = computed(() => window.location.origin)
const host = computed(() => window.location.host)

const managerConfig = computed(() => {
  const configs: Record<string, string> = {
    npm: `
      <div class="code-block-wrapper">
        <pre><code>registry=${registry.value}/repo/npm-virtual/</code></pre>
      </div>
      <div class="config-note">
        <i class="fa-solid fa-info-circle"></i>
        <span>运行 <code>npm login</code> 并输入您的账号密码进行认证</span>
      </div>
    `,
    maven: `
      <div class="code-block-wrapper">
        <pre><code>&lt;mirror&gt;
  &lt;id&gt;moonlight&lt;/id&gt;
  &lt;mirrorOf&gt;central&lt;/mirrorOf&gt;
  &lt;url&gt;${registry.value}/repo/maven-virtual/&lt;/url&gt;
&lt;/mirror&gt;</code></pre>
      </div>
      <div class="config-note">
        <i class="fa-solid fa-info-circle"></i>
        <span>在 settings.xml 的 servers 节点中配置用户名密码</span>
      </div>
    `,
    pypi: `
      <div class="code-block-wrapper">
        <pre><code>[global]
index-url = ${registry.value}/repo/pypi-virtual/simple/
trusted-host = ${host.value}</code></pre>
      </div>
      <div class="config-note">
        <i class="fa-solid fa-info-circle"></i>
        <span>使用 pip config 设置，或在安装时使用 --index-url 参数</span>
      </div>
    `,
    go: `
      <div class="code-block-wrapper">
        <pre><code>export GOPROXY=${registry.value}/go,https://proxy.golang.org,direct
export GOSUMDB=off</code></pre>
      </div>
      <div class="config-note">
        <i class="fa-solid fa-info-circle"></i>
        <span>Go 模块代理无需额外认证</span>
      </div>
    `,
    yum: `
      <div class="code-block-wrapper">
        <pre><code># CentOS/RHEL (Yum)
cat > /etc/yum.repos.d/moonlight.repo << EOF
[moonlight]
name=Moonlight Repository
baseurl=${registry.value}/yum/
enabled=1
gpgcheck=0
EOF

# Debian/Ubuntu (APT)
echo "deb [trusted=yes] ${registry.value}/apt/ /" > /etc/apt/sources.list.d/moonlight.list
apt-get update</code></pre>
      </div>
      <div class="config-note">
        <i class="fa-solid fa-info-circle"></i>
        <span>根据您的操作系统选择对应的配置方式</span>
      </div>
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
    yum: 'yum repolist | grep moonlight'
  }
  return commands[selectedManager.value]
})

const guides = [
  {
    name: 'npm',
    title: 'NPM 配置',
    description: '配置 npm 使用私有仓库',
    file: '.npmrc',
    icon: 'fa-solid fa-box',
    color: 'linear-gradient(135deg, #CB3837 0%, #911F27 100%)'
  },
  {
    name: 'maven',
    title: 'Maven 配置',
    description: '配置 Maven 镜像和认证',
    file: 'settings.xml',
    icon: 'fa-solid fa-file-xml',
    color: 'linear-gradient(135deg, #C71A36 0%, #8B0000 100%)'
  },
  {
    name: 'pypi',
    title: 'PyPI 配置',
    description: '配置 pip 使用私有索引',
    file: 'pip.conf',
    icon: 'fa-solid fa-code',
    color: 'linear-gradient(135deg, #3776AB 0%, #2E68A6 100%)'
  },
  {
    name: 'go',
    title: 'Go 配置',
    description: '配置 GOPROXY 环境变量',
    file: 'go-env.sh',
    icon: 'fa-solid fa-file-code',
    color: 'linear-gradient(135deg, #00ADD8 0%, #007D9C 100%)'
  },
  {
    name: 'yum',
    title: 'Yum/APT 配置',
    description: '配置 Yum 或 APT 使用私有仓库',
    file: 'repo.conf',
    icon: 'fa-solid fa-server',
    color: 'linear-gradient(135deg, #FB8C00 0%, #E65100 100%)'
  }
]

const faqs = [
  {
    name: 'auth',
    title: '如何进行认证配置？',
    content: `
      <p>当前版本使用用户名密码进行认证：</p>
      <ul>
        <li><strong>NPM/PyPI：</strong>使用您的账号密码进行认证</li>
        <li><strong>Maven：</strong>在 settings.xml 中配置 server 信息</li>
        <li><strong>Go：</strong>通过 GOPROXY 配置，无需额外认证</li>
      </ul>
      <div class="info-note">
        <i class="fa-solid fa-info-circle"></i>
        <span>访问令牌功能即将推出</span>
      </div>
    `
  },
  {
    name: 'npm-adduser',
    title: 'npm adduser 不工作怎么办？',
    content: `
      <p>当前版本暂不支持 <code>npm adduser</code> 命令。</p>
      <p>请手动配置 <code>.npmrc</code> 文件，使用用户名密码认证。</p>
    `
  },
  {
    name: 'publish',
    title: '如何发布包？',
    content: `
      <div class="command-list">
        <div class="command-item">
          <span class="command-label">NPM:</span>
          <code>npm publish --registry=http://your-registry/repo/npm-local/</code>
        </div>
        <div class="command-item">
          <span class="command-label">Maven:</span>
          <code>mvn clean deploy</code>
        </div>
        <div class="command-item">
          <span class="command-label">PyPI:</span>
          <code>twine upload --repository-url http://your-registry/pypi/upload/ dist/*</code>
        </div>
      </div>
    `
  },
  {
    name: 'go-checksum',
    title: 'Go get 报错 checksum mismatch？',
    content: `
      <p>当前版本不支持校验和数据库，请禁用：</p>
      <div class="code-block-wrapper small">
        <pre><code>export GOSUMDB=off</code></pre>
      </div>
    `
  }
]

const filteredFaqs = computed(() => {
  if (!searchQuery.value) return faqs
  const query = searchQuery.value.toLowerCase()
  return faqs.filter(faq => 
    faq.title.toLowerCase().includes(query) || 
    faq.content.toLowerCase().includes(query)
  )
})

const downloadTemplate = (filename: string) => {
  window.open(`/docs/templates/${filename}`, '_blank')
}
</script>

<style scoped>
.public-help-page {
  min-height: 100vh;
  background: linear-gradient(180deg, #f8fafc 0%, #f1f5f9 100%);
  padding: 32px 24px;
}

.help-hero {
  background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
  border-radius: 16px;
  padding: 48px 24px;
  text-align: center;
  margin-bottom: 24px;
  box-shadow: 0 8px 32px rgba(102, 126, 234, 0.3);
}

.hero-content {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 16px;
}

.hero-icon {
  width: 72px;
  height: 72px;
  background: rgba(255, 255, 255, 0.2);
  border-radius: 20px;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 32px;
  color: white;
}

.help-hero h1 {
  margin: 0;
  font-size: 32px;
  font-weight: 700;
  color: white;
}

.help-hero p {
  margin: 0;
  font-size: 16px;
  color: rgba(255, 255, 255, 0.9);
}

.help-content {
  background: white;
  border-radius: 16px;
  box-shadow: 0 4px 20px rgba(0, 0, 0, 0.08);
  overflow: hidden;
}

.help-tabs {
  margin: 0;
}

.help-tabs :deep(.el-tabs__header) {
  background: #f8fafc;
  border-bottom: 1px solid #e2e8f0;
  padding: 0 24px;
}

.help-tabs :deep(.el-tabs__item) {
  font-size: 15px;
  font-weight: 500;
  color: #64748b;
  padding: 16px 24px;
  margin-right: 8px;
}

.help-tabs :deep(.el-tabs__item.is-active) {
  color: #8b5cf6;
  font-weight: 600;
}

.help-tabs :deep(.el-tabs__content) {
  padding: 0;
}

.tab-content {
  padding: 24px;
}

.steps-container {
  margin-bottom: 24px;
}

.steps-wrapper :deep(.el-steps__item) {
  padding: 0 16px;
}

.steps-wrapper :deep(.el-steps__title) {
  font-size: 14px;
  font-weight: 600;
  color: #475569;
}

.steps-wrapper :deep(.el-steps__description) {
  font-size: 12px;
  color: #94a3b8;
}

.step-card {
  background: #fff;
  border-radius: 16px;
  padding: 24px;
  box-shadow: 0 4px 16px rgba(0, 0, 0, 0.06);
}

.success-card {
  border: 1px solid #dcfce7;
  background: linear-gradient(135deg, #f0fdf4 0%, #fff 100%);
}

.step-header {
  display: flex;
  align-items: center;
  gap: 16px;
  margin-bottom: 24px;
  padding-bottom: 20px;
  border-bottom: 1px solid #e2e8f0;
}

.step-number {
  width: 40px;
  height: 40px;
  background: linear-gradient(135deg, #8b5cf6 0%, #7c3aed 100%);
  border-radius: 12px;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 18px;
  font-weight: 700;
  color: white;
}

.step-number.success {
  background: linear-gradient(135deg, #10b981 0%, #059669 100%);
}

.step-header h3 {
  margin: 0;
  font-size: 20px;
  font-weight: 600;
  color: #1e293b;
}

.step-desc {
  margin: 4px 0 0;
  font-size: 14px;
  color: #64748b;
}

.auth-timeline {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.timeline-item {
  display: flex;
  align-items: flex-start;
  gap: 16px;
  padding: 16px;
  background: #f8fafc;
  border-radius: 12px;
}

.timeline-icon {
  width: 44px;
  height: 44px;
  background: linear-gradient(135deg, #8b5cf6 0%, #7c3aed 100%);
  border-radius: 12px;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 20px;
  color: white;
  flex-shrink: 0;
}

.timeline-content strong {
  font-size: 14px;
  color: #1e293b;
}

.timeline-content p {
  margin: 4px 0 0;
  font-size: 13px;
  color: #64748b;
}

.info-alert {
  display: flex;
  align-items: center;
  gap: 8px;
  background: #fef3c7;
  border-radius: 8px;
  padding: 12px 16px;
  margin: 20px 0;
  font-size: 14px;
  color: #92400e;
}

.info-alert i {
  color: #f59e0b;
}

.step-actions {
  display: flex;
  gap: 12px;
  margin-top: 24px;
  justify-content: flex-end;
}

.step-actions :deep(.el-button) {
  display: flex;
  align-items: center;
  gap: 8px;
}

.manager-selector {
  margin-bottom: 24px;
}

.manager-group {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
}

.manager-option {
  padding: 12px 20px;
  border-radius: 10px;
  font-size: 14px;
}

.manager-option :deep(.el-radio-button__inner) {
  padding: 8px 16px;
  border-radius: 10px;
  font-weight: 500;
}

.manager-option :deep(.el-radio-button__inner.is-active) {
  background: #8b5cf6;
  border-color: #8b5cf6;
}

.config-card {
  background: #f8fafc;
  border-radius: 12px;
  padding: 20px;
  margin-bottom: 24px;
}

.config-title {
  display: flex;
  align-items: center;
  gap: 8px;
  margin: 0 0 16px;
  font-size: 16px;
  font-weight: 600;
  color: #1e293b;
}

.config-title i {
  color: #8b5cf6;
}

.config-content {
  font-size: 14px;
}

.code-block-wrapper {
  background: #1e293b;
  border-radius: 8px;
  padding: 16px;
  overflow-x: auto;
}

.code-block-wrapper pre {
  margin: 0;
}

.code-block-wrapper code {
  font-family: 'Fira Code', 'Consolas', monospace;
  font-size: 13px;
  color: #e2e8f0;
  line-height: 1.6;
}

.code-block-wrapper.small {
  padding: 12px;
}

.config-note {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 12px;
  font-size: 13px;
  color: #64748b;
}

.config-note i {
  color: #0ea5e9;
}

.config-note code {
  background: #e0f2fe;
  padding: 2px 6px;
  border-radius: 4px;
  color: #0369a1;
  font-family: 'Fira Code', monospace;
  font-size: 12px;
}

.verify-command {
  margin-bottom: 24px;
}

.success-result {
  text-align: center;
  padding: 32px;
  background: rgba(16, 185, 129, 0.05);
  border-radius: 12px;
}

.success-icon {
  width: 80px;
  height: 80px;
  background: linear-gradient(135deg, #10b981 0%, #059669 100%);
  border-radius: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
  margin: 0 auto 16px;
  font-size: 40px;
  color: white;
}

.success-result h4 {
  margin: 0 0 8px;
  font-size: 24px;
  font-weight: 600;
  color: #1e293b;
}

.success-result p {
  margin: 0 0 20px;
  font-size: 14px;
  color: #64748b;
}

.guide-intro {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 24px;
  padding: 16px;
  background: #f8fafc;
  border-radius: 12px;
}

.guide-intro i {
  font-size: 24px;
  color: #8b5cf6;
}

.guide-intro h3 {
  margin: 0;
  font-size: 16px;
  font-weight: 600;
  color: #1e293b;
}

.guide-intro p {
  margin: 4px 0 0;
  font-size: 13px;
  color: #64748b;
}

.guide-cards {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(280px, 1fr));
  gap: 16px;
}

.guide-card {
  background: #fff;
  border-radius: 12px;
  padding: 20px;
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.06);
  border: 1px solid #e2e8f0;
}

.guide-icon {
  width: 48px;
  height: 48px;
  border-radius: 12px;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 22px;
  color: white;
  margin-bottom: 16px;
}

.guide-info h4 {
  margin: 0 0 6px;
  font-size: 15px;
  font-weight: 600;
  color: #1e293b;
}

.guide-desc {
  margin: 0 0 12px;
  font-size: 13px;
  color: #64748b;
}

.guide-info code {
  background: #f1f5f9;
  padding: 4px 10px;
  border-radius: 4px;
  font-family: 'Fira Code', monospace;
  font-size: 12px;
  color: #7c3aed;
}

.guide-actions {
  margin-top: 16px;
}

.search-wrapper {
  max-width: 480px;
  margin: 0 auto 24px;
}

.faq-search :deep(.el-input__wrapper) {
  border-radius: 12px;
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.06);
}

.faq-collapse {
  border: none;
}

.faq-collapse :deep(.el-collapse-item__header) {
  font-size: 15px;
  font-weight: 600;
  color: #1e293b;
  background: #f8fafc;
  border-radius: 10px;
  margin-bottom: 10px;
  padding: 16px 20px;
  border: none;
}

.faq-collapse :deep(.el-collapse-item__header:hover) {
  background: #f1f5f9;
}

.faq-collapse :deep(.el-collapse-item__content) {
  background: transparent;
  padding: 0;
}

.faq-content {
  padding: 16px 20px;
  background: #fff;
  border-radius: 0 0 10px 10px;
  margin-top: -10px;
  margin-bottom: 10px;
}

.faq-content p {
  margin: 0 0 12px;
  font-size: 14px;
  color: #475569;
  line-height: 1.6;
}

.faq-content ul {
  margin: 8px 0 16px;
  padding-left: 20px;
}

.faq-content li {
  margin: 6px 0;
  font-size: 14px;
  color: #475569;
}

.faq-content code {
  background: #f1f5f9;
  padding: 3px 8px;
  border-radius: 4px;
  font-family: 'Fira Code', monospace;
  font-size: 13px;
  color: #7c3aed;
}

.info-note {
  display: flex;
  align-items: center;
  gap: 8px;
  background: #fef3c7;
  border-radius: 8px;
  padding: 12px;
  margin-top: 12px;
  font-size: 13px;
  color: #92400e;
}

.info-note i {
  color: #f59e0b;
}

.command-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.command-item {
  display: flex;
  align-items: center;
  gap: 12px;
}

.command-label {
  font-weight: 600;
  color: #475569;
  font-size: 14px;
  min-width: 60px;
}

.contact-section {
  display: flex;
  align-items: center;
  gap: 12px;
  background: #eff6ff;
  border-radius: 12px;
  padding: 16px 20px;
  margin-top: 24px;
}

.contact-section i {
  font-size: 24px;
  color: #3b82f6;
}

.contact-section p {
  margin: 0;
  font-size: 14px;
  color: #475569;
}

.contact-link {
  display: inline-block;
  margin-top: 4px;
  font-size: 14px;
  color: #3b82f6;
  text-decoration: none;
}

.contact-link:hover {
  text-decoration: underline;
}
</style>
