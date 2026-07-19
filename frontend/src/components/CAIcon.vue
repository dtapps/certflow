<script setup lang="ts">
import { computed } from 'vue'
const props = withDefaults(
  defineProps<{
    /** CA 的 directory_url，用于推断品牌色 */
    directoryUrl?: string
    /** CA 显示名称，用于兜底首字母 */
    name?: string
    size?: number
  }>(),
  { size: 22 },
)

// 已知品牌的徽标底色；未命中则用名称哈希取色，保证稳定且一致
const brandColors: Record<string, string> = {
  'letsencrypt.org': '#2E9B4E',
  'zerossl.com': '#00A0B0',
  'buypass.com': '#1565C0',
  'litessl.com': '#00897B',
}

const palette = [
  '#2563eb',
  '#7c3aed',
  '#db2777',
  '#dc2626',
  '#ea580c',
  '#d97706',
  '#16a34a',
  '#0891b2',
  '#4f46e5',
  '#9333ea',
]

function hostOf(url?: string): string | null {
  if (!url) return null
  try {
    return new URL(url).hostname.toLowerCase()
  } catch {
    return null
  }
}

const bg = computed(() => {
  const host = hostOf(props.directoryUrl)
  if (host) {
    for (const suffix of Object.keys(brandColors)) {
      if (host === suffix || host.endsWith('.' + suffix) || host.endsWith(suffix)) {
        return brandColors[suffix]
      }
    }
  }
  const s = props.name || props.directoryUrl || '?'
  let h = 0
  for (let i = 0; i < s.length; i++) h = (h * 31 + s.charCodeAt(i)) >>> 0
  return palette[h % palette.length]
})

const monogram = computed(() => {
  const n = (props.name || '').trim()
  return n ? n[0].toUpperCase() : '?'
})
</script>

<template>
  <div
    class="flex items-center justify-center rounded-xl shrink-0"
    :style="{
      width: size + 12 + 'px',
      height: size + 12 + 'px',
      background: bg,
    }"
  >
    <span
      class="font-bold text-white select-none"
      :style="{ fontSize: Math.round(size * 0.5) + 'px', lineHeight: 1 }"
    >
      {{ monogram }}
    </span>
  </div>
</template>
