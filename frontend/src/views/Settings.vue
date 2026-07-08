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
import type {
  Settings,
  DNSConfig,
} from '@bindings/cnb.cool/dtapp/certflow/internal/settings/models'
import { useThemeStore } from '../stores/theme'
import { useI18nStore } from '../stores/i18n'

// 内部使用的类型：确保数组不为 null
type SafeDNSConfig = Omit<DNSConfig, 'servers'> & { servers: string[] }
type SafeLogConfig = NonNullable<Settings['log']>
type SafeSettings = Omit<Settings, 'dns_configs' | 'proxy' | 'log'> & {
  dns_configs: SafeDNSConfig[]
  proxy: NonNullable<Settings['proxy']>
  log: SafeLogConfig
}

const defaultSettings: SafeSettings = {
  auto_renewal_enabled: true,
  default_renewal_days: 30,
  auto_check_expiry: true,
  check_interval: 6,
  data_dir: '~/.certflow',
  language: 'auto',
  theme: 'auto',
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
    await SettingsService.SaveSettings(settings.value as Settings)
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
    // 初始化 DNS 总开关状态
    dnsEnabled.value = safe.dns_configs.some((d) => d.id !== 'default' && d.enabled)
    // 同步 settings -> stores
    if (settings.value.theme) {
      setTheme(settings.value.theme as 'dark' | 'light' | 'auto')
    }
    if (settings.value.language) {
      setLocale(settings.value.language as 'zh-CN' | 'en-US' | 'auto')
    }
    originalSettings.value = JSON.stringify(settings.value)
  } catch (e) {
    console.error(t('settings.loadFailed'), e)
  } finally {
    loading.value = false
  }
}

// 实时切换主题
watch(
  () => settings.value.theme,
  (val) => {
    if (val) setTheme(val as 'dark' | 'light' | 'auto')
  },
)

// 实时切换语言
watch(
  () => settings.value.language,
  (val) => {
    if (val) setLocale(val as 'zh-CN' | 'en-US' | 'auto')
  },
)

// 实时设置通知权限
watch(notificationEnabled, async (enabled) => {
  try {
    if (enabled) {
      const authorized = await NotificationService.RequestPermission()
      if (!authorized) {
        notificationEnabled.value = false
        alert(t('settings.notification.permissionDenied'))
      }
    }
  } catch (e) {
    console.error('设置通知权限失败:', e)
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
    console.error('设置开机自启失败:', e)
  }
})

const handleTestNotification = async () => {
  try {
    await NotificationService.SendTestNotification()
    alert(t('settings.testNotificationSent'))
  } catch (e) {
    alert(t('settings.testNotificationFailed'))
  }
}

const handleRunRenewal = () => {
  SchedulerService.RunRenewalNow()
  alert(t('settings.renewalStarted'))
}

const handleRunExpiryCheck = () => {
  SchedulerService.RunExpiryCheckNow()
  alert(t('settings.expiryCheckStarted'))
}

const handleRunMonitorCheck = async () => {
  try {
    const domains = await MonitorService.List()
    const enabled = (domains ?? []).filter((d) => d !== null && d.enabled)
    for (const d of enabled) {
      if (d) await MonitorService.CheckNow(d.id)
    }
    alert(t('settings.monitorCheckStarted'))
  } catch (e) {
    alert(t('settings.monitorCheckFailed'))
  }
}

const handleCheckUpdate = async () => {
  const locale = await SettingsService.GetSettings()
  const lang = locale.language || 'auto'
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
    console.error('打开日志窗口失败:', e)
  }
}

const logLevelOptions = [
  { label: t('settings.log.level_debug'), value: 'DEBUG' },
  { label: t('settings.log.level_info'), value: 'INFO' },
  { label: t('settings.log.level_warn'), value: 'WARN' },
  { label: t('settings.log.level_error'), value: 'ERROR' },
]

const languageOptions = [
  { label: t('lang.auto'), value: 'auto' },
  { label: t('lang.zh'), value: 'zh-CN' },
  { label: t('lang.en'), value: 'en-US' },
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
    console.error('获取开机自启状态失败:', e)
  }
  // 获取通知权限状态
  try {
    notificationEnabled.value = await NotificationService.CheckPermission()
  } catch (e) {
    console.error('获取通知权限状态失败:', e)
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
    <div>
      <h1 class="text-2xl font-bold">{{ t('settings.title') }}</h1>
      <p class="text-sm mt-1 opacity-50">{{ t('settings.subtitle') }}</p>
    </div>

    <n-spin :show="loading">
      <!-- 续期设置 -->
      <n-card :title="t('settings.renewal.title')" size="small">
        <n-form label-placement="top">
          <n-form-item :label="t('settings.renewal.auto')">
            <div class="flex items-center gap-3">
              <n-switch v-model:value="settings.auto_renewal_enabled" />
              <span class="text-sm opacity-60">{{ t('settings.renewal.auto.desc') }}</span>
            </div>
          </n-form-item>
          <n-form-item :label="t('settings.renewal.days')">
            <div class="flex items-center gap-3">
              <n-input-number
                v-model:value="settings.default_renewal_days"
                :min="1"
                :max="90"
                class="input-width"
              />
              <span class="text-sm opacity-60">{{ t('settings.renewal.days.desc') }}</span>
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
          <n-form-item :label="t('settings.notification.check')">
            <div class="flex items-center gap-3">
              <n-switch v-model:value="settings.auto_check_expiry" />
              <span class="text-sm opacity-60">{{ t('settings.notification.check.desc') }}</span>
            </div>
          </n-form-item>
          <n-form-item :label="t('settings.notification.interval')">
            <div class="flex items-center gap-3">
              <n-input-number
                v-model:value="settings.check_interval"
                :min="1"
                :max="24"
                class="input-width"
              />
              <span class="text-sm opacity-60">{{ t('settings.notification.interval.desc') }}</span>
            </div>
          </n-form-item>
          <n-form-item label="">
            <n-button @click="handleTestNotification">
              {{ t('settings.notification.test') }}
            </n-button>
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
        </n-form>
      </n-card>

      <!-- 偏好设置 -->
      <n-card :title="t('settings.preferences.title')" size="small" class="mt-4">
        <n-form label-placement="top">
          <n-form-item :label="t('settings.preferences.language')">
            <div class="flex items-center gap-3">
              <n-select
                v-model:value="settings.language"
                :options="languageOptions"
                class="select-width"
              />
              <span class="text-sm opacity-60">{{ t('settings.preferences.language.desc') }}</span>
            </div>
          </n-form-item>
          <n-form-item :label="t('settings.preferences.theme')">
            <div class="flex items-center gap-3">
              <n-select
                v-model:value="settings.theme"
                :options="themeOptions"
                class="select-width"
              />
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
          <div class="flex justify-between py-2">
            <span class="opacity-50">{{ t('settings.about.datadir') }}</span
            ><span class="text-sm font-mono">{{ settings.data_dir }}</span>
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
                :options="logFiles.map((f) => ({ label: f, value: f }))"
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
