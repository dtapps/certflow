<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { storeToRefs } from 'pinia'
import { Window, System } from '@wailsio/runtime'
import { useThemeStore } from '../stores/theme'
import { useI18nStore } from '../stores/i18n'

const props = defineProps<{
  transparent?: boolean
}>()

const themeStore = useThemeStore()
const { isDark } = storeToRefs(themeStore)
const { t } = useI18nStore()

const isMaximised = ref(false)
const isMac = ref(false)
const isWindows = ref(false)
const isLinux = ref(false)

// 主题适配样式
const titlebarStyle = computed(() => {
  if (props.transparent) {
    return { backgroundColor: 'transparent', borderBottom: 'none' }
  }
  return {
    '--titlebar-bg': isDark.value ? '#1e1e2e' : '#f5f5f5',
    '--titlebar-border': isDark.value ? 'rgba(255,255,255,0.09)' : 'rgba(0,0,0,0.09)',
    '--titlebar-text': isDark.value ? '#e5e5e5' : '#333',
    '--win-hover': isDark.value ? 'rgba(255,255,255,0.1)' : 'rgba(0,0,0,0.05)',
    backgroundColor: 'var(--titlebar-bg)',
    borderBottom: '1px solid var(--titlebar-border)',
  }
})

onMounted(async () => {
  isMaximised.value = await Window.IsMaximised()

  // 支持 localStorage 调试：控制台设置 localStorage.setItem('debug-platform', 'win32')
  const debugPlatform = localStorage.getItem('debug-platform')

  if (debugPlatform === 'mac' || debugPlatform === 'darwin') {
    isMac.value = true
    isWindows.value = false
    isLinux.value = false
  } else if (debugPlatform === 'win32' || debugPlatform === 'windows') {
    isMac.value = false
    isWindows.value = true
    isLinux.value = false
  } else if (debugPlatform === 'linux') {
    isMac.value = false
    isWindows.value = false
    isLinux.value = true
  } else {
    // 自动检测（使用 Wails3 运行时 API）
    isMac.value = System.IsMac()
    isWindows.value = System.IsWindows()
    isLinux.value = System.IsLinux()
  }
})

function minimize() {
  Window.Minimise()
}

async function toggleMaximize() {
  await Window.ToggleMaximise()
  isMaximised.value = await Window.IsMaximised()
}

function close() {
  Window.Close()
}
</script>

<template>
  <header class="titlebar" :style="titlebarStyle">
    <!-- ===== macOS 原生样式 ===== -->
    <template v-if="isMac">
      <div class="mac-controls no-drag">
        <!-- 关闭 -->
        <button class="mac-btn mac-btn-close" @click="close" :title="t('common.close')">
          <svg viewBox="0 0 12 12" width="8" height="8">
            <path
              d="M3.5 3.5l5 5M8.5 3.5l-5 5"
              stroke="currentColor"
              stroke-width="1.1"
              stroke-linecap="round"
              fill="none"
            />
          </svg>
        </button>
        <!-- 最小化 -->
        <button class="mac-btn mac-btn-minimize" @click="minimize" :title="t('common.minimize')">
          <svg viewBox="0 0 12 12" width="8" height="8">
            <path
              d="M2.5 6h7"
              stroke="currentColor"
              stroke-width="1.1"
              stroke-linecap="round"
              fill="none"
            />
          </svg>
        </button>
        <!-- 最大化 -->
        <button
          class="mac-btn mac-btn-maximize"
          @click="toggleMaximize"
          :title="isMaximised ? t('common.restore') : t('common.maximize')"
        >
          <svg viewBox="0 0 12 12" width="8" height="8">
            <template v-if="!isMaximised">
              <!-- 绿色箭头：进入全屏 -->
              <path
                d="M3 4l3-2.5 3 2.5"
                stroke="currentColor"
                stroke-width="1.1"
                stroke-linecap="round"
                stroke-linejoin="round"
                fill="none"
              />
              <path
                d="M9 8l-3 2.5-3-2.5"
                stroke="currentColor"
                stroke-width="1.1"
                stroke-linecap="round"
                stroke-linejoin="round"
                fill="none"
              />
            </template>
            <template v-else>
              <!-- 还原箭头 -->
              <path
                d="M3.5 7.5l2.5 2.5 2.5-2.5"
                stroke="currentColor"
                stroke-width="1.1"
                stroke-linecap="round"
                stroke-linejoin="round"
                fill="none"
              />
              <path
                d="M8.5 4.5l-2.5-2.5-2.5 2.5"
                stroke="currentColor"
                stroke-width="1.1"
                stroke-linecap="round"
                stroke-linejoin="round"
                fill="none"
              />
            </template>
          </svg>
        </button>
      </div>
      <div class="drag-region"></div>
    </template>

    <!-- ===== Windows 原生样式 ===== -->
    <template v-else-if="isWindows">
      <div class="drag-region"></div>
      <div class="win-controls no-drag">
        <!-- 最小化 -->
        <button class="win-btn" @click="minimize" :title="t('common.minimize')">
          <svg viewBox="0 0 10 10" width="10" height="10">
            <path d="M0 5h10" stroke="currentColor" stroke-width="1" />
          </svg>
        </button>
        <!-- 最大化/还原 -->
        <button
          class="win-btn"
          @click="toggleMaximize"
          :title="isMaximised ? t('common.restore') : t('common.maximize')"
        >
          <svg viewBox="0 0 10 10" width="10" height="10">
            <template v-if="!isMaximised">
              <rect
                x="1"
                y="1"
                width="8"
                height="8"
                stroke="currentColor"
                stroke-width="1"
                fill="none"
              />
            </template>
            <template v-else>
              <rect
                x="0.5"
                y="2.5"
                width="6"
                height="6"
                stroke="currentColor"
                stroke-width="1"
                fill="none"
              />
              <path d="M2.5 2.5V1.5h6v6h-1" stroke="currentColor" stroke-width="1" fill="none" />
            </template>
          </svg>
        </button>
        <!-- 关闭 -->
        <button class="win-btn win-btn-close" @click="close" :title="t('common.close')">
          <svg viewBox="0 0 10 10" width="10" height="10">
            <path d="M1 1l8 8M9 1l-8 8" stroke="currentColor" stroke-width="1" />
          </svg>
        </button>
      </div>
    </template>

    <!-- ===== Linux (GNOME) 原生样式 ===== -->
    <template v-else>
      <div class="drag-region"></div>
      <div class="linux-controls no-drag">
        <!-- 最小化 -->
        <button class="linux-btn" @click="minimize" :title="t('common.minimize')">
          <svg viewBox="0 0 12 12" width="12" height="12">
            <path
              d="M2 6h8"
              stroke="currentColor"
              stroke-width="1.5"
              stroke-linecap="round"
              fill="none"
            />
          </svg>
        </button>
        <!-- 最大化/还原 -->
        <button
          class="linux-btn"
          @click="toggleMaximize"
          :title="isMaximised ? t('common.restore') : t('common.maximize')"
        >
          <svg viewBox="0 0 12 12" width="12" height="12">
            <template v-if="!isMaximised">
              <rect
                x="2"
                y="2"
                width="8"
                height="8"
                rx="1"
                stroke="currentColor"
                stroke-width="1.5"
                fill="none"
              />
            </template>
            <template v-else>
              <rect
                x="2.5"
                y="1.5"
                width="7"
                height="7"
                rx="1"
                stroke="currentColor"
                stroke-width="1.5"
                fill="none"
              />
              <path
                d="M4.5 1.5v7h7"
                stroke="currentColor"
                stroke-width="1.5"
                stroke-linecap="round"
                fill="none"
              />
            </template>
          </svg>
        </button>
        <!-- 关闭 -->
        <button class="linux-btn linux-btn-close" @click="close" :title="t('common.close')">
          <svg viewBox="0 0 12 12" width="12" height="12">
            <path
              d="M3 3l6 6M9 3l-6 6"
              stroke="currentColor"
              stroke-width="1.5"
              stroke-linecap="round"
              fill="none"
            />
          </svg>
        </button>
      </div>
    </template>
  </header>
</template>

<style scoped>
.titlebar {
  display: flex;
  align-items: center;
  height: 32px;
  user-select: none;
  flex-shrink: 0;
  position: relative;
  z-index: 200;
}

.drag-region {
  flex: 1;
  height: 100%;
  --wails-draggable: drag;
  -webkit-app-region: drag;
}

/* ===== macOS 原生交通灯按钮 ===== */
.mac-controls {
  display: flex;
  align-items: center;
  height: 100%;
  padding-left: 14px;
  gap: 8px;
}

.mac-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 12px;
  height: 12px;
  border: none;
  border-radius: 50%;
  cursor: pointer;
  padding: 0;
  transition: filter 0.15s;
}

.mac-btn svg {
  opacity: 0;
  transition: opacity 0.15s;
}

.mac-btn:hover svg {
  opacity: 1;
}

.mac-btn-close {
  background-color: #ff5f57;
}
.mac-btn-close:hover {
  background-color: #ff453a;
}
.mac-btn-close svg {
  color: #4d0000;
}

.mac-btn-minimize {
  background-color: #febc2e;
}
.mac-btn-minimize:hover {
  background-color: #f5a623;
}
.mac-btn-minimize svg {
  color: #995700;
}

.mac-btn-maximize {
  background-color: #28c840;
}
.mac-btn-maximize:hover {
  background-color: #1db934;
}
.mac-btn-maximize svg {
  color: #006500;
}

/* ===== Windows 10/11 原生按钮 ===== */
.win-controls {
  display: flex;
  height: 100%;
}

.win-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 46px;
  height: 100%;
  border: none;
  background: transparent;
  cursor: pointer;
  color: var(--titlebar-text, #333);
  transition: background-color 0.1s;
}

.win-btn:hover {
  background-color: var(--win-hover, rgba(0, 0, 0, 0.05));
}

.win-btn-close:hover {
  background-color: #c42b1c;
  color: white;
}

/* ===== Linux GNOME 原生按钮 ===== */
.linux-controls {
  display: flex;
  height: 100%;
}

.linux-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 36px;
  height: 100%;
  border: none;
  background: transparent;
  cursor: pointer;
  color: var(--titlebar-text, #5e5e5e);
  transition: background-color 0.15s;
}

.linux-btn:hover {
  background-color: var(--win-hover, rgba(0, 0, 0, 0.08));
}

.linux-btn-close:hover {
  background-color: #e74c3c;
  color: white;
}

.no-drag {
  -webkit-app-region: no-drag;
}
</style>
