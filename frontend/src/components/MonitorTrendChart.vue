<script setup lang="ts">
import { ref, computed, watch, onMounted } from 'vue'
import { use } from 'echarts/core'
import { CanvasRenderer } from 'echarts/renderers'
import { LineChart, BarChart } from 'echarts/charts'
import {
  GridComponent,
  TooltipComponent,
  LegendComponent,
  DataZoomComponent,
} from 'echarts/components'
import VChart from 'vue-echarts'
import { storeToRefs } from 'pinia'
import { useThemeStore } from '../stores/theme'
import { useI18nStore } from '../stores/i18n'
import * as MonitorService from '@bindings/cnb.cool/dtapp/certflow/monitorservicewrapper'
import type { MonitorCheckLogItem } from '@bindings/cnb.cool/dtapp/certflow/internal/monitor/models'

use([
  CanvasRenderer,
  LineChart,
  BarChart,
  GridComponent,
  TooltipComponent,
  LegendComponent,
  DataZoomComponent,
])

const props = defineProps<{
  domainId: number
}>()

const { isDark } = storeToRefs(useThemeStore())
const i18nStore = useI18nStore()
const t = (key: string, params?: Record<string, any>) => i18nStore.t(key, params)

const loading = ref(false)
const errorMsg = ref('')
const history = ref<MonitorCheckLogItem[]>([])
const range = ref(30)

const rangeOptions = computed(() => [
  { label: t('monitor.trendRange7'), value: 7 },
  { label: t('monitor.trendRange30'), value: 30 },
  { label: t('monitor.trendRange90'), value: 90 },
])

async function loadHistory() {
  if (!props.domainId) return
  loading.value = true
  errorMsg.value = ''
  try {
    const res = await MonitorService.ListHistory(props.domainId, range.value)
    history.value = (res as MonitorCheckLogItem[]) || []
  } catch (e: any) {
    errorMsg.value = e?.message || String(e)
  } finally {
    loading.value = false
  }
}

onMounted(loadHistory)
watch([() => props.domainId, range], loadHistory)

const xData = computed(() => history.value.map((h) => h.checked_at))
const noData = computed(() => !loading.value && history.value.length === 0)

const axisColor = computed(() => (isDark.value ? '#9ca3af' : '#6b7280'))
const splitColor = computed(() => (isDark.value ? 'rgba(255,255,255,0.08)' : 'rgba(0,0,0,0.06)'))

const boxStyle = computed(() => ({
  border: `1px solid ${isDark.value ? 'rgba(255,255,255,0.12)' : '#e5e7eb'}`,
  background: isDark.value ? 'transparent' : '#ffffff',
}))
const chartBoxStyle = computed(() => ({
  background: isDark.value ? 'rgba(255,255,255,0.04)' : '#f9fafb',
}))
const labelStyle = computed(() => ({
  color: isDark.value ? '#e5e7eb' : '#374151',
}))

function baseGrid() {
  return { left: 52, right: 16, top: 30, bottom: 56 }
}

const responseOption = computed(() => ({
  grid: baseGrid(),
  tooltip: { trigger: 'axis' },
  xAxis: {
    type: 'category',
    data: xData.value,
    axisLabel: { color: axisColor.value, hideOverlap: true },
    axisLine: { lineStyle: { color: splitColor.value } },
  },
  yAxis: {
    type: 'value',
    name: t('monitor.trendResponseTime'),
    nameTextStyle: { color: axisColor.value },
    axisLabel: { color: axisColor.value },
    splitLine: { lineStyle: { color: splitColor.value } },
  },
  dataZoom: [{ type: 'inside' }, { type: 'slider', height: 16, bottom: 8 }],
  series: [
    {
      name: t('monitor.trendResponseTime'),
      type: 'line',
      smooth: true,
      showSymbol: false,
      data: history.value.map((h) => h.response_time_ms),
      areaStyle: { opacity: 0.15 },
      itemStyle: { color: '#3b82f6' },
      lineStyle: { color: '#3b82f6' },
    },
  ],
}))

const certOption = computed(() => ({
  grid: baseGrid(),
  tooltip: { trigger: 'axis' },
  xAxis: {
    type: 'category',
    data: xData.value,
    axisLabel: { color: axisColor.value, hideOverlap: true },
    axisLine: { lineStyle: { color: splitColor.value } },
  },
  yAxis: {
    type: 'value',
    name: t('monitor.trendCertDays'),
    nameTextStyle: { color: axisColor.value },
    axisLabel: { color: axisColor.value },
    splitLine: { lineStyle: { color: splitColor.value } },
  },
  dataZoom: [{ type: 'inside' }, { type: 'slider', height: 16, bottom: 8 }],
  series: [
    {
      name: t('monitor.trendCertDays'),
      type: 'line',
      smooth: true,
      showSymbol: false,
      data: history.value.map((h) => h.cert_remaining_days),
      areaStyle: { opacity: 0.15 },
      itemStyle: { color: '#10b981' },
      lineStyle: { color: '#10b981' },
    },
  ],
}))

const statusOption = computed(() => ({
  grid: baseGrid(),
  tooltip: { trigger: 'axis' },
  xAxis: {
    type: 'category',
    data: xData.value,
    axisLabel: { color: axisColor.value, hideOverlap: true },
    axisLine: { lineStyle: { color: splitColor.value } },
  },
  yAxis: {
    type: 'value',
    name: t('monitor.trendStatusCode'),
    nameTextStyle: { color: axisColor.value },
    axisLabel: { color: axisColor.value },
    splitLine: { lineStyle: { color: splitColor.value } },
  },
  dataZoom: [{ type: 'inside' }, { type: 'slider', height: 16, bottom: 8 }],
  series: [
    {
      name: t('monitor.trendStatusCode'),
      type: 'line',
      step: 'middle',
      showSymbol: true,
      symbolSize: 5,
      data: history.value.map((h) => h.http_status_code),
      itemStyle: { color: '#f59e0b' },
      lineStyle: { color: '#f59e0b' },
    },
  ],
}))

const availabilityOption = computed(() => ({
  grid: baseGrid(),
  tooltip: { trigger: 'axis' },
  xAxis: {
    type: 'category',
    data: xData.value,
    axisLabel: { color: axisColor.value, hideOverlap: true },
    axisLine: { lineStyle: { color: splitColor.value } },
  },
  yAxis: {
    type: 'value',
    min: 0,
    max: 1,
    name: t('monitor.trendAvailability'),
    nameTextStyle: { color: axisColor.value },
    axisLabel: {
      color: axisColor.value,
      formatter: (val: number) => (val === 1 ? t('monitor.trendUp') : t('monitor.trendDown')),
    },
    splitLine: { lineStyle: { color: splitColor.value } },
  },
  dataZoom: [{ type: 'inside' }, { type: 'slider', height: 16, bottom: 8 }],
  series: [
    {
      name: t('monitor.trendAvailability'),
      type: 'bar',
      data: history.value.map((h) => ({
        value: h.status === 'ok' || h.status === 'warning' ? 1 : 0,
        itemStyle: {
          color: h.status === 'ok' || h.status === 'warning' ? '#10b981' : '#ef4444',
        },
      })),
    },
  ],
}))
</script>

<template>
  <div class="mt-4 rounded-lg p-3" :style="boxStyle">
    <div class="mb-3 flex items-center justify-between">
      <span class="text-sm font-semibold" :style="labelStyle">{{ t('monitor.trend') }}</span>
      <n-select v-model:value="range" :options="rangeOptions" size="small" style="width: 120px" />
    </div>

    <div v-if="loading" class="py-8 text-center text-sm" :style="labelStyle">
      {{ t('monitor.trendLoading') }}
    </div>
    <div v-else-if="noData" class="py-8 text-center text-sm" :style="labelStyle">
      {{ t('monitor.trendNoData') }}
    </div>
    <div v-else class="grid grid-cols-1 gap-3 lg:grid-cols-2">
      <div class="rounded-md p-2" :style="chartBoxStyle">
        <v-chart :option="responseOption" autoresize style="height: 240px" />
      </div>
      <div class="rounded-md p-2" :style="chartBoxStyle">
        <v-chart :option="certOption" autoresize style="height: 240px" />
      </div>
      <div class="rounded-md p-2" :style="chartBoxStyle">
        <v-chart :option="statusOption" autoresize style="height: 240px" />
      </div>
      <div class="rounded-md p-2" :style="chartBoxStyle">
        <v-chart :option="availabilityOption" autoresize style="height: 240px" />
      </div>
    </div>
  </div>
</template>
