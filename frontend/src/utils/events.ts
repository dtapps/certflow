// 应用事件名称常量，防止拼写错误
export const EventTime = 'time'
export const EventNotification = 'notification'
export const EventAuthVerified = 'auth_verified'
export const EventThemeChanged = 'theme_changed'
export const EventLocaleChanged = 'locale_changed'
export const EventNavigate = 'navigate'
export const EventWindowResized = 'window_resized'

// 事件 Payload 类型（由 Wails 绑定生成器从 Go 结构体自动生成）
export type {
  TimePayload,
  NotificationPayload,
  ThemeChangedPayload,
  LocaleChangedPayload,
  NavigatePayload,
  WindowResizedPayload,
} from '../../bindings/cnb.cool/dtapp/certflow/internal/events/models'
