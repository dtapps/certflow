<script setup lang="ts">
import { ref, watch, onMounted, onBeforeUnmount } from 'vue'
import { useRoute } from 'vue-router'
import { useTheme } from '../stores/theme'
import { useI18n } from '../stores/i18n'
import * as SettingsService from '@bindings/cnb.cool/dtapp/certflow/settingsservicewrapper'

defineProps<{ collapsed: boolean }>()
defineEmits<{ toggle: [] }>()

const route = useRoute()
const { theme: currentTheme, setTheme } = useTheme()
const { locale: currentLocale, t, setLocale } = useI18n()

const navItems = [
  { path: '/', name: 'nav.dashboard', icon: 'dashboard' },
  { path: '/certificates', name: 'nav.certificates', icon: 'certificate' },
  { path: '/ca', name: 'nav.ca', icon: 'ca' },
  { path: '/dns', name: 'nav.dns', icon: 'dns' },
  { path: '/monitor', name: 'nav.monitor', icon: 'monitor' },
  { path: '/settings', name: 'nav.settings', icon: 'settings' },
]

const isActive = (path: string) => {
  if (path === '/') return route.path === '/'
  return route.path.startsWith(path)
}

const themeLabel = (theme: string) => {
  if (theme === 'dark') return t('theme.dark')
  if (theme === 'light') return t('theme.light')
  return t('theme.auto')
}

// 下拉菜单状态
const showThemeDropdown = ref(false)
const showLocaleDropdown = ref(false)

// 点击外部关闭下拉菜单
const handleClickOutside = (e: MouseEvent) => {
  const target = e.target as HTMLElement
  if (!target.closest('.theme-dropdown')) {
    showThemeDropdown.value = false
  }
  if (!target.closest('.locale-dropdown')) {
    showLocaleDropdown.value = false
  }
}

onMounted(() => {
  document.addEventListener('click', handleClickOutside)
})

onBeforeUnmount(() => {
  document.removeEventListener('click', handleClickOutside)
})

// 同步设置到后端
watch(currentTheme, async (val) => {
  try {
    const settings = await SettingsService.GetSettings()
    if (settings.theme !== val) {
      settings.theme = val
      await SettingsService.SaveSettings(settings)
    }
  } catch (e) {
    console.error('同步主题到后端失败:', e)
  }
})

watch(currentLocale, async (val) => {
  try {
    const settings = await SettingsService.GetSettings()
    if (settings.language !== val) {
      settings.language = val
      await SettingsService.SaveSettings(settings)
    }
  } catch (e) {
    console.error('同步语言到后端失败:', e)
  }
})
</script>

<template>
  <aside
    class="flex flex-col transition-all duration-300"
    :class="collapsed ? 'w-16' : 'w-64'"
    :style="{ backgroundColor: 'var(--color-bg-surface)', borderRight: '1px solid var(--color-border)' }"
  >
    <!-- Logo -->
    <div class="flex items-center h-16 px-4" :style="{ borderBottom: '1px solid var(--color-border)' }">
      <div class="flex items-center gap-3">
        <div class="w-8 h-8 rounded-lg bg-gradient-to-br from-primary-500 to-accent-500 flex items-center justify-center">
          <svg class="w-5 h-5 text-white" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 15v2m-6 4h12a2 2 0 002-2v-6a2 2 0 00-2-2H6a2 2 0 00-2 2v6a2 2 0 002 2zm10-10V7a4 4 0 00-8 0v4h8z" />
          </svg>
        </div>
        <span v-if="!collapsed" class="text-lg font-bold text-gradient">CertFlow</span>
      </div>
    </div>

    <!-- 导航 -->
    <nav class="flex-1 py-4 space-y-1 px-2">
      <router-link
        v-for="item in navItems"
        :key="item.path"
        :to="item.path"
        class="flex items-center gap-3 px-3 py-2.5 rounded-lg transition-all duration-200"
        :style="isActive(item.path)
          ? { backgroundColor: 'rgb(76 110 245 / 0.15)', color: 'var(--color-primary-400)', border: '1px solid rgb(76 110 245 / 0.2)' }
          : { color: 'var(--color-text-secondary)' }"
      >
        <svg v-if="item.icon === 'dashboard'" class="w-5 h-5 flex-shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 6a2 2 0 012-2h2a2 2 0 012 2v2a2 2 0 01-2 2H6a2 2 0 01-2-2V6zM14 6a2 2 0 012-2h2a2 2 0 012 2v2a2 2 0 01-2 2h-2a2 2 0 01-2-2V6zM4 16a2 2 0 012-2h2a2 2 0 012 2v2a2 2 0 01-2 2H6a2 2 0 01-2-2v-2zM14 16a2 2 0 012-2h2a2 2 0 012 2v2a2 2 0 01-2 2h-2a2 2 0 01-2-2v-2z" /></svg>
        <svg v-else-if="item.icon === 'certificate'" class="w-5 h-5 flex-shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12l2 2 4-4m5.618-4.016A11.955 11.955 0 0112 2.944a11.955 11.955 0 01-8.618 3.04A12.02 12.02 0 003 9c0 5.591 3.824 10.29 9 11.622 5.176-1.332 9-6.03 9-11.622 0-1.042-.133-2.052-.382-3.016z" /></svg>
        <svg v-else-if="item.icon === 'ca'" class="w-5 h-5 flex-shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 21V5a2 2 0 00-2-2H7a2 2 0 00-2 2v16m14 0h2m-2 0h-5m-9 0H3m2 0h5M9 7h1m-1 4h1m4-4h1m-1 4h1m-5 10v-5a1 1 0 011-1h2a1 1 0 011 1v5m-4 0h4" /></svg>
        <svg v-else-if="item.icon === 'dns'" class="w-5 h-5 flex-shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M21 12a9 9 0 01-9 9m9-9a9 9 0 00-9-9m9 9H3m9 9a9 9 0 01-9-9m9 9c1.657 0 3-4.03 3-9s-1.343-9-3-9m0 18c-1.657 0-3-4.03-3-9s1.343-9 3-9m-9 9a9 9 0 019-9" /></svg>
        <svg v-else-if="item.icon === 'settings'" class="w-5 h-5 flex-shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.065 2.572c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37.996.608 2.296.07 2.572-1.065z" /><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z" /></svg>
        <svg v-else-if="item.icon === 'monitor'" class="w-5 h-5 flex-shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 19v-6a2 2 0 00-2-2H5a2 2 0 00-2 2v6a2 2 0 002 2h2a2 2 0 002-2m0 0V9a2 2 0 012-2h2a2 2 0 012 2v10m-6 0a2 2 0 002 2h2a2 2 0 002-2m0 0V5a2 2 0 012-2h2a2 2 0 012 2v14a2 2 0 01-2 2h-2a2 2 0 01-2-2z" /></svg>
        <span v-if="!collapsed" class="text-sm font-medium">{{ t(item.name) }}</span>
      </router-link>
    </nav>

    <!-- 底部工具区 -->
    <div class="p-3" :style="{ borderTop: '1px solid var(--color-border)' }">
      <!-- 展开状态 -->
      <div v-if="!collapsed" class="flex items-center justify-between mb-2 px-1">
        <!-- 主题：往右展开 -->
        <div class="relative theme-dropdown">
          <button
            @click.stop="showThemeDropdown = !showThemeDropdown"
            class="flex items-center gap-1.5 px-2 py-1 rounded-md text-xs cursor-pointer"
            :style="{ color: 'var(--color-text-secondary)' }"
          >
            <svg v-if="currentTheme === 'dark'" class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M20.354 15.354A9 9 0 018.646 3.646 9.003 9.003 0 0012 21a9.003 9.003 0 008.354-5.646z" /></svg>
            <svg v-else-if="currentTheme === 'light'" class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 3v1m0 16v1m9-9h-1M4 12H3m15.364 6.364l-.707-.707M6.343 6.343l-.707-.707m12.728 0l-.707.707M6.343 17.657l-.707.707M16 12a4 4 0 11-8 0 4 4 0 018 0z" /></svg>
            <svg v-else class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9.75 17L9 20l-1 1h8l-1-1-.75-3M3 13h18M5 17h14a2 2 0 002-2V5a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z" /></svg>
            <span>{{ themeLabel(currentTheme) }}</span>
          </button>
          <div
            v-if="showThemeDropdown"
            class="absolute bottom-full left-0 mb-1 w-28 rounded-box shadow-sm z-50"
            :style="{ backgroundColor: 'var(--color-bg-surface)', border: '1px solid var(--color-border)', color: 'var(--color-text-primary)' }"
          >
            <div class="p-1">
              <button @click="setTheme('dark'); showThemeDropdown = false" class="flex items-center gap-2 w-full px-2 py-1.5 rounded text-sm hover:bg-base-200" :style="{ color: currentTheme === 'dark' ? 'var(--color-primary-400)' : 'var(--color-text-primary)' }">🌙 {{ t('theme.dark') }}</button>
              <button @click="setTheme('light'); showThemeDropdown = false" class="flex items-center gap-2 w-full px-2 py-1.5 rounded text-sm hover:bg-base-200" :style="{ color: currentTheme === 'light' ? 'var(--color-primary-400)' : 'var(--color-text-primary)' }">☀️ {{ t('theme.light') }}</button>
              <button @click="setTheme('auto'); showThemeDropdown = false" class="flex items-center gap-2 w-full px-2 py-1.5 rounded text-sm hover:bg-base-200" :style="{ color: currentTheme === 'auto' ? 'var(--color-primary-400)' : 'var(--color-text-primary)' }">💻 {{ t('theme.auto') }}</button>
            </div>
          </div>
        </div>

        <!-- 语言：往左展开 -->
        <div class="relative locale-dropdown">
          <button
            @click.stop="showLocaleDropdown = !showLocaleDropdown"
            class="flex items-center gap-1.5 px-2 py-1 rounded-md text-xs cursor-pointer"
            :style="{ color: 'var(--color-text-secondary)' }"
          >
            <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M3 5h12M9 3v2m1.048 9.5A18.022 18.022 0 016.412 9m6.088 9h7M11 21l5-10 5 10M12.751 5C11.783 10.77 8.07 15.61 3 18.129" /></svg>
            <span>{{ currentLocale === 'auto' ? t('lang.auto') : (currentLocale === 'zh-CN' ? t('lang.zh') : t('lang.en')) }}</span>
          </button>
          <div
            v-if="showLocaleDropdown"
            class="absolute bottom-full right-0 mb-1 w-28 rounded-box shadow-sm z-50"
            :style="{ backgroundColor: 'var(--color-bg-surface)', border: '1px solid var(--color-border)', color: 'var(--color-text-primary)' }"
          >
            <div class="p-1">
              <button @click="setLocale('zh-CN'); showLocaleDropdown = false" class="flex items-center gap-2 w-full px-2 py-1.5 rounded text-sm hover:bg-base-200" :style="{ color: currentLocale === 'zh-CN' ? 'var(--color-primary-400)' : 'var(--color-text-primary)' }">🇨🇳 {{ t('lang.zh') }}</button>
              <button @click="setLocale('en-US'); showLocaleDropdown = false" class="flex items-center gap-2 w-full px-2 py-1.5 rounded text-sm hover:bg-base-200" :style="{ color: currentLocale === 'en-US' ? 'var(--color-primary-400)' : 'var(--color-text-primary)' }">🇺🇸 {{ t('lang.en') }}</button>
              <button @click="setLocale('auto'); showLocaleDropdown = false" class="flex items-center gap-2 w-full px-2 py-1.5 rounded text-sm hover:bg-base-200" :style="{ color: currentLocale === 'auto' ? 'var(--color-primary-400)' : 'var(--color-text-primary)' }">🔄 {{ t('lang.auto') }}</button>
            </div>
          </div>
        </div>
      </div>

      <!-- 收起状态 -->
      <div v-else class="flex flex-col items-center gap-1">
        <!-- 主题 -->
        <div class="relative theme-dropdown">
          <button
            @click.stop="showThemeDropdown = !showThemeDropdown"
            class="p-2 rounded-md cursor-pointer"
            :style="{ color: 'var(--color-text-secondary)' }"
            :title="themeLabel(currentTheme)"
          >
            <svg v-if="currentTheme === 'dark'" class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M20.354 15.354A9 9 0 018.646 3.646 9.003 9.003 0 0012 21a9.003 9.003 0 008.354-5.646z" /></svg>
            <svg v-else-if="currentTheme === 'light'" class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 3v1m0 16v1m9-9h-1M4 12H3m15.364 6.364l-.707-.707M6.343 6.343l-.707-.707m12.728 0l-.707.707M6.343 17.657l-.707.707M16 12a4 4 0 11-8 0 4 4 0 018 0z" /></svg>
            <svg v-else class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9.75 17L9 20l-1 1h8l-1-1-.75-3M3 13h18M5 17h14a2 2 0 002-2V5a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z" /></svg>
          </button>
          <div
            v-if="showThemeDropdown"
            class="absolute bottom-full right-0 mb-1 w-28 rounded-box shadow-sm z-50"
            :style="{ backgroundColor: 'var(--color-bg-surface)', border: '1px solid var(--color-border)', color: 'var(--color-text-primary)' }"
          >
            <div class="p-1">
              <button @click="setTheme('dark'); showThemeDropdown = false" class="flex items-center gap-2 w-full px-2 py-1.5 rounded text-sm hover:bg-base-200" :style="{ color: currentTheme === 'dark' ? 'var(--color-primary-400)' : 'var(--color-text-primary)' }">🌙 {{ t('theme.dark') }}</button>
              <button @click="setTheme('light'); showThemeDropdown = false" class="flex items-center gap-2 w-full px-2 py-1.5 rounded text-sm hover:bg-base-200" :style="{ color: currentTheme === 'light' ? 'var(--color-primary-400)' : 'var(--color-text-primary)' }">☀️ {{ t('theme.light') }}</button>
              <button @click="setTheme('auto'); showThemeDropdown = false" class="flex items-center gap-2 w-full px-2 py-1.5 rounded text-sm hover:bg-base-200" :style="{ color: currentTheme === 'auto' ? 'var(--color-primary-400)' : 'var(--color-text-primary)' }">💻 {{ t('theme.auto') }}</button>
            </div>
          </div>
        </div>
        <!-- 语言 -->
        <div class="relative locale-dropdown">
          <button
            @click.stop="showLocaleDropdown = !showLocaleDropdown"
            class="p-2 rounded-md cursor-pointer"
            :style="{ color: 'var(--color-text-secondary)' }"
            :title="currentLocale === 'auto' ? t('lang.zh') : (currentLocale === 'zh-CN' ? t('lang.en') : t('lang.auto'))"
          >
            <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M3 5h12M9 3v2m1.048 9.5A18.022 18.022 0 016.412 9m6.088 9h7M11 21l5-10 5 10M12.751 5C11.783 10.77 8.07 15.61 3 18.129" /></svg>
          </button>
          <div
            v-if="showLocaleDropdown"
            class="absolute bottom-full left-0 mb-1 w-28 rounded-box shadow-sm z-50"
            :style="{ backgroundColor: 'var(--color-bg-surface)', border: '1px solid var(--color-border)', color: 'var(--color-text-primary)' }"
          >
            <div class="p-1">
              <button @click="setLocale('zh-CN'); showLocaleDropdown = false" class="flex items-center gap-2 w-full px-2 py-1.5 rounded text-sm hover:bg-base-200" :style="{ color: currentLocale === 'zh-CN' ? 'var(--color-primary-400)' : 'var(--color-text-primary)' }">🇨🇳 {{ t('lang.zh') }}</button>
              <button @click="setLocale('en-US'); showLocaleDropdown = false" class="flex items-center gap-2 w-full px-2 py-1.5 rounded text-sm hover:bg-base-200" :style="{ color: currentLocale === 'en-US' ? 'var(--color-primary-400)' : 'var(--color-text-primary)' }">🇺🇸 {{ t('lang.en') }}</button>
              <button @click="setLocale('auto'); showLocaleDropdown = false" class="flex items-center gap-2 w-full px-2 py-1.5 rounded text-sm hover:bg-base-200" :style="{ color: currentLocale === 'auto' ? 'var(--color-primary-400)' : 'var(--color-text-primary)' }">🔄 {{ t('lang.auto') }}</button>
            </div>
          </div>
        </div>
      </div>

      <p class="text-xs text-center mt-2" :style="{ color: 'var(--color-text-muted)' }">v0.1.0 Alpha</p>
    </div>
  </aside>
</template>
