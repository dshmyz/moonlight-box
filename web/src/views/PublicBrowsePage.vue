<template>
  <div class="public-browse">
    <PackageExplorer mode="public" :default-page-size="24" :page-size-options="[12, 24, 48, 96]" />
  </div>
</template>

<script setup lang="ts">
import PackageExplorer from '@/components/package-explorer/PackageExplorer.vue'
</script>

<style scoped>
.public-browse { padding: 24px; max-width: 1400px; margin: 0 auto; }
</style>
