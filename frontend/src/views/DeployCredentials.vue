<script setup lang="ts">
import { ref } from 'vue'
import * as DeployCredentialService from '@bindings/cnb.cool/dtapp/certflow/deploycredentialservicewrapper'
import CredentialList from './CredentialList.vue'
import { useI18nStore } from '../stores/i18n'

const i18nStore = useI18nStore()
const { t } = i18nStore

const providerConfigSchema: Record<
  string,
  { key: string; labelKey: string; type: 'text' | 'password' }[]
> = {
  aliyun: [
    { key: 'access_key_id', labelKey: 'dns.config.access_key_id', type: 'text' },
    { key: 'access_key_secret', labelKey: 'dns.config.access_key_secret', type: 'password' },
    { key: 'region', labelKey: 'deploy.config.region', type: 'text' },
  ],
  tencentcloud: [
    { key: 'secret_id', labelKey: 'dns.config.secret_id', type: 'text' },
    { key: 'secret_key', labelKey: 'dns.config.secret_key', type: 'password' },
    { key: 'region', labelKey: 'deploy.config.region', type: 'text' },
  ],
  huawei: [
    { key: 'access_key_id', labelKey: 'dns.config.access_key_id', type: 'text' },
    { key: 'secret_access_key', labelKey: 'dns.config.secret_access_key', type: 'password' },
    { key: 'region', labelKey: 'deploy.config.region', type: 'text' },
  ],
  baiducloud: [
    { key: 'access_key_id', labelKey: 'dns.config.access_key_id', type: 'text' },
    { key: 'access_key_secret', labelKey: 'dns.config.access_key_secret', type: 'password' },
  ],
  ctyun: [
    { key: 'access_key_id', labelKey: 'dns.config.access_key_id', type: 'text' },
    { key: 'access_key_secret', labelKey: 'dns.config.access_key_secret', type: 'password' },
  ],
  btpanel: [
    { key: 'panel_url', labelKey: 'deploy.config.domain', type: 'text' },
    { key: 'api_key', labelKey: 'dns.config.api_key', type: 'password' },
  ],
  '1panel': [
    { key: 'panel_url', labelKey: 'deploy.config.domain', type: 'text' },
    { key: 'api_key', labelKey: 'dns.config.api_key', type: 'password' },
  ],
  acepanel: [
    { key: 'panel_url', labelKey: 'deploy.config.domain', type: 'text' },
    { key: 'api_key', labelKey: 'dns.config.api_key', type: 'password' },
  ],
}

const providerTypes = [
  { value: 'aliyun', labelKey: 'deploy.provider.aliyun' },
  { value: 'tencentcloud', labelKey: 'deploy.provider.tencentcloud' },
  { value: 'huawei', labelKey: 'deploy.provider.huawei' },
  { value: 'baiducloud', labelKey: 'deploy.provider.baidu' },
  { value: 'ctyun', labelKey: 'deploy.provider.ctyun' },
  { value: 'btpanel', labelKey: 'dns.type.btpanel' },
  { value: '1panel', labelKey: 'dns.type.1panel' },
  { value: 'acepanel', labelKey: 'dns.type.acepanel' },
]

const credentialListRef = ref<InstanceType<typeof CredentialList>>()

const loadItems = async () => {
  const list = await DeployCredentialService.ListDeployCredentials()
  return list || []
}

const createItem = async (data: any) => {
  return await DeployCredentialService.CreateDeployCredential(data)
}

const updateItem = async (id: number, data: any) => {
  return await DeployCredentialService.UpdateDeployCredential(id, data)
}

const deleteItem = async (id: number) => {
  return await DeployCredentialService.DeleteDeployCredential(id)
}
</script>

<template>
  <CredentialList
    ref="credentialListRef"
    :title="t('deploy.credentialTitle')"
    :subtitle="t('deploy.credentialSubtitle')"
    :empty-text="t('deploy.credentialEmpty')"
    :create-text="t('deploy.credentialCreate')"
    :edit-text="t('deploy.credentialEdit')"
    :provider-types="providerTypes"
    :config-schema="providerConfigSchema"
    icon-color="blue"
    :load-items="loadItems"
    :create-item="createItem"
    :update-item="updateItem"
    :delete-item="deleteItem"
  />
</template>
