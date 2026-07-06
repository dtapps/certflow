import { ref, onMounted } from 'vue'
import { defineStore } from 'pinia'
import { Events } from '@wailsio/runtime'
import {
  ListNotifications,
  CountUnread,
  MarkAsRead,
  MarkAllAsRead,
  DeleteNotification,
  ClearAllNotifications,
} from '../../bindings/cnb.cool/dtapp/certflow/notificationservicewrapper'
import * as $models from '../../bindings/cnb.cool/dtapp/certflow/models'

export interface NotificationItem {
  id: number
  title: string
  body: string
  category: string
  read: boolean
  created_at: string
}

export const useNotificationsStore = defineStore('notifications', () => {
  // 状态
  const notifications = ref<NotificationItem[]>([])
  const unreadCount = ref(0)
  let eventListenerRegistered = false
  let refreshTimer: ReturnType<typeof setInterval> | null = null

  // 方法
  async function refreshList() {
    try {
      const items = await ListNotifications(50, 0)
      if (!items) {
        notifications.value = []
        return
      }
      notifications.value = items.map((item: $models.NotificationItem) => ({
        id: item.id,
        title: item.title,
        body: item.body,
        category: item.category,
        read: item.read,
        created_at: item.created_at,
      }))
    } catch (e) {
      console.error('加载通知列表失败:', e)
    }
  }

  async function refreshUnread() {
    try {
      const count = await CountUnread()
      unreadCount.value = count
    } catch (e) {
      console.error('获取未读数量失败:', e)
    }
  }

  function setupEventListener() {
    if (eventListenerRegistered) return
    eventListenerRegistered = true
    Events.On('notification', () => {
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
    init,
    markAllRead,
    clearAll,
    remove,
    markRead,
    refreshList,
    refreshUnread,
  }
})
