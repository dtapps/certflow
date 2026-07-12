<script setup lang="ts">
import { ref } from 'vue'
import { NTabs, NTabPane } from 'naive-ui'
import { useI18nStore } from '../stores/i18n'
import DNSProviders from './DNSProviders.vue'
import DeployCredentials from './DeployCredentials.vue'

const i18nStore = useI18nStore()
const { t } = i18nStore

const activeTab = ref('dns')
</script>

<template>
  <div class="providers-page">
    <div class="page-header">
      <div>
        <h1 class="text-2xl font-bold">{{ t('nav.cloudServices') }}</h1>
        <p class="text-sm text-gray-500 mt-1">管理 DNS 凭证和部署目标</p>
      </div>
    </div>

    <n-tabs v-model:value="activeTab" type="line" class="mt-4">
      <n-tab-pane name="dns" tab="DNS 凭证">
        <DNSProviders />
      </n-tab-pane>
      <n-tab-pane name="deploy" tab="部署凭证">
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
