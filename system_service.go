package main

import (
	"context"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// SystemServiceWrapper 提供系统信息（主题等）
type SystemServiceWrapper struct {
	app *application.App
}

// NewSystemServiceWrapper 创建系统服务
func NewSystemServiceWrapper() *SystemServiceWrapper {
	return &SystemServiceWrapper{}
}

// SetApp 设置 app 引用
func (s *SystemServiceWrapper) SetApp(app *application.App) {
	s.app = app
}

// IsDarkMode 检测系统是否深色模式
func (s *SystemServiceWrapper) IsDarkMode() bool {
	if s.app == nil {
		return false
	}
	return s.app.Env.IsDarkMode()
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
