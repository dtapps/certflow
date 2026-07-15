<script setup lang="ts">
import { ref, watch, onMounted, computed } from 'vue'
import { useRouter } from 'vue-router'
import { storeToRefs } from 'pinia'
import { NInput, NPopover, NBadge, NIcon, NButton, NTag, NDropdown } from 'naive-ui'
import type { DropdownOption } from 'naive-ui'
import { useNotificationsStore } from '../stores/notifications'
import { useI18nStore } from '../stores/i18n'
import { useThemeStore } from '../stores/theme'
import { formatRelativeTime } from '../utils/format'
import * as CertificateService from '@bindings/cnb.cool/dtapp/certflow/certificateservicewrapper'
import * as CAService from '@bindings/cnb.cool/dtapp/certflow/caservicewrapper'
import * as SettingsService from '@bindings/cnb.cool/dtapp/certflow/settingsservicewrapper'
import {
  SearchOutline,
  MoonOutline,
  SunnyOutline,
  DesktopOutline,
  NotificationsOutline,
  LanguageOutline,
  PersonOutline,
} from '@vicons/ionicons5'

const searchQuery = ref('')
const searchResults = ref<{ type: string; id: number; name: string }[]>([])
const showResults = ref(false)
const router = useRouter()
const notificationsStore = useNotificationsStore()
const { notifications, unreadCount } = storeToRefs(notificationsStore)
const { markAllRead, markRead, clearAll, remove } = notificationsStore

const i18nStore = useI18nStore()
const { t } = i18nStore
const { locale: currentLocale } = storeToRefs(i18nStore)
const { setLocale } = i18nStore

const themeStore = useThemeStore()
const { theme, isDark } = storeToRefs(themeStore)
const { setTheme } = themeStore

const showNotifications = ref(false)

// 通知状态过滤（'' 表示全部）
const levelFilter = ref('')
const levelOptions = ['', 'success', 'error', 'warning', 'info']
const filteredNotifications = computed(() => {
  if (!levelFilter.value) return notifications.value
  return notifications.value.filter((n) => n.level === levelFilter.value)
})

// 动态主题样式
const topbarStyle = computed(() => ({
  borderBottomColor: isDark.value ? 'rgba(255, 255, 255, 0.09)' : 'rgba(0, 0, 0, 0.09)',
}))

const searchDropdownStyle = computed(() => ({
  borderColor: isDark.value ? 'rgba(255, 255, 255, 0.09)' : 'rgba(0, 0, 0, 0.09)',
  backgroundColor: isDark.value ? '#1a1a2e' : '#ffffff',
}))

const searchItemHoverStyle = computed(() => ({
  ':hover': {
    backgroundColor: isDark.value ? 'rgba(255, 255, 255, 0.05)' : 'rgba(0, 0, 0, 0.05)',
  },
}))

const clearSearch = () => {
  searchResults.value = []
  showResults.value = false
}

onMounted(() => {
  notificationsStore.init()
})

// 同步主题/语言到后端设置
watch(theme, async (val) => {
  try {
    const settings = await SettingsService.GetSettings()
    if (settings.theme !== val) {
      settings.theme = val
      await SettingsService.SaveSettings(settings)
    }
  } catch (e) {
    /* ignore */
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
    /* ignore */
  }
})

const goToPersonalCenter = () => {
  router.push('/personal-center')
}

const doSearch = async () => {
  const q = searchQuery.value.trim().toLowerCase()
  if (!q) {
    searchResults.value = []
    showResults.value = false
    return
  }

  const results: { type: string; id: number; name: string }[] = []

  try {
    const certs = await CertificateService.ListCertificates()
    if (certs) {
      for (const c of certs) {
        if (c.domain.toLowerCase().includes(q) || c.issuer?.toLowerCase().includes(q)) {
          results.push({ type: 'cert', id: c.id, name: c.domain })
        }
      }
    }
  } catch (e) {
    /* ignore */
  }

  try {
    const cas = await CAService.ListCA()
    if (cas) {
      for (const ca of cas) {
        if (ca.name.toLowerCase().includes(q) || ca.account_email?.toLowerCase().includes(q)) {
          results.push({ type: 'ca', id: ca.id, name: ca.name })
        }
      }
    }
  } catch (e) {
    /* ignore */
  }

  searchResults.value = results
  showResults.value = true
}

const goToResult = (result: { type: string; id: number }) => {
  showResults.value = false
  searchQuery.value = ''
  if (result.type === 'cert') {
    router.push('/certificates/' + result.id)
  } else {
    router.push('/ca')
  }
}

let debounceTimer: ReturnType<typeof setTimeout> | null = null
watch(searchQuery, () => {
  if (debounceTimer) clearTimeout(debounceTimer)
  debounceTimer = setTimeout(doSearch, 300)
})

// 通知状态 → 标签颜色（n-tag 的 type）
const getLevelColor = (level: string): 'success' | 'error' | 'warning' | 'info' | 'default' => {
  switch (level) {
    case 'success':
      return 'success'
    case 'error':
      return 'error'
    case 'warning':
      return 'warning'
    default:
      return 'info'
  }
}

// 通知状态 → 中文标签
const getLevelLabel = (level: string) => {
  switch (level) {
    case 'success':
      return t('notification.level.success')
    case 'error':
      return t('notification.level.error')
    case 'warning':
      return t('notification.level.warning')
    default:
      return t('notification.level.info')
  }
}

// 通知业务分类 → 中文标签
const getCategoryLabel = (category: string) => {
  switch (category) {
    case 'cert':
      return t('notification.category.cert')
    case 'deploy':
      return t('notification.category.deploy')
    case 'system':
      return t('notification.category.system')
    case 'monitor':
      return t('notification.category.monitor')
    case 'dns':
      return t('notification.category.dns')
    default:
      return category
  }
}

// 主题图标
const themeIcon = computed(() => {
  if (theme.value === 'dark') return MoonOutline
  if (theme.value === 'light') return SunnyOutline
  return DesktopOutline
})

// 主题下拉选项
const themeOptions = computed<DropdownOption[]>(() => [
  { label: t('theme.dark'), key: 'dark' },
  { label: t('theme.light'), key: 'light' },
  { label: t('theme.auto'), key: 'auto' },
])

// 语言下拉选项
const localeOptions = computed<DropdownOption[]>(() => [
  { label: t('lang.zh'), key: 'zh-CN' },
  { label: t('lang.en'), key: 'en-US' },
  { label: t('lang.auto'), key: 'auto' },
])

function handleThemeSelect(key: string) {
  setTheme(key as 'dark' | 'light' | 'auto')
}

function handleLocaleSelect(key: string) {
  setLocale(key as 'zh-CN' | 'en-US' | 'auto')
}
</script>

<template>
  <header class="topbar" :style="topbarStyle">
    <!-- Left: Search -->
    <div class="flex items-center gap-4">
      <div class="search-wrapper no-drag">
        <div class="relative">
          <n-input
            v-model:value="searchQuery"
            :placeholder="t('topbar.search')"
            size="small"
            clearable
            class="search-input"
            @focus="searchResults.length > 0 && (showResults = true)"
            @clear="clearSearch"
          >
            <template #prefix>
              <n-icon :size="16"><SearchOutline /></n-icon>
            </template>
          </n-input>
          <!-- Search Results Dropdown -->
          <div v-if="showResults" class="search-dropdown" :style="searchDropdownStyle">
            <div v-if="searchResults.length === 0" class="px-4 py-6 text-center text-sm opacity-50">
              {{ t('topbar.noResults') }}
            </div>
            <div v-else class="max-h-64 overflow-y-auto">
              <div
                v-for="item in searchResults"
                :key="item.type + '-' + item.id"
                class="search-item"
                @click="goToResult(item)"
              >
                <div class="flex items-center gap-2">
                  <n-tag
                    :type="item.type === 'cert' ? 'info' : 'success'"
                    size="small"
                    :bordered="false"
                  >
                    {{ item.type === 'cert' ? 'SSL' : 'CA' }}
                  </n-tag>
                  <span
                    class="text-sm truncate"
                    :style="{ color: isDark ? '#e5e5e5' : '#1a1a2e' }"
                    >{{ item.name }}</span
                  >
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>
      <div v-if="showResults" class="fixed inset-0 z-40" @click="showResults = false"></div>
    </div>

    <!-- Right: Actions -->
    <div class="flex items-center gap-2">
      <!-- 主题切换 -->
      <n-dropdown :options="themeOptions" @select="handleThemeSelect" trigger="click">
        <n-button
          quaternary
          circle
          size="small"
          class="no-drag"
          :title="t('settings.preferences.theme')"
        >
          <template #icon>
            <n-icon :size="18"><component :is="themeIcon" /></n-icon>
          </template>
        </n-button>
      </n-dropdown>

      <!-- 语言切换 -->
      <n-dropdown :options="localeOptions" @select="handleLocaleSelect" trigger="click">
        <n-button
          quaternary
          circle
          size="small"
          class="no-drag"
          :title="t('settings.preferences.language')"
        >
          <template #icon>
            <n-icon :size="18"><LanguageOutline /></n-icon>
          </template>
        </n-button>
      </n-dropdown>

      <!-- Notifications -->
      <n-popover
        v-model:show="showNotifications"
        trigger="click"
        placement="bottom-end"
        :style="{ width: '320px' }"
        class="no-drag"
      >
        <template #trigger>
          <n-badge :value="unreadCount" :max="99">
            <n-button quaternary circle size="small">
              <template #icon>
                <n-icon :size="18"><NotificationsOutline /></n-icon>
              </template>
            </n-button>
          </n-badge>
        </template>

        <template #header>
          <div class="flex items-center justify-between">
            <span class="text-sm font-medium">{{ t('topbar.notifications') }}</span>
            <div class="flex items-center gap-2">
              <n-button v-if="notifications.length > 0" text size="tiny" @click="markAllRead">
                {{ t('topbar.markAllRead') }}
              </n-button>
              <n-button v-if="notifications.length > 0" text size="tiny" @click="clearAll">
                {{ t('topbar.clear') }}
              </n-button>
            </div>
          </div>
          <div class="flex flex-wrap items-center gap-1 mt-2">
            <n-button
              v-for="lv in levelOptions"
              :key="lv"
              size="tiny"
              :type="levelFilter === lv ? 'primary' : 'default'"
              :secondary="levelFilter === lv"
              :bordered="levelFilter !== lv"
              @click="levelFilter = lv"
            >
              {{ lv === '' ? t('topbar.filterAll') : getLevelLabel(lv) }}
            </n-button>
          </div>
        </template>

        <div class="max-h-80 overflow-y-auto">
          <div
            v-if="notifications.length === 0"
            class="flex flex-col items-center justify-center py-10 opacity-50"
          >
            <n-icon :size="40" class="mb-2 opacity-40"><NotificationsOutline /></n-icon>
            <p class="text-sm">{{ t('topbar.noNotifications') }}</p>
          </div>
          <div
            v-else-if="filteredNotifications.length === 0"
            class="flex flex-col items-center justify-center py-10 opacity-50"
          >
            <n-icon :size="40" class="mb-2 opacity-40"><NotificationsOutline /></n-icon>
            <p class="text-sm">{{ t('topbar.noMatchingNotifications') }}</p>
          </div>

          <div
            v-for="item in filteredNotifications"
            :key="item.id"
            class="notification-item"
            :class="{ 'opacity-60': item.read }"
            @click="markRead(item.id)"
          >
            <div class="notification-content">
              <div class="flex-1 min-w-0">
                <div class="flex items-center gap-2 mb-0.5">
                  <n-tag :type="getLevelColor(item.level)" size="tiny" :bordered="false">
                    {{ getLevelLabel(item.level) }}
                  </n-tag>
                  <n-tag size="tiny" :bordered="false" type="default" class="opacity-50">
                    {{ getCategoryLabel(item.category) }}
                  </n-tag>
                  <p
                    class="text-xs font-medium truncate flex-1"
                    :style="{ color: isDark ? '#e5e5e5' : '#1a1a2e' }"
                  >
                    {{ item.title }}
                  </p>
                </div>
                <p class="text-xs opacity-60 line-clamp-2 mb-1">{{ item.body }}</p>
                <p class="text-xs opacity-40">{{ formatRelativeTime(item.created_at) }}</p>
              </div>
              <n-button text size="tiny" @click.stop="remove(item.id)" :title="t('topbar.delete')">
                <template #icon>
                  <n-icon :size="14"
                    ><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                      <path d="M6 18L18 6M6 6l12 12" /></svg
                  ></n-icon>
                </template>
              </n-button>
            </div>
          </div>
        </div>
      </n-popover>

      <!-- User -->
      <div
        class="user-avatar no-drag ml-2"
        :title="t('topbar.personalCenter')"
        @click="goToPersonalCenter"
      >
        A
      </div>
    </div>
  </header>
</template>

<style scoped>
.topbar {
  height: 4rem;
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 1.5rem;
  border-bottom: 1px solid rgba(0, 0, 0, 0.09);
}

:global(.dark) .topbar {
  border-bottom-color: rgba(255, 255, 255, 0.09);
}

.no-drag {
  --wails-draggable: no-drag;
}

.search-wrapper {
  position: relative;
}

.search-input {
  width: 256px;
}

.search-dropdown {
  position: absolute;
  top: 100%;
  left: 0;
  margin-top: 0.5rem;
  width: 20rem;
  border-radius: 0.75rem;
  box-shadow: 0 25px 50px -12px rgba(0, 0, 0, 0.25);
  border: 1px solid;
  overflow: hidden;
  z-index: 50;
}

.search-item {
  padding: 0.625rem 1rem;
  cursor: pointer;
  transition: background-color 0.15s;
}

.search-item:hover {
  background-color: rgba(0, 0, 0, 0.05);
}

:global(.dark) .search-item:hover {
  background-color: rgba(255, 255, 255, 0.05);
}

.notification-item {
  padding: 0.75rem 1rem;
  border-bottom: 1px solid rgba(0, 0, 0, 0.05);
  cursor: pointer;
  transition: background-color 0.15s;
}

:global(.dark) .notification-item {
  border-bottom-color: rgba(255, 255, 255, 0.05);
}

.notification-item:hover {
  background-color: rgba(0, 0, 0, 0.05);
}

:global(.dark) .notification-item:hover {
  background-color: rgba(255, 255, 255, 0.05);
}

.notification-content {
  display: flex;
  align-items: flex-start;
  gap: 0.5rem;
}

.user-avatar {
  width: 2rem;
  height: 2rem;
  border-radius: 50%;
  background: linear-gradient(to bottom right, #3b82f6, #8b5cf6);
  display: flex;
  align-items: center;
  justify-content: center;
  color: white;
  font-size: 0.75rem;
  font-weight: 500;
  cursor: pointer;
  transition: transform 0.15s;
  --wails-draggable: no-drag;
}

.user-avatar:hover {
  transform: scale(1.1);
}
</style>
