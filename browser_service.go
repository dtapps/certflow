package main

import (
	"context"

	"cnb.cool/dtapp/certflow/internal/i18n"
	"cnb.cool/dtapp/certflow/internal/logging"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// BrowserServiceWrapper 提供浏览器集成功能
type BrowserServiceWrapper struct {
	app *application.App
}

// NewBrowserServiceWrapper 创建浏览器服务
// https://v3.wails.io/features/browser/integration/
func NewBrowserServiceWrapper(app *application.App) *BrowserServiceWrapper {
	return &BrowserServiceWrapper{app: app}
}

// SetApp 设置应用实例（在 app 创建后调用）
func (s *BrowserServiceWrapper) SetApp(app *application.App) {
	s.app = app
}

// OpenURL 在系统默认浏览器中打开 URL
func (s *BrowserServiceWrapper) OpenURL(url string) error {
	if err := s.app.Browser.OpenURL(url); err != nil {
		logging.Warn("%s", i18n.T("log.open_url_failed", "URL", url, "Error", err))
		return err
	}
	return nil
}

// ServiceStartup 实现 Wails 服务接口
func (s *BrowserServiceWrapper) ServiceStartup(ctx context.Context, options application.ServiceOptions) error {
	return nil
}

// ServiceShutdown 实现 Wails 服务接口
func (s *BrowserServiceWrapper) ServiceShutdown() error {
	return nil
}

// ServiceName 实现 Wails 服务接口
func (s *BrowserServiceWrapper) ServiceName() string {
	return "BrowserService"
}
