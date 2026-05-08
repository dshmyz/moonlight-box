<template>
  <div class="api-docs">
    <iframe
      ref="swaggerFrame"
      :src="swaggerUrl"
      class="swagger-frame"
      @load="onFrameLoad"
    />
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'

const swaggerFrame = ref<HTMLIFrameElement>()
const swaggerUrl = '/docs/swagger/index.html'

const onFrameLoad = () => {
  console.log('Swagger UI loaded')
}
</script>

<style scoped>
.api-docs {
  height: calc(100vh - 100px);
  width: 100%;
}

.swagger-frame {
  width: 100%;
  height: 100%;
  border: none;
  border-radius: 12px;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.04);
}
</style>
