<script setup lang="ts">
import { h, computed, ref, onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { storeToRefs } from 'pinia'
import { NMenu, NIcon } from 'naive-ui'
import type { MenuOption } from 'naive-ui'
import { useI18nStore } from '../stores/i18n'
import { useThemeStore } from '../stores/theme'
import * as SystemService from '@bindings/cnb.cool/dtapp/certflow/systemservicewrapper'
import {
  GridOutline,
  ShieldCheckmarkOutline,
  BusinessOutline,
  GlobeOutline,
  TrendingUpOutline,
  SettingsOutline,
} from '@vicons/ionicons5'

defineProps<{ collapsed: boolean }>()
defineEmits<{ toggle: [] }>()

const route = useRoute()
const router = useRouter()
const i18nStore = useI18nStore()
const themeStore = useThemeStore()
const { t } = i18nStore
const { isDark } = storeToRefs(themeStore)

const appVersion = ref('')

onMounted(async () => {
  try {
    appVersion.value = await SystemService.GetVersion()
  } catch (e) {
    /* ignore */
  }
})

function renderIcon(icon: any) {
  return () => h(NIcon, null, { default: () => h(icon) })
}

const menuOptions = computed<MenuOption[]>(() => [
  { label: t('nav.dashboard'), key: '/', icon: renderIcon(GridOutline) },
  { label: t('nav.certificates'), key: '/certificates', icon: renderIcon(ShieldCheckmarkOutline) },
  { label: t('nav.ca'), key: '/ca', icon: renderIcon(BusinessOutline) },
  { label: t('nav.dns'), key: '/dns', icon: renderIcon(GlobeOutline) },
  { label: t('nav.monitor'), key: '/monitor', icon: renderIcon(TrendingUpOutline) },
  { label: t('nav.settings'), key: '/settings', icon: renderIcon(SettingsOutline) },
])

const activeKey = computed(() => {
  const path = route.path
  if (path === '/') return '/'
  const match = menuOptions.value.find(
    (opt) => opt.key !== '/' && path.startsWith(opt.key as string),
  )
  // 不在菜单中的页面（如 /personal-center）不选中任何菜单项
  return match?.key ?? null
})

// 动态主题样式
const themeVars = computed(() => ({
  '--sidebar-bg': isDark.value ? '#1a1a2e' : '#ffffff',
  '--sidebar-border': isDark.value ? 'rgba(255, 255, 255, 0.09)' : 'rgba(0, 0, 0, 0.09)',
  '--sidebar-text': isDark.value ? 'rgba(255, 255, 255, 0.82)' : 'rgba(0, 0, 0, 0.82)',
  '--sidebar-text-muted': isDark.value ? 'rgba(255, 255, 255, 0.4)' : 'rgba(0, 0, 0, 0.4)',
}))

function handleMenuUpdate(key: string) {
  router.push(key)
}
</script>

<template>
  <aside
    class="sidebar"
    :class="collapsed ? 'sidebar-collapsed' : 'sidebar-expanded'"
    :style="themeVars"
  >
    <!-- Logo -->
    <div class="sidebar-logo">
      <div class="flex items-center gap-3">
        <div class="w-8 h-8 rounded-lg bg-blue-500 flex items-center justify-center flex-shrink-0">
          <svg class="w-5 h-5 text-white" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path
              stroke-linecap="round"
              stroke-linejoin="round"
              stroke-width="2"
              d="M12 15v2m-6 4h12a2 2 0 002-2v-6a2 2 0 00-2-2H6a2 2 0 00-2 2v6a2 2 0 002 2zm10-10V7a4 4 0 00-8 0v4h8z"
            />
          </svg>
        </div>
        <span v-if="!collapsed" class="text-lg font-bold sidebar-text">CertFlow</span>
      </div>
    </div>

    <!-- 导航 -->
    <nav class="flex-1 pt-1 pb-4 overflow-y-auto">
      <n-menu
        :key="route.path"
        :collapsed="collapsed"
        :collapsed-width="64"
        :collapsed-icon-size="20"
        :indent="24"
        :options="menuOptions"
        :value="activeKey"
        @update:value="handleMenuUpdate"
      />
    </nav>

    <!-- 版本号 -->
    <div class="sidebar-footer">
      <p class="text-xs text-center sidebar-text-muted">{{ appVersion || 'unknown' }}</p>
    </div>
  </aside>
</template>

<style scoped>
.sidebar {
  display: flex;
  flex-direction: column;
  height: 100vh;
  flex-shrink: 0;
  border-right: 1px solid var(--sidebar-border, rgba(0, 0, 0, 0.09));
  background-color: var(--sidebar-bg, #ffffff);
  transition: width 0.3s;
}

.sidebar-collapsed {
  width: 68px;
}

.sidebar-expanded {
  width: 220px;
}

.sidebar-logo {
  display: flex;
  align-items: center;
  height: 4rem;
  padding: 0 1rem;
  border-bottom: 1px solid var(--sidebar-border, rgba(0, 0, 0, 0.09));
}

.sidebar-footer {
  padding: 0.75rem;
  border-top: 1px solid var(--sidebar-border, rgba(0, 0, 0, 0.09));
}

.sidebar-text {
  color: var(--sidebar-text, rgba(0, 0, 0, 0.82));
}

.sidebar-text-muted {
  color: var(--sidebar-text-muted, rgba(0, 0, 0, 0.4));
}
</style>
