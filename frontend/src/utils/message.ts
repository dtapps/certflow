import * as SystemService from '@bindings/cnb.cool/dtapp/certflow/systemservicewrapper'

// eslint-disable-next-line @typescript-eslint/no-explicit-any
let naiveMessage: any = null

// eslint-disable-next-line @typescript-eslint/no-explicit-any
export function initMessage(msg: any) {
  naiveMessage = msg
}

export function showMessage(text: string, type: 'success' | 'info' | 'warning' | 'error' = 'info') {
  try {
    SystemService.ShowMessage('CertFlow', text, type)
    return
  } catch {
    // Wails dialog 不可用时降级到 Naive UI
  }
  naiveMessage?.[type]?.(text)
}
