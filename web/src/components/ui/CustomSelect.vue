<template>
  <div class="custom-select" ref="selectRef" :class="{ 'is-disabled': disabled }">
    <div
      class="custom-select-trigger"
      :class="{ 'is-active': isOpen, 'has-value': hasValue }"
      @click="toggleDropdown"
    >
      <div class="custom-select-tags" v-if="multiple && selectedOptions.length > 0">
        <span
          v-for="option in selectedOptions.slice(0, maxTagCount)"
          :key="option.value"
          class="custom-select-tag"
        >
          {{ option.label }}
          <button
            type="button"
            class="custom-select-tag-close"
            @click.stop="removeOption(option)"
          >
            <svg viewBox="0 0 24 24" width="12" height="12">
              <path
                fill="currentColor"
                d="M19 6.41L17.59 5 12 10.59 6.41 5 5 6.41 10.59 12 5 17.59 6.41 19 12 13.41 17.59 19 19 17.59 13.41 12z"
              />
            </svg>
          </button>
        </span>
        <span v-if="selectedOptions.length > maxTagCount" class="custom-select-tag-count">
          +{{ selectedOptions.length - maxTagCount }}
        </span>
      </div>
      <input
        v-else-if="searchable && isOpen"
        ref="searchInputRef"
        v-model="searchQuery"
        class="custom-select-search"
        :placeholder="placeholder"
        @click.stop
      />
      <span v-else class="custom-select-placeholder">
        {{ hasValue ? selectedLabel : placeholder }}
      </span>

      <span class="custom-select-arrow">
        <svg viewBox="0 0 24 24" width="16" height="16">
          <path fill="currentColor" d="M7 10l5 5 5-5z" />
        </svg>
      </span>
    </div>

    <transition name="dropdown">
      <div v-show="isOpen" class="custom-select-dropdown">
        <div v-if="filteredOptions.length === 0" class="custom-select-empty">
          暂无数据
        </div>
        <div
          v-for="option in filteredOptions"
          :key="option.value"
          class="custom-select-option"
          :class="{
            'is-selected': isSelected(option),
            'is-disabled': option.disabled,
          }"
          @click="selectOption(option)"
        >
          <span v-if="multiple" class="custom-select-checkbox">
            <svg v-if="isSelected(option)" viewBox="0 0 24 24" width="16" height="16">
              <path
                fill="currentColor"
                d="M19 3H5c-1.1 0-2 .9-2 2v14c0 1.1.9 2 2 2h14c1.1 0 2-.9 2-2V5c0-1.1-.9-2-2-2zm-9 14l-5-5 1.41-1.41L10 14.17l7.59-7.59L19 8l-9 9z"
              />
            </svg>
            <svg v-else viewBox="0 0 24 24" width="16" height="16">
              <path
                fill="currentColor"
                d="M19 5v14H5V5h14m0-2H5c-1.1 0-2 .9-2 2v14c0 1.1.9 2 2 2h14c1.1 0 2-.9 2-2V5c0-1.1-.9-2-2-2z"
              />
            </svg>
          </span>
          {{ option.label }}
        </div>
      </div>
    </transition>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, onMounted, onBeforeUnmount } from 'vue'

interface Option {
  label: string
  value: string | number
  disabled?: boolean
}

interface Props {
  modelValue?: string | number | string[] | number[] | null
  options: Option[]
  placeholder?: string
  disabled?: boolean
  multiple?: boolean
  searchable?: boolean
  maxTagCount?: number
}

const props = withDefaults(defineProps<Props>(), {
  modelValue: null,
  placeholder: '请选择',
  disabled: false,
  multiple: false,
  searchable: false,
  maxTagCount: 3,
})

const emit = defineEmits<{
  'update:modelValue': [value: string | number | (string | number)[]]
  'change': [value: string | number | (string | number)[]]
}>()

const selectRef = ref<HTMLElement>()
const searchInputRef = ref<HTMLInputElement>()
const isOpen = ref(false)
const searchQuery = ref('')

const selectedOptions = computed(() => {
  if (props.multiple) {
    const values = Array.isArray(props.modelValue) ? props.modelValue : ([] as (string | number)[])
    return props.options.filter((opt) => values.includes(opt.value))
  }
  const option = props.options.find((opt) => opt.value === props.modelValue)
  return option ? [option] : []
})

const hasValue = computed(() => {
  if (props.multiple) {
    return Array.isArray(props.modelValue) && props.modelValue.length > 0
  }
  return props.modelValue !== '' && props.modelValue !== null && props.modelValue !== undefined
})

const selectedLabel = computed(() => {
  return selectedOptions.value[0]?.label || ''
})

const filteredOptions = computed(() => {
  if (!props.searchable || !searchQuery.value) {
    return props.options
  }
  const query = searchQuery.value.toLowerCase()
  return props.options.filter((opt) =>
    opt.label.toLowerCase().includes(query)
  )
})

const isSelected = (option: Option) => {
  if (props.multiple) {
    const values = Array.isArray(props.modelValue) ? props.modelValue : ([] as (string | number)[])
    return values.includes(option.value)
  }
  return props.modelValue === option.value
}

const selectOption = (option: Option) => {
  if (option.disabled) return

  if (props.multiple) {
    const values: (string | number)[] = Array.isArray(props.modelValue) ? [...props.modelValue] : []
    const index = values.indexOf(option.value)

    if (index > -1) {
      values.splice(index, 1)
    } else {
      values.push(option.value)
    }

    emit('update:modelValue', values)
    emit('change', values)
  } else {
    emit('update:modelValue', option.value)
    emit('change', option.value)
    isOpen.value = false
  }
}

const removeOption = (option: Option) => {
  if (props.multiple && Array.isArray(props.modelValue)) {
    const values: (string | number)[] = props.modelValue.filter((v: string | number) => v !== option.value)
    emit('update:modelValue', values)
    emit('change', values)
  }
}

const toggleDropdown = () => {
  if (props.disabled) return
  isOpen.value = !isOpen.value
  if (isOpen.value && props.searchable) {
    setTimeout(() => searchInputRef.value?.focus(), 0)
  }
}

const handleClickOutside = (event: MouseEvent) => {
  if (selectRef.value && !selectRef.value.contains(event.target as Node)) {
    isOpen.value = false
  }
}

watch(isOpen, (newVal) => {
  if (!newVal) {
    searchQuery.value = ''
  }
})

onMounted(() => {
  document.addEventListener('click', handleClickOutside)
})

onBeforeUnmount(() => {
  document.removeEventListener('click', handleClickOutside)
})
</script>

<style scoped>
.custom-select {
  position: relative;
  display: inline-block;
  width: 100%;
}

.custom-select-trigger {
  display: flex;
  align-items: center;
  justify-content: space-between;
  min-height: 40px;
  padding: 8px 14px;
  background: #fafbfc;
  border: 2px solid #e2e8f0;
  border-radius: 8px;
  cursor: pointer;
  transition: all 0.25s ease;
  font-size: 14px;
}

.custom-select-trigger:hover:not(.is-disabled) {
  border-color: #0f172a;
  background: #ffffff;
}

.custom-select-trigger.is-active {
  border-color: #0f172a;
  background: #ffffff;
  box-shadow: 0 0 0 3px rgba(15, 23, 42, 0.08);
}

.custom-select.is-disabled .custom-select-trigger {
  background: #f1f5f9;
  color: #94a3b8;
  cursor: not-allowed;
}

.custom-select-placeholder {
  color: #0f172a;
  flex: 1;
}

.custom-select-trigger:not(.has-value) .custom-select-placeholder {
  color: #94a3b8;
}

.custom-select-tags {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
  flex: 1;
}

.custom-select-tag {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 2px 8px;
  background: #f1f5f9;
  border-radius: 4px;
  font-size: 12px;
  color: #0f172a;
}

.custom-select-tag-close {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  padding: 0;
  border: none;
  background: transparent;
  color: #64748b;
  cursor: pointer;
  transition: color 0.2s;
}

.custom-select-tag-close:hover {
  color: #0f172a;
}

.custom-select-tag-count {
  padding: 2px 8px;
  background: #f1f5f9;
  border-radius: 4px;
  font-size: 12px;
  color: #64748b;
}

.custom-select-search {
  flex: 1;
  border: none;
  outline: none;
  background: transparent;
  font-size: 14px;
  color: #0f172a;
}

.custom-select-search::placeholder {
  color: #94a3b8;
}

.custom-select-arrow {
  display: inline-flex;
  align-items: center;
  color: #94a3b8;
  transition: transform 0.25s ease;
  margin-left: 8px;
}

.custom-select-trigger.is-active .custom-select-arrow {
  transform: rotate(180deg);
}

.custom-select-dropdown {
  position: absolute;
  top: calc(100% + 4px);
  left: 0;
  right: 0;
  max-height: 256px;
  overflow-y: auto;
  background: #ffffff;
  border: 1px solid #e2e8f0;
  border-radius: 8px;
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.1);
  z-index: 1000;
}

.custom-select-empty {
  padding: 12px 16px;
  color: #94a3b8;
  text-align: center;
  font-size: 14px;
}

.custom-select-option {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 10px 14px;
  cursor: pointer;
  transition: background 0.2s;
  font-size: 14px;
  color: #0f172a;
}

.custom-select-option:hover:not(.is-disabled) {
  background: #f8fafc;
}

.custom-select-option.is-selected {
  background: #f1f5f9;
  font-weight: 600;
}

.custom-select-option.is-disabled {
  color: #94a3b8;
  cursor: not-allowed;
}

.custom-select-checkbox {
  display: inline-flex;
  align-items: center;
  color: #94a3b8;
}

.custom-select-option.is-selected .custom-select-checkbox {
  color: #0f172a;
}

.dropdown-enter-active,
.dropdown-leave-active {
  transition: all 0.25s ease;
}

.dropdown-enter-from,
.dropdown-leave-to {
  opacity: 0;
  transform: translateY(-8px);
}
</style>
