<template>
  <span :class="['status-tag', `status-tag--${status}`]">
    {{ statusText }}
  </span>
</template>

<script setup lang="ts">
import { computed } from 'vue'

interface Props {
  status: 'online' | 'offline' | 'maintenance'
}

const props = defineProps<Props>()

const statusText = computed(() => {
  const map = {
    online: '在线',
    offline: '离线',
    maintenance: '维护中',
  }
  return map[props.status]
})
</script>

<style scoped>
.status-tag {
  padding: var(--spacing-xs) var(--spacing-sm);
  border-radius: var(--radius-full);
  font-size: var(--font-size-xs);
  font-weight: var(--font-weight-medium);
  display: inline-block;
}

.status-tag--online {
  background: #dcfce7;
  color: #15803d;
}

.status-tag--offline {
  background: #fee2e2;
  color: #991b1b;
}

.status-tag--maintenance {
  background: #fef3c7;
  color: #92400e;
}
</style>
