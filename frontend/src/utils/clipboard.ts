import * as ClipboardService from '@bindings/cnb.cool/dtapp/certflow/clipboardservicewrapper'
import { useI18nStore } from '../stores/i18n'

/**
 * 复制文本到剪贴板。
 * 优先使用 Wails 原生剪贴板（桌面端），失败时回退到浏览器 navigator.clipboard（Web 环境）。
 * @returns 是否复制成功
 */
export async function copyToClipboard(text: string): Promise<boolean> {
  if (text === '' || text == null) return false
  const { t } = useI18nStore()
  // 优先走 Wails 原生剪贴板
  try {
    const ok = await ClipboardService.SetText(text)
    if (ok) return true
  } catch (e) {
    console.warn(t('clipboard.wailsFallback'), e)
  }
  // 兜底：浏览器原生剪贴板（仅 Web 环境可用）
  try {
    await navigator.clipboard.writeText(text)
    return true
  } catch (e) {
    console.error(t('clipboard.copyFailed'), e)
    return false
  }
}
