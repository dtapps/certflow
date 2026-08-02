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

// GetActiveMethod 获取当前激活的认证方式
func (s *AuthServiceWrapper) GetActiveMethod() string {
	method, err := s.authService.GetActiveMethod()
	if err != nil {
		logging.Error(i18n.T("log.auth_query_active_method_failed", "Error", err))
		return ""
	}
	return method
}

// SetActiveMethod 设置激活的认证方式
func (s *AuthServiceWrapper) SetActiveMethod(method string) error {
	return s.authService.SetActiveMethod(method)
}

// GetAvailableMethods 获取已配置的认证方式列表
func (s *AuthServiceWrapper) GetAvailableMethods() []string {
	methods, err := s.authService.GetAvailableMethods()
	if err != nil {
		logging.Error(i18n.T("log.auth_query_methods_failed", "Error", err))
		return nil
	}
	return methods
}

// SetupTOTP 生成 TOTP 密钥
func (s *AuthServiceWrapper) SetupTOTP() (*auth.TOTPSetupResult, error) {
	return s.authService.SetupTOTP()
}

// VerifyTOTPSetup 验证 TOTP 设置码
func (s *AuthServiceWrapper) VerifyTOTPSetup(code string) error {
	return s.authService.VerifyTOTPSetup(code)
}

// VerifyTOTP 验证 TOTP 码
func (s *AuthServiceWrapper) VerifyTOTP(code string) bool {
	return s.authService.VerifyTOTP(code)
}

// ClearTOTP 清除 TOTP 设置
func (s *AuthServiceWrapper) ClearTOTP() error {
	return s.authService.ClearTOTP()
}

// CancelTOTP 取消未确认的 TOTP 设置
func (s *AuthServiceWrapper) CancelTOTP() error {
	return s.authService.CancelTOTP()
}

// GetTOTPInfo 获取 TOTP 信息
func (s *AuthServiceWrapper) GetTOTPInfo() (*auth.TOTPInfo, error) {
	return s.authService.GetTOTPInfo()
}

// StartPasskeyRegistration 开始 Passkey 注册
func (s *AuthServiceWrapper) StartPasskeyRegistration() (*auth.PasskeyRegistrationResponse, error) {
	return s.authService.StartPasskeyRegistration()
}

// FinishPasskeyRegistration 完成 Passkey 注册
func (s *AuthServiceWrapper) FinishPasskeyRegistration(data string) error {
	return s.authService.FinishPasskeyRegistration(data)
}

// StartPasskeyLogin 开始 Passkey 登录
func (s *AuthServiceWrapper) StartPasskeyLogin() (*auth.PasskeyAuthenticationResponse, error) {
	return s.authService.StartPasskeyLogin()
}

// FinishPasskeyLogin 完成 Passkey 登录
func (s *AuthServiceWrapper) FinishPasskeyLogin(data string) bool {
	result, err := s.authService.FinishPasskeyLogin(data)
	if err != nil {
		logging.Error(i18n.T("error.passkey_verification_failed", "Error", err))
		return false
	}
	return result
}

// ClearPasskey 清除 Passkey 设置
func (s *AuthServiceWrapper) ClearPasskey() error {
	return s.authService.ClearPasskey()
}

// GetPasskeyInfo 获取 Passkey 信息
func (s *AuthServiceWrapper) GetPasskeyInfo() (*auth.PasskeyInfo, error) {
	return s.authService.GetPasskeyInfo()
}

// Authenticate 统一验证方法
func (s *AuthServiceWrapper) Authenticate(method, credential string) bool {
	result, err := s.authService.Authenticate(method, credential)
	if err != nil {
		logging.Error(i18n.T("error.auth_method_invalid", "Method", method))
		return false
	}
	return result
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
