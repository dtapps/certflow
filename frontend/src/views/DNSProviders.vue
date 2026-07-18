<script setup lang="ts">
import { ref } from 'vue'
import * as DNSProviderService from '@bindings/cnb.cool/dtapp/certflow/dnsproviderservicewrapper'
import CredentialList from './CredentialList.vue'
import { useI18nStore } from '../stores/i18n'

const i18nStore = useI18nStore()
const { t } = i18nStore

const providerConfigSchema: Record<
  string,
  { key: string; labelKey: string; type: 'text' | 'password' }[]
> = {
  cloudflare: [
    { key: 'email', labelKey: 'dns.config.email', type: 'text' },
    { key: 'api_key', labelKey: 'dns.config.api_key', type: 'password' },
    { key: 'api_token', labelKey: 'dns.config.api_token', type: 'password' },
  ],
  aliyun: [
    { key: 'access_key_id', labelKey: 'dns.config.access_key_id', type: 'text' },
    { key: 'access_key_secret', labelKey: 'dns.config.access_key_secret', type: 'password' },
    { key: 'region_id', labelKey: 'dns.config.region_id', type: 'text' },
  ],
  huawei: [
    { key: 'access_key_id', labelKey: 'dns.config.access_key_id', type: 'text' },
    { key: 'secret_access_key', labelKey: 'dns.config.secret_access_key', type: 'password' },
    { key: 'region', labelKey: 'dns.config.region', type: 'text' },
  ],
  tencentcloud: [
    { key: 'secret_id', labelKey: 'dns.config.secret_id', type: 'text' },
    { key: 'secret_key', labelKey: 'dns.config.secret_key', type: 'password' },
    { key: 'region', labelKey: 'dns.config.region', type: 'text' },
  ],
  aws: [
    { key: 'access_key_id', labelKey: 'dns.config.access_key_id', type: 'text' },
    { key: 'secret_access_key', labelKey: 'dns.config.secret_access_key', type: 'password' },
    { key: 'region', labelKey: 'dns.config.region', type: 'text' },
  ],
  googlecloud: [
    { key: 'client_id', labelKey: 'dns.config.client_id', type: 'text' },
    { key: 'email', labelKey: 'dns.config.email', type: 'text' },
    { key: 'password', labelKey: 'dns.config.password', type: 'password' },
  ],
  baiducloud: [
    { key: 'access_key_id', labelKey: 'dns.config.access_key_id', type: 'text' },
    { key: 'secret_access_key', labelKey: 'dns.config.secret_access_key', type: 'password' },
  ],
  jdcloud: [
    { key: 'access_key_id', labelKey: 'dns.config.access_key_id', type: 'text' },
    { key: 'access_key_secret', labelKey: 'dns.config.access_key_secret', type: 'password' },
    { key: 'region_id', labelKey: 'dns.config.region_id', type: 'text' },
  ],
  volcengine: [
    { key: 'access_key', labelKey: 'dns.config.access_key', type: 'text' },
    { key: 'secret_key', labelKey: 'dns.config.secret_key', type: 'password' },
    { key: 'region', labelKey: 'dns.config.region', type: 'text' },
  ],
  edgeone: [
    { key: 'secret_id', labelKey: 'dns.config.secret_id', type: 'text' },
    { key: 'secret_key', labelKey: 'dns.config.secret_key', type: 'password' },
    { key: 'region', labelKey: 'dns.config.region', type: 'text' },
  ],
  aliesa: [
    { key: 'api_key', labelKey: 'dns.config.api_key', type: 'text' },
    { key: 'secret_key', labelKey: 'dns.config.secret_key', type: 'password' },
    { key: 'region_id', labelKey: 'dns.config.region_id', type: 'text' },
  ],
  ucloud: [
    { key: 'public_key', labelKey: 'dns.config.public_key', type: 'text' },
    { key: 'private_key', labelKey: 'dns.config.private_key', type: 'password' },
    { key: 'region', labelKey: 'dns.config.region', type: 'text' },
  ],
  westcn: [
    { key: 'username', labelKey: 'dns.config.username', type: 'text' },
    { key: 'password', labelKey: 'dns.config.password', type: 'password' },
  ],
  com35: [
    { key: 'username', labelKey: 'dns.config.username', type: 'text' },
    { key: 'password', labelKey: 'dns.config.password', type: 'password' },
  ],
  rainyun: [{ key: 'api_key', labelKey: 'dns.config.api_key', type: 'text' }],
  todaynic: [
    { key: 'auth_user_id', labelKey: 'dns.config.auth_user_id', type: 'text' },
    { key: 'api_key', labelKey: 'dns.config.api_key', type: 'text' },
  ],
  dnsla: [
    { key: 'api_id', labelKey: 'dns.config.api_id', type: 'text' },
    { key: 'api_secret', labelKey: 'dns.config.api_secret', type: 'password' },
  ],
  dns51: [
    { key: 'api_key', labelKey: 'dns.config.api_key', type: 'text' },
    { key: 'api_secret', labelKey: 'dns.config.api_secret', type: 'password' },
  ],
  xinnet: [
    { key: 'secret', labelKey: 'dns.config.secret', type: 'password' },
    { key: 'agent_id', labelKey: 'dns.config.agent_id', type: 'text' },
  ],
  porkbun: [
    { key: 'api_key', labelKey: 'dns.config.api_key', type: 'text' },
    { key: 'secret_api_key', labelKey: 'dns.config.secret_api_key', type: 'password' },
  ],
  namecheap: [
    { key: 'api_user', labelKey: 'dns.config.api_user', type: 'text' },
    { key: 'api_key', labelKey: 'dns.config.api_key', type: 'text' },
    { key: 'client_ip', labelKey: 'dns.config.client_ip', type: 'text' },
  ],
  godaddy: [
    { key: 'api_key', labelKey: 'dns.config.api_key', type: 'text' },
    { key: 'api_secret', labelKey: 'dns.config.api_secret', type: 'password' },
  ],
  gandiv5: [
    {
      key: 'personal_access_token',
      labelKey: 'dns.config.personal_access_token',
      type: 'password',
    },
  ],
  dynadot: [
    { key: 'api_key', labelKey: 'dns.config.api_key', type: 'text' },
    { key: 'api_secret', labelKey: 'dns.config.api_secret', type: 'password' },
  ],
  azuredns: [
    { key: 'subscription_id', labelKey: 'dns.config.subscription_id', type: 'text' },
    { key: 'resource_group', labelKey: 'dns.config.resource_group', type: 'text' },
    { key: 'client_id', labelKey: 'dns.config.client_id', type: 'text' },
    { key: 'client_secret', labelKey: 'dns.config.client_secret', type: 'password' },
    { key: 'tenant_id', labelKey: 'dns.config.tenant_id', type: 'text' },
  ],
  digitalocean: [{ key: 'auth_token', labelKey: 'dns.config.auth_token', type: 'password' }],
  vultr: [{ key: 'api_key', labelKey: 'dns.config.api_key', type: 'password' }],
  hetzner: [{ key: 'api_token', labelKey: 'dns.config.api_token', type: 'password' }],
  linode: [{ key: 'token', labelKey: 'dns.config.token', type: 'password' }],
  ovh: [
    { key: 'application_key', labelKey: 'dns.config.application_key', type: 'text' },
    { key: 'application_secret', labelKey: 'dns.config.application_secret', type: 'password' },
    { key: 'consumer_key', labelKey: 'dns.config.consumer_key', type: 'password' },
  ],
  dnsimple: [{ key: 'access_token', labelKey: 'dns.config.access_token', type: 'password' }],
  ns1: [{ key: 'api_key', labelKey: 'dns.config.api_key', type: 'password' }],
}

const providerTypes = [
  { value: 'cloudflare', labelKey: 'dns.type.cloudflare' },
  { value: 'aliyun', labelKey: 'dns.type.aliyun' },
  { value: 'huawei', labelKey: 'dns.type.huawei' },
  { value: 'tencentcloud', labelKey: 'dns.type.tencentcloud' },
  { value: 'aws', labelKey: 'dns.type.aws' },
  { value: 'googlecloud', labelKey: 'dns.type.googlecloud' },
  { value: 'baiducloud', labelKey: 'dns.type.baiducloud' },
  { value: 'jdcloud', labelKey: 'dns.type.jdcloud' },
  { value: 'volcengine', labelKey: 'dns.type.volcengine' },
  { value: 'edgeone', labelKey: 'dns.type.edgeone' },
  { value: 'aliesa', labelKey: 'dns.type.aliesa' },
  { value: 'ucloud', labelKey: 'dns.type.ucloud' },
  { value: 'westcn', labelKey: 'dns.type.westcn' },
  { value: 'com35', labelKey: 'dns.type.com35' },
  { value: 'rainyun', labelKey: 'dns.type.rainyun' },
  { value: 'todaynic', labelKey: 'dns.type.todaynic' },
  { value: 'dnsla', labelKey: 'dns.type.dnsla' },
  { value: 'dns51', labelKey: 'dns.type.dns51' },
  { value: 'xinnet', labelKey: 'dns.type.xinnet' },
  { value: 'porkbun', labelKey: 'dns.type.porkbun' },
  { value: 'namecheap', labelKey: 'dns.type.namecheap' },
  { value: 'godaddy', labelKey: 'dns.type.godaddy' },
  { value: 'gandiv5', labelKey: 'dns.type.gandiv5' },
  { value: 'dynadot', labelKey: 'dns.type.dynadot' },
  { value: 'azuredns', labelKey: 'dns.type.azuredns' },
  { value: 'digitalocean', labelKey: 'dns.type.digitalocean' },
  { value: 'vultr', labelKey: 'dns.type.vultr' },
  { value: 'hetzner', labelKey: 'dns.type.hetzner' },
  { value: 'linode', labelKey: 'dns.type.linode' },
  { value: 'ovh', labelKey: 'dns.type.ovh' },
  { value: 'dnsimple', labelKey: 'dns.type.dnsimple' },
  { value: 'ns1', labelKey: 'dns.type.ns1' },
]

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
    :provider-types="providerTypes"
    :config-schema="providerConfigSchema"
    icon-color="green"
    :load-items="loadItems"
    :create-item="createItem"
    :update-item="updateItem"
    :delete-item="deleteItem"
    :set-active-item="setActiveItem"
  />
</template>
