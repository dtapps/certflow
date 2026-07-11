// 各云厂商公开的常用区域（Region）。region 是固定可枚举的，无需用户手敲，
// 做成下拉预置；同时保留自定义输入（n-select tag）以兼容少见 region。
// 区域数据的 label 存放 i18n key（实际显示名在 regionOptions 中按当前语言解析），
// value 为云厂商 API 实际使用的 region 代码。

// 用 type 别名（而非 interface）：naive-ui 的 n-select option 是带索引签名的联合类型，
// interface 无法赋值过去，type 别名对象类型可以。
export type RegionOption = {
  label: string
  value: string
}

import { useI18nStore } from '../stores/i18n'

// 各厂商通用区域列表
// label 存放 i18n key，实际显示名由 regionOptions 按当前语言解析。
export const REGIONS: Record<string, RegionOption[]> = {
  aliyun: [
    { value: 'cn-hangzhou', label: 'region.aliyun.cn_hangzhou' },
    { value: 'cn-shanghai', label: 'region.aliyun.cn_shanghai' },
    { value: 'cn-beijing', label: 'region.aliyun.cn_beijing' },
    { value: 'cn-shenzhen', label: 'region.aliyun.cn_shenzhen' },
    { value: 'cn-hongkong', label: 'region.aliyun.cn_hongkong' },
    { value: 'ap-southeast-1', label: 'region.aliyun.ap_southeast_1' },
  ],
  tencentcloud: [
    { value: 'ap-guangzhou', label: 'region.tencentcloud.ap_guangzhou' },
    { value: 'ap-shanghai', label: 'region.tencentcloud.ap_shanghai' },
    { value: 'ap-beijing', label: 'region.tencentcloud.ap_beijing' },
    { value: 'ap-nanjing', label: 'region.tencentcloud.ap_nanjing' },
    { value: 'ap-chengdu', label: 'region.tencentcloud.ap_chengdu' },
    { value: 'ap-hongkong', label: 'region.tencentcloud.ap_hongkong' },
    { value: 'ap-singapore', label: 'region.tencentcloud.ap_singapore' },
  ],
  huawei: [
    { value: 'cn-north-4', label: 'region.huawei.cn_north_4' },
    { value: 'cn-north-1', label: 'region.huawei.cn_north_1' },
    { value: 'cn-east-3', label: 'region.huawei.cn_east_3' },
    { value: 'cn-east-2', label: 'region.huawei.cn_east_2' },
    { value: 'cn-south-1', label: 'region.huawei.cn_south_1' },
    { value: 'ap-southeast-1', label: 'region.huawei.ap_southeast_1' },
  ],
}

// 阿里云 ESA 为区域化服务，SDK 仅内置 cn-hangzhou / ap-southeast-1 两个 endpoint，
// 选其他 region 会连不上；故 ESA 单独限定这两个，避免用户选错。
export const ESA_REGIONS: RegionOption[] = [
  { value: 'cn-hangzhou', label: 'region.aliyun.cn_hangzhou' },
  { value: 'ap-southeast-1', label: 'region.aliyun.ap_southeast_1' },
]

// regionOptions 按厂商 + 服务返回下拉候选区域，label 按当前语言解析为本地化显示名。
export function regionOptions(provider: string, service: string): RegionOption[] {
  const raw = provider === 'aliyun' && service === 'esa' ? ESA_REGIONS : REGIONS[provider] || []
  const t = useI18nStore().t
  return raw.map((o) => ({ value: o.value, label: t(o.label) }))
}

// defaultRegionFor 返回某厂商 + 服务的默认区域（列表首项；ESA 默认 cn-hangzhou）。
export function defaultRegionFor(provider: string, service: string): string {
  if (provider === 'aliyun' && service === 'esa') return 'cn-hangzhou'
  const list = REGIONS[provider]
  return list && list.length ? list[0].value : ''
}

// regionLabel 返回区域的本地化中文名；找不到时回退为原始代码。
export function regionLabel(provider: string, service: string, code: string): string {
  if (!code) return ''
  const opt = regionOptions(provider, service).find((o) => o.value === code)
  return opt ? opt.label : code
}

// regionOf 从部署目标 config 中提取区域代码（阿里云存 region_id，其余存 region）。
export function regionOf(config: Record<string, any> | null | undefined): string {
  if (!config) return ''
  return config.region || config.region_id || ''
}
