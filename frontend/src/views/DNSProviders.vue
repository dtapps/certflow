<script setup lang="ts">
import { ref } from 'vue'
import * as DNSProviderService from '@bindings/cnb.cool/dtapp/certflow/dnsproviderservicewrapper'
import CredentialList from './CredentialList.vue'

const providerConfigSchema: Record<
  string,
  { key: string; label: string; type: 'text' | 'password' }[]
> = {
  cloudflare: [
    { key: 'email', label: 'Email', type: 'text' },
    { key: 'api_key', label: 'API Key', type: 'password' },
    { key: 'api_token', label: 'API Token', type: 'password' },
  ],
  aliyun: [
    { key: 'access_key_id', label: 'Access Key ID', type: 'text' },
    { key: 'access_key_secret', label: 'Access Key Secret', type: 'password' },
    { key: 'region_id', label: 'Region ID', type: 'text' },
  ],
  huawei: [
    { key: 'access_key_id', label: 'Access Key ID', type: 'text' },
    { key: 'secret_access_key', label: 'Secret Access Key', type: 'password' },
    { key: 'region', label: 'Region', type: 'text' },
  ],
  tencentcloud: [
    { key: 'secret_id', label: 'Secret ID', type: 'text' },
    { key: 'secret_key', label: 'Secret Key', type: 'password' },
    { key: 'region', label: 'Region', type: 'text' },
  ],
  aws: [
    { key: 'access_key_id', label: 'Access Key ID', type: 'text' },
    { key: 'secret_access_key', label: 'Secret Access Key', type: 'password' },
    { key: 'region', label: 'Region', type: 'text' },
  ],
  googlecloud: [
    { key: 'client_id', label: 'Client ID', type: 'text' },
    { key: 'email', label: 'Email', type: 'text' },
    { key: 'password', label: 'Password', type: 'password' },
  ],
  baiducloud: [
    { key: 'access_key_id', label: 'Access Key ID', type: 'text' },
    { key: 'secret_access_key', label: 'Secret Access Key', type: 'password' },
  ],
  jdcloud: [
    { key: 'access_key_id', label: 'Access Key ID', type: 'text' },
    { key: 'access_key_secret', label: 'Access Key Secret', type: 'password' },
    { key: 'region_id', label: 'Region ID', type: 'text' },
  ],
  volcengine: [
    { key: 'access_key', label: 'Access Key', type: 'text' },
    { key: 'secret_key', label: 'Secret Key', type: 'password' },
    { key: 'region', label: 'Region', type: 'text' },
  ],
  edgeone: [
    { key: 'secret_id', label: 'Secret ID', type: 'text' },
    { key: 'secret_key', label: 'Secret Key', type: 'password' },
    { key: 'region', label: 'Region', type: 'text' },
  ],
  aliesa: [
    { key: 'api_key', label: 'API Key', type: 'text' },
    { key: 'secret_key', label: 'Secret Key', type: 'password' },
    { key: 'region_id', label: 'Region ID', type: 'text' },
  ],
  ucloud: [
    { key: 'public_key', label: 'Public Key', type: 'text' },
    { key: 'private_key', label: 'Private Key', type: 'password' },
    { key: 'region', label: 'Region', type: 'text' },
  ],
  westcn: [
    { key: 'username', label: 'Username', type: 'text' },
    { key: 'password', label: 'Password', type: 'password' },
  ],
  com35: [
    { key: 'username', label: 'Username', type: 'text' },
    { key: 'password', label: 'Password', type: 'password' },
  ],
  rainyun: [{ key: 'api_key', label: 'API Key', type: 'text' }],
  todaynic: [
    { key: 'auth_user_id', label: 'Auth User ID', type: 'text' },
    { key: 'api_key', label: 'API Key', type: 'text' },
  ],
  dnsla: [
    { key: 'api_id', label: 'API ID', type: 'text' },
    { key: 'api_secret', label: 'API Secret', type: 'password' },
  ],
  dns51: [
    { key: 'api_key', label: 'API Key', type: 'text' },
    { key: 'api_secret', label: 'API Secret', type: 'password' },
  ],
  xinnet: [
    { key: 'secret', label: 'Secret', type: 'password' },
    { key: 'agent_id', label: 'Agent ID', type: 'text' },
  ],
  porkbun: [
    { key: 'api_key', label: 'API Key', type: 'text' },
    { key: 'secret_api_key', label: 'Secret API Key', type: 'password' },
  ],
  namecheap: [
    { key: 'api_user', label: 'API User', type: 'text' },
    { key: 'api_key', label: 'API Key', type: 'text' },
    { key: 'client_ip', label: 'Client IP', type: 'text' },
  ],
  godaddy: [
    { key: 'api_key', label: 'API Key', type: 'text' },
    { key: 'api_secret', label: 'API Secret', type: 'password' },
  ],
  gandiv5: [{ key: 'personal_access_token', label: 'Personal Access Token', type: 'password' }],
  dynadot: [
    { key: 'api_key', label: 'API Key', type: 'text' },
    { key: 'api_secret', label: 'API Secret', type: 'password' },
  ],
  azuredns: [
    { key: 'subscription_id', label: 'Subscription ID', type: 'text' },
    { key: 'resource_group', label: 'Resource Group', type: 'text' },
    { key: 'client_id', label: 'Client ID', type: 'text' },
    { key: 'client_secret', label: 'Client Secret', type: 'password' },
    { key: 'tenant_id', label: 'Tenant ID', type: 'text' },
  ],
  digitalocean: [{ key: 'auth_token', label: 'Auth Token', type: 'password' }],
  vultr: [{ key: 'api_key', label: 'API Key', type: 'password' }],
  hetzner: [{ key: 'api_token', label: 'API Token', type: 'password' }],
  linode: [{ key: 'token', label: 'Token', type: 'password' }],
  ovh: [
    { key: 'application_key', label: 'Application Key', type: 'text' },
    { key: 'application_secret', label: 'Application Secret', type: 'password' },
    { key: 'consumer_key', label: 'Consumer Key', type: 'password' },
  ],
  dnsimple: [{ key: 'access_token', label: 'Access Token', type: 'password' }],
  ns1: [{ key: 'api_key', label: 'API Key', type: 'password' }],
}

const providerTypes = [
  { value: 'cloudflare', label: 'Cloudflare' },
  { value: 'aliyun', label: '阿里云' },
  { value: 'huawei', label: '华为云' },
  { value: 'tencentcloud', label: '腾讯云' },
  { value: 'aws', label: 'AWS Route53' },
  { value: 'googlecloud', label: 'Google Cloud' },
  { value: 'baiducloud', label: '百度云' },
  { value: 'jdcloud', label: '京东云' },
  { value: 'volcengine', label: '火山引擎' },
  { value: 'edgeone', label: '腾讯云 EdgeOne' },
  { value: 'aliesa', label: '阿里云 ESA' },
  { value: 'ucloud', label: 'UCloud' },
  { value: 'westcn', label: '西部数码' },
  { value: 'com35', label: '三五互联' },
  { value: 'rainyun', label: '雨云' },
  { value: 'todaynic', label: '今天互联' },
  { value: 'dnsla', label: 'DNSLA' },
  { value: 'dns51', label: '51DNS' },
  { value: 'xinnet', label: '新网' },
  { value: 'porkbun', label: 'Porkbun' },
  { value: 'namecheap', label: 'Namecheap' },
  { value: 'godaddy', label: 'GoDaddy' },
  { value: 'gandiv5', label: 'Gandi' },
  { value: 'dynadot', label: 'Dynadot' },
  { value: 'azuredns', label: 'Azure DNS' },
  { value: 'digitalocean', label: 'DigitalOcean' },
  { value: 'vultr', label: 'Vultr' },
  { value: 'hetzner', label: 'Hetzner' },
  { value: 'linode', label: 'Linode' },
  { value: 'ovh', label: 'OVH' },
  { value: 'dnsimple', label: 'DNSimple' },
  { value: 'ns1', label: 'NS1' },
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
</script>

<template>
  <CredentialList
    ref="credentialListRef"
    title="DNS 凭证"
    subtitle="管理 DNS 验证用的 API 凭证"
    empty-text="暂无 DNS 凭证"
    create-text="添加 DNS 凭证"
    edit-text="编辑 DNS 凭证"
    :provider-types="providerTypes"
    :config-schema="providerConfigSchema"
    icon-color="green"
    :load-items="loadItems"
    :create-item="createItem"
    :update-item="updateItem"
    :delete-item="deleteItem"
  />
</template>
