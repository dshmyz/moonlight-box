<template>
  <div v-if="variant === 'table'" class="skeleton-table">
    <div v-for="i in 8" :key="i" class="skeleton-row">
      <div class="skeleton-cell skeleton-name"><div class="shimmer"></div></div>
      <div class="skeleton-cell skeleton-type"><div class="shimmer"></div></div>
      <div class="skeleton-cell skeleton-count"><div class="shimmer"></div></div>
      <div class="skeleton-cell skeleton-count"><div class="shimmer"></div></div>
      <div class="skeleton-cell skeleton-time"><div class="shimmer"></div></div>
      <div class="skeleton-cell skeleton-actions"><div class="shimmer"></div></div>
    </div>
  </div>
  <div v-else class="skeleton-grid">
    <div v-for="i in 8" :key="i" class="skeleton-card">
      <div class="shimmer skeleton-line short"></div>
      <div class="shimmer skeleton-line long"></div>
      <div class="shimmer skeleton-line medium"></div>
      <div class="skeleton-meta">
        <div class="shimmer skeleton-line tiny"></div>
        <div class="shimmer skeleton-line tiny"></div>
        <div class="shimmer skeleton-line tiny"></div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
defineProps<{
  variant: 'table' | 'grid'
}>()
</script>

<style scoped>
.shimmer {
  background: linear-gradient(90deg, #f1f5f9 25%, #e2e8f0 37%, #f1f5f9 63%);
  background-size: 400% 100%;
  animation: shimmer 1.4s ease infinite;
  border-radius: 4px;
  width: 100%;
  height: 100%;
}
@keyframes shimmer {
  0% { background-position: 100% 0; }
  100% { background-position: 0 0; }
}
.skeleton-table { display: flex; flex-direction: column; gap: 8px; padding: 16px; }
.skeleton-row { display: flex; gap: 12px; height: 56px; }
.skeleton-cell { border-radius: 6px; overflow: hidden; }
.skeleton-name { flex: 1; }
.skeleton-type { width: 80px; }
.skeleton-count { width: 70px; }
.skeleton-time { width: 140px; }
.skeleton-actions { width: 240px; }
.skeleton-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(320px, 1fr)); gap: 16px; }
.skeleton-card { padding: 20px; background: #fff; border-radius: 12px; border: 1px solid #f1f5f9; }
.skeleton-line { height: 14px; margin-bottom: 8px; }
.skeleton-line.short { width: 40%; }
.skeleton-line.long { width: 90%; }
.skeleton-line.medium { width: 65%; }
.skeleton-line.tiny { width: 30%; height: 10px; display: inline-block; margin-right: 8px; }
.skeleton-meta { margin-top: 16px; }
</style>
