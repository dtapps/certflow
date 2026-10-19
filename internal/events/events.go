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
	// EventWindowResized 窗口尺寸变化事件
	EventWindowResized = "window_resized"
	// EventMonitorUpdated 域名监控数据更新事件（后台检查/手动检查完成后发射，前端实时刷新）
	EventMonitorUpdated = "monitor_updated"
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

// WindowResizedPayload 窗口尺寸变化事件参数
type WindowResizedPayload struct {
	Width  int `json:"width"`  // 窗口宽度（像素）
	Height int `json:"height"` // 窗口高度（像素）
}

// MonitorUpdatedPayload 域名监控数据更新事件参数
type MonitorUpdatedPayload struct {
	DomainID  int    `json:"domain_id"`  // 发生更新的域名 ID；0 表示批量更新（前端刷新全部）
	Domain    string `json:"domain"`     // 发生更新的域名（批量更新时为空）
	Status    string `json:"status"`     // 最新状态（批量更新时为空）
	CheckedAt string `json:"checked_at"` // 检查时间（RFC3339 格式）
}
