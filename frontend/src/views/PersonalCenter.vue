<script setup lang="ts">
// @ts-nocheck
import { ref, computed, onMounted } from 'vue'
import * as AuthService from '@bindings/cnb.cool/dtapp/certflow/authservicewrapper'
import { useI18n } from '../stores/i18n'

const { t } = useI18n()

const isPasswordSet = ref(false)
const showChangeForm = ref(false)
const oldPassword = ref('')
const newPassword = ref('')
const confirmPassword = ref('')
const message = ref<{ type: 'success' | 'error'; text: string } | null>(null)
const showPassword = ref(false)
const inputType = computed(() => showPassword.value ? 'text' : 'password')

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
      <h1 class="text-2xl font-bold text-base-content">{{ t('personal.title') }}</h1>
      <p class="text-content-70 text-sm mt-1">{{ t('personal.subtitle') }}</p>
    </div>

    <!-- 消息提示 -->
    <div v-if="message" class="p-4 rounded-xl" :class="message.type === 'success' ? 'bg-success-soft border border-success-soft' : 'bg-error-soft border border-error-soft'">
      <p :class="message.type === 'success' ? 'text-success' : 'text-error'" class="text-sm">{{ message.text }}</p>
    </div>

    <!-- 密码状态卡片 -->
    <div class="glass-panel rounded-2xl p-6">
      <div class="flex items-center justify-between">
        <div class="flex items-center gap-4">
          <div class="w-12 h-12 rounded-xl flex items-center justify-center" :class="isPasswordSet ? 'bg-success-soft' : 'bg-amber-soft'">
            <svg v-if="isPasswordSet" class="w-6 h-6 text-success" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12l2 2 4-4m5.618-4.016A11.955 11.955 0 0112 2.944a11.955 11.955 0 01-8.618 3.04A12.02 12.02 0 003 9c0 5.591 3.824 10.29 9 11.622 5.176-1.332 9-6.03 9-11.622 0-1.042-.133-2.052-.382-3.016z" />
            </svg>
            <svg v-else class="w-6 h-6 text-warning" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-2.5L13.732 4c-.77-.833-1.964-.833-2.732 0L4.082 16.5c-.77.833.192 2.5 1.732 2.5z" />
            </svg>
          </div>
          <div>
            <h3 class="text-base-content font-medium">{{ t('personal.accessPassword') }}</h3>
            <p class="text-sm mt-1" :class="isPasswordSet ? 'text-success' : 'text-warning'">
              {{ isPasswordSet ? t('personal.passwordSet') : t('personal.passwordNotSet') }}
            </p>
          </div>
        </div>
        <div class="flex items-center gap-2">
          <button v-if="!isPasswordSet" @click="showChangeForm = !showChangeForm" class="btn btn-primary text-sm">
            {{ t('personal.setPassword') }}
          </button>
          <template v-else>
            <button @click="showChangeForm = !showChangeForm" class="btn btn-secondary text-sm">
              {{ showChangeForm ? t('common.cancel') : t('personal.changePassword') }}
            </button>
            <button v-if="!confirmClear" @click="confirmClear = true" class="btn btn-error text-sm">
              {{ t('personal.removePassword') }}
            </button>
            <button v-else @click="clearPassword" class="btn btn-error text-sm animate-pulse">
              {{ t('personal.confirmRemove') }}
            </button>
          </template>
        </div>
      </div>
    </div>

    <!-- 设置/修改密码表单 -->
    <div v-if="showChangeForm" class="glass-panel rounded-2xl p-6">
      <h3 class="text-base-content font-medium mb-4">{{ isPasswordSet ? t('personal.changePassword') : t('personal.setPassword') }}</h3>
      <div class="space-y-4 max-w-md">
        <div v-if="isPasswordSet">
          <label class="block text-content-80 text-sm font-medium mb-2">{{ t('personal.currentPassword') }}</label>
          <div class="relative">
            <input v-model="oldPassword" :type="inputType" :placeholder="t('personal.enterCurrentPassword')" class="input w-full pr-10" />
            <button type="button" @click="showPassword = !showPassword" class="absolute right-3 top-1/2 -translate-y-1/2 text-content-50 hover:text-content-80">
              <svg v-if="showPassword" class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13.875 18.825A10.05 10.05 0 0112 19c-4.478 0-8.268-2.943-9.543-7a9.97 9.97 0 011.563-3.029m5.858.908a3 3 0 114.243 4.243M9.878 9.878l4.242 4.242M9.88 9.88l-3.29-3.29m7.532 7.532l3.29 3.29M3 3l3.59 3.59m0 0A9.953 9.953 0 0112 5c4.478 0 8.268 2.943 9.543 7a10.025 10.025 0 01-4.132 5.411m0 0L21 21" /></svg>
              <svg v-else class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z" /><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M2.458 12C3.732 7.943 7.523 5 12 5c4.478 0 8.268 2.943 9.542 7-1.274 4.057-5.064 7-9.542 7-4.477 0-8.268-2.943-9.542-7z" /></svg>
            </button>
          </div>
        </div>
        <div>
          <label class="block text-content-80 text-sm font-medium mb-2">{{ isPasswordSet ? t('personal.newPassword') : t('personal.password') }}</label>
          <div class="relative">
            <input v-model="newPassword" :type="inputType" :placeholder="t('personal.atLeast6')" class="input w-full pr-10" />
            <button type="button" @click="showPassword = !showPassword" class="absolute right-3 top-1/2 -translate-y-1/2 text-content-50 hover:text-content-80">
              <svg v-if="showPassword" class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13.875 18.825A10.05 10.05 0 0112 19c-4.478 0-8.268-2.943-9.543-7a9.97 9.97 0 011.563-3.029m5.858.908a3 3 0 114.243 4.243M9.878 9.878l4.242 4.242M9.88 9.88l-3.29-3.29m7.532 7.532l3.29 3.29M3 3l3.59 3.59m0 0A9.953 9.953 0 0112 5c4.478 0 8.268 2.943 9.543 7a10.025 10.025 0 01-4.132 5.411m0 0L21 21" /></svg>
              <svg v-else class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z" /><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M2.458 12C3.732 7.943 7.523 5 12 5c4.478 0 8.268 2.943 9.542 7-1.274 4.057-5.064 7-9.542 7-4.477 0-8.268-2.943-9.542-7z" /></svg>
            </button>
          </div>
        </div>
        <div>
          <label class="block text-content-80 text-sm font-medium mb-2">{{ t('personal.confirmPassword') }}</label>
          <div class="relative">
            <input v-model="confirmPassword" :type="inputType" :placeholder="t('personal.enterPasswordAgain')" class="input w-full pr-10" />
            <button type="button" @click="showPassword = !showPassword" class="absolute right-3 top-1/2 -translate-y-1/2 text-content-50 hover:text-content-80">
              <svg v-if="showPassword" class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13.875 18.825A10.05 10.05 0 0112 19c-4.478 0-8.268-2.943-9.543-7a9.97 9.97 0 011.563-3.029m5.858.908a3 3 0 114.243 4.243M9.878 9.878l4.242 4.242M9.88 9.88l-3.29-3.29m7.532 7.532l3.29 3.29M3 3l3.59 3.59m0 0A9.953 9.953 0 0112 5c4.478 0 8.268 2.943 9.543 7a10.025 10.025 0 01-4.132 5.411m0 0L21 21" /></svg>
              <svg v-else class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z" /><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M2.458 12C3.732 7.943 7.523 5 12 5c4.478 0 8.268 2.943 9.542 7-1.274 4.057-5.064 7-9.542 7-4.477 0-8.268-2.943-9.542-7z" /></svg>
            </button>
          </div>
        </div>
        <button @click="isPasswordSet ? changePassword() : setPassword()" class="btn btn-primary text-sm">
          {{ isPasswordSet ? t('personal.confirmChange') : t('personal.confirmSet') }}
        </button>
      </div>
    </div>

    <!-- 安全说明 -->
    <div class="glass-panel rounded-2xl p-6">
      <h3 class="text-base-content font-medium mb-3">{{ t('personal.securityNote') }}</h3>
      <ul class="space-y-2 text-content-70 text-sm">
        <li class="flex items-start gap-2">
          <svg class="w-4 h-4 text-success mt-0.5 shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7" /></svg>
          {{ t('personal.bcryptNote') }}
        </li>
        <li class="flex items-start gap-2">
          <svg class="w-4 h-4 text-success mt-0.5 shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7" /></svg>
          {{ t('personal.verifyNote') }}
        </li>
        <li class="flex items-start gap-2">
          <svg class="w-4 h-4 text-success mt-0.5 shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7" /></svg>
          {{ t('personal.storageNote') }}
        </li>
      </ul>
    </div>
  </div>
</template>
