<script setup lang="ts">
import { computed } from 'vue'

const props = withDefaults(
  defineProps<{
    /** 凭证的 provider_type，用于推断品牌色 */
    providerType?: string
    /** 凭证显示名称，用于兜底首字母 */
    name?: string
    size?: number
  }>(),
  { size: 22 },
)

// 已知服务商的品牌色；未命中则用 provider_type 哈希取色，保证稳定且一致
const brandColors: Record<string, string> = {
  cloudflare: '#F38020',
  aliyun: '#FF6A00',
  huawei: '#CF0A2C',
  tencentcloud: '#006EFF',
  aws: '#FF9900',
  googlecloud: '#4285F4',
  baiducloud: '#2932E1',
  jdcloud: '#E1251B',
  volcengine: '#00C2B2',
  edgeone: '#1E40AF',
  ucloud: '#0084FF',
  westcn: '#E60012',
  porkbun: '#E0322F',
  namecheap: '#F47216',
  godaddy: '#00315C',
  gandiv5: '#E73934',
  dynadot: '#1F4FA8',
  azuredns: '#0078D4',
  digitalocean: '#0080FF',
  vultr: '#007BFC',
  hetzner: '#D50C2D',
  linode: '#00A95C',
  ovh: '#123F6D',
  dnsimple: '#2D8CFF',
  ns1: '#1C2B46',
  ctyun: '#C8161D',
  btpanel: '#FF7700',
  '1panel': '#2D8CF0',
  acepanel: '#3B82F6',
  aliesa: '#FF6A00',
  openrestymanager: '#0FA968',
  uuwaf: '#7C3AED',
  safeline: '#E11D48',
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

const bg = computed(() => {
  const key = (props.providerType || '').toLowerCase()
  if (key && brandColors[key]) return brandColors[key]
  const s = props.providerType || props.name || '?'
  let h = 0
  for (let i = 0; i < s.length; i++) h = (h * 31 + s.charCodeAt(i)) >>> 0
  return palette[h % palette.length]
})

// 优先用 provider_type 首字母（服务商标识更稳定），否则用名称首字母
const monogram = computed(() => {
  const key = (props.providerType || '').trim()
  if (key) return key[0].toUpperCase()
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
