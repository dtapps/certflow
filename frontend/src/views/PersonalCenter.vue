<script setup lang="ts">
// @ts-nocheck
import { ref, computed, onMounted } from 'vue'
import { NCard, NButton, NInput, NSwitch, NSpin, NForm, NFormItem, NAlert, NTag } from 'naive-ui'
import * as AuthService from '@bindings/cnb.cool/dtapp/certflow/authservicewrapper'
import { useI18nStore } from '../stores/i18n'

const i18nStore = useI18nStore()
const { t } = i18nStore

const isPasswordSet = ref(false)
const showChangeForm = ref(false)
const oldPassword = ref('')
const newPassword = ref('')
const confirmPassword = ref('')
const message = ref<{ type: 'success' | 'error'; text: string } | null>(null)
const showPassword = ref(false)

onMounted(async () => {
  isPasswordSet.value = await AuthService.IsPasswordSet()
})

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
    showChangeForm.value = false
    newPassword.value = ''
    confirmPassword.value = ''
    message.value = { type: 'success', text: t('personal.setSuccess') }
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
    showChangeForm.value = false
    oldPassword.value = ''
    newPassword.value = ''
    confirmPassword.value = ''
    message.value = { type: 'success', text: t('personal.changeSuccess') }
  } catch (e: any) {
    message.value = { type: 'error', text: e.message || t('personal.changeFailed') }
  }
}

const confirmClear = ref(false)

const clearPassword = async () => {
  message.value = null
  try {
    await AuthService.ClearPassword()
    isPasswordSet.value = false
    showChangeForm.value = false
    confirmClear.value = false
    message.value = { type: 'success', text: t('personal.removedSuccess') }
  } catch (e: any) {
    message.value = { type: 'error', text: e.message || t('personal.removeFailed') }
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

    <!-- 密码状态卡片 -->
    <n-card size="small">
      <div class="flex items-center justify-between">
        <div class="flex items-center gap-4">
          <div
            class="w-12 h-12 rounded-xl flex items-center justify-center"
            :class="
              isPasswordSet
                ? 'bg-green-50 dark:bg-green-900/30'
                : 'bg-yellow-50 dark:bg-yellow-900/30'
            "
          >
            <svg
              v-if="isPasswordSet"
              class="w-6 h-6 text-green-500"
              fill="none"
              stroke="currentColor"
              viewBox="0 0 24 24"
            >
              <path
                stroke-linecap="round"
                stroke-linejoin="round"
                stroke-width="2"
                d="M9 12l2 2 4-4m5.618-4.016A11.955 11.955 0 0112 2.944a11.955 11.955 0 01-8.618 3.04A12.02 12.02 0 003 9c0 5.591 3.824 10.29 9 11.622 5.176-1.332 9-6.03 9-11.622 0-1.042-.133-2.052-.382-3.016z"
              />
            </svg>
            <svg
              v-else
              class="w-6 h-6 text-yellow-500"
              fill="none"
              stroke="currentColor"
              viewBox="0 0 24 24"
            >
              <path
                stroke-linecap="round"
                stroke-linejoin="round"
                stroke-width="2"
                d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-2.5L13.732 4c-.77-.833-1.964-.833-2.732 0L4.082 16.5c-.77.833.192 2.5 1.732 2.5z"
              />
            </svg>
          </div>
          <div>
            <h3 class="font-medium">{{ t('personal.accessPassword') }}</h3>
            <p class="text-sm mt-1" :class="isPasswordSet ? 'text-green-500' : 'text-yellow-500'">
              {{ isPasswordSet ? t('personal.passwordSet') : t('personal.passwordNotSet') }}
            </p>
          </div>
        </div>
        <div class="flex items-center gap-2">
          <n-button
            v-if="!isPasswordSet"
            type="primary"
            size="small"
            @click="showChangeForm = !showChangeForm"
          >
            {{ t('personal.setPassword') }}
          </n-button>
          <template v-else>
            <n-button secondary size="small" @click="showChangeForm = !showChangeForm">
              {{ showChangeForm ? t('common.cancel') : t('personal.changePassword') }}
            </n-button>
            <n-button v-if="!confirmClear" type="error" size="small" @click="confirmClear = true">
              {{ t('personal.removePassword') }}
            </n-button>
            <n-button
              v-else
              type="error"
              size="small"
              @click="clearPassword"
              style="animation: pulse 1s infinite"
            >
              {{ t('personal.confirmRemove') }}
            </n-button>
          </template>
        </div>
      </div>
    </n-card>

    <!-- 设置/修改密码表单 -->
    <n-card
      v-if="showChangeForm"
      :title="isPasswordSet ? t('personal.changePassword') : t('personal.setPassword')"
      size="small"
    >
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
          <n-form-item :label="isPasswordSet ? t('personal.newPassword') : t('personal.password')">
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
          {{ t('personal.storageNote') }}
        </div>
      </div>
    </n-card>
  </div>
</template>
