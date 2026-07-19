<script setup lang="ts">
import { NButton } from 'naive-ui'
import { storeToRefs } from 'pinia'
import { useThemeStore } from '../stores/theme'
import { useActionBarStore } from '../stores/actionBar'

const themeStore = useThemeStore()
const { isDark } = storeToRefs(themeStore)
const actionBar = useActionBarStore()
</script>

<template>
  <div
    v-if="actionBar.visible"
    class="action-bar"
    :class="isDark ? 'action-bar-dark' : 'action-bar-light'"
  >
    <!-- 左侧按钮（如：上一步） -->
    <div class="action-bar-left">
      <n-button
        v-if="actionBar.left"
        :type="actionBar.left.type ?? 'default'"
        :disabled="actionBar.left.disabled"
        :loading="actionBar.left.loading"
        @click="actionBar.left?.onClick?.()"
      >
        <template v-if="actionBar.left.withIcon === 'prev'" #icon>
          <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path
              stroke-linecap="round"
              stroke-linejoin="round"
              stroke-width="2"
              d="M15 19l-7-7 7-7"
            />
          </svg>
        </template>
        {{ actionBar.left.text }}
      </n-button>
    </div>

    <!-- 中间自定义内容（可选） -->
    <slot />

    <!-- 右侧按钮（如：下一步 / 提交） -->
    <div class="action-bar-right ml-auto">
      <n-button
        v-if="actionBar.right"
        :type="actionBar.right.type ?? 'primary'"
        :disabled="actionBar.right.disabled"
        :loading="actionBar.right.loading"
        @click="actionBar.right?.onClick?.()"
      >
        {{ actionBar.right.text }}
        <template v-if="actionBar.right.withIcon === 'next'" #icon>
          <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path
              stroke-linecap="round"
              stroke-linejoin="round"
              stroke-width="2"
              d="M9 5l7 7-7 7"
            />
          </svg>
        </template>
      </n-button>
    </div>
  </div>
</template>

<style scoped>
/* 公共底部操作栏：作为内容区底部 flex 子项固定显示，整页滚动时按钮始终可见 */
.action-bar {
  display: flex;
  align-items: center;
  padding: 0.75rem 1rem;
  border-top: 1px solid transparent;
  flex-shrink: 0;
  z-index: 10;
}
.action-bar-light {
  background-color: #ffffff;
  border-top-color: #e5e5e5;
}
.action-bar-dark {
  background-color: #18181b;
  border-top-color: #404040;
}
.action-bar-right {
  display: flex;
  align-items: center;
  gap: 0.5rem;
}
</style>
