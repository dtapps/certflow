<script setup lang="ts">
// @ts-nocheck
import { ref, watch, onMounted, nextTick, computed } from 'vue'
import { storeToRefs } from 'pinia'
import { NInput, NButton } from 'naive-ui'
import * as AuthService from '@bindings/cnb.cool/dtapp/certflow/authservicewrapper'
import * as SettingsService from '@bindings/cnb.cool/dtapp/certflow/settingsservicewrapper'
import { useThemeStore } from '../stores/theme'
import { useI18nStore } from '../stores/i18n'
import { LOCALE_ZH_CN, LOCALE_EN_US, LOCALE_AUTO, type Locale } from '../locales/locale'

const emit = defineEmits<{
  verified: []
}>()

const password = ref('')
const totpCode = ref('')
const error = ref('')
const showPassword = ref(false)
const loading = ref(false)
const inputRef = ref<InstanceType<typeof NInput> | null>(null)

// 认证方式
const activeMethod = ref('')
const availableMethods = ref<string[]>([])

const themeStore = useThemeStore()
const { theme, isDark } = storeToRefs(themeStore)
const { setTheme } = themeStore

const i18nStore = useI18nStore()
const { locale } = storeToRefs(i18nStore)
const { setLocale, t } = i18nStore

// 动态样式
const overlayStyle = computed(() => ({
  background: isDark.value
    ? 'linear-gradient(135deg, #1a1a2e 0%, #16213e 50%, #0f3460 100%)'
    : 'linear-gradient(135deg, #667eea 0%, #764ba2 100%)',
}))

const cardStyle = computed(() => ({
  background: isDark.value ? 'rgba(30, 30, 45, 0.95)' : 'rgba(255, 255, 255, 0.95)',
  border: `1px solid ${isDark.value ? 'rgba(255, 255, 255, 0.1)' : 'rgba(0, 0, 0, 0.1)'}`,
  boxShadow: isDark.value
    ? '0 25px 50px -12px rgba(0, 0, 0, 0.5), 0 0 0 1px rgba(255, 255, 255, 0.05)'
    : '0 25px 50px -12px rgba(0, 0, 0, 0.25), 0 0 0 1px rgba(0, 0, 0, 0.05)',
}))

const titleStyle = computed(() => ({
  color: isDark.value ? '#ffffff' : '#1a1a2e',
}))

const subtitleStyle = computed(() => ({
  color: isDark.value ? 'rgba(255, 255, 255, 0.5)' : 'rgba(0, 0, 0, 0.5)',
}))

const dividerStyle = computed(() => ({
  background: isDark.value ? 'rgba(255, 255, 255, 0.2)' : 'rgba(0, 0, 0, 0.2)',
}))

// 初始化
onMounted(async () => {
  // 获取当前激活的认证方式
  activeMethod.value = await AuthService.GetActiveMethod()
  availableMethods.value = await AuthService.GetAvailableMethods()

  // 如果没有激活的认证方式，直接进入
  if (!activeMethod.value) {
    emit('verified')
    return
  }

  // 聚焦输入框（Passkey 不需要）
  if (activeMethod.value !== 'passkey') {
    nextTick(() => {
      const inputEl = inputRef.value?.$el?.querySelector('input')
      inputEl?.focus()
    })
  }
})

// 密码登录
const verifyPassword = async () => {
  error.value = ''
  if (!password.value) {
    error.value = t('login.enterPassword')
    return
  }

  loading.value = true
  const currentPassword = password.value
  try {
    if (await AuthService.VerifyPassword(currentPassword)) {
      emit('verified')
    } else {
      error.value = t('login.wrongPassword')
      // 震动动画
      const inputEl = inputRef.value?.$el?.querySelector('input')
      inputEl?.style.setProperty('animation', 'var(--animate-shake)')
      setTimeout(() => {
        inputEl?.style.removeProperty('animation')
      }, 500)
      // 立即清空密码并重新聚焦
      password.value = ''
      nextTick(() => {
        const el = inputRef.value?.$el?.querySelector('input')
        el?.focus()
      })
    }
  } catch (e) {
    error.value = t('login.enterPassword')
  } finally {
    loading.value = false
  }
}

// TOTP 登录
const verifyTOTP = async () => {
  error.value = ''
  if (!totpCode.value || totpCode.value.length !== 6) {
    error.value = t('login.enterTOTPCode')
    return
  }

  loading.value = true
  try {
    if (await AuthService.VerifyTOTP(totpCode.value)) {
      emit('verified')
    } else {
      error.value = t('login.totpFailed')
      totpCode.value = ''
      nextTick(() => {
        const el = inputRef.value?.$el?.querySelector('input')
        el?.focus()
      })
    }
  } catch (e) {
    error.value = t('login.totpFailed')
  } finally {
    loading.value = false
  }
}

// Passkey 登录
// URL-safe base64 编码
const bufferToBase64url = (buffer: ArrayBuffer) => {
  const bytes = new Uint8Array(buffer)
  let binary = ''
  for (let i = 0; i < bytes.length; i++) {
    binary += String.fromCharCode(bytes[i])
  }
  return btoa(binary).replace(/\+/g, '-').replace(/\//g, '_').replace(/=/g, '')
}

// URL-safe base64 解码
const base64urlToBuffer = (base64url: string) => {
  const base64 = base64url.replace(/-/g, '+').replace(/_/g, '/')
  const padded = base64.padEnd(base64.length + ((4 - (base64.length % 4)) % 4), '=')
  const binary = atob(padded)
  const buffer = new ArrayBuffer(binary.length)
  const bytes = new Uint8Array(buffer)
  for (let i = 0; i < binary.length; i++) {
    bytes[i] = binary.charCodeAt(i)
  }
  return buffer
}

const loginWithPasskey = async () => {
  loading.value = true
  error.value = ''

  // 检查 WebAuthn API 是否可用
  if (!window.PublicKeyCredential) {
    error.value = t('login.passkeyNotSupported')
    loading.value = false
    return
  }

  try {
    // 获取后端的认证选项
    const response = await AuthService.StartPasskeyLogin()
    const options = JSON.parse(response.credentialRequestOptions)

    // 调用浏览器 WebAuthn API
    const credential = await navigator.credentials.get({
      publicKey: {
        challenge: base64urlToBuffer(options.challenge),
        timeout: options.timeout || 60000,
        rpId: options.rpId,
        allowCredentials: options.allowCredentials?.map((cred: any) => ({
          id: base64urlToBuffer(cred.id),
          type: cred.type,
          transports: cred.transports,
        })),
        userVerification: options.userVerification,
      },
    })

    if (!credential) {
      error.value = t('login.passkeyFailed')
      loading.value = false
      return
    }

    // 将凭据数据转换为 JSON 传递给后端验证（使用 URL-safe base64）
    const passkeyData = JSON.stringify({
      id: credential.id,
      rawId: bufferToBase64url(credential.rawId),
      type: credential.type,
      response: {
        authenticatorData: bufferToBase64url((credential.response as any).authenticatorData),
        clientDataJSON: bufferToBase64url(credential.response.clientDataJSON),
        signature: bufferToBase64url((credential.response as any).signature),
      },
    })

    if (await AuthService.FinishPasskeyLogin(passkeyData)) {
      emit('verified')
    } else {
      error.value = t('login.passkeyFailed')
    }
  } catch (e: any) {
    console.error(t('login.passkeyFailed'), e)
    // 提供更具体的错误信息
    if (e.name === 'NotAllowedError') {
      error.value = t('login.passkeyNotAllowed')
    } else if (e.name === 'SecurityError') {
      error.value = t('login.passkeySecurityError')
    } else {
      error.value = t('login.passkeyFailed')
    }
  } finally {
    loading.value = false
  }
}

// 同步主题/语言到后端
watch(theme, async (val) => {
  try {
    const settings = await SettingsService.GetSettings()
    if (settings.theme !== val) {
      settings.theme = val
      await SettingsService.SaveSettings(settings)
    }
  } catch (e) {
    console.error(t('login.syncThemeFailed'), e)
  }
})

// 语言切换：后端只需「已解析」语言（不含 auto），与 Settings.vue autoSave / App.vue 启动同步保持一致
watch(locale, async () => {
  try {
    const settings = await SettingsService.GetSettings()
    const resolved = i18nStore.getResolvedLocale()
    if (settings.language !== resolved) {
      settings.language = resolved
      await SettingsService.SaveSettings(settings)
    }
  } catch (e) {
    console.error(t('login.syncLangFailed'), e)
  }
})

function toggleTheme() {
  const themes: ('dark' | 'light' | 'auto')[] = ['dark', 'light', 'auto']
  const idx = themes.indexOf(theme.value)
  setTheme(themes[(idx + 1) % themes.length])
}

function toggleLocale() {
  const locales: Locale[] = [LOCALE_ZH_CN, LOCALE_EN_US, LOCALE_AUTO]
  const idx = locales.indexOf(locale.value)
  setLocale(locales[(idx + 1) % locales.length])
}
</script>

<template>
  <div class="login-overlay" :style="overlayStyle" style="animation: var(--animate-fade-in)">
    <!-- 登录卡片 -->
    <div class="login-card" :style="cardStyle" style="animation: var(--animate-fade-in-scale)">
      <!-- Logo -->
      <div class="text-center mb-6">
        <div
          class="w-16 h-16 rounded-2xl flex items-center justify-center mx-auto mb-4"
          :style="{
            backgroundColor: isDark ? 'rgba(59, 130, 246, 0.2)' : 'rgba(59, 130, 246, 0.1)',
          }"
        >
          <svg class="w-8 h-8 text-blue-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path
              stroke-linecap="round"
              stroke-linejoin="round"
              stroke-width="2"
              d="M12 15v2m-6 4h12a2 2 0 002-2v-6a2 2 0 00-2-2H6a2 2 0 00-2 2v6a2 2 0 002 2zm10-10V7a4 4 0 00-8 0v4h8z"
            />
          </svg>
        </div>
        <h2 class="text-xl font-bold" :style="titleStyle">CertFlow</h2>
        <p class="text-sm mt-1" :style="subtitleStyle">{{ t('login.accessPassword') }}</p>
      </div>

      <!-- 密码登录 -->
      <form v-if="activeMethod === 'password'" @submit.prevent="verifyPassword" class="space-y-4">
        <div>
          <n-input
            ref="inputRef"
            v-model:value="password"
            :type="showPassword ? 'text' : 'password'"
            :placeholder="t('login.enterPassword')"
            :disabled="loading"
            size="large"
            autocomplete="current-password"
          >
            <template #suffix>
              <n-button text size="tiny" @click="showPassword = !showPassword" tabindex="-1">
                <template #icon>
                  <svg
                    v-if="showPassword"
                    class="w-4 h-4"
                    fill="none"
                    stroke="currentColor"
                    viewBox="0 0 24 24"
                  >
                    <path
                      stroke-linecap="round"
                      stroke-linejoin="round"
                      stroke-width="2"
                      d="M13.875 18.825A10.05 10.05 0 0112 19c-4.478 0-8.268-2.943-9.543-7a9.97 9.97 0 011.563-3.029m5.858.908a3 3 0 114.243 4.243M9.878 9.878l4.242 4.242M9.88 9.88l-3.29-3.29m7.532 7.532l3.29 3.29M3 3l3.59 3.59m0 0A9.953 9.953 0 0112 5c4.478 0 8.268 2.943 9.543 7a10.025 10.025 0 01-4.132 5.411m0 0L21 21"
                    />
                  </svg>
                  <svg v-else class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path
                      stroke-linecap="round"
                      stroke-linejoin="round"
                      stroke-width="2"
                      d="M15 12a3 3 0 11-6 0 3 3 0 016 0z"
                    />
                    <path
                      stroke-linecap="round"
                      stroke-linejoin="round"
                      stroke-width="2"
                      d="M2.458 12C3.732 7.943 7.523 5 12 5c4.478 0 8.268 2.943 9.542 7-1.274 4.057-5.064 7-9.542 7-4.477 0-8.268-2.943-9.542-7z"
                    />
                  </svg>
                </template>
              </n-button>
            </template>
          </n-input>
          <p v-if="error" class="text-red-500 text-xs mt-2">{{ error }}</p>
        </div>

        <n-button
          type="primary"
          block
          size="large"
          :loading="loading"
          :disabled="loading"
          attr-type="submit"
        >
          {{ t('login.verify') }}
        </n-button>
      </form>

      <!-- TOTP 登录 -->
      <form v-else-if="activeMethod === 'totp'" @submit.prevent="verifyTOTP" class="space-y-4">
        <div>
          <n-input
            ref="inputRef"
            v-model:value="totpCode"
            type="text"
            :placeholder="t('login.enterTOTPCode')"
            :disabled="loading"
            size="large"
            maxlength="6"
          />
          <p v-if="error" class="text-red-500 text-xs mt-2">{{ error }}</p>
        </div>

        <n-button
          type="primary"
          block
          size="large"
          :loading="loading"
          :disabled="loading"
          attr-type="submit"
        >
          {{ t('login.verify') }}
        </n-button>
      </form>

      <!-- Passkey 登录 -->
      <div v-else-if="activeMethod === 'passkey'" class="space-y-4">
        <p v-if="error" class="text-red-500 text-xs">{{ error }}</p>

        <n-button
          type="primary"
          block
          size="large"
          :loading="loading"
          :disabled="loading"
          @click="loginWithPasskey"
        >
          {{ t('login.usePasskey') }}
        </n-button>
      </div>

      <!-- 主题 & 语言切换 -->
      <div
        class="flex items-center justify-center gap-2 mt-6 pt-4 border-t"
        :style="{ borderColor: isDark ? 'rgba(255, 255, 255, 0.1)' : 'rgba(0, 0, 0, 0.1)' }"
      >
        <n-button
          quaternary
          size="small"
          @click="toggleTheme"
          :title="t('settings.preferences.theme')"
        >
          <template #icon>
            <svg
              v-if="theme === 'dark'"
              class="w-4 h-4"
              fill="none"
              stroke="currentColor"
              viewBox="0 0 24 24"
            >
              <path
                stroke-linecap="round"
                stroke-linejoin="round"
                stroke-width="2"
                d="M20.354 15.354A9 9 0 018.646 3.646 9.003 9.003 0 0012 21a9.003 9.003 0 008.354-5.646z"
              />
            </svg>
            <svg
              v-else-if="theme === 'light'"
              class="w-4 h-4"
              fill="none"
              stroke="currentColor"
              viewBox="0 0 24 24"
            >
              <path
                stroke-linecap="round"
                stroke-linejoin="round"
                stroke-width="2"
                d="M12 3v1m0 16v1m9-9h-1M4 12H3m15.364 6.364l-.707-.707M6.343 6.343l-.707-.707m12.728 0l-.707.707M6.343 17.657l-.707.707M16 12a4 4 0 11-8 0 4 4 0 018 0z"
              />
            </svg>
            <svg v-else class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path
                stroke-linecap="round"
                stroke-linejoin="round"
                stroke-width="2"
                d="M9.75 17L9 20l-1 1h8l-1-1-.75-3M3 13h18M5 17h14a2 2 0 002-2V5a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z"
              />
            </svg>
          </template>
          {{
            theme === 'dark'
              ? t('theme.dark')
              : theme === 'light'
                ? t('theme.light')
                : t('theme.auto')
          }}
        </n-button>

        <div class="w-px h-4" :style="dividerStyle"></div>

        <n-button
          quaternary
          size="small"
          @click="toggleLocale"
          :title="t('settings.preferences.language')"
        >
          <template #icon>
            <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path
                stroke-linecap="round"
                stroke-linejoin="round"
                stroke-width="2"
                d="M3 5h12M9 3v2m1.048 9.5A18.022 18.022 0 016.412 9m6.088 9h7M11 21l5-10 5 10M12.751 5C11.783 10.77 8.07 15.61 3 18.129"
              />
            </svg>
          </template>
          {{
            locale === 'auto' ? t('lang.auto') : locale === 'zh-CN' ? t('lang.zh') : t('lang.en')
          }}
        </n-button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.login-overlay {
  position: fixed;
  inset: 0;
  z-index: 100;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 1rem;
}

.login-card {
  position: relative;
  border-radius: 1rem;
  padding: 2rem;
  width: 100%;
  max-width: 24rem;
  backdrop-filter: blur(24px);
}
</style>
