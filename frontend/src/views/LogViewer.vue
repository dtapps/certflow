<script setup lang="ts">
import { ref, computed, onMounted, watch } from 'vue'
import { storeToRefs } from 'pinia'
import { NSelect, NButton, NSpin, NIcon } from 'naive-ui'
import { RefreshOutline, FolderOpenOutline } from '@vicons/ionicons5'
import * as LoggingService from '@bindings/cnb.cool/dtapp/certflow/loggingservicewrapper'
import * as FileService from '@bindings/cnb.cool/dtapp/certflow/fileservicewrapper'
import { useI18nStore } from '../stores/i18n'
import { useThemeStore } from '../stores/theme'

const i18nStore = useI18nStore()
const { t } = i18nStore

const themeStore = useThemeStore()
const { isDark } = storeToRefs(themeStore)

// 日志文件列表
const logFiles = ref<string[]>([])
const selectedLogFile = ref('')
const logTail = ref(100)
const logContent = ref('')
const loading = ref(false)

// 日志等级筛选
const selectedLevels = ref<Set<string>>(new Set(['DEBUG', 'INFO', 'WARN', 'ERROR']))
const levels = ['DEBUG', 'INFO', 'WARN', 'ERROR']
const levelColor: Record<string, 'default' | 'info' | 'warning' | 'error'> = {
  DEBUG: 'default',
  INFO: 'info',
  WARN: 'warning',
  ERROR: 'error',
}
const levelLabels: Record<string, () => string> = {
  DEBUG: () => t('settings.log.level_debug'),
  INFO: () => t('settings.log.level_info'),
  WARN: () => t('settings.log.level_warn'),
  ERROR: () => t('settings.log.level_error'),
}

const toggleLevel = (level: string) => {
  if (selectedLevels.value.has(level)) {
    selectedLevels.value.delete(level)
  } else {
    selectedLevels.value.add(level)
  }
  selectedLevels.value = new Set(selectedLevels.value)
}

// 过滤后的日志行
const filteredLines = computed(() => {
  const lines = logContent.value.split('\n').filter((line) => line.trim())
  return lines.filter((line) => {
    for (const level of selectedLevels.value) {
      if (line.includes(`[${level}]`)) return true
    }
    return false
  })
})

// 高亮日志等级
const highlightLine = (line: string) => {
  return line
    .replace(/\[DEBUG\]/g, '<span class="log-debug">[DEBUG]</span>')
    .replace(/\[INFO\]/g, '<span class="log-info">[INFO]</span>')
    .replace(/\[WARN\]/g, '<span class="log-warn">[WARN]</span>')
    .replace(/\[ERROR\]/g, '<span class="log-error">[ERROR]</span>')
    .replace(/(\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}\.\d{3})/g, '<span class="log-time">$1</span>')
}

// 主题相关样式
const pageStyle = computed(() => ({
  backgroundColor: isDark.value ? '#1a1a2e' : '#ffffff',
  color: isDark.value ? '#e5e5e5' : '#1a1a2e',
}))

const toolbarStyle = computed(() => ({
  borderBottomColor: isDark.value ? 'rgba(255, 255, 255, 0.09)' : 'rgba(0, 0, 0, 0.09)',
}))

const statusStyle = computed(() => ({
  borderTopColor: isDark.value ? 'rgba(255, 255, 255, 0.09)' : 'rgba(0, 0, 0, 0.09)',
}))

const logLineStyle = computed(() => ({
  backgroundColor: isDark.value ? 'transparent' : 'transparent',
}))

// 加载日志文件列表
const loadLogFiles = async () => {
  try {
    const files = await LoggingService.GetLogFiles()
    logFiles.value = files || []
    if (files && files.length > 0 && !selectedLogFile.value) {
      selectedLogFile.value = files[files.length - 1]
    }
  } catch (e) {
    console.error(t('settings.log.loadListFailed'), e)
  }
}

// 加载日志内容
const loadLogContent = async () => {
  if (!selectedLogFile.value) return
  loading.value = true
  try {
    const content = await LoggingService.ReadLog(selectedLogFile.value, logTail.value)
    logContent.value = content || ''
  } catch (e) {
    console.error(t('settings.log.loadFailed'), e)
    logContent.value = t('settings.log.loadFailed') + ' ' + (e as Error).message
  } finally {
    loading.value = false
  }
}

// 打开日志目录
const openLogDir = async () => {
  const dir = await LoggingService.GetLogDir()
  if (dir) await FileService.OpenDirectory(dir)
}

// 刷新
const refresh = async () => {
  await loadLogFiles()
  await loadLogContent()
}

// 监听文件选择变化
watch(selectedLogFile, () => {
  loadLogContent()
})

onMounted(() => {
  loadLogFiles()
  loadLogContent()
})

const logTailOptions = [
  { label: t('settings.log.last100'), value: 100 },
  { label: t('settings.log.last500'), value: 500 },
  { label: t('settings.log.all'), value: 0 },
]

const logFileOptions = computed(() => logFiles.value.map((f) => ({ label: f, value: f })))
</script>

<template>
  <div class="flex flex-col h-screen" :style="pageStyle">
    <!-- 工具栏 -->
    <div class="flex items-center gap-2 p-3 border-b" :style="toolbarStyle">
      <n-select
        v-model:value="selectedLogFile"
        :options="logFileOptions"
        size="small"
        style="width: 192px"
      />

      <n-select
        v-model:value="logTail"
        :options="logTailOptions"
        size="small"
        style="width: 112px"
        @update:value="loadLogContent()"
      />

      <div class="flex items-center gap-1 ml-2">
        <n-button
          v-for="level in levels"
          :key="level"
          size="tiny"
          :type="selectedLevels.has(level) ? levelColor[level] : 'default'"
          @click="toggleLevel(level)"
        >
          {{ levelLabels[level]() }}
        </n-button>
      </div>

      <div class="flex-1"></div>

      <n-button quaternary circle size="small" @click="refresh" :disabled="loading">
        <template #icon>
          <n-icon :size="16" :class="{ 'animate-spin': loading }"><RefreshOutline /></n-icon>
        </template>
      </n-button>

      <n-button
        quaternary
        circle
        size="small"
        @click="openLogDir"
        :title="t('settings.log.openDir')"
      >
        <template #icon>
          <n-icon :size="16"><FolderOpenOutline /></n-icon>
        </template>
      </n-button>
    </div>

    <!-- 日志内容 -->
    <div class="flex-1 overflow-auto p-4 font-mono text-sm leading-relaxed">
      <n-spin :show="loading">
        <div
          v-if="!loading && filteredLines.length === 0"
          class="flex items-center justify-center h-32 opacity-50"
        >
          {{ t('settings.log.noContent') }}
        </div>
        <div v-else class="space-y-0.5">
          <div
            v-for="(line, index) in filteredLines"
            :key="index"
            class="px-2 py-0.5 rounded hover:bg-black/5 dark:hover:bg-white/5"
            style="white-space: pre-wrap; word-break: break-all"
            v-html="highlightLine(line)"
          ></div>
        </div>
      </n-spin>
    </div>

    <!-- 状态栏 -->
    <div
      class="flex items-center justify-between px-4 py-2 border-t text-xs opacity-50"
      :style="statusStyle"
    >
      <span>{{ filteredLines.length }} {{ t('common.unit') }}</span>
      <span>{{ selectedLogFile }}</span>
    </div>
  </div>
</template>
