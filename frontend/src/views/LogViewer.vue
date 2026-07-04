<script setup lang="ts">
import { ref, computed, onMounted, watch } from 'vue'
import * as LoggingService from '@bindings/cnb.cool/dtapp/certflow/loggingservicewrapper'
import * as FileService from '@bindings/cnb.cool/dtapp/certflow/fileservicewrapper'
import { useI18n } from '../stores/i18n'

const { t } = useI18n()

// 日志文件列表
const logFiles = ref<string[]>([])
const selectedLogFile = ref('')
const logTail = ref(100)
const logContent = ref('')
const loading = ref(false)

// 日志等级筛选
const selectedLevels = ref<Set<string>>(new Set(['DEBUG', 'INFO', 'WARN', 'ERROR']))
const levels = ['DEBUG', 'INFO', 'WARN', 'ERROR']
const levelColor: Record<string, string> = {
  DEBUG: 'neutral',
  INFO: 'info',
  WARN: 'warning',
  ERROR: 'error',
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
  const lines = logContent.value.split('\n').filter(line => line.trim())
  return lines.filter(line => {
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
</script>

<template>
  <div class="flex flex-col h-screen bg-base-100">
    <!-- 工具栏 -->
    <div class="flex items-center gap-2 p-3 border-b border-base-300">
      <select v-model="selectedLogFile" class="select select-sm select-bordered w-48">
        <option v-for="f in logFiles" :key="f" :value="f">{{ f }}</option>
      </select>

      <select v-model.number="logTail" class="select select-sm select-bordered w-28" @change="loadLogContent()">
        <option :value="100">{{ t('settings.log.last100') }}</option>
        <option :value="500">{{ t('settings.log.last500') }}</option>
        <option :value="0">{{ t('settings.log.all') }}</option>
      </select>

      <div class="flex items-center gap-1 ml-2">
        <button
          v-for="level in levels"
          :key="level"
          @click="toggleLevel(level)"
          class="btn btn-xs"
          :class="selectedLevels.has(level) ? `btn-${levelColor[level]}` : 'btn-ghost'"
        >
          {{ level }}
        </button>
      </div>

      <div class="flex-1"></div>

      <button @click="refresh" class="btn btn-ghost btn-sm" :disabled="loading">
        <svg class="w-4 h-4" :class="{ 'animate-spin': loading }" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
        </svg>
      </button>

      <button @click="openLogDir" class="btn btn-ghost btn-sm" :title="t('settings.log.openDir')">
        <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M3 7v10a2 2 0 002 2h14a2 2 0 002-2V9a2 2 0 00-2-2h-6l-2-2H5a2 2 0 00-2 2z" />
        </svg>
      </button>
    </div>

    <!-- 日志内容 -->
    <div class="flex-1 overflow-auto p-4 font-mono text-sm leading-relaxed">
      <div v-if="loading" class="flex items-center justify-center h-32">
        <span class="loading loading-spinner loading-md"></span>
      </div>
      <div v-else-if="filteredLines.length === 0" class="flex items-center justify-center h-32 text-content-50">
        {{ t('settings.log.noContent') }}
      </div>
      <div v-else class="space-y-0.5">
        <div
          v-for="(line, index) in filteredLines"
          :key="index"
          class="log-line hover:bg-base-200 px-2 py-0.5 rounded"
          v-html="highlightLine(line)"
        ></div>
      </div>
    </div>

    <!-- 状态栏 -->
    <div class="flex items-center justify-between px-4 py-2 border-t border-base-300 text-xs text-content-50">
      <span>{{ filteredLines.length }} {{ t('common.unit') }}</span>
      <span>{{ selectedLogFile }}</span>
    </div>
  </div>
</template>

<style scoped>
.log-line {
  white-space: pre-wrap;
  word-break: break-all;
}
</style>
