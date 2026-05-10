<template>
  <div class="about-page">
    <div class="about-hero">
      <div class="hero-icon">
        <svg viewBox="0 0 80 80" xmlns="http://www.w3.org/2000/svg">
          <defs>
            <linearGradient id="moonlight-about" x1="0%" y1="0%" x2="100%" y2="100%">
              <stop offset="0%" style="stop-color:#c4b5fd" />
              <stop offset="100%" style="stop-color:#8b5cf6" />
            </linearGradient>
          </defs>
          <circle cx="40" cy="40" r="24" fill="url(#moonlight-about)"/>
          <circle cx="48" cy="32" r="20" fill="#fff"/>
          <circle cx="20" cy="20" r="3" fill="#c4b5fd"/>
          <circle cx="64" cy="28" r="2" fill="#c4b5fd" opacity="0.6"/>
          <circle cx="56" cy="60" r="2.4" fill="#c4b5fd" opacity="0.4"/>
        </svg>
      </div>
      <h1>Moonlight Registry</h1>
      <p class="version">版本 1.0.0</p>
    </div>

    <div class="about-content">
      <el-tabs v-model="activeTab" type="card" class="about-tabs">
        <el-tab-pane label="产品介绍" name="intro">
          <div class="intro-content">
            <div class="intro-card">
              <div class="intro-icon">
                <i class="fa-solid fa-cube"></i>
              </div>
              <div class="intro-text">
                <h3>统一的多语言制品管理</h3>
                <p>支持 NPM、Maven、PyPI、Go Modules、YUM、APT 等多种包管理器的制品管理，一个平台满足您所有的制品管理需求。</p>
              </div>
            </div>

            <div class="intro-card">
              <div class="intro-icon">
                <i class="fa-solid fa-bolt"></i>
              </div>
              <div class="intro-text">
                <h3>高性能代理访问</h3>
                <p>支持代理访问远程仓库，内置缓存加速，智能故障转移，显著提升团队开发效率。</p>
              </div>
            </div>

            <div class="intro-card">
              <div class="intro-icon">
                <i class="fa-solid fa-shield-halved"></i>
              </div>
              <div class="intro-text">
                <h3>安全可靠的访问控制</h3>
                <p>基于角色的权限管理（RBAC），支持 CAS 单点登录，细粒度的仓库访问控制，保障您的制品安全。</p>
              </div>
            </div>

            <div class="intro-card">
              <div class="intro-icon">
                <i class="fa-solid fa-virus-slash"></i>
              </div>
              <div class="intro-text">
                <h3>漏洞扫描防护</h3>
                <p>包上传自动扫描，支持拦截高危漏洞包，保障您的供应链安全。</p>
              </div>
            </div>

            <div class="intro-card">
              <div class="intro-icon">
                <i class="fa-solid fa-robot"></i>
              </div>
              <div class="intro-text">
                <h3>AI 智能助手</h3>
                <p>集成大语言模型，支持日志查询、数据库查询、包信息查询、安全分析等工具调用。</p>
              </div>
            </div>

            <div class="intro-card">
              <div class="intro-icon">
                <i class="fa-solid fa-chart-line"></i>
              </div>
              <div class="intro-text">
                <h3>全面的操作审计</h3>
                <p>记录所有操作日志，支持审计追踪，Prometheus 指标采集，满足企业合规要求。</p>
              </div>
            </div>
          </div>
        </el-tab-pane>

        <el-tab-pane label="技术特性" name="tech">
          <div class="tech-grid">
            <div class="tech-item">
              <div class="tech-icon">
                <i class="fa-solid fa-server"></i>
              </div>
              <h4>分布式架构</h4>
              <p>支持集群部署横向扩展，轻松应对高并发访问场景</p>
            </div>
            <div class="tech-item">
              <div class="tech-icon">
                <i class="fa-solid fa-database"></i>
              </div>
              <h4>多种存储后端</h4>
              <p>支持本地存储、S3 兼容对象存储，灵活适配不同基础设施</p>
            </div>
            <div class="tech-item">
              <div class="tech-icon">
                <i class="fa-solid fa-layer-group"></i>
              </div>
              <h4>仓库分组</h4>
              <p>支持虚拟仓库组，聚合多个托管仓库，简化客户端配置</p>
            </div>
            <div class="tech-item">
              <div class="tech-icon">
                <i class="fa-solid fa-clock-rotate-left"></i>
              </div>
              <h4>智能缓存</h4>
              <p>自动缓存远程制品，加速重复下载，降低外部依赖风险</p>
            </div>
            <div class="tech-item">
              <div class="tech-icon">
                <i class="fa-solid fa-ban"></i>
              </div>
              <h4>访问阻断</h4>
              <p>基于包名、版本、域名等规则的访问控制，有效阻止潜在威胁</p>
            </div>
            <div class="tech-item">
              <div class="tech-icon">
                <i class="fa-solid fa-code-branch"></i>
              </div>
              <h4>数据迁移</h4>
              <p>支持从 Nexus Repository Manager 等主流仓库平滑迁移数据</p>
            </div>
            <div class="tech-item">
              <div class="tech-icon">
                <i class="fa-solid fa-rotate-right"></i>
              </div>
              <h4>配置备份</h4>
              <p>支持系统和仓库配置的备份与恢复，保障配置安全</p>
            </div>
          </div>
        </el-tab-pane>

        <el-tab-pane label="技术栈" name="stack">
          <div class="stack-section">
            <h3>后端技术</h3>
            <div class="stack-tags">
              <el-tag type="info" effect="plain">Go 1.26+</el-tag>
              <el-tag type="info" effect="plain">Gin Framework</el-tag>
              <el-tag type="info" effect="plain">GORM</el-tag>
              <el-tag type="info" effect="plain">SQLite / PostgreSQL</el-tag>
              <el-tag type="info" effect="plain">JWT</el-tag>
            </div>
          </div>
          <div class="stack-section">
            <h3>前端技术</h3>
            <div class="stack-tags">
              <el-tag type="info" effect="plain">Vue 3</el-tag>
              <el-tag type="info" effect="plain">TypeScript</el-tag>
              <el-tag type="info" effect="plain">Vite</el-tag>
              <el-tag type="info" effect="plain">Element Plus</el-tag>
              <el-tag type="info" effect="plain">Pinia</el-tag>
            </div>
          </div>
          <div class="stack-section">
            <h3>基础设施</h3>
            <div class="stack-tags">
              <el-tag type="info" effect="plain">Docker</el-tag>
              <el-tag type="info" effect="plain">S3 Compatible Storage</el-tag>
              <el-tag type="info" effect="plain">Linux/macOS/Windows</el-tag>
            </div>
          </div>
        </el-tab-pane>
      </el-tabs>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'

const activeTab = ref('intro')
</script>

<style scoped>
.about-page {
  min-height: 100vh;
  background: linear-gradient(180deg, #f8fafc 0%, #f1f5f9 100%);
  padding: 32px 24px;
}

.about-hero {
  background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
  border-radius: 16px;
  padding: 48px 24px;
  text-align: center;
  margin-bottom: 24px;
  box-shadow: 0 8px 32px rgba(102, 126, 234, 0.3);
}

.hero-icon svg {
  width: 100px;
  height: 100px;
  margin-bottom: 16px;
}

.about-hero h1 {
  margin: 0;
  font-size: 36px;
  font-weight: 700;
  color: white;
  letter-spacing: -0.5px;
}

.version {
  margin: 8px 0 0;
  font-size: 14px;
  color: rgba(255, 255, 255, 0.8);
  background: rgba(255, 255, 255, 0.15);
  display: inline-block;
  padding: 4px 16px;
  border-radius: 20px;
}

.about-content {
  background: white;
  border-radius: 16px;
  box-shadow: 0 4px 20px rgba(0, 0, 0, 0.08);
  overflow: hidden;
}

.about-tabs {
  margin: 0;
}

.about-tabs :deep(.el-tabs__header) {
  background: #f8fafc;
  border-bottom: 1px solid #e2e8f0;
  padding: 0 24px;
  margin: 0;
}

.about-tabs :deep(.el-tabs__item) {
  font-size: 15px;
  font-weight: 500;
  color: #64748b;
  padding: 16px 24px;
  margin-right: 8px;
}

.about-tabs :deep(.el-tabs__item.is-active) {
  color: #8b5cf6;
  font-weight: 600;
}

.about-tabs :deep(.el-tabs__content) {
  padding: 0;
}

.intro-content {
  padding: 24px;
  display: flex;
  flex-direction: column;
  gap: 20px;
}

.intro-card {
  display: flex;
  align-items: flex-start;
  gap: 20px;
  padding: 24px;
  background: #f8fafc;
  border-radius: 12px;
  border: 1px solid #e2e8f0;
  transition: all 0.2s ease;
}

.intro-card:hover {
  border-color: #8b5cf6;
  box-shadow: 0 4px 12px rgba(139, 92, 246, 0.1);
}

.intro-icon {
  width: 56px;
  height: 56px;
  background: linear-gradient(135deg, #8b5cf6 0%, #7c3aed 100%);
  border-radius: 14px;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 24px;
  color: white;
  flex-shrink: 0;
}

.intro-text h3 {
  margin: 0 0 8px;
  font-size: 18px;
  font-weight: 600;
  color: #1e293b;
}

.intro-text p {
  margin: 0;
  font-size: 14px;
  color: #64748b;
  line-height: 1.6;
}

.tech-grid {
  padding: 24px;
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(280px, 1fr));
  gap: 20px;
}

.tech-item {
  padding: 24px;
  background: #f8fafc;
  border-radius: 12px;
  border: 1px solid #e2e8f0;
  text-align: center;
  transition: all 0.2s ease;
}

.tech-item:hover {
  border-color: #8b5cf6;
  box-shadow: 0 4px 12px rgba(139, 92, 246, 0.1);
}

.tech-icon {
  width: 48px;
  height: 48px;
  background: linear-gradient(135deg, #8b5cf6 0%, #7c3aed 100%);
  border-radius: 12px;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 20px;
  color: white;
  margin: 0 auto 16px;
}

.tech-item h4 {
  margin: 0 0 8px;
  font-size: 16px;
  font-weight: 600;
  color: #1e293b;
}

.tech-item p {
  margin: 0;
  font-size: 13px;
  color: #64748b;
  line-height: 1.5;
}

.stack-section {
  padding: 24px;
  border-bottom: 1px solid #e2e8f0;
}

.stack-section:last-child {
  border-bottom: none;
}

.stack-section h3 {
  margin: 0 0 16px;
  font-size: 16px;
  font-weight: 600;
  color: #1e293b;
}

.stack-tags {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
}

.stack-tags :deep(.el-tag) {
  padding: 8px 16px;
  font-size: 13px;
  font-weight: 500;
  border-radius: 8px;
}
</style>
