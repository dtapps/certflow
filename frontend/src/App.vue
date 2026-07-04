<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { useRoute } from 'vue-router'
import Sidebar from './components/Sidebar.vue'
import TopBar from './components/TopBar.vue'
import LoginDialog from './components/LoginDialog.vue'
import * as AuthService from '@bindings/cnb.cool/dtapp/certflow/authservicewrapper'
import { useTheme } from './stores/theme'

const route = useRoute()
const sidebarOpen = ref(true)
const isAuthenticated = ref(true)
const { theme } = useTheme()

// 独立页面（不显示导航）
const isStandalonePage = computed(() => route.path === '/log-viewer')

function toggleSidebar() {
  sidebarOpen.value = !sidebarOpen.value
}

function handleVerified() {
  isAuthenticated.value = true
}

onMounted(async () => {
  isAuthenticated.value = !(await AuthService.IsPasswordSet())
})
</script>

<template>
  <!-- 登录弹窗 -->
  <LoginDialog v-if="!isAuthenticated" @verified="handleVerified" />

  <!-- 独立页面（不带导航） -->
  <router-view v-else-if="isStandalonePage" v-slot="{ Component }">
    <component :is="Component" />
  </router-view>

  <!-- 主界面 -->
  <div v-else class="flex h-full w-full overflow-y-hidden" :style="{ backgroundColor: 'var(--color-bg-base)' }">
    <!-- Sidebar -->
    <Sidebar :collapsed="!sidebarOpen" @toggle="toggleSidebar" />

    <!-- Main Content -->
    <div class="flex-1 flex flex-col overflow-hidden w-full">
      <TopBar @toggle-sidebar="toggleSidebar" />
      <main class="flex-1 overflow-y-auto p-4 w-full">
        <router-view v-slot="{ Component, route }">
          <component :is="Component" :key="route.path" />
        </router-view>
      </main>
    </div>
  </div>
</template>




