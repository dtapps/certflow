<script setup lang="ts">
// @ts-nocheck
import { ref, watch, onMounted, nextTick } from 'vue'
import * as AuthService from '@bindings/cnb.cool/dtapp/certflow/authservicewrapper'
import * as SettingsService from '@bindings/cnb.cool/dtapp/certflow/settingsservicewrapper'
import { useTheme } from '../stores/theme'
import { useI18n } from '../stores/i18n'

const emit = defineEmits<{
  verified: []
}>()

const password = ref('')
const error = ref('')
const showPassword = ref(false)
const loading = ref(false)
const inputRef = ref<HTMLInputElement | null>(null)

const { theme, setTheme } = useTheme()
const { locale, setLocale, t } = useI18n()

// 自动聚焦输入框
onMounted(() => {
  nextTick(() => {
    inputRef.value?.focus()
  })
})

const verify = async () => {
  error.value = ''
  if (!password.value) {
    error.value = t('login.enterPassword')
    return
  }

  loading.value = true
  try {
    if (await AuthService.VerifyPassword(password.value)) {
      emit('verified')
    } else {
      error.value = t('login.wrongPassword')
      password.value = ''
      // 输入框震动效果
      inputRef.value?.style.setProperty('animation', 'var(--animate-shake)')
      setTimeout(() => {
        inputRef.value?.style.removeProperty('animation')
      }, 500)
    }
  } catch (e) {
    error.value = t('login.enterPassword')
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

watch(locale, async (val) => {
  try {
    const settings = await SettingsService.GetSettings()
    if (settings.language !== val) {
      settings.language = val
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
  const locales: ('zh-CN' | 'en-US' | 'auto')[] = ['zh-CN', 'en-US', 'auto']
  const idx = locales.indexOf(locale.value)
  setLocale(locales[(idx + 1) % locales.length])
}
</script>

<template>
  <div class="fixed inset-0 z-[100] flex items-center justify-center p-4">
    <!-- 背景遮罩 -->
    <div class="absolute inset-0 overlay backdrop-blur-md"></div>

    <!-- 登录卡片 -->
    <div class="relative glass-panel rounded-2xl p-8 w-full max-w-sm" style="animation: var(--animate-fade-in-scale)">
      <!-- Logo -->
      <div class="text-center mb-6">
        <div class="w-16 h-16 rounded-2xl bg-primary-soft flex items-center justify-center mx-auto mb-4">
          <svg class="w-8 h-8 text-primary" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 15v2m-6 4h12a2 2 0 002-2v-6a2 2 0 00-2-2H6a2 2 0 00-2 2v6a2 2 0 002 2zm10-10V7a4 4 0 00-8 0v4h8z" />
          </svg>
        </div>
        <h2 class="text-xl font-bold text-base-content">CertFlow</h2>
        <p class="text-content-50 text-sm mt-1">{{ t('login.accessPassword') }}</p>
      </div>

      <!-- 密码表单 -->
      <form @submit.prevent="verify" class="space-y-4">
        <div>
          <div class="relative">
            <input
              ref="inputRef"
              v-model="password"
              :type="showPassword ? 'text' : 'password'"
              :placeholder="t('login.enterPassword')"
              class="input w-full pr-10"
              :class="{ 'input-error': error }"
              :disabled="loading"
              autocomplete="current-password"
            />
            <button
              type="button"
              @click="showPassword = !showPassword"
              class="absolute right-3 top-1/2 -translate-y-1/2 text-content-50 hover:text-content-80"
              tabindex="-1"
            >
              <svg v-if="showPassword" class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13.875 18.825A10.05 10.05 0 0112 19c-4.478 0-8.268-2.943-9.543-7a9.97 9.97 0 011.563-3.029m5.858.908a3 3 0 114.243 4.243M9.878 9.878l4.242 4.242M9.88 9.88l-3.29-3.29m7.532 7.532l3.29 3.29M3 3l3.59 3.59m0 0A9.953 9.953 0 0112 5c4.478 0 8.268 2.943 9.543 7a10.025 10.025 0 01-4.132 5.411m0 0L21 21" />
              </svg>
              <svg v-else class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z" />
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M2.458 12C3.732 7.943 7.523 5 12 5c4.478 0 8.268 2.943 9.542 7-1.274 4.057-5.064 7-9.542 7-4.477 0-8.268-2.943-9.542-7z" />
              </svg>
            </button>
          </div>
          <p v-if="error" class="text-error text-xs mt-2">{{ error }}</p>
        </div>

        <button
          type="submit"
          class="btn btn-primary w-full text-sm"
          :class="{ 'btn-disabled': loading }"
          :disabled="loading"
        >
          <span v-if="loading" class="loading loading-spinner loading-sm"></span>
          <span v-else>{{ t('login.verify') }}</span>
        </button>
      </form>

      <!-- 主题 & 语言切换 -->
      <div class="flex items-center justify-center gap-2 mt-6 pt-4 border-t border-base-300">
        <button
          @click="toggleTheme"
          class="btn btn-ghost btn-sm gap-1 text-sm"
          :title="t('settings.preferences.theme')"
        >
          <svg v-if="theme === 'dark'" class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M20.354 15.354A9 9 0 018.646 3.646 9.003 9.003 0 0012 21a9.003 9.003 0 008.354-5.646z" />
          </svg>
          <svg v-else-if="theme === 'light'" class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 3v1m0 16v1m9-9h-1M4 12H3m15.364 6.364l-.707-.707M6.343 6.343l-.707-.707m12.728 0l-.707.707M6.343 17.657l-.707.707M16 12a4 4 0 11-8 0 4 4 0 018 0z" />
          </svg>
          <svg v-else class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9.75 17L9 20l-1 1h8l-1-1-.75-3M3 13h18M5 17h14a2 2 0 002-2V5a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z" />
          </svg>
          <span>{{ theme === 'dark' ? t('theme.dark') : theme === 'light' ? t('theme.light') : t('theme.auto') }}</span>
        </button>

        <div class="w-px h-4 bg-base-300"></div>

        <button
          @click="toggleLocale"
          class="btn btn-ghost btn-sm gap-1 text-sm"
          :title="t('settings.preferences.language')"
        >
          <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M3 5h12M9 3v2m1.048 9.5A18.022 18.022 0 016.412 9m6.088 9h7M11 21l5-10 5 10M12.751 5C11.783 10.77 8.07 15.61 3 18.129" />
          </svg>
          <span>{{ locale === 'auto' ? t('lang.auto') : (locale === 'zh-CN' ? t('lang.zh') : t('lang.en')) }}</span>
        </button>
      </div>
    </div>
  </div>
</template>
