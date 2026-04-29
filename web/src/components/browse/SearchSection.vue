<template>
  <div class="search-section">
    <div class="search-header">
      <h1 class="page-title">包仓库</h1>
      <p class="page-desc">搜索、浏览和发现所有可用的软件包</p>
    </div>
  </div>
</template>

<script setup lang="ts">
defineEmits<{
  search: []
}>()
</script>

<style scoped>
.search-section {
  margin-bottom: 20px;
}

.page-title {
  font-size: 28px;
  font-weight: 700;
  color: #1a1a2e;
  margin: 0 0 8px;
  letter-spacing: -0.5px;
}

.page-desc {
  color: #8b8fa3;
  font-size: 15px;
  margin: 0;
  font-weight: 400;
}
</style>
