<template>
  <div class="component-showcase">
    <div class="showcase-header">
      <h1 class="page-title">自定义组件库</h1>
      <p class="page-desc">基于首页设计风格的自定义UI组件展示</p>
    </div>

    <!-- Button Component -->
    <CustomCard title="按钮组件 (CustomButton)" subtitle="支持多种类型和尺寸" class="showcase-section">
      <div class="demo-block">
        <h4 class="demo-title">按钮类型</h4>
        <div class="demo-row">
          <CustomButton type="primary">主要按钮</CustomButton>
          <CustomButton type="secondary">次要按钮</CustomButton>
          <CustomButton type="outline">轮廓按钮</CustomButton>
          <CustomButton type="text">文字按钮</CustomButton>
        </div>
      </div>

      <div class="demo-block">
        <h4 class="demo-title">按钮尺寸</h4>
        <div class="demo-row">
          <CustomButton type="primary" size="small">小号按钮</CustomButton>
          <CustomButton type="primary" size="medium">中号按钮</CustomButton>
          <CustomButton type="primary" size="large">大号按钮</CustomButton>
        </div>
      </div>

      <div class="demo-block">
        <h4 class="demo-title">按钮状态</h4>
        <div class="demo-row">
          <CustomButton type="primary" :loading="buttonLoading" @click="handleButtonClick">
            {{ buttonLoading ? '加载中...' : '点击加载' }}
          </CustomButton>
          <CustomButton type="primary" disabled>禁用按钮</CustomButton>
          <CustomButton type="primary" :icon="SearchIcon">带图标按钮</CustomButton>
        </div>
      </div>
    </CustomCard>

    <!-- Input Component -->
    <CustomCard title="输入框组件 (CustomInput)" subtitle="支持前缀/后缀图标、清除按钮、密码显示切换" class="showcase-section">
      <div class="demo-block">
        <h4 class="demo-title">基础输入框</h4>
        <div class="demo-grid">
          <CustomInput
            v-model="inputValue1"
            placeholder="请输入内容"
            @enter="handleInputEnter"
          />
          <CustomInput
            v-model="inputValue2"
            placeholder="可清空的输入框"
            clearable
            @clear="handleInputClear"
          />
        </div>
      </div>

      <div class="demo-block">
        <h4 class="demo-title">带图标的输入框</h4>
        <div class="demo-grid">
          <CustomInput
            v-model="inputValue3"
            placeholder="搜索..."
            :prefix-icon="SearchIcon"
          />
          <CustomInput
            v-model="inputValue4"
            type="password"
            placeholder="密码输入框"
            :prefix-icon="LockIcon"
          />
        </div>
      </div>

      <div class="demo-block">
        <h4 class="demo-title">禁用状态</h4>
        <div class="demo-grid">
          <CustomInput
            v-model="inputValue5"
            placeholder="禁用输入框"
            disabled
          />
        </div>
      </div>
    </CustomCard>

    <!-- Select Component -->
    <CustomCard title="选择器组件 (CustomSelect)" subtitle="支持搜索、多选" class="showcase-section">
      <div class="demo-block">
        <h4 class="demo-title">单选选择器</h4>
        <div class="demo-grid">
          <CustomSelect
            v-model="selectValue1"
            :options="selectOptions"
            placeholder="请选择框架"
          />
          <CustomSelect
            v-model="selectValue2"
            :options="selectOptions"
            placeholder="可搜索的选择器"
            searchable
          />
        </div>
      </div>

      <div class="demo-block">
        <h4 class="demo-title">多选选择器</h4>
        <div class="demo-grid">
          <CustomSelect
            v-model="multiSelectValue"
            :options="selectOptions"
            placeholder="请选择多个框架"
            multiple
          />
        </div>
      </div>

      <div class="demo-block">
        <h4 class="demo-title">禁用状态</h4>
        <div class="demo-grid">
          <CustomSelect
            v-model="selectValue1"
            :options="selectOptions"
            placeholder="禁用选择器"
            disabled
          />
        </div>
      </div>
    </CustomCard>

    <!-- Card Component -->
    <CustomCard title="卡片组件 (CustomCard)" subtitle="支持标题、副标题、插槽" class="showcase-section">
      <div class="demo-block">
        <h4 class="demo-title">基础卡片</h4>
        <div class="demo-grid-cards">
          <CustomCard title="卡片标题" subtitle="这是卡片的副标题">
            <p>这是卡片的内容区域，可以放置任何内容。</p>
          </CustomCard>

          <CustomCard title="可悬停卡片" hoverable>
            <p>鼠标悬停时会显示阴影效果。</p>
          </CustomCard>

          <CustomCard title="可点击卡片" clickable @click="handleCardClick">
            <p>点击此卡片会触发点击事件。</p>
          </CustomCard>
        </div>
      </div>

      <div class="demo-block">
        <h4 class="demo-title">带插槽的卡片</h4>
        <CustomCard>
          <template #header>
            <div style="display: flex; justify-content: space-between; align-items: center;">
              <h3 style="margin: 0; font-size: 18px; font-weight: 700; color: #0f172a;">
                自定义头部
              </h3>
              <CustomButton type="primary" size="small">操作</CustomButton>
            </div>
          </template>
          <p>这是使用自定义头部插槽的卡片示例。</p>
          <template #footer>
            <div style="display: flex; gap: 8px; justify-content: flex-end;">
              <CustomButton type="secondary" size="small">取消</CustomButton>
              <CustomButton type="primary" size="small">确定</CustomButton>
            </div>
          </template>
        </CustomCard>
      </div>
    </CustomCard>

    <!-- Combined Demo -->
    <CustomCard title="组合示例" subtitle="展示组件的实际应用场景" class="showcase-section">
      <div class="demo-block">
        <h4 class="demo-title">登录表单</h4>
        <div class="login-demo">
          <CustomInput
            v-model="loginForm.username"
            placeholder="用户名"
            :prefix-icon="UserIcon"
            style="margin-bottom: 16px;"
          />
          <CustomInput
            v-model="loginForm.password"
            type="password"
            placeholder="密码"
            :prefix-icon="LockIcon"
            style="margin-bottom: 16px;"
          />
          <CustomButton type="primary" style="width: 100%;" @click="handleLogin">
            登录
          </CustomButton>
        </div>
      </div>

      <div class="demo-block">
        <h4 class="demo-title">搜索表单</h4>
        <div class="search-demo">
          <CustomInput
            v-model="searchForm.keyword"
            placeholder="搜索包名、描述或标签..."
            :prefix-icon="SearchIcon"
            clearable
            style="flex: 1; margin-right: 12px;"
          />
          <CustomSelect
            v-model="searchForm.type"
            :options="typeOptions"
            placeholder="选择类型"
            style="width: 150px; margin-right: 12px;"
          />
          <CustomButton type="primary" :icon="SearchIcon" @click="handleSearch">
            搜索
          </CustomButton>
        </div>
      </div>
    </CustomCard>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive } from 'vue'
import { Search, Lock, User } from '@element-plus/icons-vue'
import CustomButton from '@/components/ui/CustomButton.vue'
import CustomInput from '@/components/ui/CustomInput.vue'
import CustomSelect from '@/components/ui/CustomSelect.vue'
import CustomCard from '@/components/ui/CustomCard.vue'

const SearchIcon = Search
const LockIcon = Lock
const UserIcon = User

// Button demos
const buttonLoading = ref(false)

const handleButtonClick = () => {
  buttonLoading.value = true
  setTimeout(() => {
    buttonLoading.value = false
  }, 2000)
}

// Input demos
const inputValue1 = ref('')
const inputValue2 = ref('')
const inputValue3 = ref('')
const inputValue4 = ref('')
const inputValue5 = ref('禁用的内容')

const handleInputEnter = () => {
  alert(`输入内容: ${inputValue1.value}`)
}

const handleInputClear = () => {
  console.log('输入框已清空')
}

// Select demos
const selectValue1 = ref('')
const selectValue2 = ref('')
const multiSelectValue = ref<string[]>([])

const selectOptions = [
  { label: 'Vue', value: 'vue' },
  { label: 'React', value: 'react' },
  { label: 'Angular', value: 'angular' },
  { label: 'Svelte', value: 'svelte' },
  { label: 'Solid', value: 'solid' },
]

const typeOptions = [
  { label: '全部', value: 'all' },
  { label: 'npm', value: 'npm' },
  { label: 'Maven', value: 'maven' },
  { label: 'PyPI', value: 'pypi' },
  { label: 'Go', value: 'go' },
]

// Card demos
const handleCardClick = () => {
  alert('卡片被点击了！')
}

// Combined demos
const loginForm = reactive({
  username: '',
  password: '',
})

const searchForm = reactive({
  keyword: '',
  type: 'all',
})

const handleLogin = () => {
  alert(`登录信息:\n用户名: ${loginForm.username}\n密码: ${loginForm.password}`)
}

const handleSearch = () => {
  alert(`搜索信息:\n关键词: ${searchForm.keyword}\n类型: ${searchForm.type}`)
}
</script>

<style scoped>
.component-showcase {
  padding: 24px;
  max-width: 1200px;
  margin: 0 auto;
}

.showcase-header {
  margin-bottom: 32px;
  padding: 24px;
  background: #ffffff;
  border: 1px solid #e2e8f0;
  border-radius: 12px;
}

.page-title {
  font-size: 28px;
  font-weight: 800;
  color: #0f172a;
  margin: 0 0 6px;
  letter-spacing: -0.8px;
  line-height: 1.1;
}

.page-desc {
  color: #64748b;
  font-size: 14px;
  margin: 0;
  line-height: 1.5;
}

.showcase-section {
  margin-bottom: 24px;
}

.demo-block {
  margin-bottom: 24px;
}

.demo-block:last-child {
  margin-bottom: 0;
}

.demo-title {
  font-size: 14px;
  font-weight: 600;
  color: #0f172a;
  margin: 0 0 12px;
}

.demo-row {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
  align-items: center;
}

.demo-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(280px, 1fr));
  gap: 16px;
}

.demo-grid-cards {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(300px, 1fr));
  gap: 16px;
}

.login-demo {
  max-width: 400px;
  padding: 24px;
  background: #fafbfc;
  border: 1px solid #e2e8f0;
  border-radius: 8px;
}

.search-demo {
  display: flex;
  align-items: center;
  padding: 16px;
  background: #fafbfc;
  border: 1px solid #e2e8f0;
  border-radius: 8px;
}
</style>
