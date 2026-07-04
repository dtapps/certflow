package main

import (
	"context"

	"cnb.cool/dtapp/certflow/internal/auth"
	"cnb.cool/dtapp/certflow/internal/i18n"
	"cnb.cool/dtapp/certflow/internal/logging"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// AuthServiceWrapper 包装 auth.Service 以适配 Wails v3 服务接口
type AuthServiceWrapper struct {
	authService *auth.AuthService
}

// NewAuthServiceWrapper 创建新的认证服务包装器
func NewAuthServiceWrapper(authService *auth.AuthService) *AuthServiceWrapper {
	return &AuthServiceWrapper{authService: authService}
}

// IsPasswordSet 检查是否已设置密码
func (s *AuthServiceWrapper) IsPasswordSet() bool {
	return s.authService.IsPasswordSet()
}

// SetPassword 设置密码
func (s *AuthServiceWrapper) SetPassword(password string) error {
	if err := s.authService.SetPassword(password); err != nil {
		logging.Error("%s: %v", i18n.T("log.auth_set_password_failed"), err)
		return err
	}
	return nil
}

// VerifyPassword 验证密码
func (s *AuthServiceWrapper) VerifyPassword(password string) bool {
	return s.authService.VerifyPassword(password)
}

// ChangePassword 修改密码
func (s *AuthServiceWrapper) ChangePassword(oldPassword, newPassword string) error {
	if err := s.authService.ChangePassword(oldPassword, newPassword); err != nil {
		logging.Error("%s: %v", i18n.T("log.auth_change_password_failed"), err)
		return err
	}
	return nil
}

// ClearPassword 清除密码
func (s *AuthServiceWrapper) ClearPassword() error {
	if err := s.authService.ClearPassword(); err != nil {
		logging.Error("%s: %v", i18n.T("log.auth_clear_password_failed"), err)
		return err
	}
	return nil
}

// ServiceStartup 实现 Wails 服务接口
func (s *AuthServiceWrapper) ServiceStartup(ctx context.Context, options application.ServiceOptions) error {
	return nil
}

// ServiceShutdown 实现 Wails 服务接口
func (s *AuthServiceWrapper) ServiceShutdown() error {
	return nil
}

// ServiceName 实现 Wails 服务接口
func (s *AuthServiceWrapper) ServiceName() string {
	return "AuthService"
}
