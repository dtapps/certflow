package main

import (
	"context"

	"cnb.cool/dtapp/certflow/internal/i18n"
	"cnb.cool/dtapp/certflow/internal/logging"
	"cnb.cool/dtapp/certflow/internal/useragent"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// SystemServiceWrapper 提供系统信息（主题等）
type SystemServiceWrapper struct {
	app        *application.App
	mainWindow application.Window
}

// NewSystemServiceWrapper 创建系统服务
func NewSystemServiceWrapper() *SystemServiceWrapper {
	return &SystemServiceWrapper{}
}

// SetApp 设置 app 引用
func (s *SystemServiceWrapper) SetApp(app *application.App) {
	s.app = app
}

// setMainWindow 设置主窗口引用（内部初始化用，不暴露给前端 RPC）
func (s *SystemServiceWrapper) setMainWindow(w application.Window) {
	s.mainWindow = w
}

// IsDarkMode 检测系统是否深色模式
func (s *SystemServiceWrapper) IsDarkMode() bool {
	if s.app == nil {
		return false
	}
	return s.app.Env.IsDarkMode()
}

// GetVersion 获取应用版本号
func (s *SystemServiceWrapper) GetVersion() string {
	return currentVersion
}

// SetUserAgent 设置全局 User-Agent（前端启动时传入 WebView 的 navigator.userAgent）。
// 写入 internal/useragent 全局状态，所有经全局 transport（httplog 包裹）发出的
// HTTP 请求在未显式设置 UA 时自动带上；monitor 检查请求也从该全局状态读取。
func (s *SystemServiceWrapper) SetUserAgent(ua string) {
	logging.Info(i18n.T("log.system.set_user_agent", "UA", ua))
	useragent.Set(ua)
}

// ReportFrontendError 接收前端运行时的错误并写入独立的 frontend.log（logging 包，
// 等价于 slog，不混入 certflow.log）。用于在生产版本中定位前端问题：开发版可通过
// 浏览器控制台查看报错，但发布版 esbuild 会 drop 掉 console，错误被静默吞噬，
// 故由前端统一拦截后上报到后端落盘。
// 参数：
//   - level：错误级别（error/warn/info），直接进对应级别日志。
//   - message：错误描述。
//   - stack：错误堆栈或上下文（可为空）。
//   - url：触发错误的页面/路由（可为空）。
func (s *SystemServiceWrapper) ReportFrontendError(level, message, stack, url string) {
	if level == "" {
		level = "error"
	}
	// 限制单条长度，避免超长堆栈撑爆日志行（保留尾部最有价值的部分）。
	const maxLen = 4000
	if len(stack) > maxLen {
		stack = stack[len(stack)-maxLen:]
	}
	if len(message) > maxLen {
		message = message[:maxLen]
	}
	loc := url
	if loc == "" {
		loc = "frontend"
	}
	fl := logging.Frontend()
	switch level {
	case "warn":
		fl.Warn("[%s] %s | stack: %s", loc, message, stack)
	case "info":
		fl.Info("[%s] %s | stack: %s", loc, message, stack)
	case "debug":
		fl.Debug("[%s] %s | stack: %s", loc, message, stack)
	default:
		fl.Error("[%s] %s | stack: %s", loc, message, stack)
	}
}

// SetWindowAppearance 设置主窗口外观（标题栏主题）
// func (s *SystemServiceWrapper) SetWindowAppearance(isDark bool) {
// 	logging.Debug("%s: isDark=%v", i18n.T("log.set_window_appearance"), isDark)
// 	if s.mainWindow == nil {
// 		logging.Warn("%s", i18n.T("log.main_window_nil"))
// 		return
// 	}
// 	if isDark {
// 		s.mainWindow.SetBackgroundColour(application.RGBA{Red: 26, Green: 27, Blue: 30, Alpha: 255})
// 	} else {
// 		s.mainWindow.SetBackgroundColour(application.RGBA{Red: 245, Green: 245, Blue: 245, Alpha: 255})
// 	}
// }

// ShowMessage 显示原生对话框（前端调用）
func (s *SystemServiceWrapper) ShowMessage(title, message, msgType string) {
	if s.app == nil {
		return
	}
	switch msgType {
	case "warning":
		s.app.Dialog.Warning().SetTitle(title).SetMessage(message).Show()
	case "error":
		s.app.Dialog.Error().SetTitle(title).SetMessage(message).Show()
	default:
		s.app.Dialog.Info().SetTitle(title).SetMessage(message).Show()
	}
}

// ServiceStartup 实现 Wails 服务接口
func (s *SystemServiceWrapper) ServiceStartup(ctx context.Context, options application.ServiceOptions) error {
	return nil
}

// ServiceShutdown 实现 Wails 服务接口
func (s *SystemServiceWrapper) ServiceShutdown() error {
	return nil
}

// ServiceName 实现 Wails 服务接口
func (s *SystemServiceWrapper) ServiceName() string {
	return "SystemService"
}
