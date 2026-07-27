<script setup lang="ts">
import { ref } from 'vue'
import * as DNSProviderService from '@bindings/cnb.cool/dtapp/certflow/dnsproviderservicewrapper'
import CredentialList from './CredentialList.vue'
import { useI18nStore } from '../stores/i18n'
import { dnsProviderConfigSchema, dnsProviderTypes } from '../utils/dnsProviderConfig'

const i18nStore = useI18nStore()
const { t } = i18nStore

const credentialListRef = ref<InstanceType<typeof CredentialList>>()

const loadItems = async () => {
  const list = await DNSProviderService.ListDNSProviders()
  return list || []
}

const createItem = async (data: any) => {
  return await DNSProviderService.CreateDNSProvider(data)
}

const updateItem = async (id: number, data: any) => {
  return await DNSProviderService.UpdateDNSProvider(id, data)
}

const deleteItem = async (id: number) => {
  return await DNSProviderService.DeleteDNSProvider(id)
}

const setActiveItem = async (id: number, active: boolean) => {
  await DNSProviderService.SetActive(id, active)
}
</script>

<template>
  <CredentialList
    ref="credentialListRef"
    :title="t('dns.credentialTitle')"
    :subtitle="t('dns.credentialSubtitle')"
    :empty-text="t('dns.noProvider')"
    :create-text="t('dns.addProvider')"
    :edit-text="t('dns.editProvider')"
    :provider-types="dnsProviderTypes"
    :config-schema="dnsProviderConfigSchema"
    :load-items="loadItems"
    :create-item="createItem"
    :update-item="updateItem"
    :delete-item="deleteItem"
    :set-active-item="setActiveItem"
  />
</template>
