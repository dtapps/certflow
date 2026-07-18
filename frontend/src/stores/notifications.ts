import { ref, onMounted } from 'vue'
import { defineStore } from 'pinia'
import { Events } from '@wailsio/runtime'
import { useI18nStore } from './i18n'
import {
  ListNotifications,
  CountUnread,
  MarkAsRead,
  MarkAllAsRead,
  DeleteNotification,
  ClearAllNotifications,
} from '../../bindings/cnb.cool/dtapp/certflow/notificationservicewrapper'
import * as $models from '../../bindings/cnb.cool/dtapp/certflow/models'
import { EventNotification } from '../utils/events'

export interface NotificationItem {
  id: number
  title: string
  body: string
  category: string
  level: string
  read: boolean
  created_at: string
}

export const useNotificationsStore = defineStore('notifications', () => {
  const { t } = useI18nStore()
  // 状态
  const notifications = ref<NotificationItem[]>([])
  const unreadCount = ref(0)
  let eventListenerRegistered = false
  let refreshTimer: ReturnType<typeof setInterval> | null = null

  // 分页加载状态
  const PAGE_SIZE = 50
  const offset = ref(0)
  const hasMore = ref(false)
  const loadingMore = ref(false)

  function mapItems(items: $models.NotificationItem[]) {
    return items.map((item) => ({
      id: item.id,
      title: item.title,
      body: item.body,
      category: item.category,
      level: item.level,
      read: item.read,
      created_at: item.created_at,
    }))
  }

  // 方法
  async function refreshList() {
    try {
      const items = await ListNotifications(PAGE_SIZE, 0)
      offset.value = 0
      if (!items) {
        notifications.value = []
        hasMore.value = false
        return
      }
      notifications.value = mapItems(items)
      hasMore.value = items.length === PAGE_SIZE
    } catch (e) {
      console.error(t('notifications.loadListFailed'), e)
    }
  }

  // 加载更多（滚动到底部时调用）
  async function loadMore() {
    if (loadingMore.value || !hasMore.value) return
    loadingMore.value = true
    try {
      const items = await ListNotifications(PAGE_SIZE, offset.value + PAGE_SIZE)
      if (items && items.length > 0) {
        offset.value += PAGE_SIZE
        notifications.value.push(...mapItems(items))
        hasMore.value = items.length === PAGE_SIZE
      } else {
        hasMore.value = false
      }
    } catch (e) {
      console.error(t('notifications.loadListFailed'), e)
    } finally {
      loadingMore.value = false
    }
  }

  async function refreshUnread() {
    try {
      const count = await CountUnread()
      unreadCount.value = count
    } catch (e) {
      console.error(t('notifications.getUnreadCountFailed'), e)
    }
  }

  function setupEventListener() {
    if (eventListenerRegistered) return
    eventListenerRegistered = true
    Events.On(EventNotification, (ev) => {
      console.log(
        t('event.received')
          .replace('{name}', EventNotification)
          .replace('{data}', JSON.stringify(ev.data)),
      )
      refreshList()
      refreshUnread()
    })
  }

  async function init() {
    setupEventListener()
    await refreshList()
    await refreshUnread()
    if (!refreshTimer) {
      refreshTimer = setInterval(refreshUnread, 30000)
    }
  }

  async function markAllRead() {
    await MarkAllAsRead()
    notifications.value.forEach((n) => {
      n.read = true
    })
    unreadCount.value = 0
  }

  async function clearAll() {
    await ClearAllNotifications()
    notifications.value = []
    unreadCount.value = 0
  }

  async function remove(id: number) {
    await DeleteNotification(id)
    notifications.value = notifications.value.filter((n) => n.id !== id)
    await refreshUnread()
  }

  async function markRead(id: number) {
    await MarkAsRead(id)
    const item = notifications.value.find((n) => n.id === id)
    if (item) item.read = true
    await refreshUnread()
  }

  return {
    notifications,
    unreadCount,
    hasMore,
    loadingMore,
    init,
    markAllRead,
    clearAll,
    remove,
    markRead,
    refreshList,
    refreshUnread,
    loadMore,
  }
})
