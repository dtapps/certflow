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
	Time string `json:"time"` // 当前时间（ISO 格式）
}

// NotificationPayload 通知事件参数
type NotificationPayload struct {
	Title    string `json:"title"`              // 通知标题
	Subtitle string `json:"subtitle,omitempty"` // 通知副标题（可选）
	Body     string `json:"body,omitempty"`     // 通知正文（可选）
	Category string `json:"category"`           // 通知业务分类
	Level    string `json:"level"`              // 通知状态（success/error/warning/info）
}

// ThemeChangedPayload 主题变化事件参数
type ThemeChangedPayload struct {
	Dark bool `json:"dark"` // 是否为深色主题
}

// LocaleChangedPayload 语言变化事件参数
type LocaleChangedPayload struct {
	Locale string `json:"locale"` // 当前语言（zh-CN/en-US/auto）
}

// NavigatePayload 导航事件参数
type NavigatePayload struct {
	Path string `json:"path"` // 目标路由路径
}
