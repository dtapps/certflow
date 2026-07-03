<script setup lang="ts">
import { ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { useNotifications } from '../stores/notifications'
import { useI18n } from '../stores/i18n'
import { formatRelativeTime } from '../utils/format'
import * as CertificateService from '@bindings/cnb.cool/dtapp/certflow/certificateservicewrapper'
import * as CAService from '@bindings/cnb.cool/dtapp/certflow/caservicewrapper'

defineEmits<{
  toggleSidebar: []
}>()

const searchQuery = ref('')
const searchResults = ref<{ type: string; id: number; name: string }[]>([])
const showResults = ref(false)
const router = useRouter()
const { notifications, unreadCount, markAllRead, markRead, clearAll, remove } = useNotifications()
const { t } = useI18n()

const showNotifications = ref(false)

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
  } catch (e) { /* ignore */ }

  try {
    const cas = await CAService.ListCA()
    if (cas) {
      for (const ca of cas) {
        if (ca.name.toLowerCase().includes(q) || ca.account_email?.toLowerCase().includes(q)) {
          results.push({ type: 'ca', id: ca.id, name: ca.name })
        }
      }
    }
  } catch (e) { /* ignore */ }

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

const getCategoryIcon = (category: string) => {
  switch (category) {
    case 'cert_applied':
    case 'cert_renewed':
      return 'success'
    case 'cert_failed':
    case 'cert_revoked':
      return 'error'
    case 'cert_expiring':
      return 'warning'
    default:
      return 'info'
  }
}
</script>

<template>
  <header class="h-16 flex items-center justify-between px-6" :style="{ backgroundColor: 'var(--color-bg-surface)', borderBottom: '1px solid var(--color-border)', '--wails-draggable': 'drag' }">
    <!-- Left: Toggle + Search -->
    <div class="flex items-center gap-4">
      <button
        @click="$emit('toggleSidebar')"
        class="p-2 rounded-lg transition-colors"
        :style="{ color: 'var(--color-text-secondary)', '--wails-draggable': 'no-drag' }"
        @mouseenter="($event.target as HTMLElement).style.setProperty('background-color', 'var(--color-bg-hover)')"
        @mouseleave="($event.target as HTMLElement).style.setProperty('background-color', 'transparent')"
      >
        <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 6h16M4 12h16M4 18h16" />
        </svg>
      </button>

      <div class="relative" style="--wails-draggable: no-drag">
        <svg class="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24" :style="{ color: 'var(--color-text-muted)' }">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
        </svg>
        <input
          v-model="searchQuery"
          type="text"
          :placeholder="t('topbar.search')"
          class="input pl-10 w-64 text-sm"
          @focus="searchResults.length > 0 && (showResults = true)"
        />
        <!-- Search Results Dropdown -->
        <div
          v-if="showResults"
          class="absolute top-full left-0 mt-2 w-80 rounded-xl shadow-xl border overflow-hidden z-50"
          :style="{ backgroundColor: 'var(--color-bg-surface)', borderColor: 'var(--color-border)' }"
        >
          <div v-if="searchResults.length === 0" class="px-4 py-6 text-center text-sm" :style="{ color: 'var(--color-text-muted)' }">
            {{ t('topbar.noResults') }}
          </div>
          <div v-else class="max-h-64 overflow-y-auto">
            <div
              v-for="item in searchResults"
              :key="item.type + '-' + item.id"
              class="px-4 py-2.5 cursor-pointer transition-colors border-b last:border-b-0"
              :style="{ borderColor: 'var(--color-border)' }"
              @click="goToResult(item)"
              @mouseenter="($event.currentTarget as HTMLElement).style.setProperty('background-color', 'var(--color-bg-hover)')"
              @mouseleave="($event.currentTarget as HTMLElement).style.setProperty('background-color', 'transparent')"
            >
              <div class="flex items-center gap-2">
                <span class="px-1.5 py-0.5 rounded text-[10px] font-medium" :style="item.type === 'cert' ? { backgroundColor: 'var(--color-primary)', color: 'var(--color-primary-content)' } : { backgroundColor: 'var(--color-accent)', color: 'var(--color-accent-content)' }">
                  {{ item.type === 'cert' ? 'SSL' : 'CA' }}
                </span>
                <span class="text-sm truncate" :style="{ color: 'var(--color-text-primary)' }">{{ item.name }}</span>
              </div>
            </div>
          </div>
        </div>
        <div v-if="showResults" class="fixed inset-0 z-40" @click="showResults = false"></div>
      </div>
    </div>

    <!-- Right: Actions -->
    <div class="flex items-center gap-3">
      <!-- Notifications -->
      <div class="relative" style="--wails-draggable: no-drag">
        <button
          @click="showNotifications = !showNotifications"
          class="p-2 rounded-lg transition-colors relative"
          :style="{ color: 'var(--color-text-secondary)' }"
          @mouseenter="($event.target as HTMLElement).style.setProperty('background-color', 'var(--color-bg-hover)')"
          @mouseleave="($event.target as HTMLElement).style.setProperty('background-color', 'transparent')"
        >
          <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 17h5l-1.405-1.405A2.032 2.032 0 0118 14.158V11a6.002 6.002 0 00-4-5.659V5a2 2 0 10-4 0v.341C7.67 6.165 6 8.388 6 11v3.159c0 .538-.214 1.055-.595 1.436L4 17h5m6 0v1a3 3 0 11-6 0v-1m6 0H9" />
          </svg>
          <span
            v-if="unreadCount > 0"
            class="absolute -top-0.5 -right-0.5 min-w-[18px] h-[18px] flex items-center justify-center text-[10px] font-bold text-white bg-error rounded-full px-1"
          >
            {{ unreadCount > 99 ? '99+' : unreadCount }}
          </span>
        </button>

        <!-- Notification Dropdown -->
        <div
          v-if="showNotifications"
          class="absolute right-0 top-full mt-2 w-80 rounded-xl shadow-xl border overflow-hidden z-50"
          :style="{ backgroundColor: 'var(--color-bg-surface)', borderColor: 'var(--color-border)' }"
        >
          <!-- Header -->
          <div class="flex items-center justify-between px-4 py-3 border-b" :style="{ borderColor: 'var(--color-border)' }">
            <span class="text-sm font-medium" :style="{ color: 'var(--color-text-primary)' }">{{ t('topbar.notifications') }}</span>
            <div class="flex items-center gap-2">
              <button
                v-if="notifications.length > 0"
                @click="markAllRead"
                class="text-xs px-2 py-1 rounded-md transition-colors"
                :style="{ color: 'var(--color-text-muted)' }"
                @mouseenter="($event.target as HTMLElement).style.setProperty('background-color', 'var(--color-bg-hover)')"
                @mouseleave="($event.target as HTMLElement).style.setProperty('background-color', 'transparent')"
              >
                {{ t('topbar.markAllRead') }}
              </button>
              <button
                v-if="notifications.length > 0"
                @click="clearAll"
                class="text-xs px-2 py-1 rounded-md transition-colors"
                :style="{ color: 'var(--color-text-muted)' }"
                @mouseenter="($event.target as HTMLElement).style.setProperty('background-color', 'var(--color-bg-hover)')"
                @mouseleave="($event.target as HTMLElement).style.setProperty('background-color', 'transparent')"
              >
                {{ t('topbar.clear') }}
              </button>
            </div>
          </div>

          <!-- Notification List -->
          <div class="max-h-80 overflow-y-auto">
            <div
              v-if="notifications.length === 0"
              class="flex flex-col items-center justify-center py-10"
              :style="{ color: 'var(--color-text-muted)' }"
            >
              <svg class="w-10 h-10 mb-2 opacity-40" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M15 17h5l-1.405-1.405A2.032 2.032 0 0118 14.158V11a6.002 6.002 0 00-4-5.659V5a2 2 0 10-4 0v.341C7.67 6.165 6 8.388 6 11v3.159c0 .538-.214 1.055-.595 1.436L4 17h5m6 0v1a3 3 0 11-6 0v-1m6 0H9" />
              </svg>
              <p class="text-sm">{{ t('topbar.noNotifications') }}</p>
            </div>

            <div
              v-for="item in notifications"
              :key="item.id"
              class="px-4 py-3 border-b cursor-pointer transition-colors"
              :style="{ borderBottomColor: 'var(--color-border)' }"
              :class="{ 'opacity-60': item.read }"
              @click="markRead(item.id)"
              @mouseenter="($event.currentTarget as HTMLElement).style.setProperty('background-color', 'var(--color-bg-hover)')"
              @mouseleave="($event.currentTarget as HTMLElement).style.setProperty('background-color', 'transparent')"
            >
              <div class="flex items-start gap-3">
                <div
                  class="w-2 h-2 rounded-full mt-1.5 flex-shrink-0"
                  :style="{
                    backgroundColor:
                      getCategoryIcon(item.category) === 'success' ? 'var(--color-success)' :
                      getCategoryIcon(item.category) === 'error' ? 'var(--color-error)' :
                      getCategoryIcon(item.category) === 'warning' ? 'var(--color-warning)' : 'var(--color-info)'
                  }"
                ></div>
                <div class="flex-1 min-w-0">
                  <div class="flex items-center justify-between gap-2">
                    <p class="text-sm font-medium truncate" :style="{ color: 'var(--color-text-primary)' }">{{ item.title }}</p>
                    <button
                      @click.stop="remove(item.id)"
                      class="flex-shrink-0 p-0.5 rounded opacity-0 group-hover:opacity-100 transition-opacity"
                      :style="{ color: 'var(--color-text-muted)' }"
                      :title="t('topbar.delete')"
                    >
                      <svg class="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
                      </svg>
                    </button>
                  </div>
                  <p class="text-xs mt-0.5 line-clamp-2" :style="{ color: 'var(--color-text-secondary)' }">{{ item.body }}</p>
                  <p class="text-xs mt-1" :style="{ color: 'var(--color-text-muted)' }">{{ formatRelativeTime(item.created_at) }}</p>
                </div>
              </div>
            </div>
          </div>
        </div>

        <!-- Backdrop -->
        <div
          v-if="showNotifications"
          class="fixed inset-0 z-40"
          @click="showNotifications = false"
        ></div>
      </div>

      <!-- User -->
      <button @click="goToPersonalCenter" class="flex items-center gap-2 p-1.5 rounded-lg transition-colors" :title="t('topbar.personalCenter')" style="--wails-draggable: no-drag" @mouseenter="($event.target as HTMLElement).style.setProperty('background-color', 'var(--color-bg-hover)')" @mouseleave="($event.target as HTMLElement).style.setProperty('background-color', 'transparent')">
        <div class="w-8 h-8 rounded-full bg-gradient-to-br from-primary-500 to-accent-500 flex items-center justify-center text-white text-sm font-medium">
          A
        </div>
      </button>
    </div>
  </header>
</template>
