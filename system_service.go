package main

import (
	"context"

	"cnb.cool/dtapp/certflow/internal/i18n"
	"cnb.cool/dtapp/certflow/internal/logging"
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

// SetMainWindow 设置主窗口引用
func (s *SystemServiceWrapper) SetMainWindow(w application.Window) {
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

// SetWindowAppearance 设置主窗口外观（标题栏主题）
func (s *SystemServiceWrapper) SetWindowAppearance(isDark bool) {
	logging.Debug("%s: isDark=%v", i18n.T("log.set_window_appearance"), isDark)
	if s.mainWindow == nil {
		logging.Warn("%s", i18n.T("log.main_window_nil"))
		return
	}
	if isDark {
		s.mainWindow.SetBackgroundColour(application.RGBA{Red: 26, Green: 27, Blue: 30, Alpha: 255})
	} else {
		s.mainWindow.SetBackgroundColour(application.RGBA{Red: 245, Green: 245, Blue: 245, Alpha: 255})
	}
}

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
