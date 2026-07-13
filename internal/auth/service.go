package auth

import (
	"context"
	"fmt"
	"sync"

	"cnb.cool/dtapp/certflow/ent"
	"cnb.cool/dtapp/certflow/ent/authmethod"
	"cnb.cool/dtapp/certflow/internal/i18n"
	"cnb.cool/dtapp/certflow/internal/logging"
	"golang.org/x/crypto/bcrypt"
)

// AuthService 认证服务
type AuthService struct {
	mu sync.RWMutex
	db *ent.Client
}

// NewAuthService 创建新的认证服务
func NewAuthService(db *ent.Client) *AuthService {
	return &AuthService{db: db}
}

// IsPasswordSet 检查是否已设置密码
func (s *AuthService) IsPasswordSet() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ctx := context.Background()
	exists, err := s.db.AuthMethod.Query().
		Where(authmethod.MethodEQ("password")).
		Exist(ctx)
	if err != nil {
		logging.Error(i18n.T("log.auth_query_password_failed", "Error", err))
		return false
	}
	return exists
}

// SetPassword 设置密码（明文传入，内部哈希存储）
func (s *AuthService) SetPassword(plainPassword string) error {
	if len(plainPassword) < 6 {
		return fmt.Errorf("%s", i18n.T("error.password_too_short"))
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(plainPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("%s", i18n.T("error.password_hash_failed", "Error", err))
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	ctx := context.Background()

	// 检查是否已存在密码认证方式
	exists, err := s.db.AuthMethod.Query().
		Where(authmethod.MethodEQ("password")).
		Exist(ctx)
	if err != nil {
		return fmt.Errorf("%s", i18n.T("error.load_auth_failed"))
	}

	if exists {
		// 更新现有的密码
		_, err = s.db.AuthMethod.Update().
			Where(authmethod.MethodEQ("password")).
			SetPasswordHash(string(hash)).
			Save(ctx)
	} else {
		// 创建新的密码认证方式
		_, err = s.db.AuthMethod.Create().
			SetMethod("password").
			SetIsActive(true).
			SetPasswordHash(string(hash)).
			Save(ctx)
	}

	return err
}

// VerifyPassword 验证密码
func (s *AuthService) VerifyPassword(plainPassword string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ctx := context.Background()

	// 获取密码认证方式
	am, err := s.db.AuthMethod.Query().
		Where(authmethod.MethodEQ("password")).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			logging.Debug(i18n.T("log.auth_no_password_set"))
			return true // 未设置密码时，验证通过
		}
		logging.Error(i18n.T("log.auth_query_password_failed", "Error", err))
		return false
	}

	if am.PasswordHash == "" {
		logging.Debug(i18n.T("log.auth_no_password_set"))
		return true // 未设置密码时，验证通过
	}

	result := bcrypt.CompareHashAndPassword([]byte(am.PasswordHash), []byte(plainPassword)) == nil
	if !result {
		logging.Warn(i18n.T("log.auth_password_wrong"))
	}
	return result
}

// ChangePassword 修改密码（需要验证旧密码）
func (s *AuthService) ChangePassword(oldPassword, newPassword string) error {
	if !s.VerifyPassword(oldPassword) {
		return fmt.Errorf("%s", i18n.T("error.password_incorrect"))
	}

	if len(newPassword) < 6 {
		return fmt.Errorf("%s", i18n.T("error.password_too_short"))
	}

	return s.SetPassword(newPassword)
}

// ClearPassword 清除密码（取消密码保护）
func (s *AuthService) ClearPassword() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	ctx := context.Background()

	// 删除密码认证方式
	_, err := s.db.AuthMethod.Delete().
		Where(authmethod.MethodEQ("password")).
		Exec(ctx)
	if err != nil {
		return err
	}
	return s.ensureActiveMethod(ctx)
}

// ensureActiveMethod 删除认证方式后若已无任何激活方式，自动激活另一个已配置的方式。
// 只有当所有认证方式都被移除时，应用才会变为免验证。
func (s *AuthService) ensureActiveMethod(ctx context.Context) error {
	hasActive, err := s.db.AuthMethod.Query().
		Where(authmethod.IsActiveEQ(true)).
		Exist(ctx)
	if err != nil {
		return err
	}
	if hasActive {
		return nil
	}

	others, err := s.db.AuthMethod.Query().All(ctx)
	if err != nil {
		return err
	}
	if len(others) == 0 {
		return nil
	}

	_, err = s.db.AuthMethod.Update().
		Where(authmethod.MethodEQ(others[0].Method)).
		SetIsActive(true).
		Save(ctx)
	return err
}

// GetActiveMethod 获取当前激活的认证方式
func (s *AuthService) GetActiveMethod() (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ctx := context.Background()

	am, err := s.db.AuthMethod.Query().
		Where(authmethod.IsActiveEQ(true)).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return "", nil // 没有激活的认证方式
		}
		return "", fmt.Errorf("%s", i18n.T("error.load_auth_failed"))
	}
	return am.Method.String(), nil
}

// SetActiveMethod 设置激活的认证方式
func (s *AuthService) SetActiveMethod(method string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	ctx := context.Background()

	// 验证方法是否有效
	if method != authmethod.MethodPassword.String() && method != authmethod.MethodTotp.String() && method != authmethod.MethodPasskey.String() && method != authmethod.MethodBiometric.String() {
		return fmt.Errorf("%s", i18n.T("error.auth_method_invalid", "Method", method))
	}

	// 检查该方法是否已配置
	exists, err := s.db.AuthMethod.Query().
		Where(authmethod.MethodEQ(authmethod.Method(method))).
		Exist(ctx)
	if err != nil {
		return fmt.Errorf("%s", i18n.T("error.load_auth_failed"))
	}
	if !exists {
		return fmt.Errorf("%s", i18n.T("error.auth_method_not_configured", "Method", method))
	}

	// 取消所有方法的激活状态
	_, err = s.db.AuthMethod.Update().
		SetIsActive(false).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("%s", i18n.T("error.load_auth_failed"))
	}

	// 激活指定方法
	_, err = s.db.AuthMethod.Update().
		Where(authmethod.MethodEQ(authmethod.Method(method))).
		SetIsActive(true).
		Save(ctx)
	return err
}

// GetAvailableMethods 获取已配置的认证方式列表
func (s *AuthService) GetAvailableMethods() ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ctx := context.Background()

	methods, err := s.db.AuthMethod.Query().
		Select(authmethod.FieldMethod).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("%s", i18n.T("error.load_auth_failed"))
	}

	result := make([]string, len(methods))
	for i, m := range methods {
		result[i] = m.Method.String()
	}
	return result, nil
}

// Authenticate 统一验证方法
func (s *AuthService) Authenticate(method, credential string) (bool, error) {
	switch method {
	case "password":
		return s.VerifyPassword(credential), nil
	case "totp":
		return s.VerifyTOTP(credential), nil
	case "passkey":
		return s.FinishPasskeyLogin(credential)
	case "biometric":
		return s.authenticateBiometric(credential), nil
	default:
		return false, fmt.Errorf("%s", i18n.T("error.auth_method_invalid", "Method", method))
	}
}
