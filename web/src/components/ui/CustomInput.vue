<template>
  <div class="custom-input-wrapper" :class="{ 'is-disabled': disabled }">
    <div class="custom-input-prefix" v-if="$slots.prefix || prefixIcon">
      <slot name="prefix">
        <component v-if="prefixIcon" :is="prefixIcon" class="custom-input-icon" />
      </slot>
    </div>

    <input
      ref="inputRef"
      :type="showPassword ? 'text' : type"
      :value="modelValue"
      :placeholder="placeholder"
      :disabled="disabled"
      :readonly="readonly"
      :maxlength="maxlength"
      class="custom-input"
      :class="{
        'has-prefix': $slots.prefix || prefixIcon,
        'has-suffix': $slots.suffix || suffixIcon || clearable || (type === 'password' && showPasswordToggle),
      }"
      @input="handleInput"
      @focus="handleFocus"
      @blur="handleBlur"
      @keyup.enter="handleEnter"
    />

    <div class="custom-input-suffix" v-if="$slots.suffix || suffixIcon || clearable || (type === 'password' && showPasswordToggle)">
      <button
        v-if="clearable && modelValue && !disabled"
        type="button"
        class="custom-input-clear"
        @click="handleClear"
      >
        <svg viewBox="0 0 24 24" width="16" height="16">
          <path
            fill="currentColor"
            d="M19 6.41L17.59 5 12 10.59 6.41 5 5 6.41 10.59 12 5 17.59 6.41 19 12 13.41 17.59 19 19 17.59 13.41 12z"
          />
        </svg>
      </button>

      <button
        v-if="type === 'password' && showPasswordToggle && !disabled"
        type="button"
        class="custom-input-password-toggle"
        @click="togglePassword"
      >
        <svg v-if="!showPassword" viewBox="0 0 24 24" width="16" height="16">
          <path
            fill="currentColor"
            d="M12 4.5C7 4.5 2.73 7.61 1 12c1.73 4.39 6 7.5 11 7.5s9.27-3.11 11-7.5c-1.73-4.39-6-7.5-11-7.5zM12 17c-2.76 0-5-2.24-5-5s2.24-5 5-5 5 2.24 5 5-2.24 5-5 5zm0-8c-1.66 0-3 1.34-3 3s1.34 3 3 3 3-1.34 3-3-1.34-3-3-3z"
          />
        </svg>
        <svg v-else viewBox="0 0 24 24" width="16" height="16">
          <path
            fill="currentColor"
            d="M12 7c2.76 0 5 2.24 5 5 0 .65-.13 1.26-.36 1.83l2.92 2.92c1.51-1.26 2.7-2.89 3.43-4.75-1.73-4.39-6-7.5-11-7.5-1.4 0-2.74.25-3.98.7l2.16 2.16C10.74 7.13 11.35 7 12 7zM2 4.27l2.28 2.28.46.46C3.08 8.3 1.78 10.02 1 12c1.73 4.39 6 7.5 11 7.5 1.55 0 3.03-.3 4.38-.84l.42.42L19.73 22 21 20.73 3.27 3 2 4.27zM7.53 9.8l1.55 1.55c-.05.21-.08.43-.08.65 0 1.66 1.34 3 3 3 .22 0 .44-.03.65-.08l1.55 1.55c-.67.33-1.41.53-2.2.53-2.76 0-5-2.24-5-5 0-.79.2-1.53.53-2.2zm4.31-.78l3.15 3.15.02-.16c0-1.66-1.34-3-3-3l-.17.01z"
          />
        </svg>
      </button>

      <slot name="suffix">
        <component v-if="suffixIcon" :is="suffixIcon" class="custom-input-icon" />
      </slot>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, type Component } from 'vue'

interface Props {
  modelValue?: string | number
  type?: string
  placeholder?: string
  disabled?: boolean
  readonly?: boolean
  clearable?: boolean
  showPasswordToggle?: boolean
  maxlength?: number
  prefixIcon?: Component
  suffixIcon?: Component
}

const props = withDefaults(defineProps<Props>(), {
  modelValue: '',
  type: 'text',
  placeholder: '',
  disabled: false,
  readonly: false,
  clearable: false,
  showPasswordToggle: true,
})

const emit = defineEmits<{
  'update:modelValue': [value: string | number]
  input: [event: Event]
  focus: [event: FocusEvent]
  blur: [event: FocusEvent]
  enter: [event: KeyboardEvent]
  clear: []
}>()

const inputRef = ref<HTMLInputElement>()
const showPassword = ref(false)

const handleInput = (event: Event) => {
  const target = event.target as HTMLInputElement
  emit('update:modelValue', target.value)
  emit('input', event)
}

const handleFocus = (event: FocusEvent) => {
  emit('focus', event)
}

const handleBlur = (event: FocusEvent) => {
  emit('blur', event)
}

const handleEnter = (event: KeyboardEvent) => {
  emit('enter', event)
}

const handleClear = () => {
  emit('update:modelValue', '')
  emit('clear')
  inputRef.value?.focus()
}

const togglePassword = () => {
  showPassword.value = !showPassword.value
}

const focus = () => {
  inputRef.value?.focus()
}

const blur = () => {
  inputRef.value?.blur()
}

defineExpose({
  focus,
  blur,
  inputRef,
})
</script>

<style scoped>
.custom-input-wrapper {
  position: relative;
  display: inline-flex;
  align-items: center;
  width: 100%;
}

.custom-input {
  width: 100%;
  padding: 10px 14px;
  font-size: var(--font-size-base);
  color: #0f172a;
  background: #fafbfc;
  border: 2px solid #e2e8f0;
  border-radius: var(--radius-lg);
  outline: none;
  transition: all var(--transition-base);
  font-family: inherit;
}

.custom-input::placeholder {
  color: #94a3b8;
}

.custom-input:hover:not(:disabled):not(:focus) {
  border-color: #0f172a;
  background: #ffffff;
}

.custom-input:focus {
  border-color: #0f172a;
  background: #ffffff;
  box-shadow: 0 0 0 3px rgba(15, 23, 42, 0.08);
}

.custom-input:disabled {
  background: #f1f5f9;
  color: #94a3b8;
  cursor: not-allowed;
}

.custom-input.has-prefix {
  padding-left: 40px;
}

.custom-input.has-suffix {
  padding-right: 40px;
}

.custom-input-prefix,
.custom-input-suffix {
  position: absolute;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  color: #94a3b8;
  z-index: 1;
}

.custom-input-prefix {
  left: var(--spacing-md);
}

.custom-input-suffix {
  right: var(--spacing-md);
  gap: var(--spacing-xs);
}

.custom-input-icon {
  font-size: var(--font-size-lg);
  display: inline-flex;
}

.custom-input-clear,
.custom-input-password-toggle {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  padding: 0;
  border: none;
  background: transparent;
  color: #94a3b8;
  cursor: pointer;
  transition: color var(--transition-fast);
}

.custom-input-clear:hover,
.custom-input-password-toggle:hover {
  color: #0f172a;
}

.is-disabled {
  opacity: 0.6;
}
</style>
