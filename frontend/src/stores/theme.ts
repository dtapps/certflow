import { ref, computed, watch } from 'vue'
import { defineStore } from 'pinia'
import { darkTheme, lightTheme } from 'naive-ui'
import type { GlobalThemeOverrides } from 'naive-ui'
import * as SystemService from '@bindings/cnb.cool/dtapp/certflow/systemservicewrapper'
import { useI18nStore } from './i18n'

export type ThemeMode = 'dark' | 'light' | 'auto'

const STORAGE_KEY = 'certflow-theme'

// Naive UI 主题覆盖配置
const baseThemeOverrides: GlobalThemeOverrides = {
  common: {
    borderRadius: '8px',
    fontFamily: "'PingFang SC', -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif",
  },
  Card: { borderRadius: '12px' },
  Button: { borderRadiusMedium: '8px' },
  Input: { borderRadius: '8px' },
  Tag: { borderRadius: '9999px' },
}

export const useThemeStore = defineStore('theme', () => {
  const { t } = useI18nStore()
  // 状态
  const theme = ref<ThemeMode>((localStorage.getItem(STORAGE_KEY) as ThemeMode) || 'auto')
  const systemDark = ref(false)

  // 计算属性
  const isDark = computed(() => {
    if (theme.value === 'auto') return systemDark.value
    return theme.value === 'dark'
  })

  const naiveTheme = computed(() => (isDark.value ? darkTheme : lightTheme))
  const naiveThemeOverrides = computed(() =>
    isDark.value ? baseThemeOverrides : baseThemeOverrides,
  )

  // 同步窗口标题栏背景色
  async function syncWindowAppearance(dark: boolean) {
    try {
      await SystemService.SetWindowAppearance(dark)
    } catch (e) {
      console.error(t('theme.windowAppearanceFailed'), e)
    }
  }

  // 方法
  function setTheme(mode: ThemeMode) {
    theme.value = mode
    localStorage.setItem(STORAGE_KEY, mode)
  }

  async function initTheme() {
    try {
      systemDark.value = await SystemService.IsDarkMode()
    } catch (e) {
      console.error(t('theme.detectSystemThemeFailed'), e)
    }
    // 初始化时同步窗口外观
    syncWindowAppearance(isDark.value)
  }

  // 监听 isDark 变化，同步窗口外观
  watch(isDark, (val) => {
    syncWindowAppearance(val)
  })

  // 跨窗口同步：监听 localStorage 变化
  window.addEventListener('storage', (e) => {
    if (e.key === STORAGE_KEY && e.newValue) {
      theme.value = e.newValue as ThemeMode
    }
  })

  // 初始化
  initTheme()

  return {
    theme,
    isDark,
    naiveTheme,
    naiveThemeOverrides,
    setTheme,
  }
})
