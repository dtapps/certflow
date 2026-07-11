import * as SystemService from '@bindings/cnb.cool/dtapp/certflow/systemservicewrapper'
import { useI18nStore } from '../stores/i18n'

// eslint-disable-next-line @typescript-eslint/no-explicit-any
let naiveMessage: any = null

// eslint-disable-next-line @typescript-eslint/no-explicit-any
export function initMessage(msg: any) {
  naiveMessage = msg
}

// SEP 为后端 i18n.Error 信封的分隔符（不可见控制字符 U+001F），与后端 internal/i18n/error.go 保持一致。
const SEP = String.fromCharCode(0x1f)

// MAX_MSG_LEN 限制传给原生对话框的文案长度。
// 原生（Wails）对话框按内容自适应高度且不滚动，过长会把底部“确定/关闭”按钮挤出可视区，
// 故超过该长度时截断并提示去日志查看完整信息。
const MAX_MSG_LEN = 500

function fitMessage(text: string): string {
  if (text.length <= MAX_MSG_LEN) return text
  return text.slice(0, MAX_MSG_LEN) + '…\n\n' + useI18nStore().t('message.error_truncated')
}

// translateBackend 解析后端错误信封（key + 参数），由前端按自身语言重新翻译，实现与后端解耦。
// 信封格式：<已翻译文本> + SEP + <i18n key> + SEP + <参数JSON>。
// 若不含信封或前端缺少对应 key，则回退展示后端已翻译文本（后端语言已随界面语言同步）。
export function translateBackend(text: string | undefined): string {
  if (!text) return ''
  const idx = text.indexOf(SEP)
  if (idx < 0) return text
  const translated = text.slice(0, idx)
  const rest = text.slice(idx + 1)
  const idx2 = rest.indexOf(SEP)
  if (idx2 < 0) return translated
  const key = rest.slice(0, idx2)
  const payload = rest.slice(idx2 + 1)
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const params: Record<string, any> = (() => {
    try {
      return JSON.parse(payload)
    } catch {
      return {}
    }
  })()
  const localized = useI18nStore().t(key, params)
  // t 在缺 key 时返回 key 本身，此时回退到后端翻译文本
  return localized === key ? translated : localized
}

export function showMessage(text: string, type: 'success' | 'info' | 'warning' | 'error' = 'info') {
  // 防御：若后端错误携带 i18n 信封（含不可见分隔符），仅展示信封前的已翻译文本，避免乱码。
  const clean = text.includes(SEP) ? text.split(SEP)[0] : text
  const display = fitMessage(clean)
  try {
    SystemService.ShowMessage('CertFlow', display, type)
    return
  } catch {
    // Wails dialog 不可用时降级到 Naive UI
  }
  naiveMessage?.[type]?.(display)
}
