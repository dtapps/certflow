import { describe, it, expect, beforeEach } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import {
  REGIONS,
  ESA_REGIONS,
  regionOptions,
  defaultRegionFor,
  regionLabel,
  regionOf,
} from '../utils/region'

beforeEach(() => {
  setActivePinia(createPinia())
})

describe('REGIONS', () => {
  it('包含三大厂商且首项符合预期', () => {
    expect(Object.keys(REGIONS).sort()).toEqual(['aliyun', 'huawei', 'tencentcloud'])
    expect(REGIONS.aliyun[0].value).toBe('cn-hangzhou')
    expect(REGIONS.tencentcloud).toHaveLength(7)
    expect(REGIONS.huawei).toHaveLength(6)
  })

  it('ESA 仅限两个 endpoint', () => {
    expect(ESA_REGIONS.map((o) => o.value)).toEqual(['cn-hangzhou', 'ap-southeast-1'])
  })
})

describe('defaultRegionFor', () => {
  it('ESA 默认 cn-hangzhou', () => {
    expect(defaultRegionFor('aliyun', 'esa')).toBe('cn-hangzhou')
  })

  it('非 ESA 服务取厂商列表首项', () => {
    expect(defaultRegionFor('aliyun', 'cdn')).toBe('cn-hangzhou')
    expect(defaultRegionFor('tencentcloud', 'cdn')).toBe('ap-guangzhou')
    expect(defaultRegionFor('huawei', 'cdn')).toBe('cn-north-4')
  })

  it('未知厂商返回空串', () => {
    expect(defaultRegionFor('unknown', 'cdn')).toBe('')
  })
})

describe('regionOptions', () => {
  it('ESA 只返回两个候选', () => {
    const opts = regionOptions('aliyun', 'esa')
    expect(opts).toHaveLength(2)
    expect(opts[0]).toEqual({ value: 'cn-hangzhou', label: '华东1（杭州）' })
  })

  it('label 按当前语言（默认中文）解析', () => {
    const opts = regionOptions('tencentcloud', 'cdn')
    expect(opts).toHaveLength(7)
    expect(opts[0].value).toBe('ap-guangzhou')
    expect(opts[0].label).not.toBe(opts[0].value) // 已被 i18n 解析为显示名
  })

  it('未知厂商返回空列表', () => {
    expect(regionOptions('unknown', 'cdn')).toEqual([])
  })
})

describe('regionLabel', () => {
  it('返回本地化显示名', () => {
    expect(regionLabel('aliyun', 'esa', 'cn-hangzhou')).toBe('华东1（杭州）')
  })

  it('找不到时回退为原始代码', () => {
    expect(regionLabel('aliyun', 'esa', 'ap-northeast-1')).toBe('ap-northeast-1')
  })

  it('空代码返回空串', () => {
    expect(regionLabel('aliyun', 'esa', '')).toBe('')
  })
})

describe('regionOf', () => {
  it('从 config 提取区域代码（region 优先，其次 region_id）', () => {
    expect(regionOf(null)).toBe('')
    expect(regionOf(undefined)).toBe('')
    expect(regionOf({})).toBe('')
    expect(regionOf({ region: 'ap-guangzhou' })).toBe('ap-guangzhou')
    expect(regionOf({ region_id: 'cn-north-4' })).toBe('cn-north-4')
  })
})
