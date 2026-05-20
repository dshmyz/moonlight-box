<template>
  <section class="hero-section">
    <div class="star-field" aria-hidden="true" />
    <div class="nebula-overlay" />
    <div class="moon-decor" aria-hidden="true">
      <svg viewBox="0 0 200 200" xmlns="http://www.w3.org/2000/svg">
        <defs>
          <radialGradient id="moon-glow" cx="50%" cy="50%" r="50%">
            <stop offset="0%" style="stop-color:var(--lunar-accent);stop-opacity:0.2" />
            <stop offset="40%" style="stop-color:var(--lunar-accent);stop-opacity:0.05" />
            <stop offset="100%" style="stop-color:var(--lunar-accent);stop-opacity:0" />
          </radialGradient>
          <radialGradient id="moon-inner-glow" cx="40%" cy="40%" r="60%">
            <stop offset="0%" style="stop-color:#f5f3ff;stop-opacity:0.4" />
            <stop offset="100%" style="stop-color:var(--lunar-accent);stop-opacity:0" />
          </radialGradient>
          <linearGradient id="moon-fill" x1="0%" y1="0%" x2="100%" y2="100%">
            <stop offset="0%" style="stop-color:var(--lunar-accent)" />
            <stop offset="100%" style="stop-color:var(--lunar-accent-soft)" />
          </linearGradient>
        </defs>
        <circle cx="100" cy="100" r="80" fill="url(#moon-glow)" />
        <circle cx="100" cy="100" r="40" fill="url(#moon-inner-glow)" />
        <circle cx="100" cy="100" r="38" fill="url(#moon-fill)" />
        <circle cx="114" cy="86" r="30" fill="var(--lunar-bg-deep)" />
      </svg>
    </div>

    <div class="hero-content">
      <div class="hero-header-row">
        <div class="hero-badge">
          <span class="badge-dot" />
          <span>Moonlight Box</span>
        </div>
        <h1 class="hero-title">
          <span class="title-line">软件包</span>
          <span class="title-accent">中心</span>
        </h1>
      </div>
      <p class="hero-desc">统一管理、搜索和分发多语言软件包</p>

      <div class="hero-search-glass">
        <el-input
          v-model="searchQuery"
          placeholder="搜索包名、描述或标签..."
          size="large"
          clearable
          class="lunar-search-input"
          @keyup.enter="handleSearch"
          @clear="handleSearch"
        >
          <template #prefix>
            <el-icon><Search /></el-icon>
          </template>
        </el-input>
        <button class="lunar-search-btn" @click="handleSearch">
          <el-icon><Search /></el-icon>
          搜索
        </button>
      </div>

      <div class="hero-type-pills">
        <button
          v-for="pill in typePills"
          :key="pill.value"
          class="type-pill"
          :class="{ 'pill-active': selectedType === pill.value }"
          @click="selectType(pill.value)"
        >
          <span class="pill-dot" :style="{ background: pill.color }" />
          {{ pill.label }}
        </button>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { Search } from '@element-plus/icons-vue'

const props = defineProps<{
  searchQuery?: string
  selectedType?: string
}>()

const emit = defineEmits<{
  search: [query: string, type: string]
  'update:searchQuery': [value: string]
  'update:selectedType': [value: string]
}>()

// 使用 computed 实现双向绑定
const searchQuery = computed({
  get: () => props.searchQuery ?? '',
  set: (val) => emit('update:searchQuery', val)
})

const selectedType = computed({
  get: () => props.selectedType ?? 'all',
  set: (val) => emit('update:selectedType', val)
})

const typePills = [
  { label: '全部', value: 'all', color: '#c4b5fd' },
  { label: 'npm', value: 'npm', color: '#cb3837' },
  { label: 'Maven', value: 'maven', color: '#e65100' },
  { label: 'PyPI', value: 'pypi', color: '#3775a9' },
  { label: 'Go', value: 'go', color: '#00add8' },
  { label: 'Yum', value: 'yum', color: '#2e6da4' },
  { label: 'Apt', value: 'apt', color: '#d70a53' },
  { label: 'Generic', value: 'generic', color: '#64748b' },
]

function handleSearch() {
  emit('search', searchQuery.value, selectedType.value)
}

function selectType(value: string) {
  selectedType.value = value
  emit('search', searchQuery.value, selectedType.value)
}
</script>

<style scoped>
.hero-section {
  position: relative;
  min-height: 220px;
  background: var(--lunar-gradient-hero);
  border-radius: 16px;
  overflow: hidden;
  display: flex;
  align-items: center;
  justify-content: flex-start;
  padding: 32px 32px 28px;
  margin-bottom: 24px;
  transition: background 1s ease;
}

.star-field {
  position: absolute;
  inset: 0;
  overflow: hidden;
  pointer-events: none;
  opacity: calc(var(--lunar-star-opacity) * 0.5);
  transition: opacity 1s ease;
}

.star-field::before,
.star-field::after {
  content: '';
  position: absolute;
  width: 2px;
  height: 2px;
  border-radius: 50%;
  background: var(--lunar-silver);
  box-shadow:
    30px 20px 0 0 var(--lunar-silver),
    120px 60px 0 0 var(--lunar-silver),
    200px 40px 0 0 var(--lunar-silver),
    280px 80px 0 0 var(--lunar-silver),
    350px 30px 0 0 var(--lunar-silver),
    450px 70px 0 0 var(--lunar-silver),
    520px 50px 0 0 var(--lunar-silver),
    60px 100px 0 0 var(--lunar-accent),
    150px 130px 0 0 var(--lunar-accent),
    240px 90px 0 0 var(--lunar-accent),
    320px 150px 0 0 var(--lunar-accent),
    400px 110px 0 0 var(--lunar-accent),
    480px 140px 0 0 var(--lunar-accent);
  animation: starTwinkleSoft 6s ease-in-out infinite;
}

.star-field::after {
  animation-delay: 3s;
  transform: scale(0.8);
  opacity: 0.6;
}

@keyframes starTwinkleSoft {
  0%, 100% { opacity: 0.4; }
  50% { opacity: 0.7; }
}

.moon-decor {
  position: absolute;
  right: 1%;
  top: -12%;
  width: 140px;
  height: 140px;
  opacity: calc(var(--lunar-hero-opacity) * 0.75);
  transition: opacity 1s ease;
  pointer-events: none;
}

.moon-decor svg {
  width: 100%;
  height: 100%;
}

.nebula-overlay {
  position: absolute;
  inset: 0;
  background:
    radial-gradient(ellipse at 20% 60%, rgba(139, 92, 246, 0.08) 0%, transparent 50%),
    radial-gradient(ellipse at 80% 20%, rgba(196, 181, 253, 0.06) 0%, transparent 40%);
  pointer-events: none;
}

.hero-content {
  position: relative;
  z-index: 2;
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  text-align: left;
  max-width: 520px;
  width: 100%;
  padding-left: 6%;
}

.hero-header-row {
  display: flex;
  align-items: center;
  gap: 14px;
  margin-bottom: 8px;
}

.hero-badge {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 3px 10px;
  background: var(--lunar-bg-glass);
  border: 1px solid var(--lunar-border);
  border-radius: 100px;
  font-size: 11px;
  color: var(--lunar-accent);
  font-weight: 600;
  letter-spacing: 1px;
  transition: background 0.5s ease, border-color 0.5s ease, color 0.5s ease;
}

.badge-dot {
  width: 5px;
  height: 5px;
  border-radius: 50%;
  background: var(--lunar-accent);
}

.hero-title {
  font-size: 32px;
  font-weight: 800;
  color: var(--lunar-silver);
  letter-spacing: -1.5px;
  line-height: 1.1;
  margin: 0;
  transition: color 0.5s ease;
}

.title-line {
  display: inline;
}

.title-accent {
  display: inline;
  color: var(--lunar-accent);
}

.hero-desc {
  font-size: 14px;
  color: var(--lunar-silver-muted);
  margin: 0 0 20px;
  line-height: 1.5;
  font-weight: 500;
  transition: color 0.5s ease;
}

.hero-search-glass {
  display: flex;
  align-items: stretch;
  width: 100%;
  max-width: 440px;
  background: var(--lunar-bg-glass);
  backdrop-filter: blur(16px);
  -webkit-backdrop-filter: blur(16px);
  border: 1px solid var(--lunar-border);
  border-radius: 12px;
  overflow: hidden;
  transition: border-color 0.3s ease, box-shadow 0.3s ease, background 0.5s ease;
}

.hero-search-glass:focus-within {
  border-color: var(--lunar-border-hover);
  box-shadow: var(--lunar-shadow-glow), 0 0 0 1px var(--lunar-border-hover);
}

.lunar-search-input {
  flex: 1;
}

.lunar-search-input :deep(.el-input__wrapper) {
  background: transparent;
  border: none;
  box-shadow: none;
  padding: 0 16px;
  height: 44px;
}

.lunar-search-input :deep(.el-input__wrapper:hover),
.lunar-search-input :deep(.el-input__wrapper.is-focus) {
  background: transparent;
  border: none;
  box-shadow: none;
}

.lunar-search-input :deep(.el-input__inner) {
  font-size: 14px;
  color: var(--lunar-silver);
  caret-color: var(--lunar-accent);
  height: 44px;
  line-height: 44px;
}

.lunar-search-input :deep(.el-input__inner::placeholder) {
  color: var(--lunar-silver-dim);
}

.lunar-search-input :deep(.el-input__prefix) {
  color: var(--lunar-silver-muted);
  height: 44px;
  line-height: 44px;
}

.lunar-search-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  padding: 0 20px;
  height: 44px;
  background: var(--lunar-gradient-btn);
  color: var(--lunar-bg-deep);
  font-size: 14px;
  font-weight: 700;
  border: none;
  cursor: pointer;
  transition: all 0.2s ease;
  letter-spacing: 0.5px;
  white-space: nowrap;
}

.lunar-search-btn:hover {
  filter: brightness(1.1);
}

.lunar-search-btn:active {
  transform: scale(0.97);
}

.hero-type-pills {
  display: flex;
  gap: 6px;
  margin-top: 14px;
  flex-wrap: nowrap;
  overflow-x: auto;
  justify-content: flex-start;
  scrollbar-width: none;
}

.hero-type-pills::-webkit-scrollbar {
  display: none;
}

.type-pill {
  display: flex;
  align-items: center;
  gap: 4px;
  padding: 4px 10px;
  background: var(--lunar-bg-glass);
  border: 1px solid var(--lunar-border);
  border-radius: 100px;
  font-size: 12px;
  font-weight: 600;
  color: var(--lunar-silver-muted);
  cursor: pointer;
  transition: all 0.25s ease;
  white-space: nowrap;
  flex-shrink: 0;
}

.type-pill:hover {
  color: var(--lunar-silver);
  border-color: var(--lunar-border-hover);
  background: rgba(196, 181, 253, 0.08);
}

.pill-active {
  color: var(--lunar-accent);
  border-color: var(--lunar-accent);
  background: rgba(196, 181, 253, 0.15);
}

.pill-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  flex-shrink: 0;
}

@media (max-width: 768px) {
  .hero-section {
    min-height: 180px;
    padding: 24px 20px 20px;
    justify-content: center;
  }

  .hero-content {
    align-items: center;
    text-align: center;
    padding-left: 0;
    max-width: 100%;
  }

  .hero-header-row {
    justify-content: center;
  }

  .hero-search-glass {
    max-width: 100%;
  }

  .hero-type-pills {
    justify-content: flex-start;
  }

  .hero-title {
    font-size: 24px;
    letter-spacing: -1px;
  }

  .hero-desc {
    font-size: 13px;
  }

  .moon-decor {
    width: 60px;
    height: 60px;
    right: 2%;
    top: -8%;
  }
}
</style>