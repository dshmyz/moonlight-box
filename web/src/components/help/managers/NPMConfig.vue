<template>
  <div class="npm-config">
    <el-alert type="info" :closable="false">
      NPM 是 Node.js 的包管理器，用于安装、发布和管理 JavaScript 包
    </el-alert>
    <div class="config-methods">
      <div class="method-card">
        <div class="method-header">
          <div class="method-badge">推荐</div>
          <h4>方式一：配置文件</h4>
        </div>
        <p class="method-desc">通过编辑 <code>~/.npmrc</code> 文件进行配置，适合长期使用</p>
        
        <div class="method-steps">
          <div class="step-item">
            <div class="step-number">1</div>
            <div class="step-content">
              <h5>下载配置模板</h5>
              <el-button type="primary" @click="downloadTemplate('.npmrc')">
                <i class="fa-solid fa-download"></i>
                下载 .npmrc 模板
              </el-button>
            </div>
          </div>

          <div class="step-item">
            <div class="step-number">2</div>
            <div class="step-content">
              <h5>放置配置文件</h5>
              <p>将下载的文件放到 <code>~/.npmrc</code> 或项目根目录</p>
              <code-block :code="npmrcContent" title="~/.npmrc" />
            </div>
          </div>

          <div class="step-item">
            <div class="step-number">3</div>
            <div class="step-content">
              <h5>替换认证信息</h5>
              <p>将 <code>YOUR_TOKEN_HERE</code> 替换为您的访问令牌</p>
              <code-block code="sed -i 's/YOUR_TOKEN_HERE/your-actual-token/g' ~/.npmrc" />
            </div>
          </div>
        </div>
      </div>

      <div class="method-card">
        <div class="method-header">
          <h4>方式二：命令行配置</h4>
        </div>
        <p class="method-desc">通过命令行快速配置，适合临时使用</p>
        
        <div class="method-steps">
          <div class="step-item">
            <div class="step-number">1</div>
            <div class="step-content">
              <h5>设置仓库地址</h5>
              <code-block :code="`npm config set registry ${registryUrl}`" />
            </div>
          </div>

          <div class="step-item">
            <div class="step-number">2</div>
            <div class="step-content">
              <h5>设置认证信息</h5>
              <code-block :code="`npm config set //${host}/repo/npm-virtual/:_authToken YOUR_TOKEN_HERE`" />
            </div>
          </div>

          <div class="step-item">
            <div class="step-number">3</div>
            <div class="step-content">
              <h5>验证配置</h5>
              <code-block code="npm config list" />
            </div>
          </div>
        </div>
      </div>

      <div class="method-card">
        <div class="method-header">
          <h4>发布包</h4>
        </div>
        
        <div class="method-steps">
          <div class="step-item">
            <div class="step-number">1</div>
            <div class="step-content">
              <h5>发布到本地仓库</h5>
              <code-block :code="`npm publish --registry=${registryUrl}`" />
            </div>
          </div>

          <div class="step-item">
            <div class="step-number">2</div>
            <div class="step-content">
              <h5>或在 package.json 中配置</h5>
              <code-block :code="packageJsonContent" title="package.json" />
            </div>
          </div>
        </div>
      </div>
    </div>

    <div class="troubleshooting">
      <h4>常见问题</h4>
      <el-collapse>
        <el-collapse-item title="npm install 报错 404 Not Found" name="404">
          <p><strong>可能的原因：</strong></p>
          <ol>
            <li>仓库地址错误 - 检查 <code>npm config get registry</code></li>
            <li>包不存在 - 确认包已发布到仓库</li>
            <li>认证失败 - 检查令牌是否正确</li>
          </ol>
        </el-collapse-item>

        <el-collapse-item title="npm adduser 不工作" name="adduser">
          <p>当前版本暂不支持 <code>npm adduser</code> 命令。</p>
          <p><strong>替代方案：</strong></p>
          <ol>
            <li>通过 Web UI 获取令牌</li>
            <li>手动配置 .npmrc 文件</li>
            <li>联系管理员获取预配置文件</li>
          </ol>
        </el-collapse-item>

        <el-collapse-item title="如何删除已发布的包" name="unpublish">
          <code-block code="npm unpublish package-name@1.0.0 --registry=http://your-registry/repo/npm-local/" />
          <el-alert type="warning" :closable="false">
            需要相应权限才能删除包
          </el-alert>
        </el-collapse-item>
      </el-collapse>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import CodeBlock from '@/components/help/CodeBlock.vue'

const registryUrl = computed(() => {
  return `${window.location.origin}/repo/npm-virtual/`
})

const host = computed(() => {
  return window.location.host
})

const npmrcContent = computed(() => {
  return `# NPM 配置文件
registry=${registryUrl.value}

# 认证信息
//${host.value}/repo/npm-virtual/:_authToken=YOUR_TOKEN_HERE

# 作用域包配置（可选）
# @mycompany:registry=${window.location.origin}/repo/npm-local/`
})

const packageJsonContent = computed(() => {
  return JSON.stringify({
    publishConfig: {
      registry: registryUrl.value
    }
  }, null, 2)
})

const downloadTemplate = (filename: string) => {
  window.open(`/docs/templates/${filename}`, '_blank')
}
</script>

<style scoped>
.npm-config {
  padding: 0;
  width: 100%;
}

.npm-config :deep(.el-alert) {
  border-radius: 8px;
  padding: 12px 16px;
  font-size: 13px;
  margin: 20px 0;
}

.config-methods {
  display: flex;
  flex-direction: column;
  gap: 20px;
  margin-bottom: 32px;
}

.method-card {
  background: #fff;
  border: 1px solid #e5e7eb;
  border-radius: 8px;
  padding: 24px;
}

.method-header {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 10px;
}

.method-header h4 {
  margin: 0;
  font-size: 15px;
  font-weight: 600;
  color: #0f172a;
}

.method-badge {
  background: #10b981;
  color: #fff;
  padding: 3px 10px;
  border-radius: 4px;
  font-size: 11px;
  font-weight: 600;
}

.method-desc {
  margin: 0 0 20px;
  color: #6b7280;
  font-size: 14px;
  line-height: 1.5;
}

.method-desc code {
  background: #f3f4f6;
  padding: 2px 8px;
  border-radius: 4px;
  font-family: 'SF Mono', 'Fira Code', 'Consolas', monospace;
  font-size: 13px;
  color: #7c3aed;
}

.method-steps {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.step-item {
  display: flex;
  gap: 12px;
}

.step-number {
  width: 28px;
  height: 28px;
  border-radius: 50%;
  background: #2563eb;
  color: #fff;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 13px;
  font-weight: 600;
  flex-shrink: 0;
}

.step-content {
  flex: 1;
  min-width: 0;
}

.step-content h5 {
  margin: 0 0 10px;
  font-size: 14px;
  font-weight: 600;
  color: #374151;
}

.step-content p {
  margin: 0 0 10px;
  color: #6b7280;
  font-size: 13px;
  line-height: 1.5;
}

.step-content :deep(.el-button) {
  border-radius: 6px;
  padding: 8px 16px;
  font-weight: 500;
  font-size: 13px;
}

.troubleshooting {
  margin-top: 32px;
  padding-top: 24px;
  border-top: 1px solid #e5e7eb;
}

.troubleshooting h4 {
  margin: 0 0 16px;
  font-size: 15px;
  font-weight: 600;
  color: #0f172a;
}

.troubleshooting :deep(.el-collapse-item__header) {
  font-size: 14px;
  font-weight: 500;
  color: #4b5563;
  background: #f9fafb;
  border-radius: 6px;
  margin-bottom: 10px;
  padding: 14px 16px;
  border: 1px solid #e5e7eb;
}

.troubleshooting :deep(.el-collapse-item__content) {
  padding: 16px;
  background: #fff;
  border: 1px solid #e5e7eb;
  border-top: none;
  border-radius: 0 0 6px 6px;
  margin-bottom: 10px;
}

.troubleshooting p {
  margin: 8px 0;
  color: #4b5563;
  font-size: 14px;
  line-height: 1.6;
}

.troubleshooting ol {
  padding-left: 20px;
  margin: 10px 0;
}

.troubleshooting li {
  margin: 6px 0;
  color: #4b5563;
  font-size: 14px;
  line-height: 1.6;
}

.troubleshooting code {
  background: #f3f4f6;
  padding: 2px 8px;
  border-radius: 4px;
  font-family: 'SF Mono', 'Fira Code', 'Consolas', monospace;
  font-size: 13px;
  color: #7c3aed;
}

.troubleshooting :deep(.el-alert) {
  border-radius: 6px;
  margin-top: 10px;
  font-size: 13px;
}
</style>
