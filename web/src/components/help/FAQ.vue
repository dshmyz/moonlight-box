<template>
  <div class="faq">
    <h2>❓ 常见问题</h2>

    <el-input
      v-model="searchQuery"
      placeholder="搜索问题..."
      prefix-icon="Search"
      clearable
      style="margin-bottom: 20px"
    />

    <el-collapse v-model="activeQuestions">
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
            <h4 class="question">{{ item.question }}</h4>
            <div class="answer" v-html="item.answer"></div>
          </div>
        </div>
      </el-collapse-item>
    </el-collapse>

    <el-alert
      type="info"
      :closable="false"
      style="margin-top: 20px"
    >
      <template #title>
        没有找到答案？
      </template>
      <p style="margin-top: 10px">
        联系管理员：<a href="mailto:admin@company.com">admin@company.com</a>
      </p>
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
        question: '如何获取访问令牌？',
        answer: `
          <p>有以下几种方式：</p>
          <ol>
            <li>通过 Web UI 登录后，在"个人设置" → "访问令牌"中生成</li>
            <li>通过 API：<code>POST /api/v1/auth/login</code></li>
            <li>联系管理员获取预配置文件</li>
          </ol>
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
            <li>通过 Web UI 获取令牌</li>
            <li>手动配置 <code>.npmrc</code> 文件</li>
            <li>联系管理员获取预配置文件</li>
          </ol>
        `
      },
      {
        question: '如何发布作用域包？',
        answer: `
          <p>配置作用域仓库：</p>
          <pre><code>@mycompany:registry=http://your-registry/repo/npm-local/</code></pre>
          <p>或使用命令行：</p>
          <pre><code>npm publish --registry=http://your-registry/repo/npm-local/</code></pre>
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
          <p>当前上传端点为 <code>/pypi/upload/</code>，请使用：</p>
          <pre><code>twine upload --repository-url http://your-registry/pypi/upload/ dist/*</code></pre>
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
  padding: 20px;
}

.faq h2 {
  margin-bottom: 20px;
}

.faq-category {
  padding: 10px 0;
}

.faq-item {
  margin-bottom: 20px;
  padding-bottom: 20px;
  border-bottom: 1px solid #e4e7ed;
}

.faq-item:last-child {
  border-bottom: none;
}

.question {
  margin-bottom: 10px;
  color: #303133;
}

.answer {
  color: #606266;
  line-height: 1.6;
}

.answer table {
  width: 100%;
  border-collapse: collapse;
  margin: 10px 0;
}

.answer th,
.answer td {
  border: 1px solid #e4e7ed;
  padding: 8px;
  text-align: left;
}

.answer th {
  background: #f5f7fa;
}

.answer pre {
  background: #f5f7fa;
  padding: 10px;
  border-radius: 4px;
  overflow-x: auto;
}

.answer code {
  background: #f5f7fa;
  padding: 2px 6px;
  border-radius: 3px;
  font-family: 'Courier New', monospace;
}

.answer ol,
.answer ul {
  padding-left: 20px;
}

.answer li {
  margin: 5px 0;
}
</style>
