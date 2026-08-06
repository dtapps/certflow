<script setup lang="ts">
import { ref, onMounted, watch, computed } from 'vue'
import { storeToRefs } from 'pinia'
import {
  NCard,
  NButton,
  NInput,
  NInputNumber,
  NSelect,
  NSwitch,
  NSpin,
  NForm,
  NFormItem,
  NTag,
  useMessage,
  useDialog,
} from 'naive-ui'
import * as SettingsService from '@bindings/cnb.cool/dtapp/certflow/settingsservicewrapper'
import * as NotificationService from '@bindings/cnb.cool/dtapp/certflow/notificationservicewrapper'
import * as SchedulerService from '@bindings/cnb.cool/dtapp/certflow/schedulerservicewrapper'
import * as MonitorService from '@bindings/cnb.cool/dtapp/certflow/monitorservicewrapper'
import * as LoggingService from '@bindings/cnb.cool/dtapp/certflow/loggingservicewrapper'
import * as BrowserService from '@bindings/cnb.cool/dtapp/certflow/browserservicewrapper'
import * as FileService from '@bindings/cnb.cool/dtapp/certflow/fileservicewrapper'
import * as WindowService from '@bindings/cnb.cool/dtapp/certflow/windowservicewrapper'
import * as AutostartService from '@bindings/cnb.cool/dtapp/certflow/autostartservicewrapper'
import * as SystemService from '@bindings/cnb.cool/dtapp/certflow/systemservicewrapper'
import * as DataService from '@bindings/cnb.cool/dtapp/certflow/internal/data'
import { copyToClipboard } from '../utils/clipboard'
import type {
  Settings,
  DNSConfig,
} from '@bindings/cnb.cool/dtapp/certflow/internal/settings/models'
import { useThemeStore } from '../stores/theme'
import { useI18nStore } from '../stores/i18n'
import { LOCALE_ZH_CN, LOCALE_EN_US, LOCALE_AUTO, type Locale } from '../locales/locale'
import { initMessage, showMessage } from '../utils/message'

// 内部使用的类型：确保数组不为 null
type SafeDNSConfig = Omit<DNSConfig, 'servers'> & { servers: string[] }
type SafeLogConfig = NonNullable<Settings['log']>
type SafeSettings = Omit<Settings, 'dns_configs' | 'proxy' | 'log'> & {
  dns_configs: SafeDNSConfig[]
  proxy: NonNullable<Settings['proxy']>
  log: SafeLogConfig
}

const defaultSettings: SafeSettings = {
  renew_interval: 1,
  auto_check_expiry: true,
  check_interval: 6,
  monitor_history_days: 90,
  http_log_retention_days: 30,
  language: 'auto',
  theme: 'auto',
  prerelease: false,
  dns_configs: [],
  proxy: { enabled: false, protocol: 'http', host: '', port: 8080 },
  log: { level: 'INFO', max_mb: 10, max_backups: 5 },
}

const settings = ref<SafeSettings>({ ...defaultSettings })
const originalSettings = ref<string>('')
const dnsEnabled = ref(false)
const autostartEnabled = ref(false)
const notificationEnabled = ref(false)

const loading = ref(false)
const appVersion = ref('')

// 日志查看器状态
const logFiles = ref<string[]>([])
const selectedLogFile = ref('certflow.log')
const logTail = ref(100)
const logContent = ref('')

const themeStore = useThemeStore()
const { theme: currentTheme, isDark } = storeToRefs(themeStore)
const { setTheme } = themeStore

const i18nStore = useI18nStore()
const { locale: currentLocale } = storeToRefs(i18nStore)
const { t, setLocale } = i18nStore

// 语言选择代理：v-model 写入经 setLocale 落盘（保留 auto 选择），
// 而持久化的 settings.language 由下方 watch 同步为「已解析」语言供后端使用。
const languageModel = computed({
  get: () => i18nStore.locale,
  set: (val: Locale) => setLocale(val),
})

// 主题选择代理：与 languageModel 对称，v-model 写入经 setTheme 落盘（保留 auto 选择），
// 后端持久化由 autoSave 接管，不在此处反向写回 store。
const themeModel = computed({
  get: () => themeStore.theme,
  set: (val: 'dark' | 'light' | 'auto') => setTheme(val),
})

const message = useMessage()
initMessage(message)
const dialog = useDialog()

// 数据管理（导入/导出）
const exporting = ref(false)
const importing = ref(false)
// 导入进度状态（由后端 GetImportStatus 轮询填充）
const importStatus = ref<{
  running: boolean
  stage: string
  current: number
  total: number
  error: string
  finished_at: number
}>({ running: false, stage: '', current: 0, total: 0, error: '', finished_at: 0 })

// 检测是否有变更
const hasChanges = computed(() => {
  return JSON.stringify(settings.value) !== originalSettings.value
})

// 防抖工具
const debounce = <T extends (...args: any[]) => any>(fn: T, ms: number) => {
  let timer: ReturnType<typeof setTimeout>
  return (...args: Parameters<T>) => {
    clearTimeout(timer)
    timer = setTimeout(() => fn(...args), ms)
  }
}

// 自动保存（防抖 500ms）
const autoSave = debounce(async () => {
  try {
    // 后端 i18n 与 cnb/GitHub 源判断需要「已解析」的语言（不含 auto），
    // 保存时把语言字段替换为解析结果，而 settings 中仍保留用户原始选择。
    await SettingsService.SaveSettings({
      ...settings.value,
      // 后端 i18n 与 cnb/GitHub 源判断需要「已解析」语言（不含 auto），
      // 更新源判断/日志/通知语言均以此为准；主题也以 store 真相源写入。
      language: i18nStore.getResolvedLocale(),
      theme: themeStore.theme,
    } as Settings)
    originalSettings.value = JSON.stringify(settings.value)
  } catch (e) {
    console.error(t('settings.saveFailed'), e)
  }
}, 500)

watch(
  settings,
  () => {
    if (hasChanges.value) autoSave()
  },
  { deep: true },
)

// 主题相关样式
const logContentStyle = computed(() => ({
  backgroundColor: isDark.value ? '#1f2937' : '#f3f4f6',
  color: isDark.value ? '#e5e5e5' : '#374151',
}))

const dnsItemStyle = computed(() => (enabled: boolean) => ({
  borderColor: enabled
    ? isDark.value
      ? '#3b82f6'
      : '#93c5fd'
    : isDark.value
      ? '#374151'
      : '#e5e7eb',
  backgroundColor: enabled
    ? isDark.value
      ? 'rgba(59, 130, 246, 0.1)'
      : '#eff6ff'
    : isDark.value
      ? 'rgba(31, 41, 55, 0.5)'
      : '#f9fafb',
}))

const dnsInputStyle = computed(() => ({
  backgroundColor: isDark.value ? '#111827' : '#ffffff',
  color: isDark.value ? '#e5e5e5' : '#374151',
  fontFamily: 'monospace',
  fontSize: '12px',
}))

// 同步 store -> settings（其他页面修改时）
watch(currentTheme, (val) => {
  settings.value.theme = val
})
watch(currentLocale, (val) => {
  settings.value.language = val
})

const loadSettings = async () => {
  loading.value = true
  try {
    const loaded = await SettingsService.GetSettings()
    // 确保数组不为 null（Wails 生成的类型中标注为 | null）
    const safe: SafeSettings = {
      ...loaded,
      dns_configs: (loaded.dns_configs || []).map((d) => ({
        ...d,
        servers: d.servers || [],
      })),
      proxy: loaded.proxy || { enabled: false, protocol: 'http', host: '', port: 8080 },
      log: loaded.log || { level: 'INFO', max_mb: 10, max_backups: 5 },
    }
    settings.value = safe
    console.debug(
      t('log.settingsLoad', {
        data: JSON.stringify({
          language: safe.language,
          theme: safe.theme,
          proxy: safe.proxy,
          dns_configs: safe.dns_configs.length,
          log: safe.log,
        }),
      }),
    )
    // 初始化 DNS 总开关状态
    dnsEnabled.value = safe.dns_configs.some((d) => d.enabled)
    // 关键：表单字段对齐「前端真相源」(store/localStorage)，绝不反向写回 store。
    // 这样即便后端持久化的 language/theme 与用户选择不一致，也不会覆盖 UI 选择，
    // 从而避免「进设置页语言/主题就变」的跳变问题。
    settings.value.theme = themeStore.theme
    settings.value.language = i18nStore.getLocale()
    originalSettings.value = JSON.stringify(settings.value)
  } catch (e) {
    console.error(t('settings.loadFailed'), e)
  } finally {
    loading.value = false
  }
}

// 实时设置通知权限
watch(notificationEnabled, async (enabled) => {
  try {
    if (enabled) {
      const authorized = await NotificationService.RequestPermission()
      if (!authorized) {
        notificationEnabled.value = false
        showMessage(t('settings.notification.permissionDenied'), 'warning')
      }
    }
  } catch (e) {
    console.error(t('settings.notification.permissionFailed'), e)
  }
})

// 实时设置开机自启
watch(autostartEnabled, async (enabled) => {
  try {
    if (enabled) {
      await AutostartService.Enable()
    } else {
      await AutostartService.Disable()
    }
  } catch (e) {
    console.error(t('settings.autostart.setFailed'), e)
  }
})

const handleTestNotification = async () => {
  try {
    await NotificationService.SendTestNotification()
    showMessage(t('settings.testNotificationSent'), 'success')
  } catch (e) {
    showMessage(t('settings.testNotificationFailed'), 'error')
  }
}

const handleRunRenewal = () => {
  SchedulerService.RunRenewalNow()
  showMessage(t('settings.renewalStarted'), 'success')
}

const handleRunExpiryCheck = () => {
  SchedulerService.RunExpiryCheckNow()
  showMessage(t('settings.expiryCheckStarted'), 'success')
}

const handleRunMonitorCheck = async () => {
  try {
    const domains = await MonitorService.List()
    const enabled = (domains ?? []).filter((d) => d !== null && d.enabled)
    for (const d of enabled) {
      if (d) await MonitorService.CheckNow(d.id)
    }
    showMessage(t('settings.monitorCheckStarted'), 'success')
  } catch (e) {
    showMessage(t('settings.monitorCheckFailed'), 'error')
  }
}

// 导出全部业务数据为 zip 文件（原生保存对话框）
const handleExportData = async () => {
  exporting.value = true
  try {
    const path = await DataService.Service.ExportData()
    if (path) {
      showMessage(t('settings.data.export.success'), 'success')
    }
  } catch (e) {
    showMessage(String(e), 'error')
  } finally {
    exporting.value = false
  }
}

// 导入导入进度轮询定时器
let importPollTimer: ReturnType<typeof setInterval> | null = null

// 启动进度轮询：导入改为后端异步执行，前端通过 GetImportStatus 轮询展示进度条，
// 直到 running=false，再依据 error 给出最终结果。
const startImportPoll = () => {
  if (importPollTimer) {
    clearInterval(importPollTimer)
  }
  importPollTimer = setInterval(async () => {
    try {
      const status = await DataService.Service.GetImportStatus()
      importStatus.value = status
      if (!status.running) {
        if (importPollTimer) {
          clearInterval(importPollTimer)
          importPollTimer = null
        }
        importing.value = false
        if (status.error) {
          showMessage(status.error, 'error')
        } else {
          showMessage(t('settings.data.import.success'), 'success')
        }
      }
    } catch {
      // 轮询失败忽略，下一周期重试
    }
  }, 300)
}

// 导入备份文件（先确认，再清空替换；原生打开对话框选文件）
const handleImportData = async () => {
  dialog.warning({
    title: t('settings.data.import'),
    content: t('settings.data.import.confirm'),
    positiveText: t('common.confirm') ?? 'OK',
    negativeText: t('common.cancel') ?? 'Cancel',
    onPositiveClick: async () => {
      importing.value = true
      importStatus.value = {
        running: true,
        stage: '',
        current: 0,
        total: 0,
        error: '',
        finished_at: 0,
      }
      try {
        // ImportData 仅负责弹窗选文件+解压，真正的数据库写入在后端异步执行并带进度
        await DataService.Service.ImportData()
        startImportPoll()
      } catch (e) {
        importing.value = false
        showMessage(String(e), 'error')
      }
    },
  })
}

const handleCheckUpdate = async () => {
  // 用「已解析」语言（不含 auto）判断走 cnb 还是 GitHub 发布页
  const lang = i18nStore.getResolvedLocale()
  if (lang === 'zh-CN') {
    await BrowserService.OpenURL('https://cnb.cool/dtapp/certflow/-/releases')
  } else {
    await BrowserService.OpenURL('https://github.com/dtapps/certflow/releases')
  }
}

// 切换自定义 DNS 总开关
const toggleDNS = () => {
  // dnsEnabled 仅控制列表显示，不修改各条目的 enabled 状态
}

// 同步 dnsEnabled 状态（有任何启用的 DNS 条目就开启）
const syncDnsEnabled = () => {
  dnsEnabled.value = settings.value.dns_configs.some((d) => d.enabled)
}

watch(
  () => settings.value.dns_configs.map((d) => `${d.id}:${d.enabled}`).join(','),
  () => {
    syncDnsEnabled()
  },
)

// 添加自定义 DNS
const addCustomDNS = () => {
  const id = 'custom_' + Date.now()
  settings.value.dns_configs.push({
    id,
    name: t('settings.network.customDNSName'),
    enabled: true,
    builtin: false,
    servers: [],
  })
}

// 移除 DNS 配置
const removeDNS = (id: string) => {
  const idx = settings.value.dns_configs.findIndex((d) => d.id === id)
  if (idx > -1) {
    settings.value.dns_configs.splice(idx, 1)
  }
}

// 日志查看器方法
const loadLogFiles = async () => {
  try {
    const files = await LoggingService.GetLogFiles()
    logFiles.value = files || []
    if (files && files.length > 0 && !files.includes(selectedLogFile.value)) {
      selectedLogFile.value = files[files.length - 1]
    }
  } catch (e) {
    console.error(t('settings.log.loadListFailed'), e)
  }
}

const loadLogContent = async () => {
  if (!selectedLogFile.value) return
  try {
    const content = await LoggingService.ReadLog(selectedLogFile.value, logTail.value)
    logContent.value = content || ''
  } catch (e) {
    console.error(t('settings.log.loadFailed'), e)
    logContent.value = t('settings.log.loadFailed') + ' ' + (e as Error).message
  }
}

const refreshLogs = async () => {
  await loadLogFiles()
  await loadLogContent()
}

const openLogDir = async () => {
  const dir = await LoggingService.GetLogDir()
  if (dir) await FileService.OpenDirectory(dir)
}

const openLogFullscreen = async () => {
  try {
    await WindowService.OpenWindow({
      WindowName: 'log-viewer',
      Title: t('settings.log.viewer'),
      Content: '',
      Width: 1000,
      Height: 700,
      BgColor: '',
      TextColor: '',
      FontFamily: '',
      FontSize: 13,
    })
  } catch (e) {
    console.error(t('settings.log.openWindowFailed'), e)
  }
}

// 复制日志内容到剪贴板
const copyLog = async () => {
  if (!logContent.value) {
    showMessage(t('settings.log.noContent'), 'warning')
    return
  }
  const ok = await copyToClipboard(logContent.value)
  showMessage(
    ok ? t('settings.log.copied') : t('settings.log.copyFailed'),
    ok ? 'success' : 'error',
  )
}

const logLevelOptions = [
  { label: t('settings.log.level_debug'), value: 'DEBUG' },
  { label: t('settings.log.level_info'), value: 'INFO' },
  { label: t('settings.log.level_warn'), value: 'WARN' },
  { label: t('settings.log.level_error'), value: 'ERROR' },
]

const languageOptions = [
  { label: t('lang.auto'), value: LOCALE_AUTO },
  { label: t('lang.zh'), value: LOCALE_ZH_CN },
  { label: t('lang.en'), value: LOCALE_EN_US },
]

const themeOptions = [
  { label: t('theme.auto'), value: 'auto' },
  { label: t('theme.dark'), value: 'dark' },
  { label: t('theme.light'), value: 'light' },
]

const logTailOptions = [
  { label: t('settings.log.last100'), value: 100 },
  { label: t('settings.log.last500'), value: 500 },
  { label: t('settings.log.all'), value: 0 },
]

onMounted(async () => {
  loadSettings()
  loadLogFiles()
  loadLogContent()
  // 获取开机自启状态
  try {
    autostartEnabled.value = await AutostartService.IsEnabled()
  } catch (e) {
    console.error(t('settings.autostart.getStatusFailed'), e)
  }
  // 获取通知权限状态
  try {
    notificationEnabled.value = await NotificationService.CheckPermission()
  } catch (e) {
    console.error(t('settings.notification.getPermissionFailed'), e)
  }
  // 获取版本号
  try {
    appVersion.value = await SystemService.GetVersion()
  } catch (e) {
    /* ignore */
  }
})
</script>

<template>
  <div class="page">
    <div class="flex items-start justify-between gap-4">
      <div>
        <h1 class="text-2xl font-bold">{{ t('settings.title') }}</h1>
        <p class="text-sm mt-1 opacity-50">{{ t('settings.subtitle') }}</p>
      </div>
      <span :title="t('settings.restartNotice.desc')" class="shrink-0">
        <n-tag type="warning" :bordered="false" size="small" class="mt-1 cursor-help">
          {{ t('settings.restartNotice.title') }}
        </n-tag>
      </span>
    </div>

    <n-spin :show="loading">
      <!-- 续期设置 -->
      <n-card :title="t('settings.renewal.title')" size="small">
        <n-form label-placement="top">
          <n-form-item :label="t('settings.renewal.interval')">
            <div class="flex items-center gap-3">
              <n-input-number
                v-model:value="settings.renew_interval"
                :min="1"
                :max="24"
                class="input-width"
              />
              <span class="text-sm opacity-60">{{ t('settings.renewal.interval.desc') }}</span>
            </div>
          </n-form-item>
          <n-form-item :label="t('settings.renewal.auto_check')">
            <div class="flex items-center gap-3">
              <n-switch v-model:value="settings.auto_check_expiry" />
              <span class="text-sm opacity-60">{{ t('settings.renewal.auto_check.desc') }}</span>
            </div>
          </n-form-item>
          <n-form-item :label="t('settings.renewal.expiry_interval')">
            <div class="flex items-center gap-3">
              <n-input-number
                v-model:value="settings.check_interval"
                :min="1"
                :max="24"
                class="input-width"
              />
              <span class="text-sm opacity-60">{{
                t('settings.renewal.expiry_interval.desc')
              }}</span>
            </div>
          </n-form-item>
        </n-form>
      </n-card>

      <!-- 启动设置 -->
      <n-card :title="t('settings.autostart.title')" size="small" class="mt-4">
        <n-form label-placement="top">
          <n-form-item :label="t('settings.autostart.enabled')">
            <div class="flex items-center gap-3">
              <n-switch v-model:value="autostartEnabled" />
              <span class="text-sm opacity-60">{{ t('settings.autostart.enabled.desc') }}</span>
            </div>
          </n-form-item>
        </n-form>
      </n-card>

      <!-- 通知设置 -->
      <n-card :title="t('settings.notification.title')" size="small" class="mt-4">
        <n-form label-placement="top">
          <n-form-item :label="t('settings.notification.enabled')">
            <div class="flex items-center gap-3">
              <n-switch v-model:value="notificationEnabled" />
              <span class="text-sm opacity-60">{{ t('settings.notification.enabled.desc') }}</span>
            </div>
          </n-form-item>
          <n-form-item label="">
            <n-button @click="handleTestNotification">
              {{ t('settings.notification.test') }}
            </n-button>
          </n-form-item>
        </n-form>
      </n-card>

      <!-- 监控设置 -->
      <n-card :title="t('settings.monitor.title')" size="small" class="mt-4">
        <n-form label-placement="top">
          <n-form-item :label="t('settings.monitor.historyDays')">
            <div class="flex items-center gap-3">
              <n-input-number
                v-model:value="settings.monitor_history_days"
                :min="1"
                :max="3650"
                class="input-width"
              />
              <span class="text-sm opacity-60">{{ t('settings.monitor.historyDays.desc') }}</span>
            </div>
          </n-form-item>
        </n-form>
      </n-card>

      <!-- 维护操作 -->
      <n-card :title="t('settings.maintenance.title')" size="small" class="mt-4">
        <n-form label-placement="top">
          <n-form-item :label="t('settings.maintenance.renewal')">
            <div class="flex items-center gap-3">
              <n-button secondary @click="handleRunRenewal">{{
                t('settings.maintenance.run')
              }}</n-button>
              <span class="text-sm opacity-60">{{ t('settings.maintenance.renewal.desc') }}</span>
            </div>
          </n-form-item>
          <n-form-item :label="t('settings.maintenance.expiry')">
            <div class="flex items-center gap-3">
              <n-button secondary @click="handleRunExpiryCheck">{{
                t('settings.maintenance.run')
              }}</n-button>
              <span class="text-sm opacity-60">{{ t('settings.maintenance.expiry.desc') }}</span>
            </div>
          </n-form-item>
          <n-form-item :label="t('settings.maintenance.monitor')">
            <div class="flex items-center gap-3">
              <n-button secondary @click="handleRunMonitorCheck">{{
                t('settings.maintenance.run')
              }}</n-button>
              <span class="text-sm opacity-60">{{ t('settings.maintenance.monitor.desc') }}</span>
            </div>
          </n-form-item>
        </n-form>
      </n-card>

      <!-- 数据管理 -->
      <n-card :title="t('settings.data.title')" size="small" class="mt-4">
        <n-form label-placement="top">
          <n-form-item :label="t('settings.data.export')">
            <div class="flex items-center gap-3">
              <n-button
                secondary
                type="primary"
                :loading="exporting"
                :disabled="importing"
                @click="handleExportData"
              >
                {{ t('settings.data.export') }}
              </n-button>
              <span class="text-sm opacity-60">{{ t('settings.data.export.desc') }}</span>
            </div>
          </n-form-item>
          <n-form-item :label="t('settings.data.import')">
            <div class="flex w-full flex-col gap-3">
              <div class="flex items-center gap-3">
                <n-button
                  secondary
                  type="warning"
                  :loading="importing"
                  :disabled="exporting || importStatus.running"
                  @click="handleImportData"
                >
                  {{ t('settings.data.import') }}
                </n-button>
                <span class="text-sm opacity-60">{{ t('settings.data.import.desc') }}</span>
              </div>
              <div
                v-if="importStatus.running || (importStatus.finished_at && importStatus.stage)"
                class="w-full"
              >
                <n-progress
                  type="line"
                  :percentage="
                    importStatus.total > 0
                      ? Math.round((importStatus.current / importStatus.total) * 100)
                      : importStatus.running
                        ? 5
                        : 100
                  "
                  :status="
                    importStatus.error ? 'error' : importStatus.running ? undefined : 'success'
                  "
                  :processing="importStatus.running"
                  :height="12"
                />
                <div class="mt-1 text-xs opacity-60">
                  {{ importStatus.stage || '' }}
                  <span v-if="importStatus.total > 0"
                    >（{{ importStatus.current }}/{{ importStatus.total }}）</span
                  >
                </div>
              </div>
            </div>
          </n-form-item>
        </n-form>
      </n-card>

      <!-- 网络设置 -->
      <n-card :title="t('settings.network.title')" size="small" class="mt-4">
        <n-form label-placement="top">
          <!-- DNS 解析服务器 -->
          <div>
            <div class="flex items-center justify-between mb-3">
              <div>
                <p class="font-medium text-sm">{{ t('settings.network.customDNS') }}</p>
                <p class="text-xs mt-0.5 opacity-50">{{ t('settings.network.customDNSDesc') }}</p>
              </div>
              <n-switch v-model:value="dnsEnabled" @update:value="toggleDNS()" />
            </div>
            <div v-if="dnsEnabled" class="pl-4 border-l-2 border-blue-500">
              <div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-3">
                <div
                  v-for="dns in settings.dns_configs"
                  :key="dns.id"
                  class="dns-item"
                  :style="dnsItemStyle(dns.enabled)"
                >
                  <n-switch v-model:value="dns.enabled" size="small" class="mt-0.5" />
                  <div class="flex-1 min-w-0">
                    <div class="flex items-center gap-2">
                      <span class="text-sm font-medium truncate">{{ dns.name }}</span>
                      <n-tag v-if="dns.builtin" size="tiny" :bordered="false">内置</n-tag>
                    </div>
                    <n-input
                      v-if="!dns.builtin"
                      :value="(dns.servers || []).join(', ')"
                      @update:value="
                        (v: string) => {
                          dns.servers = v
                            .split(',')
                            .map((s) => s.trim())
                            .filter(Boolean)
                        }
                      "
                      size="small"
                      :style="dnsInputStyle"
                      :placeholder="t('settings.network.dnsPlaceholder')"
                    />
                    <p v-else class="text-xs font-mono mt-1 break-all opacity-50">
                      {{ (dns.servers || []).join(', ') }}
                    </p>
                  </div>
                  <n-button
                    v-if="!dns.builtin"
                    text
                    size="tiny"
                    type="error"
                    @click="removeDNS(dns.id)"
                    :title="t('settings.network.delete')"
                  >
                    <template #icon>
                      <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path
                          stroke-linecap="round"
                          stroke-linejoin="round"
                          stroke-width="2"
                          d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16"
                        />
                      </svg>
                    </template>
                  </n-button>
                </div>
              </div>
              <n-button text size="small" type="primary" @click="addCustomDNS" class="mt-3">
                {{ t('settings.network.addCustomDNS') }}
              </n-button>
            </div>
          </div>

          <!-- 代理 -->
          <div class="pt-3 border-t border-neutral-200 dark:border-neutral-700 mt-4">
            <div class="flex items-center justify-between mb-3">
              <div>
                <p class="font-medium text-sm">{{ t('settings.network.httpProxy') }}</p>
                <p class="text-xs mt-0.5 opacity-50">{{ t('settings.network.httpProxyDesc') }}</p>
              </div>
              <n-switch v-model:value="settings.proxy.enabled" />
            </div>
            <div
              v-if="settings.proxy.enabled"
              class="grid grid-cols-2 gap-3 pl-4 border-l-2 border-blue-500"
            >
              <div class="col-span-2 flex items-center gap-2">
                <n-select
                  v-model:value="settings.proxy.protocol"
                  :options="[
                    { label: 'HTTP', value: 'http' },
                    { label: 'HTTPS', value: 'https' },
                    { label: 'SOCKS5', value: 'socks5' },
                  ]"
                  class="proxy-protocol"
                  size="small"
                />
                <n-input
                  v-model:value="settings.proxy.host"
                  :placeholder="t('settings.network.hostPlaceholder')"
                  size="small"
                  class="flex-1"
                />
                <n-input-number
                  v-model:value="settings.proxy.port"
                  :min="1"
                  :max="65535"
                  size="small"
                  class="proxy-port"
                />
              </div>
              <n-input
                v-model:value="settings.proxy.username"
                :placeholder="t('settings.network.usernamePlaceholder')"
                size="small"
              />
              <n-input
                v-model:value="settings.proxy.password"
                type="password"
                :placeholder="t('settings.network.passwordPlaceholder')"
                size="small"
                show-password-on="click"
              />
            </div>
          </div>

          <!-- HTTP 请求日志保留 -->
          <div class="pt-3 border-t border-neutral-200 dark:border-neutral-700 mt-4">
            <div class="flex items-center justify-between mb-3">
              <div>
                <p class="font-medium text-sm">{{ t('settings.network.httpLogRetention') }}</p>
                <p class="text-xs mt-0.5 opacity-50">
                  {{ t('settings.network.httpLogRetentionDesc') }}
                </p>
              </div>
              <n-input-number
                v-model:value="settings.http_log_retention_days"
                :min="1"
                :max="3650"
                size="small"
                style="width: 110px"
              />
            </div>
          </div>
        </n-form>
      </n-card>

      <!-- 偏好设置 -->
      <n-card :title="t('settings.preferences.title')" size="small" class="mt-4">
        <n-form label-placement="top">
          <n-form-item :label="t('settings.preferences.language')">
            <div class="flex items-center gap-3">
              <n-select
                v-model:value="languageModel"
                :options="languageOptions"
                class="select-width"
              />
              <span class="text-sm opacity-60">{{ t('settings.preferences.language.desc') }}</span>
            </div>
          </n-form-item>
          <n-form-item :label="t('settings.preferences.theme')">
            <div class="flex items-center gap-3">
              <n-select v-model:value="themeModel" :options="themeOptions" class="select-width" />
              <span class="text-sm opacity-60">{{ t('settings.preferences.theme.desc') }}</span>
            </div>
          </n-form-item>
        </n-form>
      </n-card>

      <!-- 关于 -->
      <n-card :title="t('settings.about.title')" size="small" class="mt-4">
        <div class="space-y-3">
          <div class="flex justify-between py-2">
            <span class="opacity-50">{{ t('settings.about.name') }}</span
            ><span>CertFlow</span>
          </div>
          <div class="flex justify-between py-2">
            <span class="opacity-50">{{ t('settings.about.version') }}</span
            ><span>{{ appVersion || 'unknown' }}</span>
          </div>
          <div class="flex items-center justify-between py-2">
            <div>
              <span class="opacity-50">{{ t('settings.about.prerelease') }}</span>
              <p class="text-xs opacity-40 mt-0.5">{{ t('settings.about.prerelease.desc') }}</p>
            </div>
            <n-switch v-model:value="settings.prerelease" />
          </div>
          <div class="pt-2 border-t border-neutral-200 dark:border-neutral-700">
            <n-button secondary block @click="handleCheckUpdate">
              {{ t('settings.about.checkUpdate') }}
            </n-button>
          </div>
        </div>
      </n-card>

      <!-- 日志设置 -->
      <n-card :title="t('settings.log.title')" size="small" class="mt-4">
        <n-form label-placement="top">
          <n-form-item :label="t('settings.log.level')">
            <div class="flex items-center gap-3">
              <n-select
                v-model:value="settings.log.level"
                :options="logLevelOptions"
                class="select-width"
              />
              <span class="text-sm opacity-60">{{ t('settings.log.levelDesc') }}</span>
            </div>
          </n-form-item>
          <n-form-item :label="t('settings.log.maxSize')">
            <div class="flex items-center gap-2">
              <n-input-number
                v-model:value="settings.log.max_mb"
                :min="1"
                :max="100"
                class="input-width-sm"
                size="small"
              />
              <span class="text-sm opacity-50">MB</span>
            </div>
          </n-form-item>
          <n-form-item :label="t('settings.log.maxBackups')">
            <div class="flex items-center gap-2">
              <n-input-number
                v-model:value="settings.log.max_backups"
                :min="1"
                :max="20"
                class="input-width-sm"
                size="small"
              />
              <span class="text-sm opacity-50">{{ t('common.unit') }}</span>
            </div>
          </n-form-item>
        </n-form>

        <!-- 日志查看器 -->
        <div class="mt-6 pt-4 border-t border-neutral-200 dark:border-neutral-700">
          <div class="flex items-center justify-between mb-3">
            <p class="font-medium text-sm">{{ t('settings.log.viewer') }}</p>
            <div class="flex items-center gap-2">
              <n-select
                v-model:value="selectedLogFile"
                :options="
                  logFiles.map((f) => ({
                    label: f.endsWith('.gz') ? f + ' (' + t('settings.log.archive') + ')' : f,
                    value: f,
                  }))
                "
                size="small"
                class="log-file-select"
                @update:value="loadLogContent()"
              />
              <n-select
                v-model:value="logTail"
                :options="logTailOptions"
                size="small"
                class="log-tail-select"
                @update:value="loadLogContent()"
              />
              <n-button quaternary circle size="small" @click="refreshLogs()">
                <template #icon>
                  <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path
                      stroke-linecap="round"
                      stroke-linejoin="round"
                      stroke-width="2"
                      d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15"
                    />
                  </svg>
                </template>
              </n-button>
              <n-button
                quaternary
                circle
                size="small"
                @click="openLogDir()"
                :title="t('settings.log.openDir')"
              >
                <template #icon>
                  <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path
                      stroke-linecap="round"
                      stroke-linejoin="round"
                      stroke-width="2"
                      d="M3 7v10a2 2 0 002 2h14a2 2 0 002-2V9a2 2 0 00-2-2h-6l-2-2H5a2 2 0 00-2 2z"
                    />
                  </svg>
                </template>
              </n-button>
              <n-button
                quaternary
                circle
                size="small"
                @click="openLogFullscreen()"
                :title="t('settings.log.fullscreen')"
              >
                <template #icon>
                  <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path
                      stroke-linecap="round"
                      stroke-linejoin="round"
                      stroke-width="2"
                      d="M4 8V4m0 0h4M4 4l5 5m11-1V4m0 0h-4m4 0l-5 5M4 16v4m0 0h4m-4 0l5-5m11 5l-5-5m5 5v-4m0 4h-4"
                    />
                  </svg>
                </template>
              </n-button>
              <n-button
                quaternary
                circle
                size="small"
                @click="copyLog()"
                :title="t('settings.log.copy')"
              >
                <template #icon>
                  <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path
                      stroke-linecap="round"
                      stroke-linejoin="round"
                      stroke-width="2"
                      d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z"
                    />
                  </svg>
                </template>
              </n-button>
            </div>
          </div>
          <div class="log-content" :style="logContentStyle">
            <span v-if="logContent">{{ logContent }}</span>
            <span v-else class="opacity-50">{{ t('settings.log.noContent') }}</span>
          </div>
        </div>
      </n-card>
    </n-spin>
  </div>
</template>

<style scoped>
.input-width {
  width: 100px;
}

.input-width-sm {
  width: 80px;
}

.select-width {
  width: 128px;
}

.dns-item {
  display: flex;
  align-items: flex-start;
  gap: 0.75rem;
  padding: 0.75rem;
  border-radius: 0.75rem;
  border: 1px solid;
  transition: all 0.15s;
}

.dns-input {
  margin-top: 4px;
}

.proxy-protocol {
  width: 96px;
}

.proxy-port {
  width: 80px;
}

.log-file-select {
  width: 192px;
}

.log-tail-select {
  width: 112px;
}

.log-content {
  border-radius: 0.5rem;
  padding: 0.75rem;
  max-height: 16rem;
  overflow: auto;
  font-family: monospace;
  font-size: 0.75rem;
  line-height: 1.625;
  white-space: pre-wrap;
}
</style>
