import { ref, onMounted, onUnmounted } from 'vue'

type LunarTheme = 'lunar-dawn' | 'lunar-night'

function resolveTheme(): LunarTheme {
  const hour = new Date().getHours()
  if (hour >= 8 && hour < 19) return 'lunar-dawn'
  return 'lunar-night'
}

const currentTheme = ref<LunarTheme>(resolveTheme())

let timer: ReturnType<typeof setInterval> | null = null

function applyThemeBackground(theme: LunarTheme) {
  if (theme === 'lunar-night') {
    document.body.style.background = '#0b0f2b'
  } else {
    document.body.style.background = '#f5f7fa'
  }
}

export function useLunarTheme() {
  const updateTheme = () => {
    currentTheme.value = resolveTheme()
    applyThemeBackground(currentTheme.value)
  }

  onMounted(() => {
    applyThemeBackground(currentTheme.value)
    timer = setInterval(updateTheme, 60 * 1000)
  })

  onUnmounted(() => {
    if (timer) {
      clearInterval(timer)
      timer = null
    }
  })

  return {
    themeClass: currentTheme,
    isDark: () => currentTheme.value === 'lunar-night',
    isLight: () => currentTheme.value === 'lunar-dawn',
  }
}