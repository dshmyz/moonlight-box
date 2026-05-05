<template>
  <div class="npm-config">
    <h3>NPM 配置指南</h3>

    <el-alert type="info" :closable="false" style="margin-bottom: 20px">
      NPM 是 Node.js 的包管理器，用于安装、发布和管理 JavaScript 包
    </el-alert>

    <el-tabs>
      <el-tab-pane label="方式一：配置文件（推荐）">
        <div class="config-section">
          <h4>1. 下载配置模板</h4>
          <el-button type="primary" @click="downloadTemplate('.npmrc')">
            下载 .npmrc 模板
          </el-button>
        </div>

        <div class="config-section">
          <h4>2. 配置文件内容</h4>
          <p>将下载的文件放到 <code>~/.npmrc</code> 或项目根目录</p>
          <code-block
            :code="npmrcContent"
            title="~/.npmrc"
          />
        </div>

        <div class="config-section">
          <h4>3. 替换认证信息</h4>
          <p>将 <code>YOUR_TOKEN_HERE</code> 替换为您的访问令牌</p>
          <code-block code="sed -i 's/YOUR_TOKEN_HERE/your-actual-token/g' ~/.npmrc" />
        </div>
      </el-tab-pane>

      <el-tab-pane label="方式二：命令行配置">
        <div class="config-section">
          <h4>设置仓库地址</h4>
          <code-block :code="`npm config set registry ${registryUrl}`" />
        </div>

        <div class="config-section">
          <h4>设置认证信息</h4>
          <code-block :code="`npm config set //${host}/repo/npm-virtual/:_authToken YOUR_TOKEN_HERE`" />
        </div>

        <div class="config-section">
          <h4>验证配置</h4>
          <code-block code="npm config list" />
        </div>
      </el-tab-pane>

      <el-tab-pane label="发布包">
        <div class="config-section">
          <h4>发布到本地仓库</h4>
          <code-block :code="`npm publish --registry=${registryUrl}`" />
        </div>

        <div class="config-section">
          <h4>或在 package.json 中配置</h4>
          <code-block
            :code="packageJsonContent"
            title="package.json"
          />
        </div>
      </el-tab-pane>
    </el-tabs>

    <el-divider />

    <div class="troubleshooting">
      <h4>🔧 常见问题</h4>
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
          <el-alert type="warning" :closable="false" style="margin-top: 10px">
            ⚠️ 需要相应权限才能删除包
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
  padding: 20px;
}

.npm-config h3 {
  margin-bottom: 15px;
}

.config-section {
  margin: 20px 0;
}

.config-section h4 {
  margin-bottom: 10px;
  color: #303133;
}

.config-section p {
  margin: 10px 0;
  color: #606266;
}

.troubleshooting {
  margin-top: 20px;
}

.troubleshooting h4 {
  margin-bottom: 15px;
}

.troubleshooting ol {
  padding-left: 20px;
}

.troubleshooting li {
  margin: 5px 0;
}
</style>
