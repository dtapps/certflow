import { ref, watch } from 'vue'
import * as SystemService from '@bindings/cnb.cool/dtapp/certflow/systemservicewrapper'
import { Events } from '@wailsio/runtime'

export type ThemeMode = 'dark' | 'light' | 'auto'

function getSystemThemeFallback(): 'dark' | 'light' {
  if (window.matchMedia && window.matchMedia('(prefers-color-scheme: light)').matches) {
    return 'light'
  }
  return 'dark'
}

const storedTheme = localStorage.getItem('certflow-theme') as ThemeMode | null
const currentTheme = ref<ThemeMode>(storedTheme || 'auto')

// 优先使用 Wails3 API，否则回退到 matchMedia
async function getSystemTheme(): Promise<'dark' | 'light'> {
  try {
    const isDark = await SystemService.IsDarkMode()
    return isDark ? 'dark' : 'light'
  } catch {
    return getSystemThemeFallback()
  }
}

function resolveTheme(theme: ThemeMode, systemDark?: 'dark' | 'light'): 'dark' | 'light' {
  if (theme === 'auto') {
    return systemDark || getSystemThemeFallback()
  }
  return theme
}

function applyTheme(theme: ThemeMode, systemDark?: 'dark' | 'light') {
  const resolved = resolveTheme(theme, systemDark)
  document.documentElement.setAttribute('data-theme', resolved)
  localStorage.setItem('certflow-theme', theme)
}

// Apply on init
applyTheme(currentTheme.value)

// Listen for system theme changes from Wails3
try {
  Events.On('theme_changed', () => {
    if (currentTheme.value === 'auto') {
      applyTheme('auto')
    }
  })
} catch {
  // Fallback: use matchMedia if Wails runtime not available
  if (window.matchMedia) {
    window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', () => {
      if (currentTheme.value === 'auto') {
        applyTheme('auto')
      }
    })
  }
}

watch(currentTheme, (val) => {
  applyTheme(val)
})

export function useTheme() {
  function setTheme(theme: ThemeMode) {
    currentTheme.value = theme
  }

  return {
    theme: currentTheme,
    setTheme,
  }
}
