package auth

import (
	"context"
	"fmt"

	"cnb.cool/dtapp/certflow/internal/ent"
	"cnb.cool/dtapp/certflow/internal/ent/authmethod"
	"cnb.cool/dtapp/certflow/internal/i18n"
	"cnb.cool/dtapp/certflow/internal/logging"
	"github.com/pquerna/otp/totp"
)

// TOTPSetupResult TOTP 设置结果
type TOTPSetupResult struct {
	Secret      string `json:"secret"`       // Base32 编码的密钥
	URL         string `json:"url"`          // otpauth:// URL
	Issuer      string `json:"issuer"`       // 发行者名称
	AccountName string `json:"account_name"` // 账户名
}

// TOTPInfo TOTP 信息
type TOTPInfo struct {
	IsConfigured bool   `json:"is_configured"` // 是否已配置
	Issuer       string `json:"issuer"`        // 发行者名称
	AccountName  string `json:"account_name"`  // 账户名
	CreatedAt    string `json:"created_at"`    // 创建时间
}

// SetupTOTP 生成 TOTP 密钥和 QR 码 URL
func (s *AuthService) SetupTOTP() (*TOTPSetupResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	ctx := context.Background()

	// 检查是否已经配置了 TOTP
	exists, err := s.db.AuthMethod.Query().
		Where(authmethod.MethodEQ("totp")).
		Exist(ctx)
	if err != nil {
		return nil, fmt.Errorf("%s", i18n.T("error.totp_generate_failed", "Error", err))
	}
	if exists {
		return nil, fmt.Errorf("%s", i18n.T("error.totp_already_configured"))
	}

	// 生成 TOTP 密钥
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      "CertFlow",
		AccountName: "user",
	})
	if err != nil {
		return nil, fmt.Errorf("%s", i18n.T("error.totp_generate_failed", "Error", err))
	}

	// 创建 TOTP 认证方式
	am, err := s.db.AuthMethod.Create().
		SetMethod("totp").
		SetIsActive(false).
		Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("%s", i18n.T("error.totp_setup_failed", "Error", err))
	}

	// 保存 TOTP 凭据
	_, err = s.db.TOTPCredential.Create().
		SetSecret(key.Secret()).
		SetIssuer("CertFlow").
		SetAccountName("user").
		SetAuthMethod(am).
		Save(ctx)
	if err != nil {
		// 回滚认证方式
		_ = s.db.AuthMethod.DeleteOneID(am.ID).Exec(ctx)
		return nil, fmt.Errorf("%s", i18n.T("error.totp_setup_failed", "Error", err))
	}

	logging.Info(i18n.T("log.totp_key_generated"))

	return &TOTPSetupResult{
		Secret:      key.Secret(),
		URL:         key.URL(),
		Issuer:      "CertFlow",
		AccountName: "user",
	}, nil
}

// VerifyTOTPSetup 验证初始设置的 TOTP 码
func (s *AuthService) VerifyTOTPSetup(code string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	ctx := context.Background()

	// 获取 TOTP 凭据
	tc, err := s.db.TOTPCredential.Query().
		WithAuthMethod().
		First(ctx)
	if err != nil {
		return fmt.Errorf("%s", i18n.T("error.totp_setup_failed", "Error", err))
	}

	logging.Info(i18n.T("log.totp_verifying"))

	// 验证 TOTP 码
	valid := totp.Validate(code, tc.Secret)
	if !valid {
		logging.Warn(i18n.T("log.totp_verify_failed"))
		return fmt.Errorf("%s", i18n.T("error.totp_verify_failed"))
	}

	logging.Info(i18n.T("log.totp_verify_success"))

	// 验证通过，激活 TOTP 认证方式
	_, err = s.db.AuthMethod.Update().
		Where(authmethod.IDEQ(tc.AuthMethodID)).
		SetIsActive(true).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("%s", i18n.T("error.totp_setup_failed", "Error", err))
	}

	// 如果之前有其他激活的方式，取消激活
	_, err = s.db.AuthMethod.Update().
		Where(
			authmethod.IsActiveEQ(true),
			authmethod.MethodNEQ("totp"),
		).
		SetIsActive(false).
		Save(ctx)
	if err != nil {
		logging.Warn(i18n.T("log.passkey_deactivate_failed", "Error", err))
	}

	logging.Info(i18n.T("log.totp_activated"))
	return nil
}

// VerifyTOTP 验证 TOTP 码（登录时）
func (s *AuthService) VerifyTOTP(code string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ctx := context.Background()

	// 获取 TOTP 凭据
	tc, err := s.db.TOTPCredential.Query().First(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return false
		}
		logging.Error(i18n.T("log.totp_query_failed", "Error", err))
		return false
	}

	// 验证 TOTP 码
	valid := totp.Validate(code, tc.Secret)
	if !valid {
		logging.Warn(i18n.T("log.totp_verify_failed"))
	}
	return valid
}

// ClearTOTP 清除 TOTP 设置
func (s *AuthService) ClearTOTP() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	ctx := context.Background()

	// 先删除 TOTP 凭据
	_, err := s.db.TOTPCredential.Delete().
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("%s", i18n.T("error.totp_setup_failed", "Error", err))
	}

	// 再删除 TOTP 认证方式
	if _, err := s.db.AuthMethod.Delete().
		Where(authmethod.MethodEQ("totp")).
		Exec(ctx); err != nil {
		return fmt.Errorf("%s", i18n.T("error.totp_setup_failed", "Error", err))
	}
	return s.ensureActiveMethod(ctx)
}

// CancelTOTP 取消未确认的 TOTP 设置，清理残留数据
func (s *AuthService) CancelTOTP() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	ctx := context.Background()

	// 只删除未激活的 TOTP 数据
	_, err := s.db.TOTPCredential.Delete().
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("%s", i18n.T("error.totp_setup_failed", "Error", err))
	}

	_, err = s.db.AuthMethod.Delete().
		Where(authmethod.MethodEQ("totp")).
		Exec(ctx)
	return err
}

// GetTOTPInfo 获取 TOTP 设置信息
func (s *AuthService) GetTOTPInfo() (*TOTPInfo, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ctx := context.Background()

	// 是否已配置只看记录是否存在，与是否激活无关（激活状态由 activeMethod 单独表达）
	am, err := s.db.AuthMethod.Query().
		Where(authmethod.MethodEQ("totp")).
		WithTotpCredentials().
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return &TOTPInfo{IsConfigured: false}, nil
		}
		return nil, fmt.Errorf("%s", i18n.T("error.totp_setup_failed", "Error", err))
	}

	info := &TOTPInfo{
		IsConfigured: true,
		Issuer:       "CertFlow",
		AccountName:  "user",
		CreatedAt:    am.CreatedAt.Format("2006-01-02 15:04:05"),
	}

	return info, nil
}
