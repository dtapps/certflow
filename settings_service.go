package main

import (
	"context"

	"cnb.cool/dtapp/certflow/internal/i18n"
	"cnb.cool/dtapp/certflow/internal/logging"
	"cnb.cool/dtapp/certflow/internal/settings"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// SettingsServiceWrapper 包装 settings.Service 以适配 Wails v3 服务接口
type SettingsServiceWrapper struct {
	settingsService *settings.Service
}

// NewSettingsServiceWrapper 创建新的设置服务包装器
func NewSettingsServiceWrapper(settingsService *settings.Service) *SettingsServiceWrapper {
	return &SettingsServiceWrapper{settingsService: settingsService}
}

// GetSettings 获取当前设置
func (s *SettingsServiceWrapper) GetSettings() settings.Settings {
	return s.settingsService.Get()
}

// SaveSettings 保存设置
func (s *SettingsServiceWrapper) SaveSettings(input settings.Settings) error {
	// 应用日志级别变更
	if input.Log.Level != "" {
		logging.Global().SetLevel(logging.ParseLevel(input.Log.Level))
	}
	// 同步语言设置到后端 i18n
	i18n.SetLocale(input.Language)
	if err := s.settingsService.Save(input); err != nil {
		logging.Error(i18n.T("log.settings_save_failed", "Error", err))
		return err
	}
	return nil
}

// ServiceStartup 实现 Wails 服务接口
func (s *SettingsServiceWrapper) ServiceStartup(ctx context.Context, options application.ServiceOptions) error {
	return nil
}

// ServiceShutdown 实现 Wails 服务接口
func (s *SettingsServiceWrapper) ServiceShutdown() error {
	return nil
}

// ServiceName 实现 Wails 服务接口
func (s *SettingsServiceWrapper) ServiceName() string {
	return "SettingsService"
}
