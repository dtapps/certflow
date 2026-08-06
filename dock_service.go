package main

import (
	"context"

	"cnb.cool/dtapp/certflow/internal/i18n"
	"cnb.cool/dtapp/certflow/internal/logging"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/services/dock"
)

// DockServiceWrapper Dock 服务包装器
type DockServiceWrapper struct {
	app         *application.App
	dockService *dock.DockService
}

// NewDockServiceWrapper 创建 Dock 服务
// https://v3.wails.io/features/platform/dock/
func NewDockServiceWrapper() *DockServiceWrapper {
	return &DockServiceWrapper{
		dockService: dock.New(),
	}
}

// SetApp 设置 app 引用（用于获取应用生命周期）
func (s *DockServiceWrapper) SetApp(app *application.App) {
	s.app = app
}

// HideAppIcon 隐藏 Dock 图标（仅 macOS）
func (s *DockServiceWrapper) HideAppIcon() {
	s.dockService.HideAppIcon()
}

// ShowAppIcon 显示 Dock 图标（仅 macOS）
func (s *DockServiceWrapper) ShowAppIcon() {
	s.dockService.ShowAppIcon()
}

// SetBadge 设置 Dock 徽章
func (s *DockServiceWrapper) SetBadge(label string) error {
	if err := s.dockService.SetBadge(label); err != nil {
		logging.Warn(i18n.T("log.set_badge_failed", "Error", err))
		return err
	}
	return nil
}

// RemoveBadge 移除 Dock 徽章
func (s *DockServiceWrapper) RemoveBadge() error {
	if err := s.dockService.RemoveBadge(); err != nil {
		logging.Warn(i18n.T("log.remove_badge_failed", "Error", err))
		return err
	}
	return nil
}

// GetBadge 获取当前 Dock 徽章
func (s *DockServiceWrapper) GetBadge() *string {
	return s.dockService.GetBadge()
}

// ServiceStartup 实现 Wails 服务接口
func (s *DockServiceWrapper) ServiceStartup(ctx context.Context, options application.ServiceOptions) error {
	return nil
}

// ServiceShutdown 实现 Wails 服务接口
func (s *DockServiceWrapper) ServiceShutdown() error {
	return nil
}

// ServiceName 实现 Wails 服务接口
func (s *DockServiceWrapper) ServiceName() string {
	return "DockService"
}
