import { useI18nStore } from '../stores/i18n'

// 面板/防火墙类部署目标
const panelProviders = ['btpanel', 'aapanel', '1panel', 'acepanel', 'aawaf']
export const isPanelProvider = (p: string) => panelProviders.includes(p)

// 部署服务选项（按厂商）
export function servicesByProvider(providerType: string): { label: string; value: string }[] {
  const { t } = useI18nStore()
  switch (providerType) {
    case 'aliyun':
      return [
        { label: t('deploy.service.cdn'), value: 'cdn' },
        { label: t('deploy.service.dcdn'), value: 'dcdn' },
        { label: t('deploy.service.esa'), value: 'esa' },
        { label: t('deploy.service.ga'), value: 'ga' },
      ]
    case 'tencentcloud':
      return [
        { label: t('deploy.service.cdn'), value: 'cdn' },
        { label: t('deploy.service.edgeone'), value: 'edgeone' },
        { label: t('deploy.service.ecdn'), value: 'ecdn' },
      ]
    case 'huawei':
      return [
        { label: t('deploy.service.cdn'), value: 'cdn' },
        { label: t('deploy.service.waf'), value: 'waf' },
        { label: t('deploy.service.elb'), value: 'elb' },
      ]
    case 'baiducloud':
      return [
        { label: t('deploy.service.cdn'), value: 'cdn' },
        { label: t('deploy.service.drcdn'), value: 'drcdn' },
      ]
    case 'ctyun':
      return [
        { label: t('deploy.service.ctcdn'), value: 'ctcdn' },
        { label: t('deploy.service.icdn'), value: 'icdn' },
        { label: t('deploy.service.accessone'), value: 'accessone' },
      ]
    case 'volcengine':
      return [
        { label: t('deploy.service.cdn'), value: 'cdn' },
        { label: t('deploy.service.dcdn'), value: 'dcdn' },
      ]
    case 'btpanel':
    case 'aapanel':
    case '1panel':
    case 'acepanel':
    case 'aawaf':
      return [{ label: t('deploy.service.site'), value: 'site' }]
    default:
      return [{ label: t('deploy.service.cdn'), value: 'cdn' }]
  }
}

// 服务标签：面板/防火墙类直接返回 provider 标签，其他按 deploy_service 查
export function serviceLabel(deployService?: string, providerType?: string): string {
  const { t } = useI18nStore()
  if (providerType && isPanelProvider(providerType)) {
    return t('deploy.service.site')
  }
  // 非面板类按 deploy_service 查
  const allServiceOptions = [
    { label: t('deploy.service.cdn'), value: 'cdn' },
    { label: t('deploy.service.dcdn'), value: 'dcdn' },
    { label: t('deploy.service.edgeone'), value: 'edgeone' },
    { label: t('deploy.service.esa'), value: 'esa' },
    { label: t('deploy.service.slb'), value: 'slb' },
    { label: t('deploy.service.waf'), value: 'waf' },
    { label: t('deploy.service.elb'), value: 'elb' },
    { label: t('deploy.service.scm'), value: 'scm' },
    { label: t('deploy.service.ga'), value: 'ga' },
    { label: t('deploy.service.drcdn'), value: 'drcdn' },
    { label: t('deploy.service.ecdn'), value: 'ecdn' },
    { label: t('deploy.service.ctcdn'), value: 'ctcdn' },
    { label: t('deploy.service.icdn'), value: 'icdn' },
    { label: t('deploy.service.accessone'), value: 'accessone' },
    { label: t('deploy.service.site'), value: 'site' },
  ]
  return allServiceOptions.find((o) => o.value === deployService)?.label || deployService || ''
}

// provider 标签
export function providerLabel(providerType?: string): string {
  const { t } = useI18nStore()
  const providerOptions = [
    { label: t('deploy.provider.aliyun'), value: 'aliyun' },
    { label: t('deploy.provider.tencentcloud'), value: 'tencentcloud' },
    { label: t('deploy.provider.huawei'), value: 'huawei' },
    { label: t('deploy.provider.baidu'), value: 'baiducloud' },
    { label: t('deploy.provider.ctyun'), value: 'ctyun' },
    { label: t('deploy.provider.volcengine'), value: 'volcengine' },
    { label: t('deploy.provider.btpanel'), value: 'btpanel' },
    { label: t('deploy.provider.aapanel'), value: 'aapanel' },
    { label: t('deploy.provider.1panel'), value: '1panel' },
    { label: t('deploy.provider.acepanel'), value: 'acepanel' },
    { label: t('deploy.provider.aawaf'), value: 'aawaf' },
  ]
  return providerOptions.find((o) => o.value === providerType)?.label || providerType || ''
}
