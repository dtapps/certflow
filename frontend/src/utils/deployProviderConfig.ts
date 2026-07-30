// 部署 Provider 配置（SSL 部署 + 部署凭证用）
export type ConfigField = { key: string; labelKey: string; type: 'text' | 'password' }
export type ProviderTypeOption = { value: string; labelKey: string }

export const deployProviderTypes: ProviderTypeOption[] = [
  { value: 'aliyun', labelKey: 'deploy.provider.aliyun' },
  { value: 'tencentcloud', labelKey: 'deploy.provider.tencentcloud' },
  { value: 'huawei', labelKey: 'deploy.provider.huawei' },
  { value: 'baiducloud', labelKey: 'deploy.provider.baidu' },
  { value: 'ctyun', labelKey: 'deploy.provider.ctyun' },
  { value: 'volcengine', labelKey: 'deploy.provider.volcengine' },
  { value: 'btpanel', labelKey: 'dns.type.btpanel' },
  { value: '1panel', labelKey: 'dns.type.1panel' },
  { value: 'acepanel', labelKey: 'dns.type.acepanel' },
  { value: 'aapanel', labelKey: 'dns.type.aapanel' },
  { value: 'aawaf', labelKey: 'dns.type.aawaf' },
  { value: 'openrestymanager', labelKey: 'dns.type.openrestymanager' },
  { value: 'uuwaf', labelKey: 'dns.type.uuwaf' },
  { value: 'safeline', labelKey: 'dns.type.safeline' },
]

// 部署目标表单用：provider 下拉选项（含已解析的 label）
export function deployProviderOptions(
  t: (key: string) => string,
): { label: string; value: string }[] {
  return deployProviderTypes.map((p) => ({ label: t(p.labelKey), value: p.value }))
}

export const deployProviderConfigSchema: Record<string, ConfigField[]> = {
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
  volcengine: [
    { key: 'access_key_id', labelKey: 'dns.config.access_key_id', type: 'text' },
    { key: 'access_key_secret', labelKey: 'dns.config.access_key_secret', type: 'password' },
  ],
  btpanel: [
    { key: 'panel_url', labelKey: 'deploy.config.panelUrl', type: 'text' },
    { key: 'api_key', labelKey: 'dns.config.api_key', type: 'password' },
  ],
  '1panel': [
    { key: 'panel_url', labelKey: 'deploy.config.panelUrl', type: 'text' },
    { key: 'api_key', labelKey: 'dns.config.api_key', type: 'password' },
  ],
  acepanel: [
    { key: 'panel_url', labelKey: 'deploy.config.panelUrl', type: 'text' },
    { key: 'token_id', labelKey: 'deploy.config.tokenId', type: 'password' },
    { key: 'token_secret', labelKey: 'deploy.config.tokenSecret', type: 'password' },
  ],
  aapanel: [
    { key: 'panel_url', labelKey: 'deploy.config.panelUrl', type: 'text' },
    { key: 'api_key', labelKey: 'dns.config.api_key', type: 'password' },
  ],
  aawaf: [
    { key: 'panel_url', labelKey: 'deploy.config.panelUrl', type: 'text' },
    { key: 'api_key', labelKey: 'dns.config.api_key', type: 'password' },
  ],
  openrestymanager: [
    { key: 'panel_url', labelKey: 'deploy.config.panelUrl', type: 'text' },
    { key: 'jwt_secret', labelKey: 'deploy.config.jwtSecret', type: 'password' },
  ],
  uuwaf: [
    { key: 'panel_url', labelKey: 'deploy.config.panelUrl', type: 'text' },
    { key: 'api_key', labelKey: 'dns.config.api_key', type: 'password' },
  ],
  safeline: [
    { key: 'panel_url', labelKey: 'deploy.config.panelUrl', type: 'text' },
    { key: 'api_key', labelKey: 'dns.config.api_key', type: 'password' },
  ],
}
