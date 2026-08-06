package main

import (
	"context"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// AutostartServiceWrapper 自动启动服务包装器
type AutostartServiceWrapper struct {
	app *application.App
}

// NewAutostartServiceWrapper 创建自动启动服务
// https://v3.wails.io/features/autostart/basics/
func NewAutostartServiceWrapper() *AutostartServiceWrapper {
	return &AutostartServiceWrapper{}
}

// SetApp 设置 app 引用（用于获取应用生命周期）
func (s *AutostartServiceWrapper) SetApp(app *application.App) {
	s.app = app
}

// Enable 启用开机自启
func (s *AutostartServiceWrapper) Enable() error {
	if s.app == nil {
		return nil
	}
	return s.app.Autostart.Enable()
}

// Disable 禁用开机自启
func (s *AutostartServiceWrapper) Disable() error {
	if s.app == nil {
		return nil
	}
	return s.app.Autostart.Disable()
}

// IsEnabled 检查是否启用开机自启
func (s *AutostartServiceWrapper) IsEnabled() bool {
	if s.app == nil {
		return false
	}
	enabled, _ := s.app.Autostart.IsEnabled()
	return enabled
}

// ServiceStartup 实现 Wails 服务接口
func (s *AutostartServiceWrapper) ServiceStartup(ctx context.Context, options application.ServiceOptions) error {
	return nil
}

// ServiceShutdown 实现 Wails 服务接口
func (s *AutostartServiceWrapper) ServiceShutdown() error {
	return nil
}

// ServiceName 实现 Wails 服务接口
func (s *AutostartServiceWrapper) ServiceName() string {
	return "AutostartService"
}
