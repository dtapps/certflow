<script setup lang="ts">
// @ts-nocheck
import { ref, computed, onMounted, onBeforeUnmount, watch } from 'vue'
import { NCard, NButton, NInput, NSwitch, NSpin, NForm, NFormItem, NAlert, NTag } from 'naive-ui'
import * as AuthService from '@bindings/cnb.cool/dtapp/certflow/authservicewrapper'
import { useI18nStore } from '../stores/i18n'
import QRCode from 'qrcode'

const i18nStore = useI18nStore()
const { t } = i18nStore

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

// 通用状态
const message = ref<{ type: 'success' | 'error'; text: string } | null>(null)

// 密码状态
const isPasswordSet = ref(false)
const showPasswordForm = ref(false)
const oldPassword = ref('')
const newPassword = ref('')
const confirmPassword = ref('')
const showPassword = ref(false)
const confirmClearPassword = ref(false)
const confirmClearTOTP = ref(false)

// TOTP 状态
const isTOTPSet = ref(false)
const totpSetupResult = ref<{ secret: string; url: string } | null>(null)
const totpCode = ref('')
const showTOTPForm = ref(false)
const totpQRCode = ref('')
const totpCountdown = ref(0)
let totpTimer: ReturnType<typeof setInterval> | null = null

// Passkey 状态
const isPasskeySet = ref(false)
const passkeyInfo = ref<{ credential_count: number } | null>(null)
const showPasskeyForm = ref(false)

// 认证方式状态
const activeMethod = ref('')
const availableMethods = ref<string[]>([])

// 认证方式名称映射
const methodLabel = (method: string) => {
  if (!method) return '-'
  return t(`personal.method.${method}`)
}

onMounted(async () => {
  await loadAuthInfo()
})

onBeforeUnmount(() => {
  // 切换页面时取消未确认的 TOTP 设置
  if (showTOTPForm.value) {
    cancelTOTPSetup()
  }
})

// 监听 TOTP 设置结果，生成 QR 码
watch(totpSetupResult, async (newVal) => {
  if (newVal?.url) {
    try {
      totpQRCode.value = await QRCode.toDataURL(newVal.url, {
        width: 200,
        margin: 2,
        color: {
          dark: '#000000',
          light: '#ffffff',
        },
      })
    } catch (err) {
      console.error(t('personal.qrCodeGenerateFailed'), err)
    }
  } else {
    totpQRCode.value = ''
  }
})

const loadAuthInfo = async () => {
  try {
    // 加载密码状态
    isPasswordSet.value = await AuthService.IsPasswordSet()

    // 加载 TOTP 状态
    const totpInfo = await AuthService.GetTOTPInfo()
    isTOTPSet.value = totpInfo?.is_configured || false

    // 加载 Passkey 状态
    const passkeyInfoResult = await AuthService.GetPasskeyInfo()
    isPasskeySet.value = passkeyInfoResult?.is_configured || false
    passkeyInfo.value = passkeyInfoResult

    // 加载认证方式
    activeMethod.value = await AuthService.GetActiveMethod()
    availableMethods.value = await AuthService.GetAvailableMethods()
    console.debug(
      t('log.personalLoad', {
        data: JSON.stringify({
          isPasswordSet: isPasswordSet.value,
          isTOTPSet: isTOTPSet.value,
          isPasskeySet: isPasskeySet.value,
          activeMethod: activeMethod.value,
          availableMethods: availableMethods.value,
        }),
      }),
    )
  } catch (e) {
    console.error(t('personal.authLoadFailed'), e)
  }
}

// ==================== 密码操作 ====================

const setPassword = async () => {
  message.value = null
  if (newPassword.value.length < 6) {
    message.value = { type: 'error', text: t('personal.minLength') }
    return
  }
  if (newPassword.value !== confirmPassword.value) {
    message.value = { type: 'error', text: t('personal.mismatch') }
    return
  }
  try {
    await AuthService.SetPassword(newPassword.value)
    isPasswordSet.value = true
    showPasswordForm.value = false
    newPassword.value = ''
    confirmPassword.value = ''
    message.value = { type: 'success', text: t('personal.setSuccess') }
    await loadAuthInfo()
  } catch (e: any) {
    message.value = { type: 'error', text: e.message || t('personal.setFailed') }
  }
}

const changePassword = async () => {
  message.value = null
  if (newPassword.value.length < 6) {
    message.value = { type: 'error', text: t('personal.newMinLength') }
    return
  }
  if (newPassword.value !== confirmPassword.value) {
    message.value = { type: 'error', text: t('personal.mismatch') }
    return
  }
  try {
    await AuthService.ChangePassword(oldPassword.value, newPassword.value)
    showPasswordForm.value = false
    oldPassword.value = ''
    newPassword.value = ''
    confirmPassword.value = ''
    message.value = { type: 'success', text: t('personal.changeSuccess') }
  } catch (e: any) {
    message.value = { type: 'error', text: e.message || t('personal.changeFailed') }
  }
}

const clearPassword = async () => {
  message.value = null
  try {
    await AuthService.ClearPassword()
    isPasswordSet.value = false
    showPasswordForm.value = false
    confirmClearPassword.value = false
    message.value = { type: 'success', text: t('personal.removedSuccess') }
    await loadAuthInfo()
  } catch (e: any) {
    message.value = { type: 'error', text: e.message || t('personal.removeFailed') }
  }
}

// ==================== TOTP 操作 ====================

const setupTOTP = async () => {
  message.value = null
  try {
    const result = await AuthService.SetupTOTP()
    totpSetupResult.value = result
    showTOTPForm.value = true
    // 启动 120 秒倒计时
    totpCountdown.value = 120
    totpTimer = setInterval(() => {
      totpCountdown.value--
      if (totpCountdown.value <= 0) {
        // 超时，取消设置
        cancelTOTPSetup()
      }
    }, 1000)
  } catch (e: any) {
    message.value = { type: 'error', text: e.message || t('personal.totpSetupFailed') }
  }
}

const verifyTOTPSetup = async () => {
  message.value = null
  if (!totpCode.value || totpCode.value.length !== 6) {
    message.value = { type: 'error', text: t('personal.verifyTOTPCode') }
    return
  }
  try {
    await AuthService.VerifyTOTPSetup(totpCode.value)
    clearTOTPTimer()
    isTOTPSet.value = true
    showTOTPForm.value = false
    totpSetupResult.value = null
    totpCode.value = ''
    message.value = { type: 'success', text: t('personal.totpSetupSuccess') }
    await loadAuthInfo()
  } catch (e: any) {
    message.value = { type: 'error', text: e.message || t('personal.totpSetupFailed') }
  }
}

const clearTOTPTimer = () => {
  if (totpTimer) {
    clearInterval(totpTimer)
    totpTimer = null
  }
  totpCountdown.value = 0
}

const cancelTOTPSetup = async () => {
  clearTOTPTimer()
  try {
    await AuthService.CancelTOTP()
  } catch {
    // 忽略错误
  }
  showTOTPForm.value = false
  totpSetupResult.value = null
  totpCode.value = ''
  totpQRCode.value = ''
  message.value = { type: 'error', text: t('personal.totpSetupCancelled') }
}

const clearTOTP = async () => {
  message.value = null
  try {
    clearTOTPTimer()
    await AuthService.ClearTOTP()
    isTOTPSet.value = false
    showTOTPForm.value = false
    totpSetupResult.value = null
    confirmClearTOTP.value = false
    message.value = { type: 'success', text: t('personal.removedSuccess') }
    await loadAuthInfo()
  } catch (e: any) {
    message.value = { type: 'error', text: e.message || t('personal.removeFailed') }
  }
}

// ==================== Passkey 操作 ====================

const registerPasskey = async () => {
  message.value = null

  // 检查 WebAuthn API 是否可用
  if (!window.PublicKeyCredential) {
    message.value = { type: 'error', text: t('personal.passkeyNotSupported') }
    return
  }

  try {
    // 获取后端的注册选项
    const response = await AuthService.StartPasskeyRegistration()
    const options = JSON.parse(response.credentialCreationOptions)

    // 调用浏览器 WebAuthn API 创建凭据
    const credential = await navigator.credentials.create({
      publicKey: {
        rp: options.rp,
        user: {
          id: base64urlToBuffer(options.user.id),
          name: options.user.name,
          displayName: options.user.displayName,
        },
        challenge: base64urlToBuffer(options.challenge),
        pubKeyCredParams: options.pubKeyCredParams,
        authenticatorSelection: options.authenticatorSelection,
        timeout: options.timeout || 60000,
        attestation: options.attestation || 'none',
      },
    })

    if (!credential) {
      message.value = { type: 'error', text: t('personal.passkeyRegisterFailed') }
      return
    }

    // 将凭据数据转换为 JSON 传递给后端
    const passkeyData = JSON.stringify({
      id: credential.id,
      rawId: bufferToBase64url(credential.rawId),
      type: credential.type,
      response: {
        attestationObject: bufferToBase64url((credential.response as any).attestationObject),
        clientDataJSON: bufferToBase64url(credential.response.clientDataJSON),
      },
    })

    await AuthService.FinishPasskeyRegistration(passkeyData)
    isPasskeySet.value = true
    showPasskeyForm.value = false
    message.value = { type: 'success', text: t('personal.passkeyRegisterSuccess') }
    await loadAuthInfo()
  } catch (e: any) {
    console.error(t('personal.passkeyRegisterFailed'), e)
    // 提供更具体的错误信息
    if (e.name === 'NotAllowedError') {
      message.value = { type: 'error', text: t('personal.passkeyNotAllowed') }
    } else if (e.name === 'SecurityError') {
      message.value = { type: 'error', text: t('personal.passkeySecurityError') }
    } else {
      message.value = { type: 'error', text: e.message || t('personal.passkeyRegisterFailed') }
    }
  }
}

const clearPasskey = async () => {
  message.value = null
  try {
    await AuthService.ClearPasskey()
    isPasskeySet.value = false
    showPasskeyForm.value = false
    passkeyInfo.value = null
    message.value = { type: 'success', text: t('personal.removedSuccess') }
    await loadAuthInfo()
  } catch (e: any) {
    message.value = { type: 'error', text: e.message || t('personal.removeFailed') }
  }
}

// ==================== 认证方式切换 ====================

const switchMethod = async (method: string) => {
  message.value = null
  try {
    await AuthService.SetActiveMethod(method)
    activeMethod.value = method
    message.value = { type: 'success', text: t('personal.switchSuccess') }
    await loadAuthInfo()
  } catch (e: any) {
    message.value = { type: 'error', text: e.message || t('personal.switchFailed') }
  }
}
</script>

<template>
  <div class="page">
    <div>
      <h1 class="text-2xl font-bold">{{ t('personal.title') }}</h1>
      <p class="text-sm mt-1 opacity-60">{{ t('personal.subtitle') }}</p>
    </div>

    <!-- 消息提示 -->
    <n-alert v-if="message" :type="message.type">
      {{ message.text }}
    </n-alert>

    <!-- 认证方式管理 -->
    <n-card :title="t('personal.authMethod')" size="small">
      <div class="space-y-4">
        <!-- 当前激活方式 -->
        <div class="flex items-center gap-2">
          <span class="text-sm">{{ t('personal.activeMethod') }}:</span>
          <n-tag type="primary" size="small">
            {{ methodLabel(activeMethod) }}
          </n-tag>
        </div>

        <!-- 密码 -->
        <div class="flex items-center justify-between p-3 rounded-lg border">
          <div class="flex items-center gap-3">
            <div
              class="w-10 h-10 rounded-lg flex items-center justify-center bg-blue-50 dark:bg-blue-900/30"
            >
              <svg
                class="w-5 h-5 text-blue-500"
                fill="none"
                stroke="currentColor"
                viewBox="0 0 24 24"
              >
                <path
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  stroke-width="2"
                  d="M12 15v2m-6 4h12a2 2 0 002-2v-6a2 2 0 00-2-2H6a2 2 0 00-2 2v6a2 2 0 002 2zm10-10V7a4 4 0 00-8 0v4h8z"
                />
              </svg>
            </div>
            <div>
              <h4 class="font-medium">{{ t('personal.accessPassword') }}</h4>
              <p class="text-xs" :class="isPasswordSet ? 'text-green-500' : 'text-yellow-500'">
                {{ isPasswordSet ? t('personal.passwordSet') : t('personal.passwordNotSet') }}
              </p>
            </div>
          </div>
          <div class="flex flex-col items-end gap-2">
            <div class="flex items-center gap-2">
              <n-button v-if="activeMethod === 'password'" type="primary" size="small" disabled>
                {{ t('personal.activeMethod') }}
              </n-button>

              <n-button
                v-if="!isPasswordSet"
                type="primary"
                size="small"
                @click="showPasswordForm = !showPasswordForm"
              >
                {{ t('personal.setPassword') }}
              </n-button>

              <template v-if="isPasswordSet">
                <n-button
                  v-if="activeMethod !== 'password'"
                  secondary
                  size="small"
                  @click="switchMethod('password')"
                >
                  {{ t('personal.switchMethod') }}
                </n-button>
                <n-button secondary size="small" @click="showPasswordForm = !showPasswordForm">
                  {{ showPasswordForm ? t('common.cancel') : t('personal.changePassword') }}
                </n-button>
                <n-button
                  v-if="!confirmClearPassword"
                  type="error"
                  size="small"
                  @click="confirmClearPassword = true"
                >
                  {{ t('personal.removePassword') }}
                </n-button>
                <n-button v-else type="error" size="small" @click="clearPassword">
                  {{ t('personal.confirmRemove') }}
                </n-button>
              </template>
            </div>

            <n-alert
              v-if="confirmClearPassword && availableMethods.length <= 1"
              type="warning"
              size="small"
              class="max-w-xs"
            >
              {{ t('personal.removeMakesAppOpen') }}
            </n-alert>
          </div>
        </div>

        <!-- 密码表单 -->
        <n-card v-if="showPasswordForm" size="small">
          <div class="max-w-md">
            <n-form label-placement="top">
              <n-form-item v-if="isPasswordSet" :label="t('personal.currentPassword')">
                <n-input
                  v-model:value="oldPassword"
                  :type="showPassword ? 'text' : 'password'"
                  :placeholder="t('personal.enterCurrentPassword')"
                  show-password-on="click"
                />
              </n-form-item>
              <n-form-item
                :label="isPasswordSet ? t('personal.newPassword') : t('personal.password')"
              >
                <n-input
                  v-model:value="newPassword"
                  :type="showPassword ? 'text' : 'password'"
                  :placeholder="t('personal.atLeast6')"
                  show-password-on="click"
                />
              </n-form-item>
              <n-form-item :label="t('personal.confirmPassword')">
                <n-input
                  v-model:value="confirmPassword"
                  :type="showPassword ? 'text' : 'password'"
                  :placeholder="t('personal.enterPasswordAgain')"
                  show-password-on="click"
                />
              </n-form-item>
              <n-button type="primary" @click="isPasswordSet ? changePassword() : setPassword()">
                {{ isPasswordSet ? t('personal.confirmChange') : t('personal.confirmSet') }}
              </n-button>
            </n-form>
          </div>
        </n-card>

        <!-- TOTP -->
        <div class="flex items-center justify-between p-3 rounded-lg border">
          <div class="flex items-center gap-3">
            <div
              class="w-10 h-10 rounded-lg flex items-center justify-center bg-green-50 dark:bg-green-900/30"
            >
              <svg
                class="w-5 h-5 text-green-500"
                fill="none"
                stroke="currentColor"
                viewBox="0 0 24 24"
              >
                <path
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  stroke-width="2"
                  d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z"
                />
              </svg>
            </div>
            <div>
              <h4 class="font-medium">{{ t('personal.totp') }}</h4>
              <p class="text-xs" :class="isTOTPSet ? 'text-green-500' : 'text-yellow-500'">
                {{ isTOTPSet ? t('personal.totpSet') : t('personal.totpNotSet') }}
              </p>
            </div>
          </div>
          <div class="flex flex-col items-end gap-2">
            <div class="flex items-center gap-2">
              <n-button v-if="activeMethod === 'totp'" type="primary" size="small" disabled>
                {{ t('personal.activeMethod') }}
              </n-button>

              <n-button v-if="!isTOTPSet" type="primary" size="small" @click="setupTOTP">
                {{ t('personal.setupTOTP') }}
              </n-button>

              <template v-if="isTOTPSet">
                <n-button
                  v-if="activeMethod !== 'totp'"
                  secondary
                  size="small"
                  @click="switchMethod('totp')"
                >
                  {{ t('personal.switchMethod') }}
                </n-button>
                <n-button
                  v-if="!confirmClearTOTP"
                  type="error"
                  size="small"
                  @click="confirmClearTOTP = true"
                >
                  {{ t('personal.clearTOTP') }}
                </n-button>
                <n-button v-else type="error" size="small" @click="clearTOTP">
                  {{ t('personal.confirmRemove') }}
                </n-button>
              </template>
            </div>

            <n-alert
              v-if="confirmClearTOTP && availableMethods.length <= 1"
              type="warning"
              size="small"
              class="max-w-xs"
            >
              {{ t('personal.removeMakesAppOpen') }}
            </n-alert>
          </div>
        </div>

        <!-- TOTP 设置表单 -->
        <n-card v-if="showTOTPForm && totpSetupResult" size="small">
          <div class="space-y-4">
            <p class="text-sm">{{ t('personal.scanQRCode') }}</p>
            <div class="flex justify-center">
              <img v-if="totpQRCode" :src="totpQRCode" alt="TOTP QR Code" class="w-48 h-48" />
              <div
                v-else
                class="w-48 h-48 bg-gray-100 dark:bg-gray-800 rounded-lg flex items-center justify-center"
              >
                <p class="text-xs text-gray-500">Loading...</p>
              </div>
            </div>
            <div>
              <p class="text-sm mb-1">{{ t('personal.manualKey') }}:</p>
              <n-input :value="totpSetupResult.secret" readonly size="small" />
            </div>
            <n-form-item :label="t('personal.verifyTOTPCode')">
              <n-input
                v-model:value="totpCode"
                type="text"
                maxlength="6"
                :placeholder="t('personal.verifyTOTPCode')"
              />
            </n-form-item>
            <n-alert v-if="totpCountdown > 0" type="warning" size="small">
              {{ t('personal.totpTimeout', { seconds: totpCountdown }) }}
            </n-alert>
            <div class="flex gap-2">
              <n-button type="primary" @click="verifyTOTPSetup">
                {{ t('personal.confirmSet') }}
              </n-button>
              <n-button @click="cancelTOTPSetup">
                {{ t('common.cancel') }}
              </n-button>
            </div>
          </div>
        </n-card>

        <!-- Passkey (桌面应用暂不支持) -->
        <div class="flex items-center justify-between p-3 rounded-lg border opacity-60">
          <div class="flex items-center gap-3">
            <div
              class="w-10 h-10 rounded-lg flex items-center justify-center bg-gray-100 dark:bg-gray-800"
            >
              <svg
                class="w-5 h-5 text-gray-400"
                fill="none"
                stroke="currentColor"
                viewBox="0 0 24 24"
              >
                <path
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  stroke-width="2"
                  d="M15 7a2 2 0 012 2m4 0a6 6 0 01-7.743 5.743L11 17H9v2H7v2H4a1 1 0 01-1-1v-2.586a1 1 0 01.293-.707l5.964-5.964A6 6 0 1121 9z"
                />
              </svg>
            </div>
            <div>
              <h4 class="font-medium">{{ t('personal.passkey') }}</h4>
              <p class="text-xs text-gray-500">
                {{ t('personal.passkeyDesktopNotSupported') }}
              </p>
            </div>
          </div>
          <n-tag type="warning" size="small">
            {{ t('personal.passkeyExperimental') }}
          </n-tag>
        </div>
      </div>
    </n-card>

    <!-- 安全说明 -->
    <n-card :title="t('personal.securityNote')" size="small">
      <div class="space-y-2 text-sm">
        <div class="flex items-start gap-2">
          <svg
            class="w-4 h-4 text-green-500 mt-0.5 shrink-0"
            fill="none"
            stroke="currentColor"
            viewBox="0 0 24 24"
          >
            <path
              stroke-linecap="round"
              stroke-linejoin="round"
              stroke-width="2"
              d="M5 13l4 4L19 7"
            />
          </svg>
          {{ t('personal.bcryptNote') }}
        </div>
        <div class="flex items-start gap-2">
          <svg
            class="w-4 h-4 text-green-500 mt-0.5 shrink-0"
            fill="none"
            stroke="currentColor"
            viewBox="0 0 24 24"
          >
            <path
              stroke-linecap="round"
              stroke-linejoin="round"
              stroke-width="2"
              d="M5 13l4 4L19 7"
            />
          </svg>
          {{ t('personal.verifyNote') }}
        </div>
        <div class="flex items-start gap-2">
          <svg
            class="w-4 h-4 text-green-500 mt-0.5 shrink-0"
            fill="none"
            stroke="currentColor"
            viewBox="0 0 24 24"
          >
            <path
              stroke-linecap="round"
              stroke-linejoin="round"
              stroke-width="2"
              d="M5 13l4 4L19 7"
            />
          </svg>
          {{ t('personal.totpNote') }}
        </div>
        <div class="flex items-start gap-2">
          <svg
            class="w-4 h-4 text-green-500 mt-0.5 shrink-0"
            fill="none"
            stroke="currentColor"
            viewBox="0 0 24 24"
          >
            <path
              stroke-linecap="round"
              stroke-linejoin="round"
              stroke-width="2"
              d="M5 13l4 4L19 7"
            />
          </svg>
          {{ t('personal.passkeyNote') }}
        </div>
        <div class="flex items-start gap-2">
          <svg
            class="w-4 h-4 text-green-500 mt-0.5 shrink-0"
            fill="none"
            stroke="currentColor"
            viewBox="0 0 24 24"
          >
            <path
              stroke-linecap="round"
              stroke-linejoin="round"
              stroke-width="2"
              d="M5 13l4 4L19 7"
            />
          </svg>
          {{ t('personal.storageNote') }}
        </div>
      </div>
    </n-card>
  </div>
</template>
