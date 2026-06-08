<template>
  <div class="faq">
    <div class="section-header">
      <h2><i class="fa-solid fa-circle-question"></i> 常见问题</h2>
      <p class="section-desc">查找常见问题的解答，快速解决您遇到的问题</p>
    </div>

    <div class="search-box">
      <el-input
        v-model="searchQuery"
        placeholder="搜索问题..."
        prefix-icon="Search"
        clearable
        size="large"
      />
    </div>

    <div class="faq-content">
      <el-collapse v-model="activeQuestions" accordion class="faq-collapse">
        <el-collapse-item
          v-for="category in filteredCategories"
          :key="category.name"
          :title="category.name"
          :name="category.name"
        >
          <div class="faq-category">
            <div
              v-for="(item, index) in category.items"
              :key="index"
              class="faq-item"
            >
              <div class="question-wrapper">
                <i class="fa-solid fa-question-circle question-icon"></i>
                <h4 class="question">{{ item.question }}</h4>
              </div>
              <div class="answer" v-html="item.answer"></div>
            </div>
          </div>
        </el-collapse-item>
      </el-collapse>
    </div>

    <el-alert
      type="info"
      :closable="false"
      class="contact-alert"
    >
      <template #title>
        <i class="fa-solid fa-message-circle"></i>
        没有找到答案？
      </template>
      <p>联系管理员：<a href="mailto:admin@company.com">admin@company.com</a></p>
    </el-alert>
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'

const searchQuery = ref('')
const activeQuestions = ref(['通用问题'])

const categories = [
  {
    name: '通用问题',
    items: [
      {
        question: '如何进行认证配置？',
        answer: `
          <p>当前版本使用用户名密码进行认证：</p>
          <ol>
            <li><strong>NPM/PyPI：</strong>使用您的账号密码进行认证</li>
            <li><strong>Maven：</strong>在 settings.xml 中配置 server 信息</li>
            <li><strong>Go：</strong>通过 GOPROXY 配置，无需额外认证</li>
          </ol>
          <p style="margin-top: 10px; color: #64748b;">
            <i class="fa-solid fa-info-circle"></i> 访问令牌功能即将推出
          </p>
        `
      },
      {
        question: '本地仓库和代理仓库有什么区别？',
        answer: `
          <table>
            <tr>
              <th>类型</th>
              <th>说明</th>
              <th>用途</th>
            </tr>
            <tr>
              <td>本地仓库</td>
              <td>存储内部开发的包</td>
              <td>发布和托管内部包</td>
            </tr>
            <tr>
              <td>代理仓库</td>
              <td>代理外部仓库</td>
              <td>缓存外部包，加速下载</td>
            </tr>
            <tr>
              <td>虚拟仓库</td>
              <td>聚合多个仓库</td>
              <td>统一访问入口</td>
            </tr>
          </table>
        `
      }
    ]
  },
  {
    name: 'NPM 相关',
    items: [
      {
        question: '为什么 npm adduser 不工作？',
        answer: `
          <p>当前版本暂不支持 <code>npm adduser</code> 命令。</p>
          <p><strong>替代方案：</strong></p>
          <ol>
            <li>手动配置 <code>.npmrc</code> 文件，使用用户名密码认证</li>
            <li>联系管理员获取预配置文件</li>
          </ol>
        `
      },
      {
        question: '如何发布作用域包？',
        answer: `
          <p>配置作用域仓库：</p>
          <pre><code>@mycompany:registry=http://your-registry/repository/npm-local/</code></pre>
          <p>或使用命令行：</p>
          <pre><code>npm publish --registry=http://your-registry/repository/npm-local/</code></pre>
        `
      }
    ]
  },
  {
    name: 'Maven 相关',
    items: [
      {
        question: 'Maven 发布包失败，提示 401 Unauthorized？',
        answer: `
          <p>检查以下配置：</p>
          <ol>
            <li>settings.xml 中的 server 配置</li>
            <li>pom.xml 中的 repository id 是否匹配</li>
            <li>用户名和密码是否正确</li>
          </ol>
        `
      }
    ]
  },
  {
    name: 'PyPI 相关',
    items: [
      {
        question: '使用 twine 上传包失败？',
        answer: `
          <p>当前 PyPI hosted 上传使用仓库文件路径：</p>
          <pre><code>curl -X PUT http://your-registry/repository/pypi-local/packages/example-1.0.0-py3-none-any.whl --data-binary @dist/example-1.0.0-py3-none-any.whl</code></pre>
        `
      }
    ]
  },
  {
    name: 'Go 相关',
    items: [
      {
        question: 'go get 报错 "checksum mismatch"？',
        answer: `
          <p>当前版本不支持校验和数据库，请禁用：</p>
          <pre><code>export GOSUMDB=off</code></pre>
        `
      }
    ]
  }
]

const filteredCategories = computed(() => {
  if (!searchQuery.value) {
    return categories
  }

  return categories.map(category => ({
    ...category,
    items: category.items.filter(
      item =>
        item.question.toLowerCase().includes(searchQuery.value.toLowerCase()) ||
        item.answer.toLowerCase().includes(searchQuery.value.toLowerCase())
    )
  })).filter(category => category.items.length > 0)
})
</script>

<style scoped>
.faq {
  padding: 0;
}

.section-header {
  text-align: center;
  margin-bottom: 0;
  padding: 48px 48px 32px;
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

.search-box {
  max-width: 560px;
  margin: 0 auto 36px;
  padding: 0 48px;
}

.search-box :deep(.el-input__wrapper) {
  border-radius: 12px;
  box-shadow: 0 2px 12px rgba(0, 0, 0, 0.08);
  padding: 6px 18px;
  transition: all 0.2s ease;
}

.search-box :deep(.el-input__wrapper:hover) {
  box-shadow: 0 4px 16px rgba(0, 0, 0, 0.12);
}

.faq-content {
  max-width: 880px;
  margin: 0 auto;
  padding: 0 48px 48px;
}

.faq-collapse {
  border: none;
}

.faq-collapse :deep(.el-collapse-item__header) {
  font-size: 16px;
  font-weight: 600;
  color: #0f172a;
  background: #fff;
  border-radius: 12px;
  margin-bottom: 16px;
  padding: 20px 24px;
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.06);
  border: 1px solid #e2e8f0;
  height: auto;
  line-height: 1.6;
  transition: all 0.2s ease;
}

.faq-collapse :deep(.el-collapse-item__header:hover) {
  background: #f8fafc;
  border-color: #cbd5e1;
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.08);
}

.faq-collapse :deep(.el-collapse-item__header.is-active) {
  border-radius: 12px 12px 0 0;
  margin-bottom: 0;
  border-bottom-color: transparent;
  background: linear-gradient(135deg, #f8fafc 0%, #f1f5f9 100%);
}

.faq-collapse :deep(.el-collapse-item__content) {
  background: #fff;
  padding: 24px;
  border-radius: 0 0 12px 12px;
  border: 1px solid #e2e8f0;
  border-top: none;
  margin-bottom: 16px;
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.04);
}

.faq-category {
  padding: 8px 0;
}

.faq-item {
  background: transparent;
  border-radius: 0;
  padding: 0;
  margin-bottom: 0;
  box-shadow: none;
}

.question-wrapper {
  display: flex;
  align-items: flex-start;
  gap: 12px;
  margin-bottom: 16px;
}

.question-icon {
  font-size: 18px;
  color: #8b5cf6;
  flex-shrink: 0;
  margin-top: 2px;
}

.question {
  font-size: 15px;
  font-weight: 600;
  color: #0f172a;
  margin: 0;
  line-height: 1.6;
}

.answer {
  color: #475569;
  line-height: 1.8;
  font-size: 15px;
}

.answer table {
  width: 100%;
  border-collapse: collapse;
  margin: 12px 0;
  background: #fff;
  border-radius: 8px;
  overflow: hidden;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.05);
}

.answer th,
.answer td {
  border-bottom: 1px solid #e2e8f0;
  padding: 12px 16px;
  text-align: left;
}

.answer th {
  background: #f8fafc;
  font-weight: 600;
  color: #475569;
}

.answer tr:last-child td {
  border-bottom: none;
}

.answer pre {
  background: #1e293b;
  color: #e2e8f0;
  padding: 16px;
  border-radius: 8px;
  overflow-x: auto;
  font-family: var(--font-family-mono);
  font-size: 13px;
  line-height: 1.6;
  margin: 12px 0;
}

.answer code {
  background: #f1f5f9;
  padding: 3px 8px;
  border-radius: 4px;
  font-family: var(--font-family-mono);
  font-size: 13px;
  color: #7c3aed;
}

.answer ol,
.answer ul {
  padding-left: 24px;
  margin: 10px 0;
}

.answer li {
  margin: 8px 0;
}

.contact-alert {
  max-width: 720px;
  margin: 24px auto 0;
  border-radius: 12px;
  background: #f0f9ff;
  border-color: #bae6fd;
}

.contact-alert :deep(.el-alert__title) {
  display: flex;
  align-items: center;
  gap: 8px;
}

.contact-alert :deep(.el-alert__title) i {
  color: #0ea5e9;
}

.contact-alert p {
  margin-top: 8px;
}

.contact-alert a {
  color: #0ea5e9;
  text-decoration: none;
}

.contact-alert a:hover {
  text-decoration: underline;
}
</style>
