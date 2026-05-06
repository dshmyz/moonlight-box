<template>
  <div
    class="custom-card"
    :class="{
      'is-hoverable': hoverable,
      'is-clickable': clickable,
    }"
    @click="handleClick"
  >
    <div v-if="$slots.header || title" class="custom-card-header">
      <slot name="header">
        <div class="custom-card-header-content">
          <h3 v-if="title" class="custom-card-title">{{ title }}</h3>
          <p v-if="subtitle" class="custom-card-subtitle">{{ subtitle }}</p>
        </div>
      </slot>
    </div>

    <div class="custom-card-body">
      <slot />
    </div>

    <div v-if="$slots.footer" class="custom-card-footer">
      <slot name="footer" />
    </div>
  </div>
</template>

<script setup lang="ts">
interface Props {
  title?: string
  subtitle?: string
  hoverable?: boolean
  clickable?: boolean
}

const props = withDefaults(defineProps<Props>(), {
  title: '',
  subtitle: '',
  hoverable: false,
  clickable: false,
})

const emit = defineEmits<{
  click: [event: MouseEvent]
}>()

const handleClick = (event: MouseEvent) => {
  if (props.clickable) {
    emit('click', event)
  }
}
</script>

<style scoped>
.custom-card {
  background: #ffffff;
  border: 1px solid #e5e7eb;
  border-radius: 12px;
  overflow: hidden;
  transition: all 0.25s ease;
}

.custom-card.is-hoverable:hover {
  border-color: #d1d5db;
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.05);
}

.custom-card.is-clickable {
  cursor: pointer;
}

.custom-card.is-clickable:hover {
  border-color: #d1d5db;
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.05);
  transform: translateY(-2px);
}

.custom-card-header {
  padding: 16px 20px;
  border-bottom: 1px solid #e5e7eb;
}

.custom-card-header-content {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.custom-card-title {
  margin: 0;
  font-size: 16px;
  font-weight: 600;
  color: #111827;
}

.custom-card-subtitle {
  margin: 0;
  font-size: 14px;
  color: #6b7280;
  line-height: 1.5;
}

.custom-card-body {
  padding: 20px;
}

.custom-card-footer {
  padding: 16px 20px;
  border-top: 1px solid #e5e7eb;
  background: #f9fafb;
}
</style>
