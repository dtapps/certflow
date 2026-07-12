<script setup lang="ts">
import { ref } from 'vue'
import * as DeployCredentialService from '@bindings/cnb.cool/dtapp/certflow/deploycredentialservicewrapper'
import CredentialList from './CredentialList.vue'

const providerConfigSchema: Record<
  string,
  { key: string; label: string; type: 'text' | 'password' }[]
> = {
  aliyun: [
    { key: 'access_key_id', label: 'Access Key ID', type: 'text' },
    { key: 'access_key_secret', label: 'Access Key Secret', type: 'password' },
    { key: 'region', label: '区域', type: 'text' },
  ],
  tencentcloud: [
    { key: 'secret_id', label: 'Secret ID', type: 'text' },
    { key: 'secret_key', label: 'Secret Key', type: 'password' },
    { key: 'region', label: '区域', type: 'text' },
  ],
  huawei: [
    { key: 'access_key_id', label: 'Access Key ID', type: 'text' },
    { key: 'secret_access_key', label: 'Secret Access Key', type: 'password' },
    { key: 'region', label: '区域', type: 'text' },
  ],
  baiducloud: [
    { key: 'access_key_id', label: 'Access Key ID', type: 'text' },
    { key: 'access_key_secret', label: 'Access Key Secret', type: 'password' },
  ],
  btpanel: [
    { key: 'panel_url', label: '面板地址', type: 'text' },
    { key: 'api_key', label: 'API Key', type: 'password' },
  ],
  '1panel': [
    { key: 'panel_url', label: '面板地址', type: 'text' },
    { key: 'api_key', label: 'API Key', type: 'password' },
  ],
  acepanel: [
    { key: 'panel_url', label: '面板地址', type: 'text' },
    { key: 'api_key', label: 'API Key', type: 'password' },
  ],
}

const providerTypes = [
  { value: 'aliyun', label: '阿里云' },
  { value: 'tencentcloud', label: '腾讯云' },
  { value: 'huawei', label: '华为云' },
  { value: 'baiducloud', label: '百度云' },
  { value: 'btpanel', label: '宝塔面板' },
  { value: '1panel', label: '1Panel' },
  { value: 'acepanel', label: 'AcePanel' },
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
    title="部署凭证"
    subtitle="管理证书部署用的 API 凭证"
    empty-text="暂无部署凭证"
    create-text="新建部署凭证"
    edit-text="编辑部署凭证"
    :provider-types="providerTypes"
    :config-schema="providerConfigSchema"
    icon-color="blue"
    :load-items="loadItems"
    :create-item="createItem"
    :update-item="updateItem"
    :delete-item="deleteItem"
  />
</template>
