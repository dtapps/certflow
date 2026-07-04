package main

import (
	"context"

	"cnb.cool/dtapp/certflow/internal/i18n"
	"cnb.cool/dtapp/certflow/internal/logging"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// ClipboardServiceWrapper 提供剪贴板功能
type ClipboardServiceWrapper struct {
	app *application.App
}

// NewClipboardServiceWrapper 创建剪贴板服务
// https://v3.wails.io/features/clipboard/basics/
func NewClipboardServiceWrapper(app *application.App) *ClipboardServiceWrapper {
	return &ClipboardServiceWrapper{app: app}
}

// SetApp 设置应用实例（在 app 创建后调用）
func (s *ClipboardServiceWrapper) SetApp(app *application.App) {
	s.app = app
}

// SetText 设置剪贴板文本
func (s *ClipboardServiceWrapper) SetText(text string) bool {
	if !s.app.Clipboard.SetText(text) {
		logging.Warn("%s", i18n.T("log.clipboard_set_failed"))
		return false
	}
	return true
}

// ServiceStartup 实现 Wails 服务接口
func (s *ClipboardServiceWrapper) ServiceStartup(ctx context.Context, options application.ServiceOptions) error {
	return nil
}

// ServiceShutdown 实现 Wails 服务接口
func (s *ClipboardServiceWrapper) ServiceShutdown() error {
	return nil
}

// ServiceName 实现 Wails 服务接口
func (s *ClipboardServiceWrapper) ServiceName() string {
	return "ClipboardService"
}
