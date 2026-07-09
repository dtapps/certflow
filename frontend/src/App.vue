<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { storeToRefs } from 'pinia'
import { NConfigProvider, NMessageProvider, NDialogProvider } from 'naive-ui'
import { Events } from '@wailsio/runtime'
import Sidebar from './components/Sidebar.vue'
import TopBar from './components/TopBar.vue'
import LoginDialog from './components/LoginDialog.vue'
import * as AuthService from '@bindings/cnb.cool/dtapp/certflow/authservicewrapper'
import { useThemeStore } from './stores/theme'
import { useI18nStore } from './stores/i18n'
import { EventAuthVerified, EventNavigate } from './utils/events'

const route = useRoute()
const router = useRouter()
const sidebarOpen = ref(true)
const isAuthenticated = ref(true)
const themeStore = useThemeStore()
const { isDark, naiveTheme, naiveThemeOverrides } = storeToRefs(themeStore)
const { t } = useI18nStore()

function toggleSidebar() {
  sidebarOpen.value = !sidebarOpen.value
}

function handleVerified() {
  isAuthenticated.value = true
}

onMounted(async () => {
  // 清除旧的认证缓存
  localStorage.removeItem('certflow-auth-verified')

  // 检查是否需要密码验证
  const needPassword = await AuthService.IsPasswordSet()
  if (needPassword) {
    // 每次启动都需要密码验证
    isAuthenticated.value = false
  } else {
    isAuthenticated.value = true
  }
})

// 监听其他窗口的认证状态同步
Events.On(EventAuthVerified, (ev) => {
  console.log(
    t('event.received')
      .replace('{name}', EventAuthVerified)
      .replace('{data}', JSON.stringify(ev.data)),
  )
  isAuthenticated.value = true
})

// 监听菜单导航事件
Events.On(EventNavigate, (ev) => {
  const data = ev.data as { path: string } | undefined
  console.log(
    t('event.received').replace('{name}', EventNavigate).replace('{data}', JSON.stringify(data)),
  )
  if (data?.path) {
    router.push(data.path)
  }
})

const mainStyle = computed(() => ({
  backgroundColor: isDark.value ? '#1a1a2e' : '#ffffff',
}))

// 设置全局 CSS 变量
const rootStyle = computed(() => ({
  '--app-text-color': isDark.value ? '#e5e5e5' : '#1a1a2e',
  '--app-text-secondary': isDark.value ? 'rgba(255, 255, 255, 0.6)' : 'rgba(0, 0, 0, 0.6)',
  '--app-text-muted': isDark.value ? 'rgba(255, 255, 255, 0.4)' : 'rgba(0, 0, 0, 0.4)',
}))
</script>

<template>
  <n-config-provider :theme="naiveTheme" :theme-overrides="naiveThemeOverrides" :style="rootStyle">
    <n-message-provider>
      <n-dialog-provider>
        <!-- 登录弹窗 -->
        <LoginDialog v-if="!isAuthenticated" @verified="handleVerified" />

        <!-- 主界面 -->
        <div v-else class="app-layout" :style="mainStyle">
          <!-- Sidebar -->
          <Sidebar :collapsed="!sidebarOpen" @toggle="toggleSidebar" />

          <!-- Main Content -->
          <div class="app-content">
            <TopBar :sidebar-open="sidebarOpen" @toggle-sidebar="toggleSidebar" />
            <main class="app-main">
              <router-view v-slot="{ Component, route }">
                <component :is="Component" :key="route.path" />
              </router-view>
            </main>
          </div>
        </div>
      </n-dialog-provider>
    </n-message-provider>
  </n-config-provider>
</template>

<style scoped>
.app-layout {
  display: flex;
  height: 100vh;
  width: 100vw;
  overflow: hidden;
}

.app-content {
  flex: 1;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.app-main {
  flex: 1;
  overflow-y: auto;
  padding: 1rem;
}
</style>
