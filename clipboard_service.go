package main

import (
	"context"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// ClipboardServiceWrapper 提供剪贴板功能
type ClipboardServiceWrapper struct {
	app *application.App
}

// NewClipboardServiceWrapper 创建剪贴板服务
func NewClipboardServiceWrapper(app *application.App) *ClipboardServiceWrapper {
	return &ClipboardServiceWrapper{app: app}
}

// SetApp 设置应用实例（在 app 创建后调用）
func (s *ClipboardServiceWrapper) SetApp(app *application.App) {
	s.app = app
}

// SetText 设置剪贴板文本
func (s *ClipboardServiceWrapper) SetText(text string) bool {
	return s.app.Clipboard.SetText(text)
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
