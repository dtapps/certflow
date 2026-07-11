package auth

import (
	"context"
	"fmt"
	"sync"

	"cnb.cool/dtapp/certflow/ent/authmethod"
	"cnb.cool/dtapp/certflow/internal/biometric"
	"cnb.cool/dtapp/certflow/internal/i18n"
	"cnb.cool/dtapp/certflow/internal/logging"
)

// BiometricInfo 生物识别信息
type BiometricInfo struct {
	IsConfigured bool   `json:"is_configured"` // 是否已配置
	Supported    bool   `json:"supported"`     // 设备是否支持
	Version      string `json:"version"`       // Helper 版本
}

// biometricHelper 全局 Helper 实例
var (
	biometricHelper     *biometric.Helper
	biometricHelperOnce sync.Once
	biometricHelperErr  error
)

// getBiometricHelper 获取 Helper 单例
func getBiometricHelper() (*biometric.Helper, error) {
	biometricHelperOnce.Do(func() {
		biometricHelper, biometricHelperErr = biometric.NewHelper()
	})
	return biometricHelper, biometricHelperErr
}

// CheckBiometricSupport 检查设备是否支持生物识别
func (s *AuthService) CheckBiometricSupport() bool {
	helper, err := getBiometricHelper()
	if err != nil {
		logging.Error(i18n.T("log.biometric_helper_failed", "Error", err))
		return false
	}
	return helper.IsSupported()
}

// SetupBiometric 设置生物识别认证
func (s *AuthService) SetupBiometric() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	ctx := context.Background()

	// 检查是否已配置
	exists, err := s.db.AuthMethod.Query().
		Where(authmethod.MethodEQ("biometric")).
		Exist(ctx)
	if err != nil {
		return fmt.Errorf("%s", i18n.T("error.biometric_helper_failed", "Error", err))
	}
	if exists {
		return fmt.Errorf("%s", i18n.T("error.biometric_already_configured"))
	}

	// 检查设备支持
	helper, err := getBiometricHelper()
	if err != nil {
		return fmt.Errorf("%s", i18n.T("error.biometric_helper_failed", "Error", err))
	}
	if !helper.IsSupported() {
		return fmt.Errorf("%s", i18n.T("error.biometric_not_supported"))
	}

	// 触发一次验证确认可用
	authCtx := context.Background()
	success, err := helper.Authenticate(authCtx, i18n.T("personal.setupBiometric"))
	if err != nil {
		return fmt.Errorf("%s", i18n.T("error.biometric_verification_failed", "Error", err))
	}
	if !success {
		return fmt.Errorf("%s", i18n.T("error.biometric_user_cancelled"))
	}

	// 创建认证方式
	_, err = s.db.AuthMethod.Create().
		SetMethod("biometric").
		SetIsActive(true).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("%s", i18n.T("error.biometric_setup_failed", "Error", err))
	}

	// 取消其他激活方式
	_, err = s.db.AuthMethod.Update().
		Where(
			authmethod.IsActiveEQ(true),
			authmethod.MethodNEQ("biometric"),
		).
		SetIsActive(false).
		Save(ctx)
	if err != nil {
		logging.Warn(i18n.T("log.biometric_deactivate_failed", "Error", err))
	}

	logging.Info(i18n.T("log.biometric_activated"))
	return nil
}

// VerifyBiometric 验证生物识别
func (s *AuthService) VerifyBiometric(reason string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	helper, err := getBiometricHelper()
	if err != nil {
		logging.Error(i18n.T("log.biometric_helper_failed", "Error", err))
		return false
	}

	ctx := context.Background()
	success, err := helper.Authenticate(ctx, reason)
	if err != nil {
		logging.Error(i18n.T("log.biometric_verify_failed", "Error", err))
		return false
	}

	if !success {
		logging.Warn(i18n.T("log.biometric_verify_cancelled"))
	}

	return success
}

// ClearBiometric 清除生物识别设置
func (s *AuthService) ClearBiometric() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	ctx := context.Background()

	// 删除生物识别认证方式
	_, err := s.db.AuthMethod.Delete().
		Where(authmethod.MethodEQ("biometric")).
		Exec(ctx)
	return err
}

// GetBiometricInfo 获取生物识别信息
func (s *AuthService) GetBiometricInfo() (*BiometricInfo, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ctx := context.Background()

	// 检查是否已配置
	configured, err := s.db.AuthMethod.Query().
		Where(authmethod.MethodEQ("biometric")).
		Exist(ctx)
	if err != nil {
		return nil, fmt.Errorf("%s", i18n.T("error.biometric_helper_failed", "Error", err))
	}

	// 检查设备支持
	supported := false
	version := ""
	helper, err := getBiometricHelper()
	if err == nil {
		supported = helper.IsSupported()
		version, _ = helper.GetVersion()
	}

	return &BiometricInfo{
		IsConfigured: configured,
		Supported:    supported,
		Version:      version,
	}, nil
}

// Authenticate 统一验证方法中的生物识别处理
func (s *AuthService) authenticateBiometric(reason string) bool {
	return s.VerifyBiometric(reason)
}
