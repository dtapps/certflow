package events

// 应用事件名称常量，防止拼写错误
const (
	// EventTime 时间同步事件
	EventTime = "time"
	// EventNotification 通知事件
	EventNotification = "notification"
	// EventAuthVerified 认证验证完成事件
	EventAuthVerified = "auth_verified"
	// EventThemeChanged 系统主题变化事件
	EventThemeChanged = "theme_changed"
	// EventLocaleChanged 语言设置变化事件
	EventLocaleChanged = "locale_changed"
	// EventNavigate 导航事件
	EventNavigate = "navigate"
)

// ---- 事件 Payload 结构体 ----

// TimePayload 时间同步事件参数
type TimePayload struct {
	Time string `json:"time"`
}

// NotificationPayload 通知事件参数
type NotificationPayload struct {
	Title    string `json:"title"`
	Subtitle string `json:"subtitle,omitempty"`
	Body     string `json:"body,omitempty"`
	Category string `json:"category"`
}

// ThemeChangedPayload 主题变化事件参数
type ThemeChangedPayload struct {
	Dark bool `json:"dark"`
}

// LocaleChangedPayload 语言变化事件参数
type LocaleChangedPayload struct {
	Locale string `json:"locale"`
}

// NavigatePayload 导航事件参数
type NavigatePayload struct {
	Path string `json:"path"`
}
