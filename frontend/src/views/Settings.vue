<script setup lang="ts">
import { ref, onMounted, watch, computed } from 'vue'
import * as SettingsService from '@bindings/cnb.cool/dtapp/certflow/settingsservicewrapper'
import * as NotificationService from '@bindings/cnb.cool/dtapp/certflow/notificationservicewrapper'
import * as SchedulerService from '@bindings/cnb.cool/dtapp/certflow/schedulerservicewrapper'
import * as MonitorService from '@bindings/cnb.cool/dtapp/certflow/monitorservicewrapper'
import * as LoggingService from '@bindings/cnb.cool/dtapp/certflow/loggingservicewrapper'
import * as BrowserService from '@bindings/cnb.cool/dtapp/certflow/browserservicewrapper'
import * as FileService from '@bindings/cnb.cool/dtapp/certflow/fileservicewrapper'
import * as WindowService from '@bindings/cnb.cool/dtapp/certflow/windowservicewrapper'
import * as AutostartService from '@bindings/cnb.cool/dtapp/certflow/autostartservicewrapper'
import type { Settings, DNSConfig } from '@bindings/cnb.cool/dtapp/certflow/internal/settings/models'
import { useTheme } from '../stores/theme'
import { useI18n } from '../stores/i18n'

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
  notification_enabled: true,
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

const loading = ref(false)
const saving = ref(false)

// Log viewer state
const logFiles = ref<string[]>([])
const selectedLogFile = ref('certflow.log')
const logTail = ref(100)
const logContent = ref('')

const { theme: currentTheme, setTheme } = useTheme()
const { locale: currentLocale, t, setLocale } = useI18n()

// 检测是否有变更
const hasChanges = computed(() => {
  return JSON.stringify(settings.value) !== originalSettings.value
})

// Sync store -> settings ref when store changes (e.g. from other pages)
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
      dns_configs: (loaded.dns_configs || []).map(d => ({
        ...d,
        servers: d.servers || [],
      })),
      proxy: loaded.proxy || { enabled: false, protocol: 'http', host: '', port: 8080 },
      log: loaded.log || { level: 'INFO', max_mb: 10, max_backups: 5 },
    }
    settings.value = safe
    // 初始化 DNS 总开关状态
    dnsEnabled.value = safe.dns_configs.some(d => d.id !== 'default' && d.enabled)
    // Sync settings -> stores on load
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

// Immediate theme switch
watch(() => settings.value.theme, (val) => {
  if (val) setTheme(val as 'dark' | 'light' | 'auto')
})

// Immediate language switch
watch(() => settings.value.language, (val) => {
  if (val) setLocale(val as 'zh-CN' | 'en-US' | 'auto')
})

// 用户开启通知时请求系统权限
watch(() => settings.value.notification_enabled, async (enabled) => {
  if (enabled) {
    const authorized = await NotificationService.RequestPermission()
    if (!authorized) {
      settings.value.notification_enabled = false
      alert(t('settings.notification.permissionDenied'))
    }
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

const handleSave = async () => {
  saving.value = true
  try {
    await SettingsService.SaveSettings(settings.value as Settings)
    originalSettings.value = JSON.stringify(settings.value)
    alert(t('settings.saveSuccess'))
  } catch (e) {
    alert(t('settings.saveFailed') + ' ' + (e as Error).message)
  } finally {
    saving.value = false
  }
}

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
    const enabled = (domains ?? []).filter(d => d !== null && d.enabled)
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
  const idx = settings.value.dns_configs.findIndex(d => d.id === id)
  if (idx > -1) {
    settings.value.dns_configs.splice(idx, 1)
  }
}

// Log viewer functions
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
  if (!selectedLogFile.value) return
  try {
    const content = await LoggingService.ReadLog(selectedLogFile.value, 0)
    await WindowService.OpenHTMLWindow({
      WindowName: 'log-viewer',
      Title: selectedLogFile.value,
      Content: content || '',
      Width: 1000,
      Height: 700,
      BgColor: '#1a1b1e',
      TextColor: '#d4d4d4',
      FontFamily: 'Menlo, Monaco, Consolas, monospace',
      FontSize: 13,
    })
  } catch (e) {
    console.error('Failed to open log window', e)
  }
}

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
})
</script>

<template>
  <div class="page">
    <div>
      <h1 class="text-2xl font-bold text-base-content">{{ t('settings.title') }}</h1>
      <p class="text-sm mt-1 text-content-50">{{ t('settings.subtitle') }}</p>
    </div>

    <!-- 续期设置 -->
    <div class="glass-panel rounded-2xl p-6">
      <h2 class="section-title">
        <svg class="w-5 h-5 text-primary" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" /></svg>
        {{ t('settings.renewal.title') }}
      </h2>
      <div class="space-y-4">
        <div class="setting-row">
          <div>
            <p class="setting-row-label">{{ t('settings.renewal.auto') }}</p>
            <p class="setting-row-desc">{{ t('settings.renewal.auto.desc') }}</p>
          </div>
          <input type="checkbox" class="toggle toggle-primary" v-model="settings.auto_renewal_enabled" />
        </div>
        <div class="setting-row">
          <div>
            <p class="setting-row-label">{{ t('settings.renewal.days') }}</p>
            <p class="setting-row-desc">{{ t('settings.renewal.days.desc') }}</p>
          </div>
          <div class="w-16"><input v-model.number="settings.default_renewal_days" type="number" min="1" max="90" class="input input-sm" /></div>
        </div>
      </div>
    </div>

    <!-- 启动设置 -->
    <div class="glass-panel rounded-2xl p-6">
      <h2 class="section-title">
        <svg class="w-5 h-5 text-primary" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 10V3L4 14h7v7l9-11h-7z" /></svg>
        {{ t('settings.autostart.title') }}
      </h2>
      <div class="space-y-4">
        <div class="setting-row">
          <div>
            <p class="setting-row-label">{{ t('settings.autostart.enabled') }}</p>
            <p class="setting-row-desc">{{ t('settings.autostart.enabled.desc') }}</p>
          </div>
          <input type="checkbox" class="toggle toggle-primary" v-model="autostartEnabled" />
        </div>
      </div>
    </div>

    <!-- 通知设置 -->
    <div class="glass-panel rounded-2xl p-6">
      <h2 class="section-title">
        <svg class="w-5 h-5 text-primary" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 17h5l-1.405-1.405A2.032 2.032 0 0118 14.158V11a6.002 6.002 0 00-4-5.659V5a2 2 0 10-4 0v.341C7.67 6.165 6 8.388 6 11v3.159c0 .538-.214 1.055-.595 1.436L4 17h5m6 0v1a3 3 0 11-6 0v-1m6 0H9" /></svg>
        {{ t('settings.notification.title') }}
      </h2>
      <div class="space-y-4">
        <div class="setting-row">
          <div>
            <p class="setting-row-label">{{ t('settings.notification.enabled') }}</p>
            <p class="setting-row-desc">{{ t('settings.notification.enabled.desc') }}</p>
          </div>
          <input type="checkbox" class="toggle toggle-primary" v-model="settings.notification_enabled" />
        </div>
        <div class="setting-row">
          <div>
            <p class="setting-row-label">{{ t('settings.notification.check') }}</p>
            <p class="setting-row-desc">{{ t('settings.notification.check.desc') }}</p>
          </div>
          <input type="checkbox" class="toggle toggle-primary" v-model="settings.auto_check_expiry" />
        </div>
        <div class="setting-row">
          <div>
            <p class="setting-row-label">{{ t('settings.notification.interval') }}</p>
            <p class="setting-row-desc">{{ t('settings.notification.interval.desc') }}</p>
          </div>
          <div class="w-16"><input v-model.number="settings.check_interval" type="number" min="1" max="24" class="input input-sm" /></div>
        </div>
        <button @click="handleTestNotification" class="btn btn-secondary mt-2">
          <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 19l9 2-9-18-9 18 9-2zm0 0v-8" /></svg>
          {{ t('settings.notification.test') }}
        </button>
      </div>
    </div>

    <!-- 维护操作 -->
    <div class="glass-panel rounded-2xl p-6">
      <h2 class="section-title">
        <svg class="w-5 h-5 text-primary" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.065 2.572c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.572 1.065c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.065-2.572c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z" /><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z" /></svg>
        {{ t('settings.maintenance.title') }}
      </h2>
      <div class="space-y-4">
        <div class="setting-row">
          <div>
            <p class="setting-row-label">{{ t('settings.maintenance.renewal') }}</p>
            <p class="setting-row-desc">{{ t('settings.maintenance.renewal.desc') }}</p>
          </div>
          <button @click="handleRunRenewal" class="btn btn-secondary">{{ t('settings.maintenance.run') }}</button>
        </div>
        <div class="setting-row">
          <div>
            <p class="setting-row-label">{{ t('settings.maintenance.expiry') }}</p>
            <p class="setting-row-desc">{{ t('settings.maintenance.expiry.desc') }}</p>
          </div>
          <button @click="handleRunExpiryCheck" class="btn btn-secondary">{{ t('settings.maintenance.run') }}</button>
        </div>
        <div class="setting-row">
          <div>
            <p class="setting-row-label">{{ t('settings.maintenance.monitor') }}</p>
            <p class="setting-row-desc">{{ t('settings.maintenance.monitor.desc') }}</p>
          </div>
          <button @click="handleRunMonitorCheck" class="btn btn-secondary">{{ t('settings.maintenance.run') }}</button>
        </div>
      </div>
    </div>

    <!-- 网络设置 -->
    <div class="glass-panel rounded-2xl p-6">
      <h2 class="section-title">
        <svg class="w-5 h-5 text-primary" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M21 12a9 9 0 01-9 9m9-9a9 9 0 00-9-9m9 9H3m9 9a9 9 0 01-9-9m9 9c1.657 0 3-4.03 3-9s-1.343-9-3-9m0 18c-1.657 0-3-4.03-3-9s1.343-9 3-9" /></svg>
        {{ t('settings.network.title') }}
      </h2>
      <div class="space-y-5">
        <!-- DNS 解析服务器 -->
        <div>
          <div class="flex items-center justify-between mb-3">
            <div>
              <p class="font-medium text-base-content text-sm">{{ t('settings.network.customDNS') }}</p>
              <p class="text-content-50 text-xs mt-0.5">{{ t('settings.network.customDNSDesc') }}</p>
            </div>
            <input type="checkbox" class="toggle toggle-primary toggle-sm" v-model="dnsEnabled" @change="toggleDNS()" />
          </div>
          <div v-if="dnsEnabled" class="pl-4 border-l-2 border-primary-soft">
            <div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-3">
              <div
                v-for="dns in settings.dns_configs"
                :key="dns.id"
                class="flex items-start gap-3 p-3 rounded-xl border transition-colors"
                :class="dns.enabled ? 'border-primary-soft bg-primary-faint' : 'border-base-300 bg-base-200-faint'"
              >
                <input type="checkbox" class="toggle toggle-sm toggle-primary mt-0.5" v-model="dns.enabled" />
                <div class="flex-1 min-w-0">
                  <div class="flex items-center gap-2">
                    <span class="text-sm font-medium text-base-content truncate">{{ dns.name }}</span>
                    <span v-if="dns.builtin" class="badge-tag badge-tag-muted text-[10px] shrink-0">内置</span>
                  </div>
                  <input
                    v-if="!dns.builtin"
                    :value="(dns.servers || []).join(', ')"
                    @change="dns.servers = ($event.target as HTMLInputElement).value.split(',').map(s => s.trim()).filter(Boolean)"
                    class="input input-sm w-full font-mono text-xs mt-1"
                    :placeholder="t('settings.network.dnsPlaceholder')"
                  />
                  <p v-else class="text-content-50 text-xs font-mono mt-1 break-all">{{ (dns.servers || []).join(', ') }}</p>
                </div>
                <button v-if="!dns.builtin" @click="removeDNS(dns.id)" class="icon-btn text-content-50 hover:text-error shrink-0" :title="t('settings.network.delete')">
                  <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" /></svg>
                </button>
              </div>
            </div>
            <button @click="addCustomDNS" class="btn btn-ghost btn-xs text-primary mt-3">
              {{ t('settings.network.addCustomDNS') }}
            </button>
          </div>
        </div>

        <!-- 代理 -->
        <div class="pt-3 border-t border-base-300">
          <div class="flex items-center justify-between mb-3">
            <div>
              <p class="font-medium text-base-content text-sm">{{ t('settings.network.httpProxy') }}</p>
              <p class="text-content-50 text-xs mt-0.5">{{ t('settings.network.httpProxyDesc') }}</p>
            </div>
            <input type="checkbox" class="toggle toggle-primary toggle-sm" v-model="settings.proxy.enabled" />
          </div>
          <div v-if="settings.proxy.enabled" class="grid grid-cols-2 gap-3 pl-4 border-l-2 border-primary-soft">
            <div class="col-span-2 flex items-center gap-2">
              <select v-model="settings.proxy.protocol" class="select select-sm select-bordered w-24 text-sm">
                <option value="http">HTTP</option>
                <option value="https">HTTPS</option>
                <option value="socks5">SOCKS5</option>
              </select>
              <input v-model="settings.proxy.host" type="text" :placeholder="t('settings.network.hostPlaceholder')" class="input input-sm input-bordered flex-1 text-sm" />
              <input v-model.number="settings.proxy.port" type="number" min="1" max="65535" class="input input-sm input-bordered w-20 text-sm" :placeholder="t('settings.network.portPlaceholder')" />
            </div>
            <input v-model="settings.proxy.username" type="text" :placeholder="t('settings.network.usernamePlaceholder')" class="input input-sm input-bordered text-sm" />
            <input v-model="settings.proxy.password" type="password" :placeholder="t('settings.network.passwordPlaceholder')" class="input input-sm input-bordered text-sm" />
          </div>
        </div>
      </div>
    </div>

    <!-- 偏好设置 -->
    <div class="glass-panel rounded-2xl p-6">
      <h2 class="section-title">
        <svg class="w-5 h-5 text-primary" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M3 5h12M9 3v2m1.048 9.5A18.022 18.022 0 016.412 9m6.088 9h7M11 21l5-10 5 10M12.751 5C11.783 10.77 8.07 15.61 3 18.129" /></svg>
        {{ t('settings.preferences.title') }}
      </h2>
      <div class="space-y-4">
        <div class="setting-row">
          <div>
            <p class="setting-row-label">{{ t('settings.preferences.language') }}</p>
            <p class="setting-row-desc">{{ t('settings.preferences.language.desc') }}</p>
          </div>
          <select v-model="settings.language" class="select select-bordered w-32 text-sm">
            <option value="auto">{{ t('lang.auto') }}</option>
            <option value="zh-CN">{{ t('lang.zh') }}</option>
            <option value="en-US">{{ t('lang.en') }}</option>
          </select>
        </div>
        <div class="setting-row">
          <div>
            <p class="setting-row-label">{{ t('settings.preferences.theme') }}</p>
            <p class="setting-row-desc">{{ t('settings.preferences.theme.desc') }}</p>
          </div>
          <select v-model="settings.theme" class="select select-bordered w-32 text-sm">
            <option value="auto">{{ t('theme.auto') }}</option>
            <option value="dark">{{ t('theme.dark') }}</option>
            <option value="light">{{ t('theme.light') }}</option>
          </select>
        </div>
      </div>
    </div>

    <!-- 关于 -->
    <div class="glass-panel rounded-2xl p-6">
      <h2 class="section-title">
        <svg class="w-5 h-5 text-primary" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" /></svg>
        {{ t('settings.about.title') }}
      </h2>
      <div class="space-y-3">
        <div class="flex justify-between py-2"><span class="text-content-50">{{ t('settings.about.name') }}</span><span class="text-base-content">CertFlow</span></div>
        <div class="flex justify-between py-2"><span class="text-content-50">{{ t('settings.about.version') }}</span><span class="text-base-content">0.1.0 Alpha</span></div>
        <div class="flex justify-between py-2"><span class="text-content-50">{{ t('settings.about.datadir') }}</span><span class="text-base-content text-sm font-mono">{{ settings.data_dir }}</span></div>
        <div class="pt-2 border-t border-base-300">
          <button @click="handleCheckUpdate" class="btn btn-secondary btn-sm w-full">
            {{ t('settings.about.checkUpdate') }}
          </button>
        </div>
      </div>
    </div>

    <!-- 日志设置 -->
    <div class="glass-panel rounded-2xl p-6">
      <h2 class="section-title">
        <svg class="w-5 h-5 text-primary" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z" /></svg>
        {{ t('settings.log.title') }}
      </h2>
      <div class="space-y-4">
        <div class="setting-row">
          <div>
            <p class="setting-row-label">{{ t('settings.log.level') }}</p>
            <p class="setting-row-desc">{{ t('settings.log.levelDesc') }}</p>
          </div>
          <select v-model="settings.log.level" class="select select-bordered w-32 text-sm">
            <option value="DEBUG">DEBUG</option>
            <option value="INFO">INFO</option>
            <option value="WARN">WARN</option>
            <option value="ERROR">ERROR</option>
          </select>
        </div>
        <div class="setting-row">
          <div>
            <p class="setting-row-label">{{ t('settings.log.maxSize') }}</p>
            <p class="setting-row-desc">{{ t('settings.log.maxSizeDesc') }}</p>
          </div>
          <div class="flex items-center gap-2">
            <input v-model.number="settings.log.max_mb" type="number" min="1" max="100" class="input input-sm w-20" />
            <span class="text-sm text-content-50">MB</span>
          </div>
        </div>
        <div class="setting-row">
          <div>
            <p class="setting-row-label">{{ t('settings.log.maxBackups') }}</p>
            <p class="setting-row-desc">{{ t('settings.log.maxBackupsDesc') }}</p>
          </div>
          <div class="flex items-center gap-2">
            <input v-model.number="settings.log.max_backups" type="number" min="1" max="20" class="input input-sm w-20" />
            <span class="text-sm text-content-50">{{ t('common.unit') }}</span>
          </div>
        </div>
      </div>

      <!-- 日志查看器 -->
      <div class="mt-6 pt-4 border-t border-base-300">
        <div class="flex items-center justify-between mb-3">
          <p class="font-medium text-base-content text-sm">{{ t('settings.log.viewer') }}</p>
          <div class="flex items-center gap-2">
            <select v-model="selectedLogFile" class="select select-sm select-bordered text-xs" @change="loadLogContent()">
              <option v-for="f in logFiles" :key="f" :value="f">{{ f }}</option>
            </select>
            <select v-model.number="logTail" class="select select-sm select-bordered text-xs" @change="loadLogContent()">
              <option :value="100">{{ t('settings.log.last100') }}</option>
              <option :value="500">{{ t('settings.log.last500') }}</option>
              <option :value="0">{{ t('settings.log.all') }}</option>
            </select>
            <button @click="refreshLogs()" class="btn btn-ghost btn-xs">
              <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" /></svg>
            </button>
            <button @click="openLogDir()" class="icon-btn text-content-50 hover:text-primary" :title="t('settings.log.openDir')">
              <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M3 7v10a2 2 0 002 2h14a2 2 0 002-2V9a2 2 0 00-2-2h-6l-2-2H5a2 2 0 00-2 2z" /></svg>
            </button>
            <button @click="openLogFullscreen()" class="icon-btn text-content-50 hover:text-primary" :title="t('settings.log.fullscreen')">
              <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 8V4m0 0h4M4 4l5 5m11-1V4m0 0h-4m4 0l-5 5M4 16v4m0 0h4m-4 0l5-5m11 5l-5-5m5 5v-4m0 4h-4" /></svg>
            </button>
          </div>
        </div>
        <div class="bg-base-300 rounded-lg p-3 max-h-64 overflow-auto font-mono text-xs leading-relaxed" style="white-space: pre-wrap;">
          <span v-if="logContent" class="text-base-content">{{ logContent }}</span>
          <span v-else class="text-content-50">{{ t('settings.log.noContent') }}</span>
        </div>
      </div>
    </div>

    <!-- 保存按钮 -->
    <div class="flex justify-end">
      <button @click="handleSave" class="btn btn-primary" :disabled="saving || !hasChanges">
        {{ saving ? t('settings.saving') : t('settings.save') }}
      </button>
    </div>
  </div>
</template>
