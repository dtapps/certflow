import { defineStore } from 'pinia'
import { ref } from 'vue'

export type ActionButtonType =
  'default' | 'primary' | 'error' | 'success' | 'warning' | 'info' | 'tertiary'

export interface ActionButton {
  text: string
  type?: ActionButtonType
  disabled?: boolean
  loading?: boolean
  /** 内置图标：prev=左箭头, next=右箭头, none=无（默认 none） */
  withIcon?: 'prev' | 'next' | 'none'
  onClick?: () => void
}

/**
 * 全局底部操作栏状态。
 * 默认不显示（visible=false），由各页面在需要时通过 setLeft/setRight + show 控制，
 * 离开页面时调用 hide 还原。组件 ActionBar.vue 固定在内容区底部。
 */
export const useActionBarStore = defineStore('actionBar', () => {
  const visible = ref(false)
  const left = ref<ActionButton | null>(null)
  const right = ref<ActionButton | null>(null)

  function show() {
    visible.value = true
  }

  function hide() {
    visible.value = false
    left.value = null
    right.value = null
  }

  function setLeft(btn: ActionButton | null) {
    left.value = btn
  }

  function setRight(btn: ActionButton | null) {
    right.value = btn
  }

  return { visible, left, right, show, hide, setLeft, setRight }
})
