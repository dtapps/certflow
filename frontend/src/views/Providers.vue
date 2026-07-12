<script setup lang="ts">
import { ref, computed, provide } from 'vue'
import { NTabs, NTabPane, NButton } from 'naive-ui'
import { useI18nStore } from '../stores/i18n'
import DNSProviders from './DNSProviders.vue'
import DeployCredentials from './DeployCredentials.vue'

const i18nStore = useI18nStore()
const { t } = i18nStore

const activeTab = ref('dns')
const showCreateModal = ref(false)

provide('showCreateModal', showCreateModal)

const pageTitle = computed(() => {
  return activeTab.value === 'dns' ? t('dns.credentialTitle') : t('deploy.credentialTitle')
})

const pageSubtitle = computed(() => {
  return activeTab.value === 'dns' ? t('dns.credentialSubtitle') : t('deploy.credentialSubtitle')
})

const createButtonText = computed(() => {
  return activeTab.value === 'dns' ? t('dns.addProvider') : t('deploy.credentialCreate')
})
</script>

<template>
  <div class="providers-page">
    <div class="page-header">
      <div>
        <h1 class="text-2xl font-bold">{{ pageTitle }}</h1>
        <p class="text-sm text-gray-500 mt-1">{{ pageSubtitle }}</p>
      </div>
      <n-button type="primary" @click="showCreateModal = true">
        <template #icon>
          <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path
              stroke-linecap="round"
              stroke-linejoin="round"
              stroke-width="2"
              d="M12 4v16m8-8H4"
            />
          </svg>
        </template>
        {{ createButtonText }}
      </n-button>
    </div>

    <n-tabs v-model:value="activeTab" type="line" class="mt-4">
      <n-tab-pane name="dns" :tab="t('dns.credentialTitle')">
        <DNSProviders />
      </n-tab-pane>
      <n-tab-pane name="deploy" :tab="t('deploy.credentialTitle')">
        <DeployCredentials />
      </n-tab-pane>
    </n-tabs>
  </div>
</template>

<style scoped>
.providers-page {
  padding: 1.5rem;
  height: 100%;
  overflow-y: auto;
}

.page-header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
}
</style>
